# Relay Config Bundle 仕様（信頼バンドル）

## 目的

組織が配布する設定バンドルを信頼の起点とし、CLIが誤った中継サーバーへ接続しないことを保証する。
CLIは `/certs` で取得した公開鍵を `relay_keys` の thumbprint でピン留めして署名検証を行い、正当な中継サーバーのみを利用する。

## 信頼モデル

- 信頼の起点は「管理者が配布するバンドル」。
- **バンドルが束縛するのは「中継サーバー（relay_url）と、その署名鍵（relay_keys）」のみ**。バンドルはスペース（Backlog のスペース/ドメイン）を束縛しない。
- 署名は改ざん検出のために利用する。
- CLIはバンドルの署名検証に成功した場合のみ設定を取り込む。
- 中継サーバーはバンドル発行用の秘密鍵を保持し、CLIに返す情報に署名する。

### 役割分担（重要）

| 関心事 | 担当 | 備考 |
|--------|------|------|
| 「この中継サーバーを信頼するか」 | **バンドル**（CLI 側） | relay_url + 署名鍵のピン留め |
| 「どのスペースにログインするか」 | **プロファイル / ログイン** | `space` / `domain` はクライアント側が保持 |
| 「誰がどのスペース・プロジェクトを使えるか（認可）」 | **中継サーバー側 `access_control`** + Backlog | クライアントのバンドルは認可ゲートではない |

中継サーバーは**スペース非依存**である。`/auth/start` はクエリパラメータ `space` / `domain` を受け取り、Backlog 認可URL（`https://{space}.{domain}/OAuth2AccessRequest.action`）を組み立てる。中継サーバーがバンドルやテナント定義からスペースを導出することはない。

### テナント＝バンドル配布単位

中継サーバーの「テナント」は**バンドルの配布単位**であり、スペースとは無関係である。
テナントは一意な `name`（任意の識別子）で識別し、配布を保護するための `passphrase` を持つ。
`name` はバンドルの識別子としてそのまま用いられ、プロファイルはこの `name` でバンドルを参照する。

## バンドル形式

### ファイル名

`<name>.backlog-cli.zip`

例: `acme-relay.backlog-cli.zip`

`name` はテナント（配布単位）の識別子。ドメイン形式である必要はない。

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
version: 2
name: acme-relay
relay_url: https://relay.example.com
issued_at: 2026-06-10T12:00:00Z
expires_at: 2026-07-10T12:00:00Z
bundle_token: eyJhbGciOiJFZERTQSIsInR5cCI6IkpXVCIsImtpZCI6IjIwMjUtMDEifQ...
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

- `version` (int, 必須): 仕様バージョン。本仕様は `2`（`1` は `allowed_domain` を持つ旧形式。「マイグレーション」を参照）。
- `name` (string, 必須): テナント（配布単位）の識別子。バンドルの一意な識別子であり、プロファイルが参照するキー。
- `relay_url` (string, 必須): CLIが利用すべき中継サーバーURL。
- `issued_at` (RFC3339, 必須): バンドル発行時刻。
- `expires_at` (RFC3339, 必須): バンドルの有効期限。
- `bundle_token` (string, 必須): 自動更新用のアクセストークン (JWT)。
- `relay_keys` (list, 必須): 許可された公開鍵の一覧。
  - `key_id` (string, 必須): サーバー側の署名鍵識別子 (JWKS の `kid`)。
  - `thumbprint` (string, 必須): RFC7638 の JWK Thumbprint (Base64URL)。
- `files` (list, 必須): 追加ファイルのハッシュ一覧。

`allowed_domain` は廃止した（バンドルはスペースを束縛しない）。

## bundle_token 仕様

### 目的

`bundle_token` はバンドル自動更新時の認証に使用する JWT。
ポータル経由でダウンロードしたバンドルには bundle_token が含まれ、CLI は以降の更新リクエストでこのトークンを使用する。

### JWT 形式

ヘッダー:
```json
{
  "alg": "EdDSA",
  "typ": "JWT",
  "kid": "2025-01"
}
```

ペイロード:
```json
{
  "sub": "acme-relay",
  "iat": 1736503200,
  "nbf": 1736503200,
  "jti": "ランダムな一意識別子"
}
```

