export type LogLevel = "info" | "warn" | "error";

export interface ToolLogEntry {
    level: LogLevel;
    ts: string;
    component: string;
    tool?: string;
    input?: unknown;
    output?: unknown;
    error?: string;
    category?: string;
    tenant?: string;
    duration_ms?: number;
    meta?: Record<string, unknown>;
}

function emit(entry: ToolLogEntry): void {
    const line = JSON.stringify(entry);
    if (entry.level === "error") {
        process.stderr.write(line + "\n");
    } else {
        process.stdout.write(line + "\n");
    }
}

export function logToolCall(opts: {
    tool: string;
    input: unknown;
    tenant?: string;
}): { finish: (result: { output?: unknown; error?: string; category?: string }) => void } {
    const start = Date.now();

    return {
        finish(result) {
            const duration_ms = Date.now() - start;
            const level: LogLevel = result.error ? "error" : "info";
            emit({
                level,
                ts: new Date().toISOString(),
                component: "tool",
                tool: opts.tool,
                input: opts.input,
                output: result.error ? undefined : truncate(result.output, 2000),
                error: result.error,
                category: result.category,
                tenant: opts.tenant,
                duration_ms,
            });
        },
    };
}

export function logSandbox(level: LogLevel, message: string, meta?: Record<string, unknown>): void {
    emit({
        level,
        ts: new Date().toISOString(),
        component: "sandbox",
        error: level === "error" ? message : undefined,
        meta: {
            ...(level !== "error" ? { message } : {}),
            ...meta,
        },
    });
}

function truncate(value: unknown, maxLen: number): unknown {
    if (value === undefined || value === null) return value;
    const s = typeof value === "string" ? value : JSON.stringify(value);
    if (s.length <= maxLen) return value;
    return s.slice(0, maxLen) + `...(truncated, ${s.length} chars total)`;
}
