/**
 * プラットフォーム非依存の統合アプリビルダー。
 *
 * relay アプリ（relay-core）と、MCP スペースが設定されている場合は MCP サーバーアプリ
 * （mcp-server）を 1 つの Hono アプリに mount する。Backlog OAuth は 1 アプリにつき
 * redirect_uri を 1 つしか登録できないため、共有 `/auth/callback` ディスパッチャも含む。
 *
 * このロジックは元々 AWS Lambda ハンドラに埋め込まれていたものを抽出し、同一ランタイムで
 * Docker / ローカル / Lambda コンテナの各ターゲットを提供できるようにしたもの。
 */

import { AsyncLocalStorage } from "node:async_hooks";
import { Hono, type Context, type Next } from "hono";
import { requestId } from "hono/request-id";
import { cors } from "hono/cors";
import {
  createRelayApp,
  createBundle,
  generateProvisioningToken,
  verifyPassphrase,
  parseConfig,
  extractRequestContext,
  type RelayConfig,
  type AuditEvent,
  type AuditLogger,
  type AuditLogReader,
  type PassphraseManager,
  type PortalAssets,
} from "@yacchi/backlog-relay-core";
import { CloudWatchLogsAuditReader } from "./audit-reader.js";
import { SecretsManagerPassphraseManager } from "./passphrase-manager.js";
import {
  createMcpApp,
  verify,
  loadSigningKeys,
  parseConfig as parseMcpConfig,
  Logger,
  LOGGER_CONTEXT_KEY,
  type McpServerConfig,
  type CreateMcpAppOptions,
  type TokenExchange,
} from "@yacchi/backlog-mcp-server";

/**
 * AsyncLocalStorage for propagating request-scoped Logger into audit logs.
 * The stored Logger already carries requestId / clientIp / userAgent bindings,
 * so audit events inherit them automatically.
 */
const loggerContextStore = new AsyncLocalStorage<{ logger: Logger }>();

/**
 * Create an AuditLogger that delegates to the request-scoped Logger stored in
 * AsyncLocalStorage. Falls back to `baseLogger` outside a request context.
 * This unifies audit event output with the structured Logger format so that
 * all log lines share the same schema (level, ts, requestId, etc.).
 */
function createLoggerAuditLogger(baseLogger: Logger): AuditLogger {
  return {
    log(event: AuditEvent) {
      const ctx = loggerContextStore.getStore();
      const logger = ctx?.logger ?? baseLogger;
      const { timestamp, result, ...fields } = event;
      const level = result === "error" ? "error" : "info";
      logger[level]({
        component: "audit",
        ...fields,
        result,
      });
    },
  };
}

/**
 * `x-mcp-authorization` から `Authorization` ヘッダーを復元する。
 *
 * CloudFront の Origin Access Control 配下では、OAC が viewer の `Authorization`
 * ヘッダーを SigV4 署名で上書きする。CloudFront Function が元の Bearer トークンを
 * `x-mcp-authorization` にコピーしておくので、ここで復元して MCP の Bearer 認証を成立
 * させる。`x-mcp-authorization` が無ければ no-op（= 素の Docker / 非 CloudFront 構成では無害）。
 */
export function restoreMcpAuthorization(req: Request): Request {
  const mcpAuth = req.headers.get("x-mcp-authorization");
  if (!mcpAuth) {
    return req;
  }
  const existing = req.headers.get("authorization");
  if (existing && existing.startsWith("Bearer ")) {
    return req;
  }
  const headers = new Headers(req.headers);
  headers.set("authorization", mcpAuth);
  return new Request(req, { headers });
}

/**
 * {@link createUnifiedApp} のオプション。
 */
export interface CreateUnifiedAppOptions {
  /** raw 設定オブジェクト（ConfigSource が secrets をマージ済み）。 */
  rawConfig: Record<string, unknown>;
  /** Portal SPA アセット（任意）。 */
  portalAssets?: PortalAssets;
  /** CloudWatch Logs のロググループ名（監査ログ閲覧用）。 */
  auditLogGroupName?: string;
  /** Secrets Manager のシークレット名（パスフレーズ管理用）。 */
  secretName?: string;
  /** 設定キャッシュ無効化コールバック（パスフレーズ更新後に呼ばれる）。 */
  onConfigInvalidate?: () => void;
  /** MCP の `backlog` ツール用の Backlog CLI バイナリパス。 */
  binPath?: string;
  /**
   * MCP サンドボックスの `runScript` 実装を生成するファクトリ。
   * MCP 有効時のみ呼ばれる。省略時は `run_script` を無効化する。
   */
  createRunScript?: (
    mcpConfig: McpServerConfig,
  ) => Promise<CreateMcpAppOptions["runScript"]>;
  /**
   * relay 設定に `server.base_url` が無い場合に使う base URL
   * （例: サーバーレスアダプタでリクエストから導出した値）。
   */
  baseUrlFallback?: string;
}

