# CLAUDE.md - Backlog CLI Project

## プロジェクト概要

Backlog用のコマンドラインインターフェース「backlog」を開発するプロジェクトです。
GitHub CLI (gh) と同様のユーザー体験を目指します。

## 技術スタック

- **言語**: Go 1.23+
- **ツール管理**: mise
- **ビルド**: GNU Make, GoReleaser
- **フロントエンド**: React + Vite (web/)

### 外部ライブラリ

| 用途            | ライブラリ                              |
|---------------|------------------------------------|
| CLI フレームワーク   | `github.com/spf13/cobra`           |
| 設定管理          | `github.com/yacchi/jubako`         |
| 対話的 UI        | `github.com/AlecAivazis/survey/v2` |
| JWT           | `github.com/golang-jwt/jwt/v5`     |
| ブラウザ起動        | `github.com/pkg/browser`           |
| パスワードハッシュ     | `golang.org/x/crypto/bcrypt`       |

## ディレクトリ構成

```
backlog-cli/
├── cmd/backlog/          # エントリーポイント
├── internal/
│   ├── cmd/              # コマンド定義 (cobra)
│   ├── api/              # Backlog API クライアント
│   ├── config/           # 設定管理 (jubako)
│   ├── auth/             # OAuth認証 (CLI側)
│   ├── relay/            # 中継サーバー
│   ├── domain/           # ドメイン操作ユーティリティ
│   ├── jwk/              # JWK操作ユーティリティ
│   └── ui/               # 対話的UI・Webアセット
├── web/                  # React SPA (認証UI/ポータル)
├── deploy/aws-cdk/       # AWS CDKデプロイ設定
├── docs/                 # 設計ドキュメント
├── version.txt           # リリースバージョン
├── .mise.toml
├── Makefile
└── go.mod
```

## 開発コマンド

```bash
# ビルド（アーティファクト作成時）
make build

# テスト
make test

# リント
make lint

# 開発用実行（go run使用）
make run ARGS="<コマンド引数>"

# 中継サーバー起動（開発用、go run使用）
make serve

# クリーン
make clean
```

## リリース手順

1. `version.txt` のバージョンを更新（例: `0.5.0` → `0.6.0`）
2. master ブランチにプッシュ
3. CI が自動で以下を実行:
   - タグ `v{version}` を作成・プッシュ
   - GoReleaser でビルド・リリース作成
   - Homebrew tap を更新

**注意**: タグはローカルで打たない。CI が `version.txt` を読んで自動生成する。

## 設計ドキュメント

- `docs/design/oauth-relay-server.md` - OAuth中継サーバー設計書
- `docs/design/relay-config-bundle.md` - Relay Config Bundle仕様書
- `docs/design/backlog-gfm-conversion.md` - Backlog記法→GFM変換仕様

## ドキュメント運用（プロジェクトメモリ）

- `docs/design/`: 「現状の実装」を説明する設計書の置き場（参照用のソースオブトゥルース）
  - 実装が変わったら必ず更新する（仕様・フロー・データ形式・セキュリティ前提・制約）
  - 実装プランから固まった内容は、ここに分割して残す
- `docs/plans/`: 「これから実装する」ための実装プランの置き場
  - 実装が完了したプランは残さない（必要な内容は `docs/design/` 側に設計書として移す）
  - 今後もプラン作成時は `docs/plans/` を使う

## 重要な設計判断

### 1. 設定の優先順位

```
コマンド引数 > 環境変数 > .backlog.yaml > ~/.config/backlog/config.yaml > デフォルト
```

### 2. OAuth認証フロー

CLIにClient Secretを持たせず、中継サーバー経由でトークンを取得します。
詳細は `docs/design/oauth-relay-server.md` を参照。

### 3. 複数ドメイン対応

backlog.jp と backlog.com の両方に対応。中継サーバーで複数の Client ID/Secret を管理。

### 4. プロジェクトローカル設定

