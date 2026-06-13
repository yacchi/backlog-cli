/**
 * Configuration source abstraction (environment-adaptive loader).
 *
 * The unified runtime loads its raw configuration from one of two sources,
 * selected automatically at startup:
 *
 * - {@link EnvConfigSource}: reads `RELAY_CONFIG` (JSON, secrets inline).
 *   Used for Docker / local execution.
 * - {@link AwsConfigSource}: reads SSM Parameter Store + Secrets Manager and
 *   merges the secrets into the raw config. Used when the same image runs as a
 *   Lambda container (`CONFIG_PARAMETER_NAME` / `RELAY_SECRETS_NAME` set).
 *
 * Both return a plain object (the *raw* config) which downstream code validates
 * with relay-core's `parseConfig`. The raw object also carries MCP-specific keys
 * (`mcp_spaces`, `mcp_script`, `mcp_default_spaces`) that Zod strips from
 * `RelayConfig` but {@link ./app.buildMcpConfig} reads directly.
 *
 * AWS SDK clients are imported statically: the container image always bundles
 * them, so dynamic import would only save tens of milliseconds on env-mode
 * startup at the cost of more complex code.
 */

import { SSMClient, GetParameterCommand } from "@aws-sdk/client-ssm";
import {
  SecretsManagerClient,
  GetSecretValueCommand,
} from "@aws-sdk/client-secrets-manager";

/**
 * Environment variable names recognized by the config loader.
 */
export const CONFIG_ENV_VARS = {
  /** Inline JSON config (Docker / local). Secrets expected inline. */
  RELAY_CONFIG: "RELAY_CONFIG",
  /** SSM Parameter Store name holding the non-secret config (AWS). */
  CONFIG_PARAMETER_NAME: "CONFIG_PARAMETER_NAME",
  /** Secrets Manager secret name holding client_secret / jwks / passphrase_hash (AWS). */
  RELAY_SECRETS_NAME: "RELAY_SECRETS_NAME",
} as const;

/**
 * A source of raw relay configuration with secrets already merged in.
 */
export interface ConfigSource {
  /** Load the raw configuration object (secrets merged). */
  loadRawConfig(): Promise<Record<string, unknown>>;
}

/**
 * Secrets payload stored in AWS Secrets Manager.
 * Mirrors the shape produced by the CDK construct.
 */
export interface RelaySecrets {
  app?: { client_secret: string };
  server?: { jwks?: string };
  tenants?: Record<string, { jwks?: string; passphrase_hash?: string }>;
}

/**
 * Merge AWS Secrets Manager values into a raw SSM config object (in place).
 *
 * Extracted as a pure function so the merge behavior can be unit-tested without
 * AWS. Kept in sync with the historical relay-aws handler logic.
 */
export function mergeSecrets(
  raw: Record<string, unknown>,
  secrets: RelaySecrets,
): Record<string, unknown> {
  if (raw.backlog_app && secrets.app) {
    raw.backlog_app = {
      ...(raw.backlog_app as Record<string, unknown>),
      client_secret: secrets.app.client_secret,
    };
  }

  // Server-level JWKS: prefer server.jwks, fall back to the first tenant's jwks
  // (covers configs created before the server-level JWKS migration).
  const serverJwks =
    secrets.server?.jwks ??
    Object.values(secrets.tenants ?? {}).find((t) => t.jwks)?.jwks;
  if (serverJwks) {
    raw.jwks = serverJwks;
  }

  // Merge per-tenant passphrase_hash from secrets.
  if (Array.isArray(raw.tenants) && secrets.tenants) {
    raw.tenants = (raw.tenants as Array<Record<string, unknown>>).map((t) => ({
      ...t,
      passphrase_hash:
        secrets.tenants?.[t.name as string]?.passphrase_hash ??
        t.passphrase_hash,
    }));
  }

  return raw;
}

/**
 * Reads inline JSON config from `RELAY_CONFIG`.
 */
export class EnvConfigSource implements ConfigSource {
  private readonly json: string;

  constructor(json: string) {
    this.json = json;
  }

  async loadRawConfig(): Promise<Record<string, unknown>> {
    return JSON.parse(this.json) as Record<string, unknown>;
  }
}

/**
 * Reads non-secret config from SSM and merges secrets from Secrets Manager.
 * Result is cached for the lifetime of the instance.
 */
export class AwsConfigSource implements ConfigSource {
  private cached: Record<string, unknown> | null = null;
  private readonly parameterName: string;
  private readonly secretName?: string;

  constructor(parameterName: string, secretName?: string) {
    this.parameterName = parameterName;
    this.secretName = secretName;
  }

  async loadRawConfig(): Promise<Record<string, unknown>> {
    if (this.cached) {
      return this.cached;
    }

    const raw = await this.loadParameter();
    if (this.secretName) {
      const secrets = await this.loadSecrets(this.secretName);
      mergeSecrets(raw, secrets);
    }

    this.cached = raw;
    return raw;
  }

  private async loadParameter(): Promise<Record<string, unknown>> {
    const client = new SSMClient({});
    const response = await client.send(
      new GetParameterCommand({
        Name: this.parameterName,
        WithDecryption: true,
      }),
    );
    if (!response.Parameter?.Value) {
      throw new Error(
        `SSM parameter ${this.parameterName} not found or empty`,
      );
    }
    return JSON.parse(response.Parameter.Value) as Record<string, unknown>;
  }

  private async loadSecrets(secretName: string): Promise<RelaySecrets> {
    const client = new SecretsManagerClient({});
    const response = await client.send(
      new GetSecretValueCommand({ SecretId: secretName }),
    );
    if (!response.SecretString) {
      throw new Error(`Secret ${secretName} not found or empty`);
    }
    return JSON.parse(response.SecretString) as RelaySecrets;
  }
}

/**
 * Select the appropriate config source from environment variables.
 *
 * `RELAY_CONFIG` takes precedence (env mode); otherwise `CONFIG_PARAMETER_NAME`
 * selects AWS mode. Throws if neither is present.
 */
export function selectConfigSource(
  env: NodeJS.ProcessEnv = process.env,
): ConfigSource {
  const envConfig = env[CONFIG_ENV_VARS.RELAY_CONFIG];
  if (envConfig) {
    return new EnvConfigSource(envConfig);
  }

  const parameterName = env[CONFIG_ENV_VARS.CONFIG_PARAMETER_NAME];
  if (parameterName) {
    return new AwsConfigSource(
      parameterName,
      env[CONFIG_ENV_VARS.RELAY_SECRETS_NAME],
    );
  }

  throw new Error(
    `Either ${CONFIG_ENV_VARS.RELAY_CONFIG} or ${CONFIG_ENV_VARS.CONFIG_PARAMETER_NAME} environment variable is required`,
  );
}
