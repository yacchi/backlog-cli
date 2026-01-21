# AI要約プロンプト自動改善機能 設計プラン

## 概要

AI要約機能において、出力用モデルと評価用モデルを使用して、プロンプトを自動的に改善する機能を実装する。

## アーキテクチャ

```
+-------------------------------------------------------------------------+
|                      backlog ai prompt optimize                          |
+-------------------------------------------------------------------------+
|                                                                          |
|  +-------------+    +------------------+    +---------------------+      |
|  |  サンプル    |--->|   出力モデル      |--->|    評価モデル        |      |
|  |  課題       |    |   (要約生成)      |    |   (スコア+改善提案)   |      |
|  +-------------+    +------------------+    +---------+-----------+      |
|                                                       |                  |
|                                                       v                  |
|  +-------------+    +------------------+    +---------------------+      |
|  |  ベスト     |<---|   プロンプト      |<---|  スコア >= 閾値?     |      |
|  |  プラクティス |    |   改善器         |    |  or 最大反復回数?    |      |
|  +-------------+    +------------------+    +---------------------+      |
|                              |                         |                 |
|                              |                         | No              |
|                              v                         v                 |
|                     +------------------+           ループ継続             |
|                     |   新旧プロンプト  |                                 |
|                     |   比較・選択     |                                 |
|                     +------------------+                                 |
|                              |                                           |
|                              v                                           |
|                     +------------------+                                 |
|                     |   履歴ストア     | -- ~/.cache/backlog/prompt/     |
|                     |   (JSONL)       |    optimization_history.jsonl   |
|                     +------------------+                                 |
+-------------------------------------------------------------------------+
```

## コマンド構造

```bash
# メインコマンド
backlog ai prompt optimize [flags]

# フラグ:
  --prompt-type string           対象プロンプト: issue_list または issue_view (デフォルト: issue_list)
  --project string               課題選定の対象プロジェクト（省略時は全プロジェクトから選定）
  --max-iterations int           最大反復回数 (設定を上書き)
  --score-threshold int          目標スコア閾値 (設定を上書き)
  --evaluation-provider string   評価モデルプロバイダー (設定を上書き)
  --dry-run                      変更を行わずに実行内容を表示
  --show-history                 最適化履歴を表示
  --apply                        最適化されたプロンプトを設定に適用
```

## 設定スキーマ

`defaults.yaml` に追加:

```yaml
ai_summary:
  # 既存設定...

  optimization:
    # 評価モデルプロバイダー（評価・課題選定・改善提案を担当）
    evaluation_provider: "claude"

    # 終了条件のスコア閾値 (1-10)
    score_threshold: 8

    # 最大反復回数
    max_iterations: 5

    # 課題選定の対象プロジェクト（空の場合は全プロジェクトから選定）
    target_project: ""

    # 出力モデル情報テンプレート（評価モデルへのコンテキスト提供用）
    output_model_context: |
      出力モデル: {provider_name}
      コマンド: {command} {args}
```

**注**: ベストプラクティス関連の設定は設けない。評価モデルが必要に応じてWeb検索等で動的に収集する。

## 最適化ループフロー

### ステップ1: セッション初期化
1. 設定読み込み・検証
2. 現在のプロンプトテンプレート取得
3. 出力モデルの情報取得（コマンド、引数、プロバイダー名）

### ステップ2: 評価モデルによる課題自動選定
評価モデルがBacklog APIを使用して精度調整に適した課題を自動選定:
1. Backlog APIで課題一覧を取得（対象プロジェクト or 全プロジェクト）
2. 評価モデルが以下の基準で課題を選定:
   - 説明文が十分に記載されている
   - 技術的内容・ビジネス内容が混在（多様性）
   - コメントがある課題を含む（issue_view評価用）
   - 異なるタイプの課題を含む（バグ、機能要望、タスク等）
3. 選定結果と選定理由を記録

### ステップ3: 要約生成
- 選定された課題に対して出力モデルで要約生成
- 入力・出力・処理時間を記録

### ステップ4: 評価モデルによる品質評価
評価モデルが要約結果を評価:
1. 評価基準（正確性、簡潔性、明瞭性、一貫性、アクション可能性）
2. 出力モデルの特性を考慮した評価
3. 出力:
   - スコア (1-10)
   - フィードバック
   - 強み・弱み
   - 改善提案

### ステップ5: 終了条件チェック
- スコア >= 閾値 → 完了
- 反復回数 >= 最大回数 → 完了（最良プロンプトを選択）
- それ以外 → 改善ステップへ

### ステップ6: 改善プロンプト生成（評価モデルが主導）
評価モデルが以下を実行:
1. **ベストプラクティス収集**（動的）
   - 必要に応じてWeb検索でプロンプトエンジニアリングの最新情報を取得
   - 公式ドキュメント（Anthropic, OpenAI等）を参照
   - 出力モデル固有のプロンプトガイドラインがあれば参照
2. **改善プロンプト生成**
   - 評価フィードバック + 収集したベストプラクティスを基に改善
   - 出力モデルの特性（コマンド、能力）を考慮
   - 新しいプロンプト候補を生成

### ステップ7: 新旧比較
1. 新プロンプトで同じ課題を要約
2. 評価モデルで新旧結果を比較
3. 勝者を次イテレーションの基準として採用

### ステップ8: 履歴保存・ループ継続
- JSONL形式で履歴に追記（選定課題、評価結果、改善内容等）
- 進捗をユーザーに表示

## ファイル構成

### 新規作成

