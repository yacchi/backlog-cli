import { handle, type LambdaEvent, type LambdaContext } from "hono/aws-lambda";
import {
    createMcpApp,
    createSandboxClient,
    parseConfig,
    type McpServerConfig,
    type CreateMcpAppOptions,
} from "@backlog-cli/mcp-server";

export const ENV_VARS = {
    MCP_CONFIG: "MCP_CONFIG",
    CONFIG_PARAMETER_NAME: "CONFIG_PARAMETER_NAME",
    BACKLOG_BIN_PATH: "BACKLOG_BIN_PATH",
    DENO_PATH: "DENO_PATH",
} as const;

let cachedConfig: McpServerConfig | null = null;
let cachedSandbox: Awaited<ReturnType<typeof createSandboxClient>> | null = null;

async function getConfig(): Promise<McpServerConfig> {
    if (cachedConfig) {
        return cachedConfig;
    }

    const envConfig = process.env[ENV_VARS.MCP_CONFIG];
    if (envConfig) {
        cachedConfig = parseConfig(envConfig);
        return cachedConfig;
    }

    const parameterName = process.env[ENV_VARS.CONFIG_PARAMETER_NAME];
    if (parameterName) {
        const { SSMClient, GetParameterCommand } = await import(
            "@aws-sdk/client-ssm"
        );
        const client = new SSMClient({});
        const response = await client.send(
            new GetParameterCommand({
                Name: parameterName,
                WithDecryption: true,
            }),
        );

        if (!response.Parameter?.Value) {
            throw new Error(`SSM parameter ${parameterName} not found or empty`);
        }

        cachedConfig = parseConfig(response.Parameter.Value);
        return cachedConfig;
    }

    throw new Error(
        `Either ${ENV_VARS.MCP_CONFIG} or ${ENV_VARS.CONFIG_PARAMETER_NAME} environment variable is required`,
    );
}

async function getSandbox(config: McpServerConfig): Promise<CreateMcpAppOptions["runScript"]> {
    const hasSandboxEnabled = Object.values(config.tenants).some(
        (t) => t.script?.enabled,
    );
    if (!hasSandboxEnabled) {
        return undefined;
    }

    if (!cachedSandbox) {
        cachedSandbox = await createSandboxClient({
            denoPath: process.env[ENV_VARS.DENO_PATH],
            binPath: process.env[ENV_VARS.BACKLOG_BIN_PATH],
        });
    }

    return (script, token, tenant) => cachedSandbox!.execute(script, token, tenant);
}

export const handler = async (event: LambdaEvent, context: LambdaContext) => {
    const config = await getConfig();
    const runScript = await getSandbox(config);

    const app = createMcpApp({
        config,
        binPath: process.env[ENV_VARS.BACKLOG_BIN_PATH],
        runScript,
    });

    const lambdaHandler = handle(app);
    return lambdaHandler(event, context);
};

export { getConfig };
