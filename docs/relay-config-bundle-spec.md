# Relay Config Bundle 仕様

## 目的

組織が配布する設定バンドルを信頼の起点とし、CLIが誤った中継サーバーへ接続しないことを保証する。
CLIは `/certs` で取得した公開鍵を `relay_keys` の thumbprint でピン留めして署名検証を行い、正当な中継サーバーのみを利用する。

## 信頼モデル

- 信頼の起点は「管理者が配布するバンドル」。
- 署名は改ざん検出のために利用する。
- CLIはバンドルの署名検証に成功した場合のみ設定を取り込む。
- 中継サーバーはバンドル発行用の秘密鍵を保持し、CLIに返す情報に署名する。

## バンドル形式

### ファイル名

`<spaceid>.<backlogdomain>.backlog-cli.zip`

例: `myspace.backlog.jp.backlog-cli.zip`

### ZIP 内の構成

必須:
- `manifest.yaml`
- `manifest.yaml.sig`

任意:
- 追加のメタデータファイル

### 署名ルール

- 署名必須なのは `manifest.yaml` のみ。
- `manifest.yaml.sig` は `manifest.yaml` の署名結果。
- 他ファイルは `manifest.yaml` 内の `files` に `name` と `sha256` を列挙する。

## manifest.yaml 仕様

```yaml
version: 1
relay_url: https://relay.example.com
allowed_domain: spaceid.backlogdomain
issued_at: 2025-01-10T12:00:00Z
expires_at: 2025-02-10T12:00:00Z
relay_keys:
  - key_id: 2025-01
    thumbprint: "..."
  - key_id: 2025-02
    thumbprint: "..."
files:
  - name: extra-metadata.json
    sha256: "..."
```

### フィールド

- `version` (int, 必須): 仕様バージョン。現時点は `1` 固定。
- `relay_url` (string, 必須): CLIが利用すべき中継サーバーURL。
- `allowed_domain` (string, 必須): `spaceid.backlogdomain` 形式。
- `issued_at` (RFC3339, 必須): バンドル発行時刻。
- `expires_at` (RFC3339, 必須): バンドルの有効期限。
- `relay_keys` (list, 必須): 許可された公開鍵の一覧。
  - `key_id` (string, 必須): サーバー側の署名鍵識別子 (JWKS の `kid`)。
  - `thumbprint` (string, 必須): RFC7638 の JWK Thumbprint (Base64URL)。
- `files` (list, 必須): 追加ファイルのハッシュ一覧。

## 署名仕様

- 署名アルゴリズム: Ed25519 固定。
- `manifest.yaml.sig` の内容: JWS JSON Serialization (General) をそのまま格納する。
- 公開鍵は `certs` エンドポイントから取得する。

## CLI 取り込み仕様

### コマンド

```
backlog config import <bundle.zip>
```

### 検証フロー

1. ZIP を解凍し、`manifest.yaml`, `manifest.yaml.sig` の存在確認。
2. `relay_url` から `certs` エンドポイントを組み立て、JWKS を取得。
3. `relay_keys` の各 `key_id` に対して JWK を取得し、RFC7638 の Thumbprint と一致することを確認。
   - `relay_keys` の `key_id` が `certs` に存在しない場合は再セットアップを要求する。
4. `manifest.yaml.sig` を JWS として検証し、`relay_keys` に一致する `kid` の署名が1つでも成功すれば有効とする。
5. `manifest.yaml` の `files` で示された全ファイルの SHA-256 を検証。
6. `expires_at` が過去なら拒否。`issued_at` が極端に未来の場合も拒否。
7. ZIP のファイル名と `allowed_domain` が一致しない場合はエラー (許容する場合は `--allow-name-mismatch`)。

### 取り込み結果

`~/.config/backlog/config.yaml` に以下を保存する。

