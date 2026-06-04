/**
 * Backlog MCP Server CDK 設定
 *
 * このファイルを config.ts にコピーして、値を設定してください:
 *   cp config.example.ts config.ts
 *
 * config.ts は .gitignore に含まれているため、
 * シークレットを含んでもリポジトリにコミットされません。
 */
import type { McpStackConfig } from "./lib/types.js";

export const config: McpStackConfig = {
    parameterName: "/backlog-mcp/config",
    parameterValue: {
        // MCP サーバーのベース URL（デプロイ後に Function URL で設定）
        base_url: "https://xxxxxxxxxx.lambda-url.ap-northeast-1.on.aws",

        // Relay サーバーの URL（トークン交換に使用）
        relay_url: "https://your-relay-server.example.com",

        // Backlog アプリ設定（Relay と同じ client_id を使用）
        backlog_apps: [
            {
                domain: "backlog.jp",
                client_id: "your-backlog-app-client-id",
            },
        ],

        // テナント設定（スペースごとのアクセス制御）
        tenants: {
            "your-space.backlog.jp": {
                cli_access: {
                    allow: [
                        "issue list *",
                        "issue view *",
                        "pr list *",
                        "pr view *",
                        "wiki list *",
                        "wiki view *",
                        "project list *",
                        "project view *",
                        "notification list *",
                        "api /api/v2/* -X GET",
                    ],
                    deny: [],
                },
                script: {
                    enabled: false,
                    max_cli_calls: 20,
                    timeout_ms: 30000,
                },
                // skill_projects: ["PROJ-A", "PROJ-B"],
            },
        },
    },

    // Secrets Manager シークレット名
    // JWE 暗号鍵（base64url-encoded 32 bytes）を格納する既存のシークレットを参照。
    //
    // 初回作成:
    //   aws secretsmanager create-secret \
    //     --name /backlog-mcp/token-key \
    //     --secret-string "$(node -e "console.log(require('crypto').randomBytes(32).toString('base64url'))")"
    //
    // ローテーション:
    //   SM のローテーション機構で新しい鍵を発行。旧鍵は AWSPREVIOUS ステージから自動取得され、
    //   復号にのみ使用される（明示的な token_key_prev 管理は不要）。
    secretName: "/backlog-mcp/token-key",
};
