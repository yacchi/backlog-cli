// Deno persistent sandbox worker — runs Pyodide (Python on WASM) in a restricted environment.
// Launched as: deno run --deny-net --deny-env --deny-run --deny-write --allow-read=<cache> sandbox-worker.mjs
//
// Communicates with Node.js parent via HTTP on 127.0.0.1:<random-port>.
// Pyodide is kept warm across requests.

import { loadPyodide } from "npm:pyodide";

// IPC callback URL (passed as argv)
const callbackUrl = Deno.args[0];
if (!callbackUrl) {
    console.error("Usage: sandbox-worker.mjs <callback-url>");
    Deno.exit(1);
}

const pyodide = await loadPyodide();

// Register backlog() bridge — calls back to Node.js over HTTP
pyodide.registerJsModule("_backlog_bridge", {
    call: (args) => {
        const resp = fetchSync(callbackUrl, args);
        return resp;
    },
});

// Setup sandbox restrictions
await pyodide.runPythonAsync(`
import sys

# Blocked modules — dangerous I/O, network, process, and code execution modules
_BLOCKED_TOP = frozenset({
    'os', 'subprocess', 'socket', 'http', 'urllib', 'xmlrpc',
    'shutil', 'pathlib', 'signal', 'ctypes', 'multiprocessing',
    'tempfile', 'glob', 'fcntl', 'termios', 'pty', 'resource',
    'select', 'selectors', 'asyncio', 'concurrent',
    'threading', 'mmap', 'webbrowser', 'ftplib', 'smtplib',
    'imaplib', 'poplib', 'nntplib', 'telnetlib', 'socketserver',
    'ssl', 'sqlite3', 'dbm', 'shelve', 'pickle',
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

def backlog(args):
    """Execute a backlog CLI command and return parsed JSON result."""
    result = _bl_call(args)
    if hasattr(result, 'to_py'):
        return result.to_py()
    return result
`);

// HTTP server for IPC with Node.js
const server = Deno.serve({ hostname: "127.0.0.1", port: 0 }, async (req) => {
    if (req.method !== "POST") {
        return new Response("POST only", { status: 405 });
    }

    const script = await req.text();
    try {
        const result = await pyodide.runPythonAsync(script);
        return new Response(
            JSON.stringify({ ok: true, result: String(result ?? "None") }),
            { headers: { "content-type": "application/json" } },
        );
    } catch (e) {
        const errorLines = String(e).split("\n").filter((l) => l.trim());
        return new Response(
            JSON.stringify({ ok: false, error: errorLines.pop() || String(e) }),
            { status: 400, headers: { "content-type": "application/json" } },
        );
    }
});

// Report port to parent
console.log(JSON.stringify({ port: server.addr.port }));

// Synchronous fetch for backlog() bridge
// Pyodide runs Python synchronously, so we need sync HTTP.
// Deno doesn't have sync fetch, so we use XMLHttpRequest polyfill approach.
// Actually, Pyodide's JS→Python bridge handles async transparently when using
// registerJsModule with async functions, but only in runPythonAsync.
// For synchronous calls within user scripts, we'd need a different approach.
// For now, the backlog() function works because Pyodide can await JS promises
// when the Python code is run via runPythonAsync.
function fetchSync(url, args) {
    // This returns a Promise that Pyodide can handle via its async bridge
    return fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ args }),
    }).then((r) => r.json());
}