- `sub`: バンドルの `name` と一致する必要がある。
- `iat`: 発行時刻 (Unix タイムスタンプ)。
- `nbf`: 有効開始時刻 (Unix タイムスタンプ)。
- `jti`: トークンの一意識別子。

### 署名

- アルゴリズム: Ed25519 (EdDSA)
- 署名鍵: `server.tenants[*].active_keys` の最初の鍵で署名
- 検証鍵: `/certs` で公開される JWKS の対応する公開鍵

### トークンの有効期限

`bundle_token` 自体に有効期限 (`exp`) は設定しない。
鍵ローテーション時に古いトークンは検証に失敗し、自動的に無効化される。

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
7. ZIP のファイル名と `name` が一致しない場合はエラー (許容する場合は `--allow-name-mismatch`)。

### 取り込み結果

`~/.config/backlog/config.yaml` に以下を保存する。

```yaml
client:
  trust:
    bundles:
      - name: "acme-relay"
        relay_url: "https://relay.example.com"
        bundle_token: "eyJhbGciOiJFZERTQSIsInR5cCI6IkpXVCIsImtpZCI6IjIwMjUtMDEifQ..."
        relay_keys:
          - key_id: "2025-01"
            thumbprint: "..."
          - key_id: "2025-02"
            thumbprint: "..."
        issued_at: "2026-06-10T12:00:00Z"
        expires_at: "2026-07-10T12:00:00Z"
        source:
          file_name: "acme-relay.backlog-cli.zip"
          sha256: "..."
        imported_at: "2026-06-10T12:34:56Z"
```

`id` フィールドは廃止し、`name` を一意キーとする。`allowed_domain` は保存しない。

同時に、既定プロファイルがこのバンドルを参照するよう設定する (既存設定がある場合は `--no-defaults` で抑止):

```yaml
profile:
  default:
    bundle: "acme-relay"
```

- バンドルからは `space` / `domain` を設定しない。スペースはログイン時にユーザーが選択し、プロファイル側が保持する。
- これにより、複数スペースのプロファイルが同一の `bundle` を参照でき、relay_url の重複保持（drift）が発生しない。

## プロファイルと relay_url の解決

### プロファイルのフィールド

```yaml
profile:
  default:
    space: "myspace"
    domain: "backlog.jp"
    bundle: "acme-relay"        # どのバンドル(name)を使うか
  another:
    space: "other"
    domain: "backlog.com"
    relay_server: "https://relay.example.com"   # bundle 無し経路（直接指定）
```

- `space` / `domain`: ログイン対象の Backlog スペース。常にプロファイルが保持する。
- `bundle` (string, 任意): 信頼するバンドルの `name`。設定時は relay_url をバンドルから解決する。
- `relay_server` (string, 任意): バンドルを使わず relay_url を直接指定する経路（手動設定・Web設定UI）。

`bundle` と `relay_server` は通常どちらか一方を用いる。両方ある場合の優先順位は下記に従う。

### relay_url の解決順位

利用時（ログイン・トークン更新等）に次の優先順で relay_url を決定する。

1. **明示オーバーライド**: コマンド引数 / 環境変数 `BACKLOG_RELAY_SERVER`
2. **バンドル参照**: `profile.bundle` が指す `client.trust.bundles[name].relay_url`
3. **インライン指定**: `profile.relay_server`
4. いずれも無ければエラー

relay_url をプロファイルに焼き付けず、バンドル参照時は常にバンドルから解決するため、バンドル更新が全プロファイルに即時反映される。

### 新規スペースプロファイルの生成

新しいスペースにログインしてプロファイルを生成する際は、primary（無ければ default）プロファイルから **`bundle` 参照を引き継ぐ**（`relay_server` を引き継いでいた従来挙動を置き換える）。
これにより複数スペースが同一バンドルを共有し、relay 情報の二重管理を避ける。

## 中継サーバーの署名付き情報返却

### 目的

クライアントが正当な中継サーバーであることを検証し、改ざんされていない情報を受け取る。

### エンドポイント

```
GET /v1/relay/tenants/{name}/info
```

`{name}` はテナント（配布単位）の識別子。

### 認証

