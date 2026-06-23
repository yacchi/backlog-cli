/**
 * Backlog OAuth リレー + MCP サーバーの統合コンテナランタイム。
 *
 * OAuth リレー（relay-core）と、MCP スペースが設定されている場合は MCP サーバー
 * （mcp-server）を 1 つの HTTP プロセスで提供する。同一イメージが以下で動作する。
 *
 * - ローカル / Docker: `RELAY_CONFIG`（JSON、secrets インライン）から設定取得
 * - AWS Lambda コンテナ: SSM + Secrets Manager から設定取得
 *   （`CONFIG_PARAMETER_NAME` / `RELAY_SECRETS_NAME`）、Lambda Web Adapter 経由
 *
 * 設定ソースは自動選択される — {@link ./config-source} を参照。
 */

import { serve } from "@hono/node-server";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  createSandboxClient,
  type McpServerConfig,
  type CreateMcpAppOptions,
} from "@yacchi/backlog-mcp-server";
import { loadPortalAssets } from "./portal-assets.js";
import { selectConfigSource, AwsConfigSource } from "./config-source.js";
import { createUnifiedApp, restoreMcpAuthorization } from "./app.js";

const __dirname = dirname(fileURLToPath(import.meta.url));

/**
 * 環境変数名（config-source の変数は {@link ./config-source} 側にある）。
 */
export const ENV_VARS = {
  HOST: "HOST",
  PORT: "PORT",
  WEB_DIST_PATH: "WEB_DIST_PATH",
  BACKLOG_BIN_PATH: "BACKLOG_BIN_PATH",
  SANDBOX_WORKER_PATH: "SANDBOX_WORKER_PATH",
} as const;

/**
 * web dist のパスを環境変数または既定値から取得する。
 */
function getWebDistPath(): string {
  const envPath = process.env[ENV_VARS.WEB_DIST_PATH];
  if (envPath) {
    return resolve(envPath);
  }
  // 既定: プロジェクトルート相対の web/dist を探す。
  return resolve(__dirname, "../../../web/dist");
}

// サンドボックス（Deno + Pyodide）は起動コストが高いため、1 つのクライアントを
// リクエスト間で再利用し、プロセス終了時にシャットダウンする。
let cachedSandbox: Awaited<ReturnType<typeof createSandboxClient>> | null = null;

/**
 * Deno サンドボックスを使う MCP の `runScript` 実装を遅延生成する。
 *
 * スペース未設定時、またはサンドボックスを起動できない場合（例: ローカルに Deno が無い）は
 * undefined を返す（run_script ツール無効）。後者はグレースフルに縮退し、Python サンドボックス
 * のツールチェーン無しでもローカルで relay + MCP `backlog` ツールが使えるようにする（警告ログを出す）。
 */
async function createRunScript(
  mcpConfig: McpServerConfig,
): Promise<CreateMcpAppOptions["runScript"]> {
  if (mcpConfig.spaces.length === 0) {
    return undefined;
  }

  if (!cachedSandbox) {
    try {
      cachedSandbox = await createSandboxClient({
        workerPath: process.env[ENV_VARS.SANDBOX_WORKER_PATH],
        binPath: process.env[ENV_VARS.BACKLOG_BIN_PATH],
      });
      process.on("SIGTERM", () => cachedSandbox?.shutdown());
      process.on("SIGINT", () => cachedSandbox?.shutdown());
    } catch (err) {
      console.warn(
        `MCP run_script disabled: sandbox failed to start (${err instanceof Error ? err.message : String(err)}). ` +
          "The relay and MCP backlog tool remain available.",
      );
      return undefined;
    }
  }

  return (script, token, scriptConfig, opts) =>
    cachedSandbox!.execute(
      script,
      token,
      scriptConfig,
      opts?.readOnly,
      opts?.files,
    );
}

/**
 * 統合 HTTP サーバーを起動する。
 */
export async function startServer(): Promise<void> {
  const configSource = selectConfigSource();
  const rawConfig = await configSource.loadRawConfig();

  const webDistPath = getWebDistPath();
  const portalAssets = await loadPortalAssets(webDistPath);

  const auditLogGroupName =
    process.env["AUDIT_LOG_GROUP_NAME"] ||
    (process.env["AWS_LAMBDA_FUNCTION_NAME"]
      ? `/aws/lambda/${process.env["AWS_LAMBDA_FUNCTION_NAME"]}`
      : undefined);
  const secretName = configSource instanceof AwsConfigSource
    ? configSource.secretName
    : undefined;
  const onConfigInvalidate = configSource instanceof AwsConfigSource
    ? () => configSource.invalidateCache()
    : undefined;

  const app = await createUnifiedApp({
    rawConfig,
    portalAssets,
    binPath: process.env[ENV_VARS.BACKLOG_BIN_PATH],
    createRunScript,
    auditLogGroupName,
    secretName,
    onConfigInvalidate,
  });

  // ポートの優先順位: PORT 環境変数（Lambda Web Adapter が設定）> config > 8080。
  const serverConfig = (rawConfig.server ?? {}) as { port?: number };
  const port =
    Number(process.env[ENV_VARS.PORT]) || serverConfig.port || 8080;
  const host = process.env[ENV_VARS.HOST] || "0.0.0.0";

  const mcpEnabled = Array.isArray(rawConfig.mcp_spaces)
    ? (rawConfig.mcp_spaces as unknown[]).length > 0
    : false;

  console.log(
    `Starting Backlog Relay${mcpEnabled ? " + MCP" : ""} server on ${host}:${port}`,
  );
  if (portalAssets) {
    console.log(`Portal assets loaded from: ${webDistPath}`);
  } else {
    console.log("Portal assets not available (build web package first)");
  }

  serve({
    // restoreMcpAuthorization は CloudFront 外では no-op。同一イメージが OAC 配下の
    // Lambda コンテナでも動くよう組み込んでいる。
    fetch: (request: Request) => app.fetch(restoreMcpAuthorization(request)),
    port,
    hostname: host,
  });
}

// 直接実行時（= コンテナのエントリポイント）に自動起動する。
startServer().catch((err) => {
  console.error("Failed to start server:", err);
  process.exit(1);
});

// カスタマイズ / テスト用にユーティリティをエクスポート。
export { loadPortalAssets } from "./portal-assets.js";
export { createUnifiedApp } from "./app.js";
export {
  selectConfigSource,
  EnvConfigSource,
  AwsConfigSource,
} from "./config-source.js";
