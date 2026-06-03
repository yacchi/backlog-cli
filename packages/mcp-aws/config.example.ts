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

        // JWE 暗号鍵 (base64url-encoded 32 bytes)
        // 生成: node -e "console.log(require('crypto').randomBytes(32).toString('base64url'))"
        token_key: "YOUR_BASE64URL_ENCODED_32_BYTE_KEY",

        // 鍵ローテーション時に旧鍵を設定（復号のみに使用）
        // token_key_prev: "OLD_KEY",

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

    // カスタムドメイン（オプション）
    // functionUrl: {
    //     domainName: "mcp.example.com",
    //     certificateArn: "arn:aws:acm:ap-northeast-1:123456789012:certificate/xxx",
    //     hostedZoneId: "Z1234567890ABC",
    // },
};