無認証。返す情報は署名付きの公開構成情報であり、信頼は `relay_keys` でピン留めした鍵による署名検証で担保する（「テナントエンドポイントの認証」を参照）。

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
    "version": 2,
    "name": "acme-relay",
    "relay_url": "https://relay.example.com",
    "issued_at": "2026-06-10T12:00:00Z",
    "expires_at": "2026-06-10T12:10:00Z",
    "update_before": "2026-06-15T00:00:00Z"
  }
}
```

- `payload` は JSON を UTF-8 でエンコードしたバイト列を Base64URL した値。
- `payload_decoded` は表示用途であり、検証対象は `payload` のみ。
- `signatures[].protected` には `alg` と `kid` を含める。
- `alg` は `EdDSA` を使用する。
- `space` / `domain` / `allowed_domain` は payload に含めない（スペース非依存のため）。

### 署名検証

- `signatures[].signature` は JWS の署名対象 (`protected + "." + payload`) に対する Ed25519 署名。
- CLI は `certs` の公開鍵で検証し、`relay_keys` に一致する `kid` の署名が1つでも成功すれば有効とする。
- `payload.relay_url` と `payload.name` がバンドルの値と一致しない場合は拒否。
- `expires_at` を過ぎた場合は拒否。
- `payload.update_before` が指定されており、`manifest.yaml.issued_at` がそれより古い場合は更新フローを開始する。
  - `update_before` は RFC3339 の日時。未指定時は更新トリガー無しとする。

## 公開鍵配布エンドポイント

### エンドポイント

```
GET /v1/relay/tenants/{name}/certs
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

サーバー設定 (`server.tenants`) に以下を設定する。テナントのキーがバンドルの `name` となる。

- `jwks`: 秘密鍵を含む JWK セット (JSON 文字列)。
- `active_keys`: 署名鍵識別子。区切り文字で複数指定可能 (例: `2025-01,2025-02`)。
  - 先頭をアクティブ鍵とみなす。
- `info_ttl`: `info` の `expires_at` までの秒数。省略時は `600` (10分)。
- `passphrase_hash`: ポータル用パスフレーズの bcrypt ハッシュ（設定した場合のみポータル経由の取得を許可）。

`allowed_domain` は廃止した。テナントはスペースを束縛しない。

例:

```yaml
server:
  tenants:
    acme-relay:
      jwks: '{"keys":[{"kty":"OKP","crv":"Ed25519","kid":"2025-01","x":"...","d":"..."}]}'
      active_keys: "2025-01,2025-02"
      info_ttl: 600
      passphrase_hash: "$2a$12$..."
```

### アクセス制御

- どのスペース/プロジェクトを許可するかは、中継サーバーの `access_control`（`allowed_space_patterns` / `allowed_project_patterns`）で行う。
- テナント設定はバンドルの配布単位（プロビジョニング用）であり、アクセス制御の代替ではない。

### URL 構成

- `relay_url` はリクエストの `Host` と `X-Forwarded-Proto` から組み立てる。

### 挙動

- `/v1/relay/tenants/{name}/certs` は `server.tenants[name].jwks` から `d` を削除した公開 JWKS を返す。
- `/v1/relay/tenants/{name}/info` は `relay_url` / `name` / `issued_at` / `expires_at` を署名付きで返す。
- trust に設定がある場合、設定保存前に `/info` を取得して署名検証を行う。
  - 検証に失敗した場合は設定保存を拒否する（Web設定フロー）。
- trust に設定がある場合、認証開始前に `/info` を取得して署名検証を行う。
  - 検証に失敗した場合は認証フローを開始しない。
- `payload.update_before` が指定されており、`manifest.yaml.issued_at` がそれより古い場合は更新フローを開始する。
  - 更新が必要な場合は設定保存/認証を中断し、バンドルの再インポートを促す。
  - 自動更新に対応する場合は `/v1/relay/tenants/{name}/bundle` を取得して再インポートする。
- `info` の署名は `server.tenants[name].active_keys` の各鍵で生成する。
- 指定された `name` のテナントが存在しない場合は 404 を返す。

## エラー方針

- 署名検証に失敗した場合は必ず停止し、認証フローに進めない。
- バンドルの期限切れはエラー。
- `name` の不一致（バンドルと info/token）はエラー。

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
- 同じ `name` のバンドルは上書き可能とする。

