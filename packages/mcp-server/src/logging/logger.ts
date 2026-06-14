export type LogLevel = "info" | "warn" | "error";

export class Logger {
    private bindings: Record<string, unknown>;

    constructor(bindings: Record<string, unknown> = {}) {
        this.bindings = bindings;
    }

    child(fields: Record<string, unknown>): Logger {
        return new Logger({ ...this.bindings, ...fields });
    }

    info(obj: Record<string, unknown>): void {
        this.emit("info", obj);
    }

    warn(obj: Record<string, unknown>): void {
        this.emit("warn", obj);
    }

    error(obj: Record<string, unknown>): void {
        this.emit("error", obj);
    }

    private emit(level: LogLevel, obj: Record<string, unknown>): void {
        const entry = {
            level,
            ts: new Date().toISOString(),
            ...this.bindings,
            ...obj,
        };
        const line = JSON.stringify(entry);
        if (level === "error") {
            process.stderr.write(line + "\n");
        } else {
            process.stdout.write(line + "\n");
        }
    }
}

export const LOGGER_CONTEXT_KEY = "logger";

export interface LoggingConfig {
    log_input: boolean;
    log_output: boolean;
}

export interface ToolCallOptions {
    tool: string;
    input: unknown;
    tenant?: string;
    loggingConfig?: LoggingConfig;
}

export function logToolCall(
    logger: Logger,
    opts: ToolCallOptions,
): { finish: (result: { output?: unknown; error?: string; category?: string }) => void } {
    const start = Date.now();

    return {
        finish(result) {
            const duration_ms = Date.now() - start;
            const level: LogLevel = result.error ? "error" : "info";

            const entry: Record<string, unknown> = {
                component: "tool",
                tool: opts.tool,
                duration_ms,
            };

            if (result.error) {
                entry.error = result.error;
            }
            if (result.category) {
                entry.category = result.category;
            }
            if (opts.tenant) {
                entry.tenant = opts.tenant;
            }

            const logInput = opts.loggingConfig?.log_input ?? true;
            const logOutput = opts.loggingConfig?.log_output ?? true;

            if (logInput) {
                entry.input = opts.input;
            } else {
                const inp = opts.input as Record<string, unknown> | undefined;
                if (inp && typeof inp === "object" && "script" in inp) {
                    entry.input = { script: inp.script };
                }
            }

            if (logOutput && !result.error) {
                entry.output = truncate(result.output, 2000);
            }

            logger[level](entry);
        },
    };
}

export function logSandbox(logger: Logger, level: LogLevel, message: string, meta?: Record<string, unknown>): void {
    const entry: Record<string, unknown> = {
        component: "sandbox",
        ...(level === "error" ? { error: message } : {}),
        meta: {
            ...(level !== "error" ? { message } : {}),
            ...meta,
        },
    };
    logger[level](entry);
}

function truncate(value: unknown, maxLen: number): unknown {
    if (value === undefined || value === null) return value;
    const s = typeof value === "string" ? value : JSON.stringify(value);
    if (s.length <= maxLen) return value;
    return s.slice(0, maxLen) + `...(truncated, ${s.length} chars total)`;
}
