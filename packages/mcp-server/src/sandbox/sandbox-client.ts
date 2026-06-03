import { spawn } from "node:child_process";
import { createInterface } from "node:readline";
import { createServer } from "node:http";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import type { TokenPayload } from "../crypto/jwe.js";
import type { McpTenant } from "../config/schema.js";
import { executeBacklogCommand } from "../tools/backlog.js";

const BOOT_TIMEOUT = 60_000;
const SCRIPT_TIMEOUT = 30_000;

export interface SandboxOptions {
    denoPath?: string;
    workerPath?: string;
    binPath?: string;
}

export interface SandboxClient {
    execute(
        script: string,
        token: TokenPayload,
        tenant: McpTenant | undefined,
    ): Promise<{ result: string; error?: string }>;
    shutdown(): void;
}

interface SandboxState {
    port: number;
    process: ReturnType<typeof spawn>;
    callbackServer: ReturnType<typeof createServer>;
    callbackPort: number;
}

export async function createSandboxClient(
    options?: SandboxOptions,
): Promise<SandboxClient> {
    let state: SandboxState | null = null;

    async function ensureRunning(
        token: TokenPayload,
        tenant: McpTenant | undefined,
    ): Promise<SandboxState> {
        if (state && !state.process.killed) {
            return state;
        }

        // Start callback server for backlog() IPC
        const { server: callbackServer, port: callbackPort } = await startCallbackServer(
            token,
            tenant,
            options?.binPath,
        );

        const callbackUrl = `http://127.0.0.1:${callbackPort}`;
        const denoPath = options?.denoPath ?? resolveDefaultDenoPath();
        const workerPath = options?.workerPath ?? resolveDefaultWorkerPath();

        const proc = spawn(denoPath, [
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

        // Wait for port from stdout
        const port = await new Promise<number>((resolve, reject) => {
            const timer = setTimeout(
                () => reject(new Error("Deno sandbox boot timeout")),
                BOOT_TIMEOUT,
            );

            const rl = createInterface({ input: proc.stdout! });
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

            proc.on("error", (err) => {
                clearTimeout(timer);
                reject(err);
            });

            proc.on("exit", (code) => {
                clearTimeout(timer);
                reject(new Error(`Deno exited with code ${code}`));
            });

            proc.stderr?.on("data", (data) => {
                process.stderr.write(data);
            });
        });

        state = { port, process: proc, callbackServer, callbackPort };
        return state;
    }

    return {
        async execute(script, token, tenant) {
            const s = await ensureRunning(token, tenant);

            const controller = new AbortController();
            const timeout = setTimeout(
                () => controller.abort(),
                tenant?.script?.timeout_ms ?? SCRIPT_TIMEOUT,
            );

            try {
                const res = await fetch(`http://127.0.0.1:${s.port}`, {
                    method: "POST",
                    body: script,
                    signal: controller.signal,
                });
                const data = (await res.json()) as {
                    ok: boolean;
                    result?: string;
                    error?: string;
                };
                if (data.ok) {
                    return { result: data.result ?? "" };
                }
                return { result: "", error: data.error ?? "Unknown error" };
            } catch (err) {
                if ((err as Error).name === "AbortError") {
                    return { result: "", error: "Script execution timed out" };
                }
                return { result: "", error: (err as Error).message };
            } finally {
                clearTimeout(timeout);
            }
        },

        shutdown() {
            if (state) {
                state.process.kill();
                state.callbackServer.close();
                state = null;
            }
        },
    };
}

async function startCallbackServer(
    token: TokenPayload,
    tenant: McpTenant | undefined,
    binPath?: string,
): Promise<{ server: ReturnType<typeof createServer>; port: number }> {
    let callCount = 0;
    const maxCalls = tenant?.script?.max_cli_calls ?? 20;

    return new Promise((resolvePromise) => {
        const server = createServer(async (req, res) => {
            if (req.method !== "POST") {
                res.writeHead(405);
                res.end();
                return;
            }

            const body = await readBody(req);
            let parsed: { args: string };
            try {
                parsed = JSON.parse(body);
            } catch {
                res.writeHead(400);
                res.end(JSON.stringify({ error: "Invalid JSON" }));
                return;
            }

            callCount++;
            if (callCount > maxCalls) {
                res.writeHead(429);
                res.end(
                    JSON.stringify({
                        error: `CLI call limit exceeded (max ${maxCalls})`,
                    }),
                );
                return;
            }

            try {
                const jsonArgs = parsed.args.includes("--json")
                    ? parsed.args
                    : `${parsed.args} --json`;

                const result = await executeBacklogCommand(
                    jsonArgs,
                    token,
                    tenant,
                    binPath,
                );

                if (result.exitCode !== 0) {
                    res.writeHead(200);
                    res.end(
                        JSON.stringify({ error: result.output }),
                    );
                    return;
                }

                try {
                    const data = JSON.parse(result.output);
                    res.writeHead(200, {
                        "Content-Type": "application/json",
                    });
                    res.end(JSON.stringify(data));
                } catch {
                    res.writeHead(200, {
                        "Content-Type": "application/json",
                    });
                    res.end(JSON.stringify(result.output));
                }
            } catch (err) {
                res.writeHead(500);
                res.end(
                    JSON.stringify({ error: (err as Error).message }),
                );
            }
        });

        server.listen(0, "127.0.0.1", () => {
            const addr = server.address();
            const port = typeof addr === "object" && addr ? addr.port : 0;
            resolvePromise({ server, port });
        });
    });
}

function readBody(req: import("node:http").IncomingMessage): Promise<string> {
    return new Promise((resolve) => {
        const chunks: Buffer[] = [];
        req.on("data", (chunk: Buffer) => chunks.push(chunk));
        req.on("end", () => resolve(Buffer.concat(chunks).toString()));
    });
}

function resolveDefaultDenoPath(): string {
    const __dirname = dirname(fileURLToPath(import.meta.url));
    return resolve(__dirname, "..", "bin", "deno");
}

function resolveDefaultWorkerPath(): string {
    const __dirname = dirname(fileURLToPath(import.meta.url));
    return resolve(__dirname, "..", "src", "sandbox", "sandbox-worker.mjs");
}
