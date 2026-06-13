/**
 * 設定ソースの抽象化（環境適応ローダ）。
 *
 * 統合ランタイムは起動時に、以下の 2 つのソースから自動選択して raw 設定を読み込む。
 *
 * - {@link EnvConfigSource}: `RELAY_CONFIG`（JSON、secrets インライン）を読む。
 *   Docker / ローカル実行で使用。
 * - {@link AwsConfigSource}: SSM Parameter Store + Secrets Manager を読み、
 *   secrets を raw 設定にマージする。同一イメージを Lambda コンテナとして動かす際に使用
 *   （`CONFIG_PARAMETER_NAME` / `RELAY_SECRETS_NAME` 設定時）。
 *
 * いずれも素のオブジェクト（*raw* 設定）を返し、後段が relay-core の `parseConfig` で
 * 検証する。raw オブジェクトは MCP 固有のキー（`mcp_spaces` / `mcp_script` /
 * `mcp_default_spaces`）も保持する。これらは Zod が `RelayConfig` から除去するが、
 * {@link ./app.buildMcpConfig} が直接読む。
 *
 * AWS SDK クライアントは static import する。コンテナイメージは常に依存を同梱するため、
 * dynamic import の利得は env モード起動時の数十ms 程度に留まり、コードの複雑さに見合わない。
 */

import { SSMClient, GetParameterCommand } from "@aws-sdk/client-ssm";
import {
  SecretsManagerClient,
  GetSecretValueCommand,
} from "@aws-sdk/client-secrets-manager";

/**
 * 設定ローダが認識する環境変数名。
 */
export const CONFIG_ENV_VARS = {
  /** インライン JSON 設定（Docker / ローカル）。secrets はインライン前提。 */
  RELAY_CONFIG: "RELAY_CONFIG",
  /** 非機密設定を保持する SSM Parameter Store 名（AWS）。 */
  CONFIG_PARAMETER_NAME: "CONFIG_PARAMETER_NAME",
  /** client_secret / jwks / passphrase_hash を保持する Secrets Manager 名（AWS）。 */
  RELAY_SECRETS_NAME: "RELAY_SECRETS_NAME",
} as const;

/**
 * secrets をマージ済みの raw relay 設定のソース。
 */
export interface ConfigSource {
  /** raw 設定オブジェクトを読み込む（secrets マージ済み）。 */
  loadRawConfig(): Promise<Record<string, unknown>>;
}

/**
 * AWS Secrets Manager に保存される secrets のペイロード。
 * CDK construct が生成する形と一致させる。
 */
export interface RelaySecrets {
  app?: { client_secret: string };
  server?: { jwks?: string };
  tenants?: Record<string, { jwks?: string; passphrase_hash?: string }>;
}

/**
 * AWS Secrets Manager の値を raw SSM 設定オブジェクトにマージする（破壊的）。
 *
 * AWS なしで単体テストできるよう純粋関数として切り出す。relay-aws の旧 handler
 * ロジックと同期を保つ。
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

  // サーバーレベル JWKS: server.jwks を優先し、無ければ最初のテナントの jwks にフォールバック
  // （サーバーレベル JWKS 移行前に作られた設定をカバー）。
  const serverJwks =
    secrets.server?.jwks ??
    Object.values(secrets.tenants ?? {}).find((t) => t.jwks)?.jwks;
  if (serverJwks) {
    raw.jwks = serverJwks;
  }

  // テナントごとの passphrase_hash を secrets からマージ。
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
 * `RELAY_CONFIG` のインライン JSON 設定を読む。
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
 * 非機密設定を SSM から読み、Secrets Manager の secrets をマージする。
 * 結果はインスタンスの生存期間中キャッシュする。
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
 * 環境変数から適切な設定ソースを選択する。
 *
 * `RELAY_CONFIG` があれば優先（env モード）、無ければ `CONFIG_PARAMETER_NAME` で
 * AWS モードを選ぶ。どちらも無ければエラー。
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