```yaml
client:
  trust:
    bundles:
      - id: "spaceid.backlogdomain"
        relay_url: "https://relay.example.com"
        allowed_domain: "spaceid.backlogdomain"
        relay_keys:
          - key_id: "2025-01"
            thumbprint: "..."
          - key_id: "2025-02"
            thumbprint: "..."
        issued_at: "2025-01-10T12:00:00Z"
        expires_at: "2025-02-10T12:00:00Z"
        source:
          file_name: "spaceid.backlogdomain.backlog-cli.zip"
          sha256: "..."
        imported_at: "2025-01-10T12:34:56Z"
```

同時に以下を自動更新する (既存設定がある場合は `--no-defaults` で抑止):

```yaml
client:
  default:
    relay_server: "https://relay.example.com"
    space: "spaceid"
    domain: "backlogdomain"
```

## 中継サーバーの署名付き情報返却

### 目的

クライアントが正当な中継サーバーであることを検証し、改ざんされていない情報を受け取る。

### エンドポイント

```
GET /v1/relay/tenants/spaceid.backlogdomain/info
```

### レスポンス形式

RFC7515 7.2.1 (JWS JSON Serialization - General) を使用する。

```json
{
  "payload": "<base64url-json>",
  "signatures": [
    {
      "protected": "<base64url-json>",
      "signature": "<base64url-sig>"
    }
  ],
  "payload_decoded": {
    "version": 1,
    "relay_url": "https://relay.example.com",
    "allowed_domain": "spaceid.backlogdomain",
    "space": "spaceid",
    "domain": "backlogdomain",
    "issued_at": "2025-01-10T12:00:00Z",
    "expires_at": "2025-01-10T12:10:00Z",
    "update_before": "2025-01-15T00:00:00Z"
  }
}
```

- `payload` は JSON を UTF-8 でエンコードしたバイト列を Base64URL した値。
- `payload_decoded` は表示用途であり、検証対象は `payload` のみ。
- `signatures[].protected` には `alg` と `kid` を含める。
- `alg` は `EdDSA` を使用する。

### 署名検証

- `signatures[].signature` は JWS の署名対象 (`protected + "." + payload`) に対する Ed25519 署名。
- CLI は `certs` の公開鍵で検証し、`relay_keys` に一致する `kid` の署名が1つでも成功すれば有効とする。
- `payload.relay_url` と `payload.allowed_domain` がバンドルの値と一致しない場合は拒否。
- `expires_at` を過ぎた場合は拒否。
- `payload.update_before` が指定されており、`manifest.yaml.issued_at` がそれより古い場合は更新フローを開始する。
  - `update_before` は RFC3339 の日時。未指定時は更新トリガー無しとする。

## 公開鍵配布エンドポイント

### エンドポイント

```
GET /v1/relay/tenants/spaceid.backlogdomain/certs
```

### レスポンス形式

```json
{
  "keys": [
    {
      "kty": "OKP",
      "crv": "Ed25519",
      "kid": "2025-01",
      "x": "<base64url>"
    }
  ]
}
```

- JWKS 形式で返す。
- `kid` は `manifest.yaml` の `relay_keys[].key_id` と `info.signatures[].protected` 内の `kid` に一致する。
- RFC7638 の Thumbprint を用いて `manifest.yaml` の `relay_keys[].thumbprint` と一致確認する。
- 複数鍵がある場合は `kid` で選択する。

## サーバー側の鍵管理

- 署名鍵は中継サーバーの環境変数で注入する。
- 実運用では Secrets Manager / SSM Parameter Store を利用し、環境変数への直接投入は避ける。
- `relay_keys` によりローテーションを可能にする。

## サーバー側の設定と実装要件

### 必須設定

- `TENANT_<DOMAIN>_ACTIVE_KEYS`: 署名鍵識別子。区切り文字で複数指定可能 (例: `2025-01,2025-02`)。
  - 先頭をアクティブ鍵とみなす。
- `TENANT_<DOMAIN>_JWKS`: 秘密鍵を含む JWK セット (JSON 文字列)。
- `TENANT_<DOMAIN>_ALLOWED_DOMAIN`: `spaceid.backlogdomain` の一致確認用。
- `TENANT_<DOMAIN>_INFO_TTL`: `info` の `expires_at` までの秒数。省略時は `600` (10分)。

### アクセス制御