### 方式B: 自動更新

- `relay_url` から `bundle` エンドポイントを取得し、更新を適用する。
- 署名検証と `relay_keys` の照合に成功した場合のみ自動更新する。

## テナントエンドポイントの認証

`/v1/relay/tenants/{name}/` 配下の `certs` / `info` / `bundle` エンドポイントは**いずれも無認証**で公開する。

### 設計判断: 無認証で問題ない理由

これらのエンドポイントが返すのは「公開鍵」と「署名済みの中継サーバー構成情報」であり、シークレットを含まない。信頼の担保は認証ではなく**署名のピン留め**で行う。

- `certs`: 公開鍵 JWKS（`d` を除去済み）を返すだけで、本質的に公開情報。
- `info`: `relay_url` / `name` / `issued_at` / `expires_at` を Ed25519 署名付きで返す。CLI は `relay_keys` の `thumbprint` でピン留めした鍵でのみ署名検証するため、無認証で取得できても改ざんや不正な構成の注入はできない。
- `bundle`: バンドル ZIP（`manifest.yaml` + 署名）を返す。中身は署名済みで改ざん不可。`bundle_token` を含むが、このトークンの用途は本エンドポイント群へのアクセスのみであり、エンドポイントが無認証である以上、開示されても追加の権限を与えない。Backlog のアクセストークン取得には別途 OAuth 認証が必須。

`bundle_token` はバンドルに同梱されるが、現状の中継サーバー実装ではこれらエンドポイントの**認証には使用しない**（将来的に列挙防止のための認証を追加する余地は残す）。

### 配布の制御

初回バンドル配布はポータルのパスフレーズ保護エンドポイント（`/api/v1/portal/...`）で制御する。パスフレーズはテナント（配布単位）ごとに設定する。直接エンドポイント（`/v1/relay/tenants/{name}/bundle`）は自動更新用であり、配布制限のゲートではない点に注意する。

### エラー

| ステータス | 原因 |
|-----------|------|
| 404 Not Found | テナント（name）が存在しない |
| 500 Internal Server Error | JWKS 未設定・署名失敗 |

## バンドル取得エンドポイント（自動更新用）

### エンドポイント

```
GET /v1/relay/tenants/{name}/bundle
```

### 認証

無認証。バンドル ZIP は署名済みで改ざん不可であり、シークレットを含まない（「テナントエンドポイントの認証」を参照）。
初回ダウンロードはポータルのパスフレーズ保護エンドポイント経由で行い、本エンドポイントは主に自動更新用とする。

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
- `relay_url` と `name` が既存設定と一致すること。
- `relay_keys` は新しいバンドルの内容で置き換える。
  - `server.tenants[*].active_keys` の鍵は必ず `relay_keys` に含める。

これにより、異なるサーバーへの置き換えや未知の鍵のみでの更新を防止する。

### サーバー側の保証

- バンドルの署名は `server.tenants[*].active_keys` の各鍵で行う。

## マイグレーション（v1 → v2）

旧仕様（`version: 1`、`allowed_domain` ベース）からの移行方針。

### バンドル形式

- v1 manifest の `allowed_domain` を **`name` として読み替える**（`name` は任意識別子のため、旧 `spaceid.backlogdomain` 文字列をそのまま name にしても問題ない）。
- v1 を受理する場合は `allowed_domain` → `name`、`bundle_token.sub`（旧 allowed_domain）→ name として扱う。新規発行は `version: 2` とする。

### 保存済み config.yaml

- `client.trust.bundles[].id` / `allowed_domain` → `name`（旧 allowed_domain 値をそのまま name に移送）。
- `profile.*.relay_server`（既存）→ そのまま「インライン指定」として有効（解決順位3）。破壊的変更はしない。
- 新規 import 以降は `profile.default.bundle = name` を設定し、relay_url はバンドルから解決する。

### サーバー側設定

- `server.tenants` のキーを `name` とみなす（旧キーが `SPACEID_BACKLOG_JP` 形式でも、バンドル `name` と一致していれば動作する）。
- `tenants[*].allowed_domain` は無視（廃止）。スペース許可は `access_control` に集約する。
