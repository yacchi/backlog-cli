# backlog-relay-aws-deploy

[`@yacchi/backlog-relay-aws-cdk`](../relay-aws) を使った
Backlog OAuth リレー + MCP サーバー + ポータルの**デプロイ雛形**です。
このディレクトリをコピーし、`config.ts` を編集して `cdk deploy` します。

デプロイされる内容（OAuth リレー / MCP サーバー / 配布ポータルとその配布方法）は
[ライブラリの README](../relay-aws/README.md) を参照してください。

## 前提条件

- Node.js 22+
- AWS 認証情報（CDK bootstrap 済みのアカウント/リージョン）

> デプロイに Go / Docker は不要です（ランタイムは公開済みコンテナイメージで、デプロイ時に
> ECR へコピーされます。Go/Docker はイメージを**ビルド**する時のみ必要）。

## 雛形としての使い方

1. このディレクトリを自分のインフラリポジトリにコピー。
2. `package.json` の `"@yacchi/backlog-relay-aws-cdk": "workspace:*"` を公開バージョン
   （例 `^0.19.1`）に置き換え、`.npmrc` を追加:
   ```ini
   @yacchi:registry=https://npm.pkg.github.com
   //npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}
   ```
3. `npm install`（または `pnpm install`）。

## デプロイ手順

```bash
# 1. Backlog で OAuth アプリを登録し Client ID / Secret を控える
#    （リダイレクト URI は初回デプロイ後に設定）

# 2. 設定ファイルを作成して編集
cp config.example.ts config.ts

# 3. 初回のみ: bootstrap
pnpm cdk bootstrap

# 4. デプロイ
pnpm deploy

# 5. 出力されたコールバック URL を Backlog OAuth のリダイレクト URI に設定
#    例: https://<your-domain>/auth/callback
```

## コマンド

| コマンド | 説明 |
|---------|------|
| `pnpm deploy` | スタックをデプロイ |
| `pnpm diff` | 変更をプレビュー |
| `pnpm synth` | CloudFormation テンプレートを生成 |
| `pnpm destroy` | スタックを削除 |
| `pnpm update-passphrase` | Secrets Manager のテナントパスフレーズをローテーション |
| `pnpm invalidate-cache` | CloudFront キャッシュを無効化 |

## 設定

`config.ts`（秘密情報を含むため gitignore）を編集します。構造は `config.example.ts`、
設定型やイメージタグ解決（`image.tag` / `image.prerelease`）は
[ライブラリの README](../relay-aws/README.md) を参照してください。

設計の詳細はリポジトリルートの `docs/design/` を参照。
