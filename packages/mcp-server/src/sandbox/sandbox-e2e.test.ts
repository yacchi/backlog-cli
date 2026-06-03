import { describe, it, expect, beforeAll, afterAll } from "vitest";
import { spawn, type ChildProcess, execSync } from "node:child_process";
import { createInterface } from "node:readline";
import { createServer, type Server } from "node:http";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const workerPath = resolve(__dirname, "sandbox-worker.mjs");

function denoAvailable(): boolean {
    try {
        execSync("deno --version", { stdio: "pipe" });
        return true;
    } catch {
        return false;
    }
}

const describeDeno = denoAvailable() ? describe : describe.skip;

describeDeno("sandbox-worker E2E", () => {
    let denoProc: ChildProcess;
    let sandboxPort: number;
    let callbackServer: Server;
    let callbackPort: number;
    let callbackRequests: Array<{ args: string }>;

    beforeAll(async () => {
        callbackRequests = [];

        // Start a mock callback server for backlog() IPC
        const { server, port } = await new Promise<{ server: Server; port: number }>((resolve) => {
            const srv = createServer((req, res) => {
                const chunks: Buffer[] = [];
                req.on("data", (c: Buffer) => chunks.push(c));
                req.on("end", () => {
                    const body = JSON.parse(Buffer.concat(chunks).toString());
                    callbackRequests.push(body);
                    res.writeHead(200, { "Content-Type": "application/json" });
                    res.end(JSON.stringify([{ id: 1, summary: "test issue" }]));
                });
            });
            srv.listen(0, "127.0.0.1", () => {
                const addr = srv.address();
                const p = typeof addr === "object" && addr ? addr.port : 0;
                resolve({ server: srv, port: p });
            });
        });
        callbackServer = server;
        callbackPort = port;

        // Start Deno sandbox worker
        const callbackUrl = `http://127.0.0.1:${callbackPort}`;
        denoProc = spawn("deno", [
            "run",
            "--allow-read",
            `--allow-net=127.0.0.1`,
            "--deny-env",
            "--deny-run",
            "--deny-write",
            workerPath,
            callbackUrl,
        ], {
            stdio: ["pipe", "pipe", "pipe"],
        });

        // Wait for port
        sandboxPort = await new Promise<number>((resolve, reject) => {
            const timer = setTimeout(() => reject(new Error("Deno boot timeout")), 60_000);
            const rl = createInterface({ input: denoProc.stdout! });
            rl.on("line", (line) => {
                clearTimeout(timer);
                try {
                    const data = JSON.parse(line) as { port: number };
                    resolve(data.port);
                } catch {
                    reject(new Error(`Bad port line: ${line}`));
                }
                rl.close();
            });
            denoProc.on("error", (err) => {
                clearTimeout(timer);
                reject(err);
            });
            denoProc.on("exit", (code) => {
                clearTimeout(timer);
                reject(new Error(`Deno exited with code ${code}`));
            });
        });
    }, 120_000);

    afterAll(() => {
        denoProc?.kill();
        callbackServer?.close();
    });

    async function runScript(script: string): Promise<{ ok: boolean; result?: string; error?: string }> {
        const res = await fetch(`http://127.0.0.1:${sandboxPort}`, {
            method: "POST",
            body: script,
        });
        return res.json() as Promise<{ ok: boolean; result?: string; error?: string }>;
    }

    it("should execute basic Python", async () => {
        const res = await runScript("1 + 2");
        expect(res.ok).toBe(true);
        expect(res.result).toBe("3");
    });

    it("should use standard library (json, math)", async () => {
        const res = await runScript(`
import json
import math
json.dumps({"pi": round(math.pi, 2)})
`);
        expect(res.ok).toBe(true);
        expect(JSON.parse(res.result!)).toEqual({ pi: 3.14 });
    });

    it("should block os module", async () => {
        const res = await runScript("import os");
        expect(res.ok).toBe(false);
        expect(res.error).toContain("blocked");
    });

    it("should block subprocess module", async () => {
        const res = await runScript("import subprocess");
        expect(res.ok).toBe(false);
        expect(res.error).toContain("blocked");
    });

    it("should block socket module", async () => {
        const res = await runScript("import socket");
        expect(res.ok).toBe(false);
        expect(res.error).toContain("blocked");
    });

    it("should block open()", async () => {
        const res = await runScript('open("/etc/passwd")');
        expect(res.ok).toBe(false);
        expect(res.error).toContain("blocked");
    });

    it("should block input()", async () => {
        const res = await runScript("input()");
        expect(res.ok).toBe(false);
        expect(res.error).toContain("blocked");
    });

    it("should block __import__ for blocked modules", async () => {
        const res = await runScript("__import__('subprocess')");
        expect(res.ok).toBe(false);
        expect(res.error).toContain("blocked");
    });

    it("should block exec(import os)", async () => {
        const res = await runScript('exec("import os")');
        expect(res.ok).toBe(false);
        expect(res.error).toContain("blocked");
    });

    it("should call backlog() via IPC bridge", async () => {
        callbackRequests = [];
        const res = await runScript(`
result = backlog("issue list --project TEST")
str(result)
`);
        expect(res.ok).toBe(true);
        expect(callbackRequests).toHaveLength(1);
        expect(callbackRequests[0].args).toBe("issue list --project TEST");
    });

    it("should allow collections, datetime, re", async () => {
        const res = await runScript(`
import collections
import datetime
import re
c = collections.Counter("hello")
c.most_common(1)[0][0]
`);
        expect(res.ok).toBe(true);
        expect(res.result).toBe("l");
    });

    it("should allow statistics", async () => {
        const res = await runScript(`
import statistics
str(statistics.mean([1, 2, 3, 4, 5]))
`);
        expect(res.ok).toBe(true);
        expect(res.result).toBe("3");
    });
});
