/**
 * Backlog OAuth Relay Server 設定
 *
 * このファイルを config.ts にコピーして、値を設定してください:
 *   cp config.example.ts config.ts
 *
 * config.ts は .gitignore に含まれているため、
 * シークレットを含んでもリポジトリにコミットされません。
 */
import type { RelayConfig } from "@yacchi/backlog-relay-aws-cdk";

// ============================================================
// Parameter Store 参照（設定の一元管理）
// ============================================================
export const config: RelayConfig = {
  parameterName: "/backlog-relay/config",
  parameterValue: {
    server: {},
    backlog_app: {
      client_id: "your-client-id",
      client_secret: "your-client-secret",
    },

    // ============================================================
    // 統合テナント設定
    // ============================================================
    // キーは "space.domain" 形式（例: "your-space.backlog.jp"）
    // relay（バンドル署名）と mcp（アクセス制御）を一箇所で管理
    //
    // tenants: {
    //   "your-space.backlog.jp": {
    //     // Relay バンドル署名設定
    //     // JWKS (Ed25519) と passphrase は省略可能 — デプロイ時に自動生成されます。
    //     // 自動生成された値は Secrets Manager に保存されます。
    //     // passphrase の平文は SM シークレットから取得できます。
    //     // default_space: "your-space.backlog.jp",  // CLI setup 時の --space デフォルト値
    //     relay: {
    //       // jwks: { keys: [...] },       // 省略で自動生成 (kid: "auto-1")
    //       // active_keys: "auto-1",       // 省略で "auto-1" がデフォルト
    //       // passphrase: "your-pass",     // 省略で自動生成（SM に平文も保存）
    //       // passphrase_length: 16,       // 自動生成時の文字数（デフォルト: 32）
    //       info_ttl: 600,
    //     },
    //     // MCP アクセス制御設定
    //     mcp: {
    //       cli_access: {
    //         allow: [
    //           "issue list *", "issue view *",
    //           "pr list *", "pr view *",
    //           "wiki list *", "wiki view *",
    //           "project list *", "project view *",
    //           "notification list *",
    //           "api /api/v2/* -X GET",
    //         ],
    //         deny: [],
    //       },
    //       script: { enabled: false, max_cli_calls: 20, timeout_ms: 30000 },
    //     },
    //   },
    // },
  },

  // ============================================================
  // MCP Server 統合（オプション）
  // ============================================================
  // mcp フィールドが存在し、いずれかのテナントに mcp 設定がある場合、
  // この Lambda が /mcp/* エンドポイントも提供します。
  // Backlog CLI のバイナリと Deno sandbox が Lambda にバンドルされます。
  //
  // MCP token key は Secrets Manager で自動生成・ローテーションされます。
  //
  // mcp: {
  //   tokenKeySecretName: "/backlog-mcp/token-key",  // default
  //   tokenKeyRotationDays: 30,                       // default: 30, 0 で無効
  // },
};
