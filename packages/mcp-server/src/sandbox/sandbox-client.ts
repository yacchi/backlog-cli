import { spawn } from "node:child_process";
import { createInterface } from "node:readline";
import { createServer } from "node:http";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import type { TokenPayload } from "../crypto/jwt.js";
import type { ScriptConfig } from "../config/schema.js";
import type { ScriptFile } from "../transport/handlers.js";
import { executeBacklogCommand } from "../tools/backlog.js";
import { materializeFiles, substituteFileRefs, createOutputDir, collectOutputFiles, type CollectedFile } from "../tools/files.js";
import { Logger, logSandbox } from "../logging/logger.js";

const BOOT_TIMEOUT = 60_000;
const SCRIPT_TIMEOUT = 30_000;

export interface SandboxOptions {
    workerPath?: string;
    binPath?: string;
    logger?: Logger;
}

export interface SandboxClient {
    execute(
        script: string,
        token: TokenPayload,
        scriptConfig: ScriptConfig | undefined,
        readOnly?: boolean,
        files?: ScriptFile[],
    ): Promise<{ result: string; error?: string; outputFiles?: CollectedFile[] }>;
    shutdown(): void;
}

interface SandboxState {
    port: number;
    process: ReturnType<typeof spawn>;
}

export async function createSandboxClient(
    options?: SandboxOptions,
): Promise<SandboxClient> {
    const logger = options?.logger ?? new Logger();
    let state: SandboxState | null = null;

    function isAlive(): boolean {
        if (!state) return false;
        if (state.process.killed) return false;
        if (state.process.exitCode !== null) return false;
        try {
            process.kill(state.process.pid!, 0);
            return true;
        } catch {
            return false;
        }
    }

    function cleanup(): void {
        if (!state) return;
        try { state.process.kill(); } catch { /* already dead */ }
        state = null;
    }

    async function ensureRunning(): Promise<SandboxState> {
        if (isAlive()) {
            return state!;
        }

        cleanup();

        const workerPath = options?.workerPath ?? resolveDefaultWorkerPath();

        const proc = spawn(workerPath, [], {
            stdio: ["pipe", "pipe", "pipe"],
        });

        logSandbox(logger, "info", "Starting sandbox worker", { workerPath });

        const port = await new Promise<number>((resolve, reject) => {
            const timer = setTimeout(
                () => {
                    logSandbox(logger, "error", "Deno sandbox boot timeout", { workerPath, timeout_ms: BOOT_TIMEOUT });
                    reject(new Error("Deno sandbox boot timeout"));
                },
                BOOT_TIMEOUT,
            );

            const stderrChunks: Buffer[] = [];

            const rl = createInterface({ input: proc.stdout! });
            rl.on("line", (line) => {
                clearTimeout(timer);
                try {
                    const data = JSON.parse(line) as { port: number };
                    logSandbox(logger, "info", "Sandbox worker started", { port: data.port });
                    resolve(data.port);
                } catch {
                    logSandbox(logger, "error", "Bad port line from sandbox worker", { line });
                    reject(new Error(`Bad port line: ${line}`));
                }
                rl.close();
            });

            proc.on("error", (err) => {
                clearTimeout(timer);
                logSandbox(logger, "error", `Sandbox process error: ${err.message}`, { workerPath });
                reject(err);
            });

            proc.on("exit", (code) => {
                clearTimeout(timer);
                const stderr = Buffer.concat(stderrChunks).toString().trim();
                const detail = stderr ? `\n${stderr.slice(0, 500)}` : "";
                logSandbox(logger, "error", "Sandbox boot failed", { exit_code: code, stderr: stderr.slice(0, 1000), workerPath });
                reject(new Error(`Sandbox boot failed (exit code ${code}).${detail}`));
            });

            proc.stderr?.on("data", (data: Buffer) => {
                stderrChunks.push(data);
                process.stderr.write(data);
            });
        });

        state = { port, process: proc };
        return state;
    }

    return {
        async execute(script, token, scriptConfig, readOnly, files) {
            const s = await ensureRunning();

            const filePaths = files?.length ? materializeFiles(files) : null;
            const accumulatedFiles: CollectedFile[] = [];

            const { server: cbServer, port: cbPort } = await startCallbackServer(
                token, scriptConfig, readOnly ?? false, options?.binPath, filePaths?.paths,
                accumulatedFiles,
            );

            const controller = new AbortController();
            const timeout = setTimeout(
                () => controller.abort(),
                scriptConfig?.timeout_ms ?? SCRIPT_TIMEOUT,
            );

            try {
                const res = await fetch(`http://127.0.0.1:${s.port}`, {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({
                        script,
                        callbackUrl: `http://127.0.0.1:${cbPort}`,
                    }),
                    signal: controller.signal,
                });
                const data = (await res.json()) as {
                    ok: boolean;
                    result?: string;
                    error?: string;
                    category?: string;
                };
                if (data.ok) {
                    return {
                        result: data.result ?? "",
                        outputFiles: accumulatedFiles.length > 0 ? accumulatedFiles : undefined,
                    };
                }
                const errorPrefix = data.category ? `[${data.category}] ` : "";
                return {
                    result: "",
                    error: `${errorPrefix}${data.error ?? "Unknown error"}`,
                    outputFiles: accumulatedFiles.length > 0 ? accumulatedFiles : undefined,
                };
            } catch (err) {
                if ((err as Error).name === "AbortError") {
                    const limitMs = scriptConfig?.timeout_ms ?? SCRIPT_TIMEOUT;
                    logSandbox(logger, "error", "Script execution timed out", { timeout_ms: limitMs, script: script.slice(0, 2000) });
                    return { result: "", error: `Script execution timed out after ${limitMs / 1000}s. Consider reducing the number of backlog() calls or simplifying the script.` };
                }

                logSandbox(logger, "error", `Sandbox execute error: ${(err as Error).message}`, { script: script.slice(0, 2000) });
                cleanup();
                throw err;
            } finally {
                clearTimeout(timeout);
                cbServer.close();
                filePaths?.cleanup();
            }
        },

        shutdown() {
            cleanup();
        },
    };
}

