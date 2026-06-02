# activity コマンド / issue list の関与課題横断取得

この文書は実装のソースオブトゥルースとする。実装が変わったら本書も更新する。

関連 Issue: #23（`backlog activity` コマンド）, #24（`issue list --involved`）

## 背景

週次進捗報告などで「自分が関わった課題」を集計したいが、`issue list --mine`（assignee のみ）
では「自分が起票したが他人担当の課題」「コメントだけした課題」が漏れる。Backlog の
`GET /api/v2/issues` には commenter 条件が無く、サーバー側フィルタだけではコメント貢献を拾えない。

そこで 2 つの独立した手段を提供する。

- `backlog activity`: ユーザーのアクション履歴フィードを第一級コマンドとして公開する（汎用）。
- `issue list --involved`: 課題ストアベースで関与課題を横断取得する（assignee ∪ author）。

両者は独立しており、`--involved` は activity フィードに依存しない。

## backlog activity コマンド（#23）

### API

- `GET /api/v2/users/{userId}/activities`（ogen: `getUserRecentUpdates`）
- ラッパー: `internal/api/activity.go` の `Client.GetUserActivities` / `ActivityListOptions`

activities API は **日付レンジ引数を持たない**（`minId` / `maxId` / `count`(≤100) / `order` / `activityTypeId[]`）。

### アクティビティ種別マッピング

`internal/cmd/activity/types.go` でセマンティック名 ↔ activityTypeId（1-26）を双方向に持つ。
`--type` はカンマ区切りのセマンティック名または数値 ID を受け付け、`ParseTypes` で解決する。
代表例: `issue-create`(1) / `issue-update`(2) / `issue-comment`(3) / `issue-bulk-update`(14) /
`git-push`(12) / `pr-add`(18) など。`--type` 省略時のデフォルトは課題系
`issue-create,issue-update,issue-comment`。

### 日付レンジのクライアント側ページング

`--since` / `--until`（YYYY-MM-DD）は API 非対応のため CLI 側で吸収する
（`internal/cmd/activity/list.go` の `fetchActivities`）。

1. `order=desc`（新しい順）, `count=100` で取得する。
2. 各 activity の `created`（RFC3339）を表示タイムゾーンで解釈した `[since 00:00, until 23:59:59...]`
   と比較し、範囲内のみ採用する。
3. desc 取得のため、`created` が `since` より前になった時点でページングを打ち切る。
4. 続く場合は `maxId = 末尾 id - 1` で次ページへ。`--limit`（デフォルト 100、0=範囲内全件）に達したら打ち切る。
5. `--order asc` 指定時は、ページング後に結果を反転して出力する。

### 出力

- `-o json`: 生 activity を出力。
- table（デフォルト）: `TYPE`(セマンティック名) / `PROJECT`(projectKey) /
  `ISSUE`(`{projectKey}-{key_id}`) / `SUMMARY` / `CREATED`。

## issue list --involved（#24）

`internal/cmd/issue/involved.go` に実装。activity フィードは Backlog 側で履歴がトリムされるため
`--involved` の土台には使わず、**全件・日付正確・トリムなしの課題ストア**を信頼の起点とする。

### --involved <user>（土台 = assignee ∪ author）

1. `cmdutil.ResolveUserID` でユーザー ID を解決（`@me` / 数値 ID / userId / 表示名）。
2. `assigneeId[]=<me>` + 既存フィルタ（`--updated-since/until`, `--state` 等）で `GetIssues`。
3. `createdUserId[]=<me>` + 同フィルタで `GetIssues`。
4. `issueKey` で union（重複排除）し、`updated` 降順にソート。`--limit` 件に切り詰める。

- `-p all` で横断、`-p KEY` で限定。
- `--involved` は `--mine` / `--assignee` / `--author` と排他（包含関係のため併用エラー）。

### --include-commented（コメント次元・クライアント側スキャン・オプトイン）

`GET /api/v2/issues` に commenter 条件が無いため、CLI 側で解決する（`scanCommentedIssues`）。

1. 候補プロジェクト = `-p` 明示時はそれ、`-p all` 時は union に出現したプロジェクト集合。
2. 候補プロジェクトで `--updated-since/until` の課題を取得する。
3. union 済みの課題を差し引く。
4. 残り各課題の `GET /api/v2/issues/:id/comments` を desc + `maxId` ページングで調べ、
   `createdUser.id == <me>` かつ `created ∈ [since, until]`（`--updated-since/until` を窓に流用）を判定する。
5. 該当を union にマージする。

- N+ リクエストになるため進捗表示（`ui.StartProgress`）を行う。
- 候補プロジェクトが特定できない場合（union が空 かつ `-p` 未指定）は全社スキャンを避けてスキップする。
- `--involved` 必須（単独指定はエラー）。

### --viewed（閲覧課題・別オプトイン）

- `GET /api/v2/users/myself/recentlyViewedIssues`（ogen: `getListOfRecentlyViewedIssues`）。
  ラッパー: `internal/api/user.go` の `Client.GetRecentlyViewedIssues`。
- 閲覧 ≠ 作業でノイズが多く、日付レンジ引数も無く履歴も直近限定のため、`--involved` には混ぜず
  明示指定時のみ。週報では「参考」扱い。
- `--viewed` 指定時は他のサーバー側フィルタを無視し、取得した課題をそのまま通常の issue list 形式で返す。
- `--involved` とは排他。

## 補足

- 期間フィルタの境界（YYYY-MM-DD）は表示タイムゾーン（`display.timezone`）で解釈する。
- ogen 生成コードは未知の JSON フィールドを無視するため、`ActivityContent` は issueKey 復元に必要な
  `key_id` と表示用 `summary` 等のみ定義する。