/**
 * raw 設定の `mcp_*` キーから MCP サーバー設定を構築する。
 *
 * MCP は relay のサーバーレベル JWKS と Backlog client_id を再利用し、relay と同じ署名鍵を
 * 使う（別のトークン鍵は持たない）。MCP 未設定、または前提が欠けている場合は null を返す。
 */
export function buildMcpConfig(
  rawConfig: Record<string, unknown>,
  relayConfig: RelayConfig,
  baseUrlFallback?: string,
): McpServerConfig | null {
  const mcpSpaces = rawConfig.mcp_spaces as
    | Array<{ pattern: string; writable: boolean }>
    | undefined;
  if (!mcpSpaces || mcpSpaces.length === 0) {
    return null;
  }

  const jwksJson = relayConfig.jwks;
  if (!jwksJson) {
    console.warn("MCP integration requires a server-level jwks in relay config");
    return null;
  }

  // base_url は任意: OAuth issuer は明示設定が無ければリクエストの host から
  // リクエストごとに導出する（resolveBaseUrl）。Function URL / CloudFront ドメインは
  // 循環依存なしでは deploy 時に確定できないため、ここで base_url を必須にしてはいけない。
  const baseUrl = relayConfig.server.base_url || baseUrlFallback;

  const mcpConfigObj: Record<string, unknown> = {
    jwks: jwksJson,
    backlog_app: {
      client_id: relayConfig.backlog_app.client_id,
    },
    spaces: mcpSpaces,
    script: rawConfig.mcp_script,
    default_spaces: rawConfig.mcp_default_spaces ?? [],
    audit: rawConfig.mcp_audit,
    logging: rawConfig.mcp_logging,
  };
  if (baseUrl) {
    mcpConfigObj.base_url = baseUrl;
  }

  return parseMcpConfig(JSON.stringify(mcpConfigObj));
}

/**
 * relay の BacklogAppConfig（client_secret を含む）を使うインプロセスの
 * {@link TokenExchange} を生成する。relay への HTTP 往復は不要。
 */
export function createDirectTokenExchange(
  relayConfig: RelayConfig,
): TokenExchange {
  const app = relayConfig.backlog_app;

  async function requestToken(
    tokenUrl: string,
    params: URLSearchParams,
  ): Promise<{
    access_token: string;
    token_type: string;
    expires_in: number;
    refresh_token: string;
  }> {
    const response = await fetch(tokenUrl, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: params.toString(),
    });
    const body = await response.text();
    if (!response.ok) {
      throw new Error(`Token request failed: ${body}`);
    }
    return JSON.parse(body);
  }

  return {
    // `space` は spaceHost 移行に伴い Backlog ホスト全体（例 "myspace.backlog.com"）。
    // space/domain に分割しないこと。
    async exchangeCode(space, code, redirectUri) {
      const params = new URLSearchParams();
      params.set("grant_type", "authorization_code");
      params.set("code", code);
      params.set("client_id", app.client_id);
      params.set("client_secret", app.client_secret);
      if (redirectUri) {
        params.set("redirect_uri", redirectUri);
      }
      return requestToken(`https://${space}/api/v2/oauth2/token`, params);
    },
    async refreshToken(space, refreshTokenValue) {
      const params = new URLSearchParams();
      params.set("grant_type", "refresh_token");
      params.set("refresh_token", refreshTokenValue);
      params.set("client_id", app.client_id);
      params.set("client_secret", app.client_secret);
      return requestToken(`https://${space}/api/v2/oauth2/token`, params);
    },
  };
}

/**
 * 統合 Hono アプリ（relay + 任意で MCP）を生成する。
 */
