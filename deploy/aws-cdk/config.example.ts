/**
 * Backlog OAuth Relay Server 設定
 *
 * このファイルを config.ts にコピーして、値を設定してください:
 *   cp config.example.ts config.ts
 *
 * config.ts は .gitignore に含まれているため、
 * シークレットを含んでもリポジトリにコミットされません。
 */
import { RelayConfig } from "./lib/types";

// ============================================================
// 方法1: インライン設定（シンプル）
// ============================================================
export const config: RelayConfig = {
  source: "inline",

  backlog: {
    // backlog.jp 用の OAuth アプリ設定
    jp: {
      clientId: "your-client-id",
      clientSecret: "your-client-secret",
    },

    // backlog.com 用の OAuth アプリ設定（オプション）
    // com: {
    //   clientId: 'your-client-id',
    //   clientSecret: 'your-client-secret',
    // },
  },

  // アクセス制御（オプション）
  // allowedSpaces: ['my-space'],      // 特定のスペースのみ許可
  // allowedProjects: ['PROJ1', 'PROJ2'], // 特定のプロジェクトのみ許可

  // 許可するホストパターン（オプション）
  // Lambda Function URL パターンは CDK が自動設定するため、通常は不要
  // カスタムドメインなど追加のパターンが必要な場合のみ指定
  // 例: allowedHostPatterns: 'relay.example.com',

  // 監査ログ（オプション）
  audit: {
    enabled: true,
  },
};

// ============================================================
// 方法2: Parameter Store 参照（設定の一元管理）
// ============================================================
// 事前に Parameter Store にパラメーターを作成しておく場合:
//
// export const config: RelayConfig = {
//   source: 'parameter-store',
//   parameterName: '/backlog-relay/config',
// };
//
// または、CDK でパラメーターも一緒に作成する場合:
//
// export const config: RelayConfig = {
//   source: 'parameter-store',
//   parameterName: '/backlog-relay/config',
//   createParameter: true,
//   parameterValue: {
//     backlog: {
//       jp: {
//         clientId: 'your-client-id',
//         clientSecret: 'your-client-secret',
//       },
//     },
//   },
// };
