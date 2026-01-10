# Backlog記法 → GFM 変換仕様

この文書は変換仕様のソースオブトゥルースとする。
実装は本仕様に準拠して更新する。

## 基本方針
- GFMに直接対応するものは変換する。
- 直接対応がなく仕様も不明確なものは原文保持し、警告を出す。
- 既にGFMとして妥当な構文は変換せず保持する。
- 変換結果は冪等であること。
- コードブロック/インラインコード内は変換対象外。

## GFM仕様の考慮点
- 参照: https://github.github.com/gfm/
- GFM固有の構文が含まれる場合、Backlog記法への誤判定を避ける。
- GFMの構文か判断できない独自記法は警告のみで保持する。

### GFMシグナル（Backlog判定を抑制）
- タスクリスト: `- [ ]` / `- [x]`
- フェンスコードブロック: fenced code (```lang)
- テーブル（`|`行 + `---`セパレータ）
- 打ち消し線: `~~text~~`
- 自動リンク: `<https://example.com>`
- 参照リンク定義: `[id]: https://example.com`

判定正規表現（例）
- タスクリスト: `^\s*[-*+]\s+\[[ xX]\]\s+`
- フェンスコード: ``^```+``
- テーブルセパレータ: `^\s*\|?.*\|.*\n\s*\|?\s*:?-{3,}:?\s*\|`
- 打ち消し線: `~~[^~]+~~`
- 自動リンク: `<https?://[^>]+>`
- 参照リンク定義: `^\s*\[[^\]]+\]:\s+\S+`

## 変換対象（確実にGFMへ変換）
### 見出し
- Backlog: 行頭`*`の数でレベル指定（`*`〜`***`）
- GFM: 行頭`#`+半角スペース
- 変換例: `* 見出し` → `# 見出し`
- 変換条件
  - 行頭の連続`*`の直後がスペースまたは文字
  - 変換対象は最大3段まで（`***`）
- 正規表現（行頭）
  - `^(\*{1,3})\s*(\S.*)$` → `#{len} $2`

### 引用
- Backlog: `{quote}...{/quote}`
- GFM: `> ...`（複数行は各行に`>`）
- 変換条件
  - `{quote}`〜`{/quote}`の範囲をブロックとして扱う
  - 中身の行頭に`>`を付与、空行は`>`のみ
- 正規表現（ブロック）
  - `(?s)\{quote\}(.*?)\{/quote\}` をブロック抽出

### コード
- Backlog: `{code}` / `{code:lang}...{/code}`
- GFM: フェンス ``` / ```lang
- 変換条件
  - `{code}`/`{code:lang}`〜`{/code}`の範囲をブロックとして扱う
  - `lang`はそのままフェンスに付与（不明なら空）
- 正規表現（ブロック）
  - `(?s)\{code(?::([a-zA-Z0-9_+-]+))?\}(.*?)\{/code\}`

### 強調/打ち消し
- Backlog: `''太字''`, `'''斜体'''`, `%%打ち消し%%`
- GFM: `**太字**`, `*斜体*`, `~~打ち消し~~`
- 変換条件
  - 入れ子や不整合は変換せず警告
  - インラインコード内は対象外
- 正規表現（インライン）
  - 太字: `''([^']+?)''`
  - 斜体: `'''([^']+?)'''`
  - 打ち消し: `%%([^%]+?)%%`

### リンク
- Backlog: `[[URL]]`, `[[label>URL]]`, `[[label:URL]]`
- GFM: `<URL>` または `[label](URL)`
- 変換条件
  - URL判定は `https?://` または `mailto:` を満たすもの
  - URL以外はWikiリンク扱いとして警告
- 正規表現（インライン）
  - `\[\[([^\]]+?)\]\]`
  - 内部で `label>url` / `label:url` を判定

### 目次
- Backlog: `#contents`
- GFM: `[toc]`
- 正規表現（行単位）
  - `^#contents\s*$` → `[toc]`

### 改行
- Backlog: `&br;`
- GFM: `<br>` または末尾2スペース+改行
- 実装はオプションで選択（デフォルトは`<br>`）
- 正規表現（インライン）
  - `&br;` → `<br>`

### リスト
- Backlog: `-`（箇条書き）, `+`（番号付き）
- GFM: `-`/`*`/`+` と `1.`
- 方針: `+`は`1.`に変換（内容保持）。`-`はそのまま（必要ならスペース補正）。
- 正規表現（行頭）
  - `^\+\s+` → `1. `
  - `^-\S` の場合は `- ` を補完

## 警告のみ（変換せず原文保持）
### 色指定
- `&color(...) { ... }`
- GFM標準に色指定はないため保持+警告

### 表（Backlog独自）
- 行末`h`やセル先頭`~`のヘッダ指定
- セル結合`||`
- GFMに正確に写像不可のため保持+警告

### Wiki添付サムネイル
- `#thumbnail(...)`
- Markdown側に明示対応がないため保持+警告

### その他未定義の独自マクロ
- 上記以外の`#xxx(...)`や`{xxx}`を検知したら保持+警告
- 正規表現（検出のみ）
  - `#([a-zA-Z0-9_+-]+)\([^\)]*\)`
  - `\{([a-zA-Z0-9_+-]+)\}`
- 既知の許可リスト（警告しない）
  - `#attach`, `#image`, `#thumbnail`, `#rev`
  - `{code}`, `{quote}`

## 警告タイプ一覧
- `color_macro`: `&color(...) { ... }`
- `table_header_h`: 表の行末`h`によるヘッダ指定
- `table_header_cell`: セル先頭`~`によるヘッダ指定
- `table_cell_merge`: `||` によるセル結合
- `thumbnail_macro`: `#thumbnail(...)`
- `unknown_hash_macro`: `#xxx(...)`（既知以外）
- `unknown_brace_macro`: `{xxx}`（既知以外）
- `wiki_link_ambiguous`: `[[...]]`がURL/課題キーではない可能性
- `emphasis_ambiguous`: `''`/`'''`の入れ子や不整合

## 警告サマリ出力フォーマット
### 標準出力（view時）
```
---
Markdown Warning Summary
- item_type: issue
- item_id: 12345
- detected_mode: backlog
- score: 3
- warnings: color_macro=2, table_cell_merge=1
```

### ログ（JSONL）
```
{"item_type":"issue","item_id":12345,"detected_mode":"backlog","score":3,"warnings":{"color_macro":2,"table_cell_merge":1}}
```

## 判定（Backlog記法かどうか）
- 強いシグナル
  - `{code`, `{quote`, `#contents`, `&br;`, `&color`, `%%...%%`, `''...''`, `[[...]]`
- 弱いシグナル
  - `*`見出し/`+`番号付きリスト/`h`終端表行
- 判定方式
  - シグナルの出現数でスコア化し、閾値超えでBacklog記法と判定
  - 閾値未満の場合は「変換しない/警告のみ」か、`--force`で変換
- 判定時の除外
  - フェンスコード/インラインコード内は判定対象外
  - `{code}`ブロック内も判定対象外

判定正規表現（例）
- `{code`: `\{code(?::[^}]+)?\}`
- `{quote`: `\{quote\}`
- `#contents`: `^#contents\s*$`
- `&br;`: `&br;`
- `&color`: `&color\([^)]*\)\s*\{`
- `%%...%%`: `%%[^%]+%%`
- `''...''`: `''[^']+''` / `'''[^']+'''`
- `[[...]]`: `\[\[[^\]]+\]\]`
- `*`見出し: `^\*{1,3}\s+\S`
- `+`番号付き: `^\+\s+\S`
- `h`終端表行: `\|.*\|h\s*$`

## 判定の優先順位（GFMシグナルとの競合）
- ルール1: GFMシグナルが1つでもある場合、Backlog記法判定のスコアを減衰
  - 既定: 強いBacklogシグナル2点、弱い1点、GFMシグナルは-2点
- ルール2: GFMシグナルが強い場合は「GFM優先」
  - 例: フェンスコード、参照リンク定義、GFMテーブルはBacklog変換対象外
- ルール3: 両方混在の場合は「変換は最小限」
  - Backlog専用構文のみ変換し、GFM構文は保持
- ルール4: 例外
  - `{code}`や`{quote}`がある場合はBacklog優先（GFMで表現できるため）
  - `[[...]]`がある場合はBacklog優先（GFMリンクに変換可能なため）

## 判定スコアと閾値
- 目標: GFMコンテンツの誤変換を避けつつ、明確なBacklog記法は拾う
- スコア付与
  - 強いBacklogシグナル: +2
  - 弱いBacklogシグナル: +1
  - GFMシグナル: -2
- 判定結果
  - 合計スコア >= 2: Backlog記法として変換
  - 合計スコア 0〜1: 不明（原文保持 + 警告）
  - 合計スコア <= -1: GFMとして保持（変換しない）
- `--force` の扱い
  - 判定が「不明」の場合のみ変換を許可
  - GFM判定（スコア<=-1）は強制変換しない

## 変換処理の詳細（ブロック/インライン）
- ブロック解析
  - `{code...}` と `{quote}` を先に抽出し、内部を変換対象外とする
  - 抽出部分は一時トークンに置換し、最後に復元
- インライン解析
  - `[[...]]` のリンク変換
  - `''`/`'''`/`%%` の装飾変換
  - `&br;` の改行変換
- 変換順序
  1) ブロック抽出（code/quote）
  2) 行単位変換（見出し/リスト/目次）
  3) インライン変換（リンク/装飾/改行）
  4) ブロック復元

