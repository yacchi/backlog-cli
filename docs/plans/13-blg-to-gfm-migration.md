# 13: Backlog記法 → GFM変換ツール（草案）

## 目的
- Backlog記法の本文をGFMへ安全に移行する。
- まずは「表示変換（view時のみ）」で検証し、破壊的変更は行わない。
- GFMに直接対応するものは変換する。
- 直接対応がなく仕様も不明確なものは原文保持し、警告を出す。
- 対象は課題（概要/コメント）、Wiki、プルリクエスト（概要/コメント）。

## 仕様方針
- 変換は「確実に意味を保てるもののみ」実施。
- 変換不能/曖昧な記法は維持し、警告ログに積み上げる。
- dry-runで差分/警告を先に確認できるようにする。
- 変換結果は冪等（同じ入力で同じ出力）。
- 初期段階では「表示のみ」で利用者の確認を優先する。
- GFM仕様は公式仕様（https://github.github.com/gfm/）を参照する。
- Backlogのドキュメントに無い記法でも、GFMとして妥当なら維持する。

## 変換仕様
- 詳細仕様は `docs/backlog-gfm-conversion-spec.md` を参照すること
- 変換対象、警告、判定、正規表現、キャッシュ仕様は本仕様を正とする

## 適用範囲
- 課題: 概要、コメント
- Wiki: 本文
- プルリクエスト: 概要、コメント
- 付随: 将来的に「共有リンク/日報」なども追加可能

## 実装フェーズ（表示のみ → 書き戻し）
### フェーズ1: viewの表示変換
- issue/pr/wiki のviewコマンドに `--markdown` を追加
- 変換結果は表示のみで、Backlog側には書き戻さない
- オプション仕様
  - `--markdown`: Backlog記法→GFM変換を強制表示
  - `--raw`: 変換を無効化して原文表示
  - `--markdown-warn`: 変換警告のサマリを表示
  - `--markdown-cache`: 分析用キャッシュ保存を有効化（既定は無効）
  - 競合時の優先: `--raw` > `--markdown`
  - 解決順: CLI > 環境変数 > config.yaml > デフォルト

### フェーズ1.5: 設定ファイルで既定化
- `config.yaml`に「markdown表示変換を常に有効」なフラグを追加
- CLI引数が設定より優先
- 設定キー案
  - `display.markdown_view` (bool, default: false)
  - `display.markdown_warn` (bool, default: true)
  - `display.markdown_cache` (bool, default: false)
  - `display.markdown_cache_raw` (bool, default: false)
  - `display.markdown_cache_excerpt` (int, default: 200)
  - 対応する環境変数も用意する（例: `BACKLOG_DISPLAY_MARKDOWN_VIEW`）
- 環境変数例
  - `BACKLOG_DISPLAY_MARKDOWN_VIEW`
  - `BACKLOG_DISPLAY_MARKDOWN_WARN`
  - `BACKLOG_DISPLAY_MARKDOWN_CACHE`
  - `BACKLOG_DISPLAY_MARKDOWN_CACHE_RAW`
  - `BACKLOG_DISPLAY_MARKDOWN_CACHE_EXCERPT`

## コマンド別の適用ポイント
- issue view
  - 対象: description, comments
- pr view
  - 対象: description, comments
- wiki view
  - 対象: content
- 表示時に「原文」と「変換後」の出し分けは既存のformatに合わせる

## 変換処理の詳細
- ブロック/インラインの変換手順、ルールIDは `docs/backlog-gfm-conversion-spec.md` を参照

### フェーズ2: 書き戻し（任意）
- 変換ルールが安定したら `--apply` 等でBacklogへ書き戻し可能にする
- dry-run/差分/警告は必須

## 出力とログ
- dry-run
  - 変換件数、警告件数、警告種別ごとの集計
  - 先頭N件のサンプルを表示
- 実行
  - 変換前後の差分はログに保存
  - 警告はログファイルに集約
- view時
  - 標準出力に変換済み本文を表示
  - `--markdown-warn`有効時は末尾に警告サマリを付加

## 分析データのキャッシュ
### 目的
- 変換前/変換後/警告/判定スコアを保存し、精度向上の分析に利用

### 保存場所
- 既定: `~/.cache/backlog/markdown/`（XDG_CACHE_HOMEがあれば優先）
- 1件ごとにJSONL形式で追記
- ファイル名
  - 既定: `events.jsonl`
  - 将来: 日付ローテーションを検討

### 保存内容
- 保存内容/スキーマは `docs/backlog-gfm-conversion-spec.md` を参照

### 取り扱い上の注意
- APIキー/個人情報は保存対象外にする
- 収集・保存は明示的に有効化（既定は無効）
- raw保存は明示的に有効化（既定は無効）
- raw無効時はメールアドレス等の簡易マスキングを行う

## 実装案（Go）
- 文字列変換の段階的パイプライン
  - Block-level変換（見出し/引用/コード/目次/表/リスト）
  - Inline変換（強調/リンク/改行）
- 変換器はAST化せず、正規表現＋状態機械で安全に処理
  - ただし将来的な拡張のため、ブロック単位に分割する構造は維持

## 実装モジュール案
- `internal/markdown/convert.go`
  - `Convert(input, opts) (output string, meta Metadata)`
- `internal/markdown/detect.go`
  - `DetectMode(input) (mode, score, warnings)`
- `internal/markdown/warnings.go`
  - 警告タイプと集計ロジック
- `internal/markdown/cache.go`
  - JSONL保存

## テスト方針
- 変換ルール単位のユニットテスト
- 代表的なBacklog記法のE2Eテスト
- 変換対象外（GFM既存構文）の保持テスト

## 既存コマンドへの統合
- viewコマンドは変換処理を「表示層でのみ」適用
- 取得済みの本文は変換せずに保持（書き戻しフェーズまで）

## 例外・注意
- 詳細は `docs/backlog-gfm-conversion-spec.md` を参照

## 受け入れ基準
- dry-runと実行モードで同じ変換対象が報告される
- 変換済みテキストに再度実行しても差分が生じない
- 警告は種別/件数/サンプルを出力できる
