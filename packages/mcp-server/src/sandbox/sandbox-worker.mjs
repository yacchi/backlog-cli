// Deno persistent sandbox worker — runs Pyodide (Python on WASM) in a restricted environment.
// Communicates with Node.js parent via HTTP on 127.0.0.1:<random-port>.
//
// Multi-tenant isolation model: each request runs on its OWN fresh Pyodide instance
// that is discarded afterward. A shared warm interpreter cannot be cleaned reliably
// between requests — Python state (sys.modules, monkeypatched builtins/stdlib) and
// MEMFS files written to arbitrary paths leak across requests, letting one tenant
// poison or exfiltrate another's data.
//
// A naive fresh loadPyodide() costs ~700ms (WASM instantiate + stdlib init), which
// would be far too slow per request. Instead we build a memory snapshot ONCE at boot
// (after applying the pure-Python sandbox setup) and restore each per-request instance
// from it in ~55ms. The snapshot cannot capture JS references, so the backlog() bridge
// (which holds a JS function) is wired up AFTER restore, not baked into the snapshot.
//
// Concurrency: instances share no mutable state, so requests run concurrently, bounded
// by a semaphore (each in-flight instance costs memory). The single JS thread means no
// CPU parallelism, but concurrent requests overlap their backlog() I/O waits. The
// callback URL is captured per-instance (a closure), never a shared global, so
// concurrent requests cannot cross-talk.

import { loadPyodide } from "npm:pyodide";

// Max concurrent in-flight instances. Defaults to the CPU count (capped for memory);
// overridable via the first CLI arg. Reading argv/navigator needs no Deno permission.
const CONCURRENCY = Math.max(
    1,
    Math.min(Number(Deno.args[0]) || navigator.hardwareConcurrency || 4, 8),
);

// Snapshot-safe setup: PURE PYTHON ONLY (no JS references). Applied once before the
// snapshot is taken, so every restored instance starts with the sandbox in place.
const SETUP_CORE = `
import sys

# Blocked modules — only things genuinely dangerous inside Pyodide/WASM.
# Most I/O and network modules (os, socket, http, urllib.request, etc.) are
# already non-functional in WASM. We keep a minimal deny-list for defense-in-depth:
# process spawning, FFI, arbitrary code execution via deserialization, and
# package-management escape hatches.
_BLOCKED_TOP = frozenset({
    'subprocess', 'ctypes', 'pickle',
    'code', 'codeop', 'compileall', 'py_compile',
    'ensurepip', 'venv', 'pip', 'setuptools',
})

# Remove any pre-imported dangerous modules from sys.modules
for m in list(sys.modules.keys()):
    if m.split('.')[0] in _BLOCKED_TOP:
        del sys.modules[m]

# Install import hook (PEP 451 find_spec)
from importlib.abc import MetaPathFinder
from importlib.machinery import ModuleSpec

class _SandboxFinder(MetaPathFinder):
    def find_spec(self, name, path, target=None):
        top = name.split('.')[0]
        if top.startswith('_') and top != '_backlog_bridge':
            return None
        if top in _BLOCKED_TOP:
            return ModuleSpec(name, _BlockLoader())
        return None

class _BlockLoader:
    def create_module(self, spec):
        return None
    def exec_module(self, mod):
        raise ImportError(f"'{mod.__name__}' is blocked in sandbox")

sys.meta_path.insert(0, _SandboxFinder())

# Override dangerous builtins.
# NOTE: open()/io.open are intentionally NOT blocked. Python file I/O in Pyodide
# targets the in-memory MEMFS, which is isolated from the host filesystem (the host
# FS is protected by the worker running with net-only Deno permissions). This lets
# scripts use temp files for write/read/analyze workflows. Files written to MEMFS
# do not leak across requests because each request runs on a fresh Pyodide instance.
import builtins
def _blocked_input(*a, **kw):
    raise PermissionError("input() is blocked in sandbox")
def _blocked_breakpoint(*a, **kw):
    raise PermissionError("breakpoint() is blocked in sandbox")

builtins.input = _blocked_input
builtins.breakpoint = _blocked_breakpoint

import io
import json as _json

def _format_result(val):
    if val is None:
        return "None"
    try:
        return _json.dumps(val, ensure_ascii=False, default=str, indent=2)
    except (TypeError, ValueError):
        return str(val)

# stdout capture buffer
class _CaptureIO(io.StringIO):
    pass

_capture_buf = _CaptureIO()
`;

// Bridge init: imports the JS-backed _backlog_bridge module. Run AFTER snapshot
// restore (it holds a JS function reference, which a snapshot cannot serialize).
const BRIDGE_INIT = `
from _backlog_bridge import call as _bl_call

async def backlog(args):
    """Execute a backlog CLI command and return the result as native Python types."""
    raw = await _bl_call(args)
    if hasattr(raw, 'to_py'):
        raw = raw.to_py()
    text = str(raw)
    try:
        data = _json.loads(text)
    except (ValueError, TypeError):
        return text
    if isinstance(data, dict) and 'error' in data and len(data) == 1:
        raise RuntimeError(data['error'])
    return data
`;

