/**
 * Backlog OAuth Relay Server 設定
 *
 * このファイルを config.ts にコピーして、値を設定してください:
 *   cp config.example.ts config.ts
 *
 * config.ts は .gitignore に含まれているため、
 * シークレットを含んでもリポジトリにコミットされません。
 */
import { RelayConfig } from "./lib/types.js";

// ============================================================
// Parameter Store 参照（設定の一元管理）
// ============================================================
export const config: RelayConfig = {
  parameterName: "/backlog-relay/config",
  parameterValue: {
    server: {
      port: 8080,
    },
    backlog_apps: [
      {
        domain: "backlog.jp",
        client_id: "your-client-id",
        client_secret: "your-client-secret",
      },
    ],
    // tenants: [
    //   {
    //     allowed_domain: "spaceid.backlog.jp",
    //     jwks: {
    //       keys: [
    //         {
    //           kty: "OKP",
    //           crv: "Ed25519",
    //           kid: "2025-01",
    //           x: "...",  // 公開鍵
    //           d: "...",  // 秘密鍵
    //         },
    //       ],
    //     },
    //     active_keys: "2025-01",
    //     info_ttl: 600,
    //     passphrase_hash: "$2a$12$...",
    //   },
    // ],
  },
};
