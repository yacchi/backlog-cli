import type { McpServerConfig } from "@backlog-cli/mcp-server";

export type McpConfigWithoutKeys = Omit<McpServerConfig, "token_key" | "token_key_prev">;

export interface McpStackConfig {
    parameterName: string;
    parameterValue?: McpConfigWithoutKeys;

    secretName: string;
}
