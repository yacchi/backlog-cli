# パッチ編集（Wiki・課題説明の部分更新と衝突検出）

## 概要

Wiki ページと課題の説明文（description）に対して、全文置換せずに部分的な更新を行う機能。
Backlog API にはバージョン管理や楽観的ロックの仕組みがないため、クライアントサイドで
Read-Modify-Write パターンと三方マージによる衝突検出を実装する。

### 解決する問題

- **Lost Update**: 複数人が同時に同じ Wiki/課題を編集すると、後から保存した方が先の変更を上書きする
- **大規模ドキュメントの部分更新**: 巨大な Wiki ページの一部だけを変更したい場合でも全文を送る必要がある
- **MCP コンテキスト最適化**: LLM が MCP 経由で更新する際、全文を生成するとトークンを大量に消費する

## 3つの編集モード

### 1. Search-and-Replace（検索置換）

JSON で検索・置換のペアを指定する。単体オブジェクトまたは配列を受け付ける。

```bash
# 単一
backlog wiki edit 12345 --patch '{"find":"旧テキスト","replace":"新テキスト"}'

# 複数
backlog issue edit PROJ-123 --patch '[{"find":"旧","replace":"新"},{"find":"古い","replace":"新しい"}]'

# stdin から読み込み
echo '[{"find":"A","replace":"B"}]' | backlog wiki edit 12345 --patch-file -
```

- JSON 形式: `{"find":"...","replace":"..."}` または配列 `[{...},{...}]`
- 対象テキストが見つからない場合はエラー（古い内容を参照していたことを検出）
- 内部で `strings.Replace(content, find, replace, 1)` を適用

### 2. Append / Prepend（追記・挿入）

```bash
backlog wiki edit 12345 --append "## 新セクション"
backlog issue edit PROJ-123 --prepend "> 更新日: 2024-01-01"
```

- 現在のコンテンツの末尾/先頭にテキストを追加
- 既存の内容には一切触れないため、衝突リスクが最も低い

### 3. Safe Full Replacement（安全な全文置換）

```bash
backlog wiki edit 12345 --content "新しい全文" --safe
backlog issue edit PROJ-123 --body "新しい本文" --safe
```

- `--safe` を付けると、書き込み前に衝突検出を行う
- 衝突があれば三方マージで自動解決を試みる
- `--safe` なしの `--content`/`--body` は従来どおり last-write-wins

## Safe Write プロトコル

すべてのパッチモードで使用される衝突検出の仕組み。

### フロー

```
1. GET wiki/issue     → base content, updated timestamp, content hash を記録
2. patchFn(base)      → ours（ユーザーの変更を適用した結果）
3. GET wiki/issue     → theirs（現在のリモート状態）
4. updated/hash 比較
   ├─ 変更なし → PATCH ours（安全に書き込み）
   └─ 変更あり → ThreeWayMerge(base, ours, theirs)
                 ├─ クリーンマージ → PATCH merged
                 └─ コンフリクト   → ConflictError を返す
```

### 衝突検出の仕組み

| 比較対象 | 目的 |
|---------|------|
| `updated` タイムスタンプ | 高速な変更検出（秒精度） |
| SHA-256 コンテンツハッシュ | 同一秒内の変更や `updated` が更新されない変更の検出 |

両方を組み合わせることで、タイムスタンプの秒精度の限界をカバーする。

### レースウィンドウ

2回目の GET と PATCH の間（数十ミリ秒）にも他者の更新が入る可能性がある。
完全な排他は不可能だが、ウィンドウは実用上無視できるレベルに縮小される。

## 三方マージアルゴリズム

`internal/textmerge/` パッケージに Go 純粋実装。外部依存なし。

### アルゴリズム

1. base/ours/theirs を行に分割
2. LCS（最長共通部分列）で base↔ours、base↔theirs の差分 Hunk を計算
3. Hunk の位置関係で分岐:
   - 重複しない → 両方適用（クリーンマージ）
   - 重複するが同一変更 → 片方を採用
   - 重複して異なる → コンフリクトマーカー挿入

### パッケージ構成

```
internal/textmerge/
├── diff.go        # LCS ベースの行差分（Hunk 列を返す）
├── merge.go       # ThreeWayMerge(base, ours, theirs) → MergeResult
└── merge_test.go  # テーブル駆動テスト
```

### API

```go
// MergeResult holds the output of a three-way merge.
type MergeResult struct {
    Content   string     // マージ結果（コンフリクトマーカー含む場合あり）
    Clean     bool       // true ならコンフリクトなし
    Conflicts []Conflict // Clean=false の場合、コンフリクト箇所のリスト
}

func ThreeWayMerge(base, ours, theirs string) MergeResult
```

## API 層

### Wiki

```go
// SafeUpdateWiki は Read-Modify-Write で安全に Wiki を更新する
func (c *Client) SafeUpdateWiki(
    ctx context.Context, wikiID int,
    patchFn func(current string) (string, error),
) (*SafeUpdateResult, error)
```

### Issue

```go
// SafeUpdateIssueDescription は課題説明を安全に部分更新する
func (c *Client) SafeUpdateIssueDescription(
    ctx context.Context, issueIDOrKey string,
    patchFn func(current string) (string, error),
) (*backlog.Issue, bool, error)
```

### patchFn ヘルパー

```go
PatchFnReplace(ops []PatchOp)         // 検索置換
PatchFnAppend(text string)            // 末尾追記
PatchFnPrepend(text string)           // 先頭挿入
PatchFnFullReplace(newContent string) // 全文置換（--safe 用）
```

## CLI フラグ

| フラグ | wiki edit | issue edit | 動作 |
|-------|-----------|------------|------|
| `--patch` / `--patch-file` | ✓ | ✓ | 検索置換（JSON、単体 or 配列） |
| `--append` | ✓ | ✓ | 末尾追記 |
| `--prepend` | ✓ | ✓ | 先頭挿入 |
| `--safe` | ✓ | ✓ | `--content`/`--body` と併用で衝突検出 |
| `--content`/`--body`（既存） | ✓ | ✓ | 全文置換（last-write-wins、変更なし） |

## 設計判断

### Git コマンド不要

三方マージを Go 純粋実装とした理由:

- MCP サーバーコンテナ（`gcr.io/distroless/nodejs24-debian12`）に Git を入れると +8〜150MB
- CLI とコンテナで異なるコードパス（Git あり/なし）はテスト・挙動差異の問題
- LCS + 行マージは ~200 行で実装可能、外部依存ゼロ
- CLI/MCP 両方で同一バイナリ内の同一コードパスを通る

### 既存動作の完全な維持

- `--content`/`--body` のみ（`--safe` なし）は従来どおり last-write-wins
- パッチモードはオプトイン（`--find`、`--append`、`--prepend`、`--safe` のいずれかを使用時のみ）
- 衝突検出が解決できない場合のフォールバックとして従来モードが常に使える