function startCallbackServer(
    token: TokenPayload,
    scriptConfig: ScriptConfig | undefined,
    readOnly: boolean,
    binPath?: string,
    filePaths?: string[],
    accumulatedFiles?: CollectedFile[],
): Promise<{ server: ReturnType<typeof createServer>; port: number }> {
    const maxCalls = scriptConfig?.max_cli_calls ?? 20;
    let callCount = 0;

    return new Promise((resolvePromise) => {
        const server = createServer(async (req, res) => {
            if (req.method !== "POST") {
                res.writeHead(405);
                res.end();
                return;
            }

            const body = await readBody(req);
            let parsed: { args: string | string[] };
            try {
                parsed = JSON.parse(body);
            } catch {
                res.writeHead(400);
                res.end(JSON.stringify({ error: "Invalid JSON" }));
                return;
            }

            let args = Array.isArray(parsed.args) ? parsed.args.join(" ") : String(parsed.args);

            if (filePaths?.length) {
                args = substituteFileRefs(args, filePaths);
            }

            callCount++;
            if (callCount > maxCalls) {
                res.writeHead(429);
                res.end(
                    JSON.stringify({
                        error: `CLI call limit exceeded: ${callCount}/${maxCalls} calls used. Reduce the number of backlog() calls or request a higher limit in config (script.max_cli_calls).`,
                    }),
                );
                return;
            }

            const outputDir = createOutputDir();
            try {
                const result = await executeBacklogCommand(
                    args,
                    token,
                    { readOnly, binPath, additionalEnv: { BACKLOG_OUTPUT_DIR: outputDir.path } },
                );

                const outputFiles = collectOutputFiles(outputDir.path);
                if (accumulatedFiles && outputFiles.length > 0) {
                    accumulatedFiles.push(...outputFiles);
                }

                if (result.exitCode !== 0) {
                    res.writeHead(200);
                    res.end(
                        JSON.stringify({ error: result.output }),
                    );
                    return;
                }

                let output: unknown;
                try {
                    output = JSON.parse(result.output);
                } catch {
                    output = result.output;
                }

                if (outputFiles.length > 0) {
                    res.writeHead(200, { "Content-Type": "application/json" });
                    res.end(JSON.stringify({
                        __backlog_output: output,
                        __backlog_files: outputFiles.map(f => ({
                            path: f.path,
                            data: f.data,
                        })),
                    }));
                } else {
                    res.writeHead(200, { "Content-Type": "application/json" });
                    res.end(JSON.stringify(output));
                }
            } catch (err) {
                res.writeHead(500);
                res.end(
                    JSON.stringify({ error: (err as Error).message }),
                );
            } finally {
                outputDir.cleanup();
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

function resolveDefaultWorkerPath(): string {
    const __dirname = dirname(fileURLToPath(import.meta.url));
    return resolve(__dirname, "..", "bin", "sandbox-worker");
}
