# @yacchi/backlog-relay-aws-cdk

Backlog の **OAuth リレー + MCP サーバー + 配布ポータル**を、CloudFront 配下の
Lambda コンテナとして AWS にデプロイする CDK construct ライブラリです。

ランタイムは公開済みのコンテナイメージ（`ghcr.io/yacchi/backlog-relay`）として
配布され、本 construct が AWS 側リソース（Lambda `DockerImageFunction`、Function
URL、CloudFront、SSM Parameter Store、ローテーション付き Secrets Manager）を構築し、
`cdk-ecr-deployment` でイメージをアカウントの ECR へコピーします。

## デプロイされるもの

1 つのコンテナイメージに以下が同梱され、1 プロセスで提供されます。

### OAuth リレー

CLI に Client Secret を持たせずに Backlog の OAuth トークンを取得・更新するための
中継サーバーです。`/auth/start`・`/auth/callback`・`/auth/token` を提供し、Client
ID/Secret は中継サーバー側（Secrets Manager）で管理します。
詳細: [`docs/design/oauth-relay-server.md`](../../docs/design/oauth-relay-server.md)

### MCP サーバー

Claude などの MCP クライアントが Backlog にアクセスするための
**Streamable HTTP エンドポイント（`/mcp`）**と OAuth 認可サーバー（DCR + PKCE、
`/.well-known/oauth-authorization-server`・`/mcp/register`・`/mcp/authorize`・
`/mcp/token`）を提供します。クライアントには `backlog` CLI 実行ツールと、Python
サンドボックス（`run_script`）が公開されます。アクセス可能なスペースは
`mcp.spaces`（正規表現 + writable）で制御します。
詳細: [`docs/design/remote-mcp-server.md`](../../docs/design/remote-mcp-server.md)

### 配布ポータル

組織メンバー向けの Web UI（`/portal/:name`）です。テナントのパスフレーズで認証し
（`POST /api/v1/portal/verify`）、後述の**設定バンドル**をダウンロードできます
（`POST /api/v1/portal/:name/bundle`）。ポータルの静的アセットはイメージに同梱されます。

## ポータルを使った配布（Relay Config Bundle）

CLI が**不正な中継サーバーへ接続しない**ことを保証するため、組織は「信頼の起点」と
なる**署名付き設定バンドル（ZIP）**をポータル経由で配布します。

1. 組織メンバーがポータルを開き、テナントのパスフレーズを入力。
2. 署名付きバンドル（`manifest.yaml` + `manifest.yaml.sig`、relay の公開鍵情報を含む）を
   ダウンロード（またはプロビジョニングキーを取得）。
3. `backlog` CLI に取り込むと、relay の公開鍵が**信頼の起点として固定**され、以降は
   その relay のみを信頼します。

詳細: [`docs/design/relay-config-bundle.md`](../../docs/design/relay-config-bundle.md)

## インストール

本パッケージは **GitHub Packages** で配布され、public でもインストール時に GitHub
トークン認証が必要です。プロジェクトに `.npmrc` を追加してください。

```ini
@yacchi:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}
```

CDK の peer dependencies と合わせてインストールします。

```bash
npm install @yacchi/backlog-relay-aws-cdk aws-cdk-lib constructs
```

> デプロイには Go / Docker は不要です（イメージは公開済みで、デプロイ時に ECR へ
> コピーされるだけ）。CDK bootstrap 済みのアカウント/リージョンが必要です。

## 使い方

```ts
import * as cdk from "aws-cdk-lib";
import { RelayStack, resolveLatestImageTag, DEFAULT_IMAGE_SOURCE } from "@yacchi/backlog-relay-aws-cdk";

const app = new cdk.App();

// イメージタグはレジストリから解決（既定は最新の安定版）。
const imageSource = DEFAULT_IMAGE_SOURCE;
const imageTag = await resolveLatestImageTag(imageSource);

new RelayStack(app, "BacklogRelay", {
  config: {
    parameterName: "/backlog-relay/config",
    parameterValue: {
      server: { port: 8080 },
      backlog_app: {
        client_id: process.env.BACKLOG_CLIENT_ID!,
        client_secret: process.env.BACKLOG_CLIENT_SECRET!,
      },
      tenants: { "myspace.backlog.jp": { passphrase: "...", default_space: "myspace.backlog.jp" } },
    },
    mcp: { spaces: [{ pattern: "myspace\\.backlog\\.jp", writable: true }] },
    cloudFront: { enabled: true, customDomain: { /* domainName, certificateArn, hostedZoneId */ } },
    image: { source: imageSource, tag: imageTag },
  },
  env: { account: process.env.CDK_DEFAULT_ACCOUNT, region: "ap-northeast-1" },
});
```

### イメージタグの解決

`resolveLatestImageTag(source, { prerelease })` はレジストリのタグ一覧から最新の
semver タグを返します。

- `prerelease: false`（既定）→ 最新の**安定版**（開発版の誤デプロイを防止）。
- `prerelease: true` → 最新の**プレリリース**（開発版を対象にする場合）。

再現性のため `image.tag` で固定バージョンを指定してください。`latest` は使いません。

## デプロイ雛形

すぐコピーして使えるデプロイアプリが本リポジトリの
**`packages/relay-aws-deploy`** にあります。そのディレクトリをコピーし、`workspace:*`
を公開バージョンに置き換え、上記 `.npmrc` を設定し、`config.ts` を編集して
（`cp config.example.ts config.ts`）`cdk deploy` してください。

## 公開 API

- `RelayStack`, `RelayStackProps` — デプロイ可能なスタック/construct。
- `resolveLatestImageTag`, `fetchImageTags`, `parseImageRef` — レジストリのタグ解決。
- `DEFAULT_IMAGE_SOURCE`, `DEFAULT_CACHE_CONFIG` — 既定値。
- 設定型: `RelayConfig`, `ContainerImageConfig`, `McpConfig`, `CloudFrontConfig`,
  `ParameterStoreValue`, `RelayTenantInput` など。