## 変換ルールの拡張ID
- `rule.heading_asterisk`
- `rule.quote_block`
- `rule.code_block`
- `rule.emphasis_bold`
- `rule.emphasis_italic`
- `rule.strikethrough`
- `rule.backlog_link`
- `rule.toc`
- `rule.line_break`
- `rule.list_plus`
- `rule.list_dash_space`

## 分析データのキャッシュ
### 保存内容
- 変換メタデータ
  - `item_type` (issue/pr/wiki/comment)
  - `item_id`
  - `parent_id` (commentの場合のみ)
  - `project_key`
  - `item_key` (issue key / pr number / wiki name)
  - `url` (対象のURL)
  - `detected_mode` (backlog/markdown/unknown)
  - `score`
  - `warnings` (type list + count)
  - `warning_lines` (type -> line numbers)
  - `rules_applied` (rule ids)
- 入出力
  - `input_hash` / `output_hash` (sha256)
  - `input_excerpt` / `output_excerpt` (先頭N文字, default: 200)
  - `input_raw` / `output_raw` は `markdown_cache_raw` がtrueの時のみ保存

### JSONLスキーマ
```
{
  "ts": "RFC3339",
  "item_type": "issue|pr|wiki|comment",
  "item_id": 12345,
  "parent_id": 999,
  "project_key": "PROJ",
  "item_key": "PROJ-123",
  "url": "https://example.backlog.com/view/PROJ-123",
  "detected_mode": "backlog|markdown|unknown",
  "score": 3,
  "warnings": {"color_macro": 2},
  "warning_lines": {"color_macro": [3, 7]},
  "rules_applied": ["rule.backlog_link", "rule.code_block"],
  "input_hash": "sha256",
  "output_hash": "sha256",
  "input_excerpt": "first N chars",
  "output_excerpt": "first N chars",
  "input_raw": "optional",
  "output_raw": "optional"
}
```

### 保存場所
- 既定: `cache.dir/markdown/`（cache.dirが未設定の場合は`~/.cache/backlog/`）
- 1件ごとにJSONL形式で追記
- ファイル名
  - 既定: `events.jsonl`
  - 将来: 日付ローテーションを検討

### 取り扱い上の注意
- APIキー/個人情報は保存対象外にする
- 収集・保存は明示的に有効化（既定は無効）
- raw保存は明示的に有効化（既定は無効）
- raw無効時はメールアドレス等の簡易マスキングを行う
