# 実装プラン: MCP トークンの暗号化（リフレッシュトークン持ち去り対策）

## 目的

MCP サーバーがクライアントに返す `access_token` / `refresh_token`（JWT）の中に
含まれる **生の Backlog アクセストークン・リフレッシュトークン**を暗号化し、
**MCP クライアントが侵害されても MCP サーバー（の鍵）を同時に侵害しない限り
利用できない**状態にする。

特に長命なリフレッシュトークンのオフサーバー持ち去り（MCP を経由せず Backlog API を
恒久的に叩かれる最悪ケース）を確実に潰すことが主目的。

## 現状の問題

- MCP サーバーが発行するトークンは **EdDSA の JWS（署名付き JWT）**
  （`packages/mcp-server/src/crypto/jwt.ts` の `sign` / `verify`）。
- JWS は**完全性（改ざん検知）のみ**で、ペイロードは base64url の平文 JSON。
  トークンを持つ者は誰でも生の `at` / `rt` を読み出せる。

```jsonc
// access token ペイロード
"space:example.backlog.jp": { "at": "<生の Backlog アクセストークン>", "exp": 1234 }
// refresh token ペイロード
"space:example.backlog.jp": { "rt": "<生の Backlog リフレッシュトークン>" }
```

- JWS 検証鍵は外部公開されていない（`/.well-known` に JWKS なし、`verifyKeys` は
  完全にサーバー内部利用）。よってトークン形式を変えても外部依存は壊れない。

## 脅威モデル（達成できること / できないこと）

クライアント侵害・サーバー無傷の前提で:

| | 効果 |
|---|---|
| リフレッシュトークンの持ち去り・オフサーバー悪用 | ✅ 防げる（最重要） |
| アクセストークンで Backlog API を直接叩く（MCP の監査・制御をバイパス） | ✅ 防げる |
| 暗号化トークンを MCP サーバーへリプレイし有効期間中に悪用 | ❌ 防げない（別レイヤで対応） |

暗号化は「生の Backlog 資格情報をサーバー鍵なしには使えなくする」もの。
リフレッシュトークンは送信先が MCP サーバーだけなので「サーバーを侵害しない限り
使えない」が完全に成立する。アクセストークンに残るのは TTL の間のリプレイ窓のみ
（短い TTL・失効・リプレイ検知で別途縮小する）。

## 設計

### 方式: 機密フィールドだけを対称鍵 AEAD で暗号化（encrypt-then-sign）

JWS のエンベロープはそのまま残し、`at` / `rt` の**値だけ**を暗号文に置き換える。

```jsonc
// before
"space:example.backlog.jp": { "at": "abc123...", "exp": 1234 }
// after
"space:example.backlog.jp": { "at": "<JWE compact>", "exp": 1234 }
```

外側の JWS が全体の完全性を担保したまま、中の秘密値だけを暗号化する。

**全体 JWE ではなくフィールド暗号化を選ぶ理由:**
- 変更が外科的。書き込みは `setSpaceAccess` / `setSpaceRefresh` の 2 箇所、
  読み出しは `jwt-auth` の抽出部と `oauth/handlers.ts` の refresh 経路のみ。
  `verify` / `kid` / `exp` まわりのフローは無傷。
- `exp` や space ドメインが平文で残るので、復号せずにルーティング・有効期限判定・
  ログ/観測ができる。
- 再発行経路ではアクセストークンの暗号文はそのままコピーでき、refresh のときだけ
  復号して Backlog を叩けばよい。

### 暗号プリミティブ

`jose` の JWE compact、`alg: "dir"` + `enc: "A256GCM"`（256bit 対称鍵による認証付き暗号）。
protected header に `kid` と、多層防御として `sp`(space) / `use`(`at`|`rt`) を入れ、
AEAD の AAD としてフィールドの取り違え・スペース間付け替えを防ぐ。

```ts
// crypto/secret.ts（新規・スケッチ）
import { CompactEncrypt, compactDecrypt } from "jose";

export async function seal(plain: string, key: Uint8Array, kid: string, sp: string, use: "at" | "rt") {
    return new CompactEncrypt(new TextEncoder().encode(plain))
        .setProtectedHeader({ alg: "dir", enc: "A256GCM", kid, sp, use })
        .encrypt(key);
}
export async function open(jwe: string, keyForKid: (kid: string) => Uint8Array) {
    const { plaintext } = await compactDecrypt(jwe, (h) => keyForKid(h.kid!));
    return new TextDecoder().decode(plaintext);
}
```

### 鍵: 署名鍵から HKDF 導出（別鍵を管理しない）

署名鍵と暗号鍵はどのみち同じ config 秘密塊（SSM/Secrets Manager）に同居するため、
別鍵にしても分離の利益はほぼ無い。管理対象を 1 つに保つため、**暗号鍵は署名鍵から
HKDF で導出**する。

Ed25519 秘密スカラー `d` を IKM として、ドメイン分離ラベル付き HKDF-SHA256 で
32 バイト鍵を生成:

