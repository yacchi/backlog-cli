import { z } from "zod";

export const SpacePatternSchema = z.object({
    pattern: z.string(),
    writable: z.boolean(),
});

export const ScriptConfigSchema = z.object({
    max_cli_calls: z.number().positive().default(20),
    timeout_ms: z.number().positive().default(30000),
});

export const AuditConfigSchema = z.object({
    collect_user_info: z.boolean().default(true),
});

export const LoggingConfigSchema = z.object({
    debug: z.boolean().default(false),
    input: z.boolean().default(false),
    output: z.boolean().default(false),
});

export const McpServerConfigSchema = z.object({
    // Optional explicit OAuth issuer. When omitted, the base URL is derived from
    // the request host at runtime (see resolveBaseUrl).
    base_url: z.string().url().optional(),
    relay_url: z.string().url().optional(),
    backlog_app: z.object({
        client_id: z.string().min(1),
    }),
    jwks: z.string().min(1),
    spaces: z.array(SpacePatternSchema).min(1),
    script: ScriptConfigSchema.optional(),
    default_spaces: z.array(z.string()).default([]),
    audit: AuditConfigSchema.optional(),
    logging: LoggingConfigSchema.optional(),
});

export type McpServerConfig = z.output<typeof McpServerConfigSchema>;
export type SpacePattern = z.output<typeof SpacePatternSchema>;
export type ScriptConfig = z.output<typeof ScriptConfigSchema>;

export function parseConfig(json: string): McpServerConfig {
    return McpServerConfigSchema.parse(JSON.parse(json));
}

export interface SpaceAccess {
    writable: boolean;
}

export function matchSpacePattern(spaceKey: string, patterns: SpacePattern[]): SpaceAccess | null {
    for (const p of patterns) {
        try {
            if (new RegExp(`^${p.pattern}$`).test(spaceKey)) {
                return { writable: p.writable };
            }
        } catch {
            // invalid regex — skip
        }
    }
    return null;
}
