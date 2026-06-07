import { z } from "zod";

export const CliAccessSchema = z.object({
    allow: z.array(z.string()),
    deny: z.array(z.string()).default([]),
});

export const ScriptConfigSchema = z.object({
    enabled: z.boolean().default(false),
    max_cli_calls: z.number().positive().default(20),
    timeout_ms: z.number().positive().default(30000),
});

export const McpTenantSchema = z.object({
    cli_access: CliAccessSchema.optional(),
    script: ScriptConfigSchema.optional(),
    skill_projects: z.array(z.string()).optional(),
});

export const McpServerConfigSchema = z.object({
    base_url: z.string().url(),
    relay_url: z.string().url().optional(),
    token_key: z.string().min(1),
    token_key_prev: z.string().optional(),
    backlog_app: z.object({
        client_id: z.string().min(1),
    }),
    tenants: z.record(z.string(), McpTenantSchema).default({}),
});

export type McpServerConfig = z.output<typeof McpServerConfigSchema>;
export type McpTenant = z.output<typeof McpTenantSchema>;
export type CliAccess = z.output<typeof CliAccessSchema>;

export function parseConfig(json: string): McpServerConfig {
    return McpServerConfigSchema.parse(JSON.parse(json));
}