```ts
import { hkdfSync } from "node:crypto"; // Deno でも node:crypto で利用可

function deriveEncKey(dBase64url: string): Uint8Array {
    const ikm = Buffer.from(dBase64url, "base64url");      // 署名鍵の private scalar
    return new Uint8Array(
        hkdfSync("sha256", ikm, Buffer.alloc(0) /*salt*/,
                 "backlog-mcp:token-enc:A256GCM:v1" /*info*/, 32),
    );
}
```

- 素の `SHA-256(d)` ではなく HKDF を使う理由: ドメイン分離（用途明示）と
  バージョニング（アルゴリズム変更時に `:v2` で切替）。コストはゼロ。
- JWE の `kid` は**署名鍵と同じ `kid`** を流用。復号時はトークンの `kid` →
  該当署名鍵の `d` → `deriveEncKey()` で鍵を再生成。別の鍵マップ管理は不要。

### ローテーション方針: D1（retired 鍵にも `d` を残す）

導出にすると「署名鍵ローテーション＝暗号鍵ローテーション」になる。暗号鍵の導出には
`d` が必要なため、現状のように retired 鍵を公開鍵部分（`x`）だけ保持すると
過去発行トークンを復号できない。

→ **outstanding なトークンが生きている間は、その鍵を `d` 付きで JWKS に残す**ルールにする。
これにより署名鍵ローテーション後もシームレスに旧トークンを復号できる。retired 鍵が
private 材料を持つが、どのみち同じ config 秘密塊の中なので追加の露出にはならない。

## 移行: 両対応せず再認証へ倒す

復号できない（＝旧平文形式 or 未知形式）トークンは弾いて再認証させる。
これでリフレッシュトークンが強制更新され、一気に新形式へ移行する。

- アクセストークン経路: `at` が復号できない → **401 Unauthorized** →
  MCP クライアントが OAuth 再認証を起動
- リフレッシュトークン経路: `rt` が復号できない → **`invalid_grant`** → 再認証

**実装上の必須要件:** 復号失敗が「ハードエラー（500/クラッシュ）」ではなく
「再認証を促す 401 / invalid_grant」に確実にマッピングされること。
`compactDecrypt` の例外を握って認証エラーに変換する経路を用意する。

デプロイ時に全ユーザーが一度だけ再認証する一過性コストは許容する。

## 実装タスク

1. **`crypto/secret.ts` 新規作成**: `seal` / `open`、`deriveEncKey`。
   復号失敗を表す専用エラー型を定義（上位で 401/invalid_grant に変換するため）。
2. **`crypto/jwt.ts` の `loadSigningKeys` 改修**:
   - パース済み JWKS JSON から各鍵の `d`（base64url）を直接読み、
     `kid → encKey(Uint8Array)` のマップを構築（`importJWK` とは別経路で `d` を取得）。
   - retired 鍵にも `d` を要求する前提に変更。
   - `SigningKeys` に `encKeys: Map<string, Uint8Array>` を追加。
3. **書き込み側**: `setSpaceAccess` / `setSpaceRefresh`（および旧 `bl_access_token` /
   `bl_refresh_token` を発行している箇所）で値を `seal()` してから格納。
   署名時の `kid` と JWE の `kid` を一致させる。
4. **読み出し側**:
   - `middleware/jwt-auth.ts` の抽出部: CLI へ `BACKLOG_ACCESS_TOKEN` を渡す直前に `open()`。
   - `oauth/handlers.ts` の refresh 経路: Backlog を叩く直前に `rt` を `open()`。
     再発行時のアクセストークン暗号文はそのままコピー。
   - 復号失敗 → 401 / invalid_grant に変換。
5. **config schema**: 必要なら retired 鍵に `d` 必須のバリデーションを追加
   （`config/schema.ts`）。新しい設定項目は増やさない（鍵は導出）。
6. **テスト**: `crypto/secret.test.ts`（seal/open ラウンドトリップ、AAD 不一致拒否、
   kid 解決、復号失敗エラー型）、`jwt.test.ts` の更新（暗号化フィールドの往復）、
   旧形式トークン→再認証エラーになる経路のテスト。
7. **ドキュメント**: 実装完了後、本プランの設計部分を `docs/design/remote-mcp-server.md`
   （または oauth-relay-server.md）へ反映し、本プランは削除。

## 残存リスクと将来の追加対策（本プラン対象外）

- **サーバーへのリプレイ**: アクセストークン TTL を短く、リフレッシュトークンの
  ローテーション＋再利用検知（要・最小限の状態管理）、失効リスト、異常検知。
- **暗号鍵（＝署名鍵 config）の漏洩**: 設計上ゲームオーバー。SecureString /
  Secrets Manager・最小権限 IAM・定期ローテーションで厳重管理。
- **リプレイすらクライアント鍵なしに不可能化**: sender-constrained token（DPoP / mTLS）。
  ただし CLI/MCP では DPoP 秘密鍵も侵害済みクライアント上に乗るため、
  TPM/セキュアエンクレーブが無い限り効果は限定的。費用対効果は低い。

## 完了チェック

```bash
make lint
make test
make build
# MCP サーバーパッケージ側のテスト（packages/mcp-server）も実行
```
