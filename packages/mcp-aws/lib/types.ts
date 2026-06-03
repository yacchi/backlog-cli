import type { McpServerConfig } from "@backlog-cli/mcp-server";

export interface McpStackConfig {
    parameterName: string;
    parameterValue?: McpServerConfig;

    functionUrl?: {
        domainName?: string;
        certificateArn?: string;
        hostedZoneId?: string;
    };
}
