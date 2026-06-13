// Sandbox containment regression test.
//
// Codifies the security guarantee that a Python script submitted to the Pyodide
// sandbox cannot reach any real host capability. The Python-level deny-list
// (subprocess/ctypes/open) is NOT the boundary — `import js` trivially exposes the
// full Deno API from inside Pyodide. The ACTUAL boundary is the Deno process
// permission set, which production bakes in as `--allow-net=127.0.0.1` only.
//
// These tests spawn the worker with exactly that flag and assert that every
// capability escape (arbitrary file read, env read, process spawn, file write,
// non-loopback network egress) is denied, while the legitimate happy path
// (stdlib + the backlog() IPC bridge) still works.

import { describe, it, expect, beforeAll, afterAll } from "vitest";
import { spawn, type ChildProcess } from "node:child_process";
import { createInterface } from "node:readline";
import { createServer, type Server } from "node:http";
import { resolve, dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { existsSync } from "node:fs";
import { delimiter } from "node:path";

const __dirname = dirname(fileURLToPath(import.meta.url));
const workerPath = resolve(__dirname, "sandbox-worker.mjs");
// The production worker is a `deno compile` binary that EMBEDS the Pyodide
// wasm/stdlib assets, so it runs with `--allow-net=127.0.0.1` and nothing else.
// This test runs the SOURCE .mjs via `deno run`, which must load those assets
// from the npm cache — so it needs read access to that directory to boot.
// Scoping read to only that directory preserves the production security
// posture: arbitrary file reads outside it are still denied.
const pyodideAssetsDir = resolve(__dirname, "..", "..", "node_modules", ".deno");

// Resolve `deno` by scanning PATH (deterministic — avoids execSync cold-start
// flakiness seen under vitest's collection phase).
function findDeno(): string | null {
    const dirs = (process.env.PATH ?? "").split(delimiter);
    for (const dir of dirs) {
        if (!dir) continue;
        const candidate = join(dir, "deno");
        if (existsSync(candidate)) return candidate;
    }
    return null;
}

const DENO = findDeno();
const d = DENO ? describe : describe.skip;

d("sandbox containment (net-only permissions)", () => {
    let proc: ChildProcess;
    let sandboxPort: number;
    let cbServer: Server;
    let cbPort: number;
    let cbHits: number;

    beforeAll(async () => {
        cbHits = 0;
        ({ server: cbServer, port: cbPort } = await new Promise<{ server: Server; port: number }>((res) => {
            const srv = createServer((req, rsp) => {
                cbHits++;
                const chunks: Buffer[] = [];
                req.on("data", (c: Buffer) => chunks.push(c));
                req.on("end", () => {
                    rsp.writeHead(200, { "Content-Type": "application/json" });
                    rsp.end(JSON.stringify([{ id: 1, summary: "alpha" }, { id: 2, summary: "beta" }]));
                });
            });
            srv.listen(0, "127.0.0.1", () => res({ server: srv, port: (srv.address() as { port: number }).port }));
        }));

        // Net to loopback + read scoped to the Pyodide asset dir only (needed to
        // boot from source; the compiled production binary needs no read at all).
        // No --allow-write/-env/-run/-ffi.
        proc = spawn(DENO!, [
            "run",
            `--allow-read=${pyodideAssetsDir}`,
            "--allow-net=127.0.0.1",
            workerPath,
        ], {
            stdio: ["pipe", "pipe", "pipe"],
        });

        sandboxPort = await new Promise<number>((res, rej) => {
            const timer = setTimeout(() => rej(new Error("boot timeout")), 90_000);
            const rl = createInterface({ input: proc.stdout! });
            rl.on("line", (line) => {
                clearTimeout(timer);
                try { res(JSON.parse(line).port as number); } catch { rej(new Error("bad port line: " + line)); }
                rl.close();
            });
            proc.on("exit", (code) => { clearTimeout(timer); rej(new Error("worker exited " + code)); });
        });
    }, 120_000);

    afterAll(() => {
        proc?.kill();
        cbServer?.close();
    });

    async function run(script: string): Promise<{ ok: boolean; result?: string; error?: string; category?: string }> {
        const res = await fetch(`http://127.0.0.1:${sandboxPort}`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ script, callbackUrl: `http://127.0.0.1:${cbPort}` }),
        });
        return res.json() as Promise<{ ok: boolean; result?: string; error?: string; category?: string }>;
    }

    // --- legitimate use still works ---

    it("runs Python standard library", async () => {
        const r = await run("import json, statistics, datetime, re, collections\nprint(statistics.mean([2, 4, 6]))");
        expect(r.ok).toBe(true);
        expect(r.result?.trim()).toBe("4");
    });

    it("runs the backlog() IPC bridge and processes results", async () => {
        const r = await run("xs = backlog('issue list -p X')\nsorted(i['summary'] for i in xs)");
        expect(r.ok).toBe(true);
        expect(JSON.parse(r.result!)).toEqual(["alpha", "beta"]);
    });

    it("supports temp-file write/read/analyze in the in-memory MEMFS", async () => {
        const r = await run([
            "import csv, statistics, json",
            "rows = backlog('issue list')",
            "with open('/tmp/work.csv', 'w', newline='') as f:",
            "    w = csv.writer(f); w.writerow(['summary'])",
            "    [w.writerow([x['summary']]) for x in rows]",
            "with open('/tmp/work.csv') as f:",
            "    n = len(list(csv.DictReader(f)))",
            "print(json.dumps({'rows': n}))",
        ].join("\n"));
        expect(r.ok).toBe(true);
        expect(JSON.parse(r.result!)).toEqual({ rows: 2 });
    });

    // Each request runs on a fresh Pyodide instance, so neither filesystem nor
    // interpreter state can leak from one request (tenant) to the next.

    it("isolates the filesystem between requests (fresh instance)", async () => {
        // A file written anywhere — even outside /tmp — must not survive.
        const w = await run("open('/evil.txt', 'w').write('SECRET'); print('written')");
        expect(w.ok).toBe(true);
        const rd = await run("import os\nprint(open('/evil.txt').read() if os.path.exists('/evil.txt') else 'GONE')");
        expect(rd.ok).toBe(true);
        expect(rd.result?.trim()).toBe("GONE");
    });

    it("isolates concurrent requests from each other", async () => {
        // Fire N requests at once. Each writes its own id to a temp file, makes a
        // backlog() call (an await point where other requests interleave), then reads
        // the file back. With per-request instances, every request must read back its
        // OWN id — proving filesystem + callback isolation under concurrency.
        const N = 8;
        const results = await Promise.all(
            Array.from({ length: N }, (_, i) =>
                run([
                    `mid = ${i}`,
                    "open('/tmp/v.txt', 'w').write(str(mid))",
                    "r = backlog('issue list')",
                    "import json",
                    "print(json.dumps({'mid': mid, 'back': int(open('/tmp/v.txt').read())}))",
                ].join("\n")),
            ),
        );
        for (const r of results) {
            expect(r.ok).toBe(true);
            const d = JSON.parse(r.result!);
            expect(d.back, `request ${d.mid} read back a foreign value`).toBe(d.mid);
        }
    });

    it("isolates interpreter state between requests (no poisoning)", async () => {
        // Poison a stdlib module + sys in one request...
        const poison = await run([
            "import json, sys",
            "json.POISONED = 'yes'",
            "json.dumps = lambda *a, **k: 'HIJACKED'",
            "sys._stash = 'leftover'",
            "print('poisoned')",
        ].join("\n"));
        expect(poison.ok).toBe(true);
        // ...the next request must see a pristine interpreter.
        const check = await run([
            "import json, sys",
            "print(getattr(json, 'POISONED', 'clean'), json.dumps({'x': 1}), getattr(sys, '_stash', 'clean'))",
        ].join("\n"));
        expect(check.ok).toBe(true);
        expect(check.result?.trim()).toBe('clean {"x": 1} clean');
    });

    // --- capability escapes are all denied by the Deno permission layer ---
    // `import js` itself succeeds (Pyodide design); what matters is that no Deno
    // API call backed by it yields a real capability.

    it("denies arbitrary file read via js.Deno.readTextFileSync", async () => {
        const r = await run("import js\njs.Deno.readTextFileSync('/etc/passwd')");
        expect(r.ok).toBe(false);
        expect(r.error).toMatch(/Requires read access|NotCapable/);
    });

    it("denies env exfiltration via js.Deno.env", async () => {
        const r = await run("import js\nstr(js.Deno.env.toObject())");
        expect(r.ok).toBe(false);
        expect(r.error).toMatch(/Requires env access|NotCapable/);
    });

    it("denies process spawn via js.Deno.Command", async () => {
        const r = await run("import js\nstr(js.Deno.Command.new('id').outputSync())");
        expect(r.ok).toBe(false);
        expect(r.error).toMatch(/Requires run access|NotCapable/);
    });

    it("denies file write via js.Deno.writeTextFileSync", async () => {
        const r = await run("import js\njs.Deno.writeTextFileSync('/tmp/escape_probe', 'x')\n'wrote'");
        expect(r.ok).toBe(false);
        expect(r.error).toMatch(/Requires write access|NotCapable/);
    });

    it("denies non-loopback network egress", async () => {
        const r = await run("import js\nawait js.fetch('http://169.254.169.254/latest/meta-data/')\n'fetched'");
        expect(r.ok).toBe(false);
        expect(r.error).toMatch(/Requires net access|NotCapable/);
    });

    // Defense-in-depth Python deny-list. Note this is NOT the security boundary
    // (the Deno permission set is) — os/socket/etc. are intentionally left
    // importable because they are non-functional under WASM + denied Deno perms.
    it("blocks the deny-listed Python modules", async () => {
        for (const mod of ["subprocess", "ctypes", "pickle"]) {
            const r = await run(`import ${mod}`);
            expect(r.ok, `${mod} should be blocked`).toBe(false);
            expect(r.error).toMatch(/blocked/);
        }
    });

    it("blocks input()/breakpoint() builtins", async () => {
        for (const expr of ["input()", "breakpoint()"]) {
            const r = await run(expr);
            expect(r.ok, `${expr} should be blocked`).toBe(false);
            expect(r.error).toMatch(/blocked/);
        }
    });

    it("open() reaches MEMFS only, never the host filesystem", async () => {
        // open() is intentionally allowed (MEMFS), but it must not see host files:
        // /etc/passwd does not exist in MEMFS, so this is a not-found, not a leak.
        const r = await run("open('/etc/passwd').read()");
        expect(r.ok).toBe(false);
        expect(r.error).toMatch(/FileNotFoundError|No such file/);
        expect(r.error).not.toMatch(/root:/);
    });

    it("blocks deny-listed imports even via exec/__import__", async () => {
        const r1 = await run("exec('import subprocess')");
        expect(r1.ok).toBe(false);
        expect(r1.error).toMatch(/blocked/);
        const r2 = await run("__import__('ctypes')");
        expect(r2.ok).toBe(false);
        expect(r2.error).toMatch(/blocked/);
    });
});