export async function createUnifiedApp(
  options: CreateUnifiedAppOptions,
): Promise<Hono> {
  const { rawConfig } = options;

  const relayConfig = parseConfig(JSON.stringify(rawConfig));
  const serverJwks = relayConfig.jwks;

  const app = new Hono();
  const serverConfig = rawConfig.server as { log_level?: string } | undefined;
  const logLevel = (serverConfig?.log_level ?? "info") as "debug" | "info" | "warn" | "error";
  const baseLogger = new Logger({}, logLevel);
  baseLogger.info({ component: "config", log_level: logLevel, log_level_raw: serverConfig?.log_level });

  const auditLogger = createLoggerAuditLogger(baseLogger);

  let auditLogReader: AuditLogReader | undefined;
  if (options.auditLogGroupName) {
    auditLogReader = new CloudWatchLogsAuditReader(options.auditLogGroupName);
  }

  let passphraseManager: PassphraseManager | undefined;
  if (options.secretName) {
    passphraseManager = new SecretsManagerPassphraseManager(
      options.secretName,
      options.onConfigInvalidate,
    );
  }

  const relayApp = createRelayApp({
    config: relayConfig,
    auditLogger,
    verifyPassphrase,
    createBundle: (tenant, domain, relayUrl, issuedBy) =>
      createBundle(tenant, domain, relayUrl, serverJwks, issuedBy),
    generateProvisionToken: (tenant, domain, relayUrl, issuedBy) =>
      generateProvisioningToken(tenant, domain, relayUrl, serverJwks, issuedBy),
    portalAssets: options.portalAssets,
    enablePortalOAuth: !!serverJwks,
    auditLogReader,
    passphraseManager,
  });

  // Request ID middleware — reuse Lambda Web Adapter's x-amzn-request-id when
  // available, otherwise fall back to a generated UUID. Hono's requestId()
  // checks X-Request-Id first (default headerName), then calls the generator.
  app.use("*", requestId({
    generator: (c) => c.req.header("x-amzn-request-id") ?? crypto.randomUUID(),
  }));

  // Access log middleware — all requests get IP/UA/method/path/status/duration.
  // Stores the request-scoped Logger in both Hono context and AsyncLocalStorage
  // so that downstream handlers AND the audit logger share the same bindings.
  app.use("*", async (c: Context, next: Next) => {
    const start = Date.now();
    const rid = c.get("requestId") as string;
    const reqCtx = extractRequestContext(c);
    const requestLogger = baseLogger.child({
      requestId: rid,
      clientIp: reqCtx.clientIp,
      userAgent: reqCtx.userAgent,
    });
    c.set(LOGGER_CONTEXT_KEY, requestLogger);
    await loggerContextStore.run({ logger: requestLogger }, async () => {
      await next();
    });
    requestLogger.info({
      component: "access",
      method: c.req.method,
      path: new URL(c.req.url).pathname,
      status: c.res.status,
      duration_ms: Date.now() - start,
    });
  });

  // MCP ブラウザクライアント（MCP Inspector 等）向けの CORS。relay エンドポイントは
  // Cookie ベースかつ same-origin なので無害。
  app.use(
    "*",
    cors({
      origin: "*",
      allowMethods: ["GET", "POST", "DELETE", "OPTIONS"],
      allowHeaders: [
        "Content-Type",
        "Authorization",
        "Accept",
        "MCP-Protocol-Version",
      ],
      exposeHeaders: ["WWW-Authenticate"],
    }),
  );

  const mcpConfig = buildMcpConfig(
    rawConfig,
    relayConfig,
    options.baseUrlFallback,
  );

  if (mcpConfig) {
    const tokenExchange = createDirectTokenExchange(relayConfig);
    const runScript = options.createRunScript
      ? await options.createRunScript(mcpConfig)
      : undefined;

    const mcpApp = await createMcpApp({
      config: mcpConfig,
      binPath: options.binPath,
      runScript,
      tokenExchange,
      callbackPath: "/auth/callback",
    });

    // 共有 /auth/callback: Backlog OAuth は 1 アプリにつき redirect_uri が 1 つのみ。
    // state を MCP JWT として検証してみて、成功すれば MCP、失敗すれば relay にフォールバック。
    const mcpKeys = await loadSigningKeys(mcpConfig.jwks);

    app.get("/auth/callback", async (c) => {
      const state = c.req.query("state");
      if (state) {
        try {
          await verify(state, mcpKeys.verifyKeys);
          const url = new URL(c.req.url);
          url.pathname = "/mcp/authorize/callback";
          return await mcpApp.fetch(new Request(url.toString(), c.req.raw));
        } catch {
          // MCP state ではない — relay にフォールバック。
        }
      }
      return relayApp.fetch(c.req.raw);
    });

    app.route("/", mcpApp);
  }

  app.route("/", relayApp);

  return app;
}
