# Backlog CLI OAuth Relay Server - AWS CDK Deployment

AWS CDK を使用して Backlog CLI OAuth リレーサーバーを AWS Lambda にデプロイします。

## 前提条件

- [AWS CLI](https://aws.amazon.com/cli/) - 認証情報が設定済み
- [AWS CDK](https://docs.aws.amazon.com/cdk/v2/guide/getting-started.html) - `npm install -g aws-cdk`
- Node.js 18+
- Go 1.24+

## クイックスタート

### 1. Backlog で OAuth アプリケーションを登録

1. Backlog スペース設定画面を開く
   - backlog.jp: `https://YOUR_SPACE.backlog.jp/EditOAuth2Settings.action`
   - backlog.com: `https://YOUR_SPACE.backlog.com/EditOAuth2Settings.action`

2. OAuth 2.0 アプリケーションを作成
   - **リダイレクト URI**: デプロイ後に設定（後述）

3. `Client ID` と `Client Secret` をメモ

### 2. 設定ファイルを作成

```bash
cd deploy/aws-cdk
pnpm install

# 設定テンプレートをコピー
cp config.example.ts config.ts

# 設定を編集
vim config.ts
```

### 3. デプロイ

```bash
# 初回のみ: CDK Bootstrap
cdk bootstrap

# デプロイ（リソース作成）
pnpm deploy:resources
```

### 4. Backlog の設定を更新

デプロイ出力の `CallbackUrl` を Backlog OAuth アプリのリダイレクト URI に設定：

```
https://xxx.lambda-url.ap-northeast-1.on.aws/auth/callback
```

### 5. Backlog CLI を設定

```bash
backlog config set relay_server https://xxx.lambda-url.ap-northeast-1.on.aws
```

## 設定方法

### Parameter Store 参照（設定の一元管理）

Parameter Store の内容は CDK デプロイ時に反映されます。

```typescript
export const config: RelayConfig = {
  parameterName: "/backlog-relay/config",
  parameterValue: {
    server: {
      backlog: {
        jp: {
          client_id: "your-client-id",
          client_secret: "your-client-secret",
        },
      },
      tenants: {
        spaceid: {
          allowed_domain: "spaceid.backlog.jp",
          jwks: '{"keys":[{"kty":"OKP","crv":"Ed25519","kid":"2025-01","x":"...","d":"..."}]}',
          active_keys: "2025-01",
          info_ttl: 600,
          passphrase_hash: "$2a$12$...",
        },
      },
    },
  },
};
```

## コマンド

```bash
# 依存関係のインストール
pnpm install

# デプロイ（リソース作成）
pnpm deploy:resources

# 変更のプレビュー
pnpm diff

# CloudFormation テンプレートの生成
pnpm synth

# CloudFront キャッシュの無効化
pnpm invalidate-cache

# スタックの削除
pnpm destroy
```

## 設定リファレンス

### 必須設定

| フィールド                        | 説明                                        |
| --------------------------------- | ------------------------------------------- |
| `server.backlog.jp` または `server.backlog.com` | 少なくとも1つの OAuth アプリ設定            |

### オプション設定

| フィールド                             | デフォルト       | 説明                             |
| -------------------------------------- | ---------------- | -------------------------------- |
| `server.access_control.allowed_spaces` | `[]`（全て許可） | 許可するスペース名のリスト       |
| `server.access_control.allowed_projects` | `[]`（全て許可） | 許可するプロジェクトキーのリスト |
| `server.audit.enabled`                | `true`           | 監査ログの有効化                 |
| `server.tenants`                      | なし             | テナント設定（バンドル署名に必須） |

## CloudFront 設定（オプション）

CloudFront を使用すると、静的アセット（CSS/JS）がエッジでキャッシュされ高速に配信されます。

### 構成パターン

| パターン | 設定 | URL |
|---------|------|-----|
| CloudFront のみ | `enabled: true` | `https://d123xxx.cloudfront.net` |
| カスタムドメイン | `enabled: true` + `domainName` + `certificateArn` | `https://relay.example.com` |

### パターン1: CloudFront のみ（推奨）

ACM 証明書やドメイン設定が不要で、すぐに使えます。

```typescript
export const config: RelayConfig = {
  parameterName: "/backlog-relay/config",
  parameterValue: { /* ... */ },

  cloudFront: {
    enabled: true,
  },
};
```

デプロイ後の出力：

```
Outputs:
DistributionId = E1234567890ABC
DistributionUrl = https://d1234567890abc.cloudfront.net
DistributionCallbackUrl = https://d1234567890abc.cloudfront.net/auth/callback
```

`DistributionCallbackUrl` を Backlog OAuth アプリのリダイレクト URI に設定してください。

### パターン2: カスタムドメイン

#### 事前準備

1. **ACM 証明書の発行（us-east-1 リージョン）**

   ```bash
   aws acm request-certificate \
     --domain-name relay.example.com \
     --validation-method DNS \
     --region us-east-1
   ```

2. **Route 53 ホストゾーン ID の確認（オプション）**

   ```bash
   aws route53 list-hosted-zones --query "HostedZones[?Name=='example.com.'].Id"
   ```

#### 設定

```typescript
export const config: RelayConfig = {
  parameterName: "/backlog-relay/config",
  parameterValue: { /* ... */ },

  cloudFront: {
    enabled: true,
    domainName: "relay.example.com",
    certificateArn: "arn:aws:acm:us-east-1:123456789012:certificate/xxx",
    hostedZoneId: "Z1234567890ABC",  // オプション: Route 53 使用時
  },
};
```

#### DNS 設定（Route 53 を使わない場合）

`hostedZoneId` を指定しない場合は、手動で CNAME レコードを作成：

```
relay.example.com  CNAME  d1234567890abc.cloudfront.net
```

### キャッシュ設定

| パス | キャッシュ TTL | 説明 |
|------|---------------|------|
| `/assets/*` | 1年 | 静的アセット（ファイル名にハッシュ含む） |
| `/v1/relay/tenants/*/certs` | 1時間〜24時間 | 公開鍵（長めにキャッシュ可能） |
| `/v1/relay/tenants/*/info` | 5分〜1時間 | 署名付き情報 |
| その他 | 0秒 | 動的コンテンツ（OAuth フロー等） |

キャッシュ TTL は `cloudFront.cache` で設定可能です：

```typescript
cloudFront: {
  enabled: true,
  cache: {
    assetsMaxAge: 365 * 24 * 60 * 60,  // 静的アセット (デフォルト: 1年)
    apiDefaultTtl: 60 * 60,            // certs/info デフォルト (デフォルト: 1時間)
    apiMaxTtl: 24 * 60 * 60,           // certs/info 最大 (デフォルト: 24時間)
    apiMinTtl: 5 * 60,                 // certs/info 最小 (デフォルト: 5分)
  },
},
```

### キャッシュの強制無効化

鍵ローテーションや設定変更時にキャッシュを即座に無効化するには、以下のコマンドを使用します：

```bash
# 全キャッシュを無効化
pnpm invalidate-cache

# スタック名を指定する場合
pnpm invalidate-cache MyCustomStackName
```

スクリプトは CloudFormation スタックから Distribution ID を自動取得し、キャッシュを無効化します。

## アーキテクチャ

### 基本構成（Function URL のみ）

```
┌─────────────┐     ┌─────────────────┐     ┌─────────────┐
│  Backlog    │────▶│  Lambda         │────▶│   Backlog   │
│    CLI      │     │  (Function URL) │     │     API     │
└─────────────┘     └─────────────────┘     └─────────────┘
                            │
                            ▼
                    ┌─────────────────┐
                    │ SSM Parameter   │
                    │     Store       │
                    └─────────────────┘
```

### カスタムドメイン構成（CloudFront）

```
┌─────────────┐     ┌─────────────────┐     ┌─────────────────┐     ┌───────────┐
│  Backlog    │────▶│   CloudFront    │────▶│     Lambda      │────▶│  Backlog  │
│    CLI      │     │ (カスタムドメイン) │     │  (Function URL) │     │    API    │
└─────────────┘     └─────────────────┘     └─────────────────┘     └───────────┘
                            │
                            │ /assets/* はエッジキャッシュ
                            ▼
                    ┌─────────────────┐
                    │   Route 53      │
                    │  (オプション)    │
                    └─────────────────┘
```

## コスト

- **AWS Lambda**: リクエストごとの課金（月100万リクエストまで無料）
- **CloudWatch Logs**: 取り込みデータ量による課金
- **SSM Parameter Store**: Standard パラメーターは無料
- **CloudFront**（カスタムドメイン使用時）: 月1000万リクエストまで無料、以降 $0.90/100万リクエスト

一般的な CLI 使用では AWS 無料利用枠内に収まります。

## トラブルシューティング

### "Domain not supported" エラー

認証しようとしているドメイン（jp または com）の設定がありません。

### 認証コールバックが失敗する

Backlog のリダイレクト URI が正確に一致していることを確認：

```
https://xxx.lambda-url.REGION.on.aws/auth/callback
```

## セキュリティ

- `config.ts` は `.gitignore` に含まれています
- シークレットを含むため、リポジトリにコミットしないでください
- Parameter Store を使用する場合、SecureString ではなく String を使用していますが、
  OAuth Client Secret は API Key ではないため、単体では Backlog API にアクセスできません
