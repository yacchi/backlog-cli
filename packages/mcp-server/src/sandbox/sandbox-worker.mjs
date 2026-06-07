// Deno persistent sandbox worker — runs Pyodide (Python on WASM) in a restricted environment.
// Communicates with Node.js parent via HTTP on 127.0.0.1:<random-port>.
// Pyodide WASM environment is kept warm across requests.
// All other state (callbackUrl, Python user variables) is per-request only.

import { loadPyodide } from "npm:pyodide";

const pyodide = await loadPyodide();

// Mutex: serialize all script executions so no request-scoped state leaks between them.
// Pyodide/WASM is single-threaded anyway — concurrent execution would just interleave awaits.
let execLock = Promise.resolve();

// Per-request callback URL — only valid during a serialized execution window
let requestCallbackUrl = "";

pyodide.registerJsModule("_backlog_bridge", {
    call: (args) => {
        const url = requestCallbackUrl;
        if (!url) {
            return Promise.reject(new Error("No callback URL — backlog() called outside request scope"));
        }
        return fetch(url, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ args }),
        }).then((r) => r.text());
    },
});

// Setup sandbox restrictions + helpers (runs once at boot)
await pyodide.runPythonAsync(`
import sys

# Blocked modules — only things genuinely dangerous inside Pyodide/WASM.
# Most I/O and network modules (os, socket, http, urllib.request, etc.) are
# already non-functional in WASM — the real sandbox boundary is Pyodide + the
# open()/input() builtin overrides above.  We keep a minimal deny-list for
# defense-in-depth: process spawning, FFI, arbitrary code execution via
# deserialization, and package-management escape hatches.
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

# Override dangerous builtins
import builtins
def _blocked_open(*a, **kw):
    raise PermissionError("open() is blocked in sandbox")
def _blocked_input(*a, **kw):
    raise PermissionError("input() is blocked in sandbox")
def _blocked_breakpoint(*a, **kw):
    raise PermissionError("breakpoint() is blocked in sandbox")

builtins.open = _blocked_open
builtins.input = _blocked_input
builtins.breakpoint = _blocked_breakpoint

# Also block io.open
import io
io.open = _blocked_open

# Setup backlog() helper
from _backlog_bridge import call as _bl_call
import json as _json

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

def _format_result(val):
    if val is None:
        return "None"
    try:
        return _json.dumps(val, ensure_ascii=False, default=str, indent=2)
    except (TypeError, ValueError):
        return str(val)

# stdout capture buffer (structure only — truncated per request)
class _CaptureIO(io.StringIO):
    pass

_capture_buf = _CaptureIO()

# Set of user-defined variable names to clean up between requests
_SANDBOX_BUILTINS = frozenset(dir())
`);

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

    // Serialize execution: wait for any prior request to finish
    const response = await new Promise((resolve) => {
        execLock = execLock.then(async () => {
            requestCallbackUrl = callbackUrl;
            try {
                const result = await executeScript(script);
                resolve(result);
            } finally {
                requestCallbackUrl = "";
            }
        });
    });

    return response;
});

async function executeScript(script) {
    try {
        // Clean user variables from previous execution
        await pyodide.runPythonAsync(`
import sys as _sys
for _n in list(globals().keys()):
    if not _n.startswith('_') and _n not in _SANDBOX_BUILTINS:
        del globals()[_n]
_capture_buf.truncate(0)
_capture_buf.seek(0)
_sys.stdout = _capture_buf
`);
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
        try { await pyodide.runPythonAsync("import sys; sys.stdout = sys.__stdout__"); } catch { /* ignore */ }
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
