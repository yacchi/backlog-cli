/**
 * AWS Lambda adapter for Backlog OAuth Relay Server.
 */

import { handle, type LambdaEvent, type LambdaContext } from "hono/aws-lambda";
import {
  createRelayApp,
  type RelayConfig,
  type AuditLogger,
  type AuditEvent,
} from "@backlog-cli/relay-core";

/**
 * Environment variable names for AWS Lambda.
 */
export const ENV_VARS = {
  RELAY_CONFIG: "RELAY_CONFIG",
  CONFIG_PARAMETER_NAME: "CONFIG_PARAMETER_NAME",
} as const;

let cachedConfig: RelayConfig | null = null;

/**
 * Parse relay configuration from environment or SSM.
 */
async function getConfig(): Promise<RelayConfig> {
  if (cachedConfig) {
    return cachedConfig;
  }

  // First, try to read from environment variable
  const envConfig = process.env[ENV_VARS.RELAY_CONFIG];
  if (envConfig) {
    cachedConfig = JSON.parse(envConfig) as RelayConfig;
    return cachedConfig;
  }

  // If SSM parameter name is provided, fetch from SSM
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
      })
    );

    if (!response.Parameter?.Value) {
      throw new Error(`SSM parameter ${parameterName} not found or empty`);
    }

    cachedConfig = JSON.parse(response.Parameter.Value) as RelayConfig;
    return cachedConfig;
  }

  throw new Error(
    `Either ${ENV_VARS.RELAY_CONFIG} or ${ENV_VARS.CONFIG_PARAMETER_NAME} environment variable is required`
  );
}

/**
 * Create an AuditLogger that logs to CloudWatch.
 */
function createAWSAuditLogger(): AuditLogger {
  return {
    log(event: AuditEvent): void {
      // CloudWatch Logs automatically captures console output
      console.log(JSON.stringify(event));
    },
  };
}

/**
 * Default Lambda handler.
 */
export const handler = async (event: LambdaEvent, context: LambdaContext) => {
  const config = await getConfig();
  const auditLogger = createAWSAuditLogger();

  const app = createRelayApp({
    config,
    auditLogger,
  });

  const lambdaHandler = handle(app);
  return lambdaHandler(event, context);
};

// Export utilities for customization
export { getConfig, createAWSAuditLogger };