`.backlog.yaml` をリポジトリルートに配置することで、Git リポジトリと Backlog プロジェクトを紐付け。

### 5. Relay Config Bundle

組織が配布する設定バンドル（ZIP）を信頼の起点とし、CLIが不正な中継サーバーへ接続しないことを保証。
詳細は `docs/design/relay-config-bundle.md` を参照。

### 6. Backlog API クライアント実装

**必須ルール**: Backlog API を呼び出す際は、以下の手順に従う。

1. **OpenAPI 定義を追加**: `docs/api/openapi.yaml` にエンドポイントを定義
2. **ogen で生成**: **必ず `make generate` を実行**してクライアントコードを生成（直接 ogen コマンドを叩かない）
3. **生成クライアントを使用**: `internal/gen/backlog/` の生成コードを `internal/api/` でラップ

**禁止事項**:
- `http.NewRequest` 等を使った直接的な HTTP リクエストの実装
- ogen 生成コードをバイパスする API 呼び出し
- `make generate` 以外の方法で ogen を実行すること

**ディレクトリ構成**:
```
docs/api/openapi.yaml          # OpenAPI 定義（ソース、手動管理）
docs/api/openapi-generated.yaml  # 参照用自動生成スペック（ogen には使わない）
docs/api/cache.json            # API ドキュメントキャッシュ（自動生成）
internal/gen/backlog/          # ogen 生成コード（自動生成、編集禁止）
internal/api/                  # API ラッパー（コマンドから呼び出す層）
```

**Backlog API ドキュメントの参照方法**:

Backlog API は公式 OpenAPI スペックを提供していない。`docs/api/cache.json` に全エンドポイントの
メタデータ（method/path/params）がキャッシュされているので、まずここを参照すること。

```bash
# キャッシュの内容確認（エンドポイント一覧）
cat docs/api/cache.json | python3 -c "
import json,sys
for url,e in json.load(sys.stdin)['endpoints'].items():
    print(f\"{e['method']:<7} {e['path']}  ({e['title']})\")
"

# 新しいエンドポイントや仕様変更を確認・同期
uv run scripts/backlog-api-sync.py check   # 新規/削除エンドポイントの確認（高速）
uv run scripts/backlog-api-sync.py sync    # キャッシュ更新 + openapi-generated.yaml 再生成

# openapi.yaml に未定義のエンドポイントを確認
uv run scripts/backlog-api-sync.py generate
```

`sync` は HTTP ETag を使った条件付きリクエストで動作する。変更がなければ 304 が返り、
HTML パースは行わない。通常の `sync` 実行コストは発見用 1 リクエスト + 304 × 155 件程度。

**新しい API エンドポイントを実装する手順**:

1. `docs/api/cache.json` でパラメーター仕様を確認
2. `docs/api/openapi-generated.yaml` から対象パスのエントリをコピー
3. `docs/api/openapi.yaml` に貼り付け、`$ref` レスポンススキーマを手動で追加
4. `make generate` を実行

## コーディング規約

- Go の標準的なスタイルに従う
- エラーは適切にラップして返す (`fmt.Errorf("context: %w", err)`)
- パッケージ間の依存は internal/ 内で完結させる
- テストは `*_test.go` に記述
- **internal/ 配下は破壊的変更可**: プロジェクト内でコンパイルが通れば後方互換性の維持は不要

## 実装完了時の必須チェック

コード変更を完了する前に、以下のチェックを**必ず**実行すること:

```bash
make lint   # リントエラーがないこと
make test   # テストがパスすること
make build  # ビルドが成功すること
```

**理由**: CIで失敗するとマージできないため、ローカルで事前に確認する。

## 注意事項

- 外部ライブラリは選定済みのもののみ使用
- 標準ライブラリで実現可能なものは標準ライブラリを優先
- セキュリティに関わる部分（トークン保存、Cookie署名、JWS署名等）は特に注意