- テナントの許可/不許可は既存の `allowedSpaces` 等のアクセス制御で行う。
- 本仕様のテナント設定はプロビジョニング用であり、アクセス制御の代替ではない。

### URL 構成

- `relay_url` はリクエストの `Host` と `X-Forwarded-Proto` から組み立てる。

### 挙動

- `/v1/relay/tenants/{domain}/certs` は `TENANT_<DOMAIN>_JWKS` から `d` を削除した公開 JWKS を返す。
- `/v1/relay/tenants/{domain}/info` は `relay_url` / `allowed_domain` / `issued_at` / `expires_at` を署名付きで返す。
- `info` の署名は `TENANT_<DOMAIN>_ACTIVE_KEYS` の各鍵で生成する。
- `allowed_domain` が設定と一致しない場合は 404 または 403 を返す。

## エラー方針

- 署名検証に失敗した場合は必ず停止し、認証フローに進めない。
- バンドルの期限切れはエラー。
- `allowed_domain` の不一致はエラー。

## 鍵ローテーション時の信頼確認

サーバーが新しい鍵を配布した場合、CLI または Web UI はユーザーに信頼確認を求める。

### CLI の挙動

- `certs` から取得した `kid` が `relay_keys` に存在しない場合、表示して確認を求める。
- `thumbprint` が未登録の場合は警告し、明示承認がない限り拒否する。
- 承認した場合のみ `relay_keys` に追加して保存する。

### Web UI の挙動

- 署名検証で未知の `kid` を検出した場合に警告を表示し、ユーザーに承認を求める。
- 承認なしに自動更新は行わない。

## バンドルの更新と有効期限

バンドルは `expires_at` を持つため、更新方法を明確にする。

### 基本方針

- 初回は手動 import。
- 2回目以降は自動更新をデフォルトとする。
- 期限切れ後は認証フローを停止し、更新を促す。

### 方式A: 手動更新

- 期限が近づいたら CLI が警告を表示する。
- ユーザーは新しいバンドルを入手し、`backlog config import` で更新する。
- 同じ `allowed_domain` のバンドルは上書き可能とする。

### 方式B: 自動更新

- `relay_url` から `bundle` エンドポイントを取得し、更新を適用する。
- 署名検証と `relay_keys` の照合に成功した場合のみ自動更新する。

## バンドル取得エンドポイント

### エンドポイント

```
GET /v1/relay/tenants/spaceid.backlogdomain/bundle
```

### レスポンス

バンドル ZIP を返す。内容は `manifest.yaml` と `manifest.yaml.sig` を含む。
`manifest.yaml.sig` は JWS JSON Serialization (General) をそのまま格納する。
`payload` は `manifest.yaml` の UTF-8 バイト列を Base64URL した値とする。

署名ファイルの例:

```json
{
  "payload": "<base64url-manifest>",
  "signatures": [
    {
      "protected": "<base64url-json>",
      "signature": "<base64url-sig>"
    },
    {
      "protected": "<base64url-json>",
      "signature": "<base64url-sig>"
    }
  ]
}
```

### info からの更新トリガー

- `/info` の `payload.update_before` が未指定の場合は更新トリガー無しとする。
- `payload.update_before` より `manifest.yaml.issued_at` が古い場合、CLI は更新フローを開始する。
- 取得したバンドルの `manifest.yaml.issued_at` が現在の設定より新しい場合のみ更新する。

## 自動更新時の配布元確認

自動更新時は「同じ配布元」であることを確認してから更新を適用する。

### 確認条件

- 新しい `manifest.yaml` は **既存の `relay_keys` のいずれかで署名検証に成功**すること。
- `relay_url` と `allowed_domain` が既存設定と一致すること。
- `relay_keys` は新しいバンドルの内容で置き換える。
  - `TENANT_<DOMAIN>_ACTIVE_KEYS` の鍵は必ず `relay_keys` に含める。

これにより、異なるサーバーへの置き換えや未知の鍵のみでの更新を防止する。

### サーバー側の保証

- バンドルの署名は `TENANT_<DOMAIN>_ACTIVE_KEYS` の各鍵で行う。
