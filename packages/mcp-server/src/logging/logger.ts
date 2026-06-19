export type LogLevel = "debug" | "info" | "warn" | "error";

const LEVEL_ORDER: Record<LogLevel, number> = {
    debug: 0,
    info: 1,
    warn: 2,
    error: 3,
};

export class Logger {
    private bindings: Record<string, unknown>;
    private minLevel: number;

    constructor(bindings: Record<string, unknown> = {}, minLevel: LogLevel = "info") {
        this.bindings = bindings;
        this.minLevel = LEVEL_ORDER[minLevel];
    }

    child(fields: Record<string, unknown>): Logger {
        const child = new Logger({ ...this.bindings, ...fields });
        child.minLevel = this.minLevel;
        return child;
    }

    debug(obj: Record<string, unknown>): void {
        this.emit("debug", obj);
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
        if (LEVEL_ORDER[level] < this.minLevel) return;
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
    input: boolean;
    output: boolean;
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

            const logInput = opts.loggingConfig?.input ?? true;
            const logOutput = opts.loggingConfig?.output ?? true;

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

            const inputBytes = byteLength(opts.input);
            const outputBytes = result.output != null ? byteLength(result.output) : 0;
            entry.input_bytes = inputBytes;
            entry.output_bytes = outputBytes;

            // Audit summary: lightweight record for the audit trail
            const audit: Record<string, unknown> = {
                component: "audit",
                action: "tool_call",
                tool: opts.tool,
                result: result.error ? "error" : "success",
                duration_ms,
                input_bytes: inputBytes,
                output_bytes: outputBytes,
            };
            if (result.error) audit.error = result.error;
            if (result.category) audit.category = result.category;
            logger[level](audit);
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

function byteLength(value: unknown): number {
    if (value === undefined || value === null) return 0;
    const s = typeof value === "string" ? value : JSON.stringify(value);
    return new TextEncoder().encode(s).byteLength;
}

function truncate(value: unknown, maxLen: number): unknown {
    if (value === undefined || value === null) return value;
    const s = typeof value === "string" ? value : JSON.stringify(value);
    if (s.length <= maxLen) return value;
    return s.slice(0, maxLen) + `...(truncated, ${s.length} chars total)`;
}
