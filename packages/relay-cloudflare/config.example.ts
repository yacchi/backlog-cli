/**
 * Backlog OAuth Relay Server 設定 (Cloudflare Workers)
 *
 * このファイルを config.ts にコピーして、値を設定してください:
 *   cp config.example.ts config.ts
 *
 * config.ts は .gitignore に含まれているため、
 * シークレットを含んでもリポジトリにコミットされません。
 *
 * 設定後、以下のコマンドでデプロイできます:
 *   pnpm deploy        # 本番環境
 *   pnpm deploy:dev    # 開発環境
 *   pnpm dev           # ローカル開発サーバー（.dev.vars を自動生成）
 */
import type { RelayConfig } from "@backlog-cli/relay-core";

/**
 * Cloudflare Workers 固有の設定
 */
export interface CloudflareConfig {
  /** デプロイ先の環境 (dev, staging, production) */
  environment?: "dev" | "staging" | "production";
}

/**
 * 設定エクスポート
 */
export const cloudflareConfig: CloudflareConfig = {
  // environment: "dev",  // 開発環境へデプロイする場合
};

/**
 * Relay サーバー設定
 */
export const config: RelayConfig = {
  server: {
    // Cloudflare Workers では base_url を設定するか、
    // allowed_host_patterns でホスト名を許可する必要があります
    // base_url: "https://your-worker.your-subdomain.workers.dev",
    allowed_host_patterns: "*.workers.dev",
    port: 8787, // ローカル開発サーバーのポート
  },
  backlog_apps: [
    {
      domain: "backlog.jp",
      client_id: "your-client-id-for-backlog-jp",
      client_secret: "your-client-secret-for-backlog-jp",
    },
    // backlog.com も使う場合は追加
    // {
    //   domain: "backlog.com",
    //   client_id: "your-client-id-for-backlog-com",
    //   client_secret: "your-client-secret-for-backlog-com",
    // },
  ],
  // マルチテナント設定（オプション）
  // tenants: [
  //   {
  //     allowed_domain: "your-space.backlog.jp",
  //     jwks: JSON.stringify({
  //       keys: [
  //         {
  //           kty: "OKP",
  //           crv: "Ed25519",
  //           kid: "2025-01",
  //           x: "...",  // 公開鍵
  //           d: "...",  // 秘密鍵
  //         },
  //       ],
  //     }),
  //     active_keys: "2025-01",
  //     info_ttl: 600,
  //     passphrase_hash: "$2a$12$...",
  //   },
  // ],
};
