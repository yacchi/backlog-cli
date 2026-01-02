/**
 * AWS Lambda adapter for Backlog OAuth Relay Server.
 */

import { handle, type LambdaEvent, type LambdaContext } from "hono/aws-lambda";
import bcrypt from "bcryptjs";
import {
  createRelayApp,
  type RelayConfig,
  type AuditLogger,
  type AuditEvent,
  type TenantConfig,
  type PortalAssets,
} from "@backlog-cli/relay-core";
import { createBundle } from "./bundle.js";
import { loadPortalAssets } from "./portal-assets.js";

/**
 * Environment variable names for AWS Lambda.
 */
export const ENV_VARS = {
  RELAY_CONFIG: "RELAY_CONFIG",
  CONFIG_PARAMETER_NAME: "CONFIG_PARAMETER_NAME",
} as const;

/**
 * Parameter Store value format (from CDK).
 */
interface ParameterStoreValue {
  server?: {
    backlog?: Record<string, { client_id: string; client_secret: string }>;
    allowed_host_patterns?: string;
    tenants?: Record<
      string,
      {
        allowed_domain: string;
        jwks?: string;
        active_keys?: string;
        info_ttl?: number;
        passphrase_hash?: string;
      }
    >;
  };
}

let cachedConfig: RelayConfig | null = null;

/**
 * Transform Parameter Store config to RelayConfig format.
 */
function transformConfig(paramValue: ParameterStoreValue): RelayConfig {
  const server = paramValue.server ?? {};

  // Transform backlog apps from Record to array
  const backlogApps = Object.entries(server.backlog ?? {}).map(
    ([domain, app]) => ({
      domain: domain === "jp" ? "backlog.jp" : domain === "com" ? "backlog.com" : domain,
      clientId: app.client_id,
      clientSecret: app.client_secret,
    })
  );

  // Transform tenants from Record to array
  const tenants = Object.entries(server.tenants ?? {}).map(
    ([_id, tenant]) => ({
      allowedDomain: tenant.allowed_domain,
      passphraseHash: tenant.passphrase_hash,
      // Additional fields for info endpoint
      jwks: tenant.jwks,
      activeKeys: tenant.active_keys,
      infoTtl: tenant.info_ttl,
    })
  );

  return {
    server: {
      port: 8080,
      allowedHostPatterns: server.allowed_host_patterns,
    },
    backlogApps,
    tenants: tenants.length > 0 ? tenants : undefined,
  };
}

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
    const parsed = JSON.parse(envConfig);
    // Check if it's already in RelayConfig format or needs transformation
    if (parsed.backlogApps) {
      cachedConfig = parsed as RelayConfig;
    } else {
      cachedConfig = transformConfig(parsed as ParameterStoreValue);
    }
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

    const parsed = JSON.parse(response.Parameter.Value);
    cachedConfig = transformConfig(parsed as ParameterStoreValue);
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
 * Verify passphrase using bcrypt.
 */
async function verifyPassphrase(
  hash: string,
  passphrase: string
): Promise<boolean> {
  try {
    return await bcrypt.compare(passphrase, hash);
  } catch {
    console.error("[verifyPassphrase] Failed to compare passphrase");
    return false;
  }
}

/**
 * Create bundle wrapper function.
 */
async function createBundleWrapper(
  tenant: TenantConfig,
  domain: string,
  relayUrl: string
): Promise<Uint8Array> {
  return createBundle(tenant, domain, relayUrl);
}

// Load portal assets at module initialization (cold start)
let cachedPortalAssets: PortalAssets | undefined;
function getPortalAssets(): PortalAssets | undefined {
  if (cachedPortalAssets === undefined) {
    cachedPortalAssets = loadPortalAssets();
  }
  return cachedPortalAssets;
}

/**
 * Default Lambda handler.
 */
export const handler = async (event: LambdaEvent, context: LambdaContext) => {
  const config = await getConfig();
  const auditLogger = createAWSAuditLogger();
  const portalAssets = getPortalAssets();

  const app = createRelayApp({
    config,
    auditLogger,
    verifyPassphrase,
    createBundle: createBundleWrapper,
    portalAssets,
  });

  const lambdaHandler = handle(app);
  return lambdaHandler(event, context);
};

// Export utilities for customization
export { getConfig, createAWSAuditLogger, verifyPassphrase };
