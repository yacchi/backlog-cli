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
    TOKEN_KEY_SECRET_NAME: "TOKEN_KEY_SECRET_NAME",
    BACKLOG_BIN_PATH: "BACKLOG_BIN_PATH",
    SANDBOX_WORKER_PATH: "SANDBOX_WORKER_PATH",
} as const;

let cachedConfig: McpServerConfig | null = null;
let cachedSandbox: Awaited<ReturnType<typeof createSandboxClient>> | null = null;

async function getConfig(): Promise<McpServerConfig> {
    if (cachedConfig) {
        return cachedConfig;
    }

    const baseConfig = await loadBaseConfig();
    const secretName = process.env[ENV_VARS.TOKEN_KEY_SECRET_NAME];
    if (secretName) {
        const { current, previous } = await getTokenKeys(secretName);
        baseConfig.token_key = current;
        if (previous) {
            baseConfig.token_key_prev = previous;
        }
    }

    cachedConfig = parseConfig(JSON.stringify(baseConfig));
    return cachedConfig;
}

async function loadBaseConfig(): Promise<Record<string, unknown>> {
    const envConfig = process.env[ENV_VARS.MCP_CONFIG];
    if (envConfig) {
        return JSON.parse(envConfig);
    }

    const parameterName = process.env[ENV_VARS.CONFIG_PARAMETER_NAME];
    if (!parameterName) {
        throw new Error(
            `Either ${ENV_VARS.MCP_CONFIG} or ${ENV_VARS.CONFIG_PARAMETER_NAME} environment variable is required`,
        );
    }

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

    return JSON.parse(response.Parameter.Value);
}

async function getTokenKeys(
    secretName: string,
): Promise<{ current: string; previous?: string }> {
    const {
        SecretsManagerClient,
        GetSecretValueCommand,
    } = await import("@aws-sdk/client-secrets-manager");
    const client = new SecretsManagerClient({});

    const currentResp = await client.send(
        new GetSecretValueCommand({
            SecretId: secretName,
            VersionStage: "AWSCURRENT",
        }),
    );
    if (!currentResp.SecretString) {
        throw new Error(`Secret ${secretName} (AWSCURRENT) not found or empty`);
    }

    let previous: string | undefined;
    try {
        const prevResp = await client.send(
            new GetSecretValueCommand({
                SecretId: secretName,
                VersionStage: "AWSPREVIOUS",
            }),
        );
        previous = prevResp.SecretString ?? undefined;
    } catch {
        // AWSPREVIOUS doesn't exist yet (no rotation has occurred)
    }

    return { current: currentResp.SecretString, previous };
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
            workerPath: process.env[ENV_VARS.SANDBOX_WORKER_PATH],
            binPath: process.env[ENV_VARS.BACKLOG_BIN_PATH],
        });
    }

    return (script, token, tenant, opts) => cachedSandbox!.execute(script, token, tenant, opts?.readOnly);
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
