/**
 * Backlog OAuth Relay Server 開発用設定
 *
 * このファイルを config.dev.ts にコピーして、値を設定してください:
 *   cp config.dev.example.ts config.dev.ts
 *
 * config.dev.ts は .gitignore に含まれているため、
 * シークレットを含んでもリポジトリにコミットされません。
 *
 * Node.js 24+ では erasableSyntaxOnly が有効なため、
 * 直接 import して使用できます。
 */
import type { RelayConfig } from "@backlog-cli/relay-core";

export const config: RelayConfig = {
  server: {
    port: 3000,
  },
  backlog_apps: [
    {
      domain: "backlog.jp",
      client_id: "YOUR_CLIENT_ID",
      client_secret: "YOUR_CLIENT_SECRET",
    },
  ],
  // tenants: [
  //   {
  //     allowed_domain: "your-space.backlog.jp",
  //     passphrase_hash: "$2a$12$...",
  //     jwks: JSON.stringify({
  //       keys: [
  //         {
  //           kty: "OKP",
  //           crv: "Ed25519",
  //           kid: "2025-01",
  //           d: "BASE64URL_ENCODED_PRIVATE_KEY",
  //         },
  //       ],
  //     }),
  //     active_keys: "2025-01",
  //   },
  // ],
};