```
packages/backlog/internal/
├── cmd/
│   └── ai/
│       ├── ai.go                      # 親コマンド: backlog ai
│       ├── prompt.go                  # サブコマンド: backlog ai prompt
│       └── optimize.go                # サブコマンド: backlog ai prompt optimize
│
└── summary/
    └── optimizer/
        ├── optimizer.go               # メイン最適化ループ
        ├── types.go                   # データ構造定義
        ├── issue_selector.go          # 課題自動選定ロジック
        ├── evaluator.go               # 評価モデルラッパー（評価・選定・改善を統括）
        ├── improver.go                # プロンプト改善ロジック
        ├── comparator.go              # A/B比較ロジック
        ├── history.go                 # JSONL履歴ストレージ
        └── optimizer_test.go          # テスト
```

### 修正対象

```
packages/backlog/internal/
├── cmd/root.go                        # aiコマンド登録
└── config/
    ├── defaults.yaml                  # optimization設定追加
    └── resolved.go                    # ResolvedPromptOptimization追加
```

## 履歴ストレージ形式

保存場所: `~/.cache/backlog/prompt/optimization_history.jsonl`

```json
{"type":"session_start","session_id":"uuid","started_at":"2025-01-12T10:00:00Z","prompt_type":"issue_list","initial_prompt":"..."}
{"type":"iteration","session_id":"uuid","iteration":{"number":1,"score":6,"feedback":"..."}}
{"type":"iteration","session_id":"uuid","iteration":{"number":2,"score":8,"feedback":"..."}}
{"type":"session_end","session_id":"uuid","completed_at":"2025-01-12T10:05:00Z","status":"completed","final_prompt":"..."}
```

## 実行時の出力例

```
$ backlog ai prompt optimize --prompt-type issue_list --project MYPROJ

issue_list プロンプトの最適化を開始...
評価プロバイダー: claude
目標スコア: 8
最大反復回数: 5

[課題選定]
評価モデルが精度調整に適した課題を選定中...
  対象プロジェクト: MYPROJ
  候補課題を取得中... 50件
  選定結果:
    MYPROJ-123: バグ修正 - 説明文充実、技術的内容
    MYPROJ-145: 機能要望 - ビジネス要件、コメント多数
    MYPROJ-201: タスク - 短い説明、シンプルな内容
    MYPROJ-189: 改善提案 - 技術・ビジネス混在
    MYPROJ-210: 調査 - 長文の説明、複雑な内容
  選定理由: 多様なタイプ・長さ・内容の課題を含めることで汎用性を評価

[反復 1/5]
現在のプロンプトで要約生成中...
  MYPROJ-123: 生成完了 (1.2秒)
  MYPROJ-145: 生成完了 (0.9秒)
  MYPROJ-201: 生成完了 (0.8秒)
  MYPROJ-189: 生成完了 (1.1秒)
  MYPROJ-210: 生成完了 (1.5秒)

評価中...
  スコア: 6/10
  強み:
    - 構造が明確
    - 適切な長さ
  弱み:
    - アクション項目が不足
    - 専門用語の説明がない

改善プロンプト生成中...
  ベストプラクティス収集中（Web検索）...
  改善案を生成中...

新旧プロンプト比較中...
  勝者: 新プロンプト (7 vs 6)

[反復 2/5]
...

最適化完了!
最終スコア: 8/10
反復回数: 3

最適化されたプロンプト:
================
[最適化後のプロンプトを表示]
================

このプロンプトを適用するには:
  backlog ai prompt optimize --prompt-type issue_list --apply
```

## 実装順序

### フェーズ1: 基盤 (4ファイル)
1. `optimizer/types.go` - データ構造定義
2. `optimizer/history.go` - JSONL読み書き
3. `config/defaults.yaml` - 設定追加
4. `config/resolved.go` - ResolvedPromptOptimization追加

### フェーズ2: コアロジック (4ファイル)
5. `optimizer/issue_selector.go` - 課題自動選定ロジック
6. `optimizer/evaluator.go` - 評価モデルラッパー（評価・選定・改善を統括）
7. `optimizer/improver.go` - 改善ロジック（Web検索によるベストプラクティス収集含む）
8. `optimizer/comparator.go` - 比較ロジック

### フェーズ3: メインオプティマイザー (1ファイル)
9. `optimizer/optimizer.go` - メインループ

### フェーズ4: コマンド統合 (4ファイル)
10. `cmd/ai/ai.go` - 親コマンド
11. `cmd/ai/prompt.go` - promptサブコマンド
12. `cmd/ai/optimize.go` - optimizeサブコマンド
13. `cmd/root.go` - aiコマンド登録

### フェーズ5: テストと仕上げ
14. `optimizer/optimizer_test.go` - ユニットテスト
15. `make generate` で設定パス再生成

## 検証方法

1. **ユニットテスト**: `make test`
2. **手動テスト**:
   ```bash
   # ドライラン
   backlog ai prompt optimize --prompt-type issue_list --project TESTPROJ --dry-run

   # 実行
   backlog ai prompt optimize --prompt-type issue_list --project TESTPROJ

   # 履歴確認
   backlog ai prompt optimize --show-history

   # 適用
   backlog ai prompt optimize --prompt-type issue_list --apply
   ```
3. **ビルド確認**: `make build`
4. **リント確認**: `make lint`

## 重要な変更対象ファイル

- `/packages/backlog/internal/summary/summary.go` - オプティマイザーが使用
- `/packages/backlog/internal/summary/ai_provider.go` - Providerインターフェース
- `/packages/backlog/internal/config/resolved.go` - 設定構造体追加
- `/packages/backlog/internal/config/defaults.yaml` - デフォルト設定追加
- `/packages/backlog/internal/cmd/root.go` - コマンド登録