// Build the snapshot once at boot. _makeSnapshot must be passed to loadPyodide for
// makeMemorySnapshot() to be allowed.
const snapshot = await (async () => {
    const boot = await loadPyodide({ _makeSnapshot: true });
    await boot.runPythonAsync(SETUP_CORE);
    return boot.makeMemorySnapshot();
})();

// Restore a fresh, isolated instance from the snapshot and wire its backlog() bridge
// to THIS request's callback URL (captured in the closure — never a shared global).
async function createInstance(callbackUrl) {
    const py = await loadPyodide({ _loadSnapshot: snapshot });
    py.registerJsModule("_backlog_bridge", {
        call: (args) =>
            fetch(callbackUrl, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ args }),
            }).then((r) => r.text()),
    });
    await py.runPythonAsync(BRIDGE_INIT);
    return py;
}

// Simple async semaphore to bound concurrent in-flight instances (memory).
class Semaphore {
    constructor(max) {
        this.max = max;
        this.cur = 0;
        this.waiters = [];
    }
    async acquire() {
        if (this.cur >= this.max) {
            await new Promise((resolve) => this.waiters.push(resolve));
        }
        this.cur++;
    }
    release() {
        this.cur--;
        const next = this.waiters.shift();
        if (next) next();
    }
}

const sem = new Semaphore(CONCURRENCY);

function autoAwaitBacklog(script) {
    return script.split("\n").map((line) => {
        if (line.trimStart().startsWith("#")) return line;
        return line.replace(/\bbacklog\s*\(/g, (match, offset) => {
            const before = line.substring(0, offset);
            if (/\bawait\s+$/.test(before)) return match;
            return "await " + match;
        });
    }).join("\n");
}

// HTTP server for IPC with Node.js
const server = Deno.serve({ hostname: "127.0.0.1", port: 0 }, async (req) => {
    if (req.method !== "POST") {
        return new Response("POST only", { status: 405 });
    }

    const body = await req.json();
    const script = autoAwaitBacklog(body.script || "");
    const callbackUrl = body.callbackUrl || "";

    if (!callbackUrl) {
        return new Response(
            JSON.stringify({ ok: false, error: "Missing callbackUrl", category: "runtime_error" }),
            { status: 400, headers: { "content-type": "application/json" } },
        );
    }

    await sem.acquire();
    try {
        const py = await createInstance(callbackUrl);
        return await executeScript(py, script);
        // Instance goes out of scope here; GC reclaims its WASM memory. It must
        // never serve another request.
    } catch (err) {
        return new Response(
            JSON.stringify({ ok: false, error: `Sandbox error: ${err.message}`, category: "runtime_error" }),
            { status: 500, headers: { "content-type": "application/json" } },
        );
    } finally {
        sem.release();
    }
});

async function executeScript(pyodide, script) {
    try {
        // Fresh instance per request — no cross-request cleanup needed. Just point
        // stdout at the capture buffer for this run.
        await pyodide.runPythonAsync(`import sys as _sys\n_sys.stdout = _capture_buf`);
        const result = await pyodide.runPythonAsync(script);
        const captured = await pyodide.runPythonAsync(`
_sys.stdout = _sys.__stdout__
_capture_buf.getvalue()
`);
        const printed = String(captured ?? "").trimEnd();
        if (captured?.destroy) captured.destroy();

        let output;
        if (printed) {
            output = printed;
        } else if (result != null && result !== undefined) {
            pyodide.globals.set("__last__", result);
            const formatted = await pyodide.runPythonAsync("_format_result(__last__)");
            output = String(formatted ?? "");
            if (formatted?.destroy) formatted.destroy();
        } else {
            output = "None";
        }
        if (result?.destroy) result.destroy();

        return new Response(
            JSON.stringify({ ok: true, result: output }),
            { headers: { "content-type": "application/json" } },
        );
    } catch (e) {
        const fullError = String(e);
        const errorLines = fullError.split("\n").filter((l) => l.trim());
        const lastLine = errorLines.pop() || fullError;
        const category = lastLine.startsWith("ImportError") ? "import_error"
            : lastLine.startsWith("PermissionError") ? "permission_error"
            : lastLine.startsWith("SyntaxError") ? "syntax_error"
            : "runtime_error";
        return new Response(
            JSON.stringify({ ok: false, error: lastLine, category }),
            { status: 400, headers: { "content-type": "application/json" } },
        );
    }
}

// Report port to parent
console.log(JSON.stringify({ port: server.addr.port }));
