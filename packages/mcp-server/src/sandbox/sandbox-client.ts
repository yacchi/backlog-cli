import { spawn } from "node:child_process";
import { createInterface } from "node:readline";
import { createServer } from "node:http";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import type { TokenPayload } from "../crypto/jwt.js";
import type { ScriptConfig } from "../config/schema.js";
import type { ScriptFile } from "../transport/handlers.js";
import { executeBacklogCommand } from "../tools/backlog.js";
import { materializeFiles, substituteFileRefs, type CollectedFile } from "../tools/files.js";
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

            try {
                const result = await executeBacklogCommand(
                    args,
                    token,
                    { readOnly, binPath, additionalEnv: { BACKLOG_DOWNLOAD_MODE: "redirect" } },
                );

                if (result.exitCode !== 0) {
                    res.writeHead(200);
                    res.end(
                        JSON.stringify({ error: result.output }),
                    );
                    return;
                }

                const downloadMeta = tryParseDownloadRedirect(result.output);
                if (downloadMeta) {
                    const fileResult = await fetchFromBacklog(
                        token.space, token.bl_access_token!,
                        downloadMeta.apiPath, downloadMeta.outPath || downloadMeta.filename,
                    );
                    if (accumulatedFiles) {
                        accumulatedFiles.push(fileResult.collected);
                    }
                    res.writeHead(200, { "Content-Type": "application/json" });
                    res.end(JSON.stringify({
                        __backlog_output: `✓ Downloaded: ${downloadMeta.filename}`,
                        __backlog_files: [{
                            path: downloadMeta.outPath || `/${downloadMeta.filename}`,
                            data: fileResult.collected.data,
                        }],
                    }));
                    return;
                }

                let output: unknown;
                try {
                    output = JSON.parse(result.output);
                } catch {
                    output = result.output;
                }
                res.writeHead(200, { "Content-Type": "application/json" });
                res.end(JSON.stringify(output));
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

interface DownloadRedirectMeta {
    __download: boolean;
    apiPath: string;
    filename: string;
    outPath?: string;
}

function tryParseDownloadRedirect(output: string): DownloadRedirectMeta | null {
    try {
        const parsed = JSON.parse(output.trim()) as DownloadRedirectMeta;
        if (parsed.__download && parsed.apiPath) return parsed;
    } catch { /* not a redirect */ }
    return null;
}

async function fetchFromBacklog(
    space: string,
    accessToken: string,
    apiPath: string,
    outPath: string,
): Promise<{ collected: CollectedFile }> {
    const url = `https://${space}/api/v2${apiPath}`;
    const resp = await fetch(url, {
        headers: { Authorization: `Bearer ${accessToken}` },
    });
    if (!resp.ok) {
        throw new Error(`Backlog API error: ${resp.status} ${resp.statusText}`);
    }

    const buf = Buffer.from(await resp.arrayBuffer());
    const ct = resp.headers.get("Content-Type") || "application/octet-stream";
    const name = outPath.split("/").pop() || "download";

    return {
        collected: {
            name,
            path: outPath.startsWith("/") ? outPath : `/${outPath}`,
            data: buf.toString("base64"),
            mimeType: ct,
            size: buf.length,
            apiPath,
        },
    };
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
