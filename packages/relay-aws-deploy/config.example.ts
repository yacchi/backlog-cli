/**
 * Backlog OAuth Relay Server 設定
 *
 * このファイルを config.ts にコピーして、値を設定してください:
 *   cp config.example.ts config.ts
 *
 * config.ts は .gitignore に含まれているため、
 * シークレットを含んでもリポジトリにコミットされません。
 *
 * 最小構成は backlog_app（Client ID/Secret）のみ。それ以外はデフォルトで動作します。
 *   - parameterName: 省略時 "/backlog-relay/config"
 *   - server:        省略可（port は省略時 8080）
 *   - cloudFront:    省略時に有効（無効化する場合のみ { enabled: false }）
 *   - jwks/passphrase: 省略時に Secrets Manager で自動生成
 */
import type { RelayConfig } from "@yacchi/backlog-relay-aws-cdk";

export const config: RelayConfig = {
  parameterValue: {
    // 必須: Backlog アプリの OAuth クレデンシャル
    backlog_app: {
      client_id: "your-client-id",
      client_secret: "your-client-secret",
    },

    // ============================================================
    // テナント設定（オプション）
    // ============================================================
    // テナント = バンドル配布単位（name で識別、Backlog スペースとは無関係）。
    // passphrase / jwks は省略でデプロイ時に自動生成され、Secrets Manager に保存される。
    //
    // tenants: {
    //   "your-org": {
    //     // passphrase: "your-pass",      // 省略で自動生成（SM に平文も保存）
    //     // passphrase_length: 32,        // 自動生成時の文字数（デフォルト: 32）
    //     default_space: "your-space.backlog.jp",  // CLI setup の --space デフォルト
    //   },
    // },
  },

  // ============================================================
  // CloudFront 設定（オプション）
  // ============================================================
  // 省略時に有効。無効化する場合のみ { enabled: false } を指定。
  // カスタムドメインは ACM 証明書（us-east-1）と Route53 が必要なため、
  // 利用する場合だけ customDomain を指定する。
  //
  // cloudFront: {
  //   customDomain: {
  //     domainName: "backlog-relay.example.com",
  //     certificateArn: "arn:aws:acm:us-east-1:...:certificate/...",  // us-east-1 のもの
  //     hostedZoneId: "Z0123456789ABCDEFGHIJ",  // 指定時のみ Route53 にレコード作成
  //   },
  // },

  // ============================================================
  // MCP Server 統合（オプション）
  // ============================================================
  // mcp.spaces を指定すると、この Lambda が /mcp エンドポイントも提供する。
  // MCP token key は Secrets Manager で自動生成・ローテーションされる。
  //
  // mcp: {
  //   spaces: [
  //     { pattern: "your-space\\.backlog\\.jp", writable: true },
  //     { pattern: ".*\\.backlog\\.(jp|com)", writable: false },
  //   ],
  //   // script: { max_cli_calls: 20, timeout_ms: 30000 },
  //   // default_spaces: ["your-space.backlog.jp"],
  //   // audit: { collect_user_info: true },
  //   // logging: { debug: false, input: false, output: false },
  // },
};
