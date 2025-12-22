# 12. Connect-RPC + Buf 移行計画（SPA認証UI）最終稿

## 概要

ブラウザ↔ローカルCLIサーバー間の通信を Connect RPC + Buf に移行し、型安全な通信とコード生成を実現する。認証状態通知は
server-streaming に置き換え、既存 WebSocket を廃止する。HTTPのHTML系エンドポイント（`/auth/popup`, `/callback`）は維持する。

## スコープ

| 対象                | 方式                       | 備考                        |
|-------------------|--------------------------|---------------------------|
| `/auth/config`    | Connect Unary            | 設定取得                      |
| `/auth/configure` | Connect Unary            | 設定保存                      |
| `/auth/ws`        | Connect Server Streaming | `SubscribeAuthEvents` で置換 |
| `/auth/popup`     | HTTP                     | HTML/リダイレクト維持             |
| `/callback`       | HTTP                     | OAuthコールバック維持             |

## 目標仕様

- SPAはConnectクライアントのみを使用し、JSON/WSの直接通信を廃止する。
- 認証状態は `SubscribeAuthEvents` のストリームで受信する。
- 既存の「切断/grace period」挙動はストリーム切断をトリガーとして継続する。
- 再接続はクライアント側で実装する（connect-webの自動再接続は前提にしない）。

## 状態遷移とパラメーター関係（実装ミス防止用）

### 状態モデル（クライアント表示用）

- `connecting` : ストリーム接続中
- `connected` : ストリーム接続済み（`pending` を受信中）
- `success` : `AuthStatus.SUCCESS` を受信
- `error` : `AuthStatus.ERROR` を受信
- `closed` : ストリーム切断後の再接続が失敗、または明示的に終了

### 状態モデル（サーバー側セッション）

- `pending` : 認証進行中
- `success` : 認証完了
- `error` : 認証失敗
- `DisconnectedAt` : ストリーム切断時刻（nilなら接続中）

### 主要パラメーターと役割

- `disconnect_grace_period` : 切断検知後にCLIを終了するまでの猶予（既定10秒）
- `ping_interval` / `ping_timeout` : 既存WSの死活監視設定。Streaming化後は「ストリーム切断検知」に置換するため、不要になれば削除対象。

### ストリーム接続時の挙動

1. クライアントが `SubscribeAuthEvents` を開始 → `connecting`
2. サーバーは現在の状態を即時送信（pending/success/error）
3. クライアントが `pending` を受信 → `connected`
4. サーバーが `success` / `error` を送信 → クライアントは `success` / `error` へ遷移し、ストリーム終了

### 切断検知とgrace period

1. ストリームが切断されたらサーバーは `DisconnectedAt` を記録
2. `disconnect_grace_period` が経過しても再接続が無ければCLIを終了
3. 期間内に再接続が成功したら `DisconnectedAt` をクリアし、終了タイマーをリセット

### クライアント再接続のルール

```text
on stream end:
  if status is success/error -> stop
  else -> retry with backoff, up to N times
  if all retries fail -> set closed
```

推奨デフォルト例:

- 再接続間隔: 1s → 2s → 5s（固定3回）
- 再接続失敗時: `closed` 扱い

## ディレクトリ構成（確定）

```
backlog-cli/
├── proto/
│   └── auth/
│       └── v1/
│           └── auth.proto
├── gen/
│   ├── go/
│   │   └── auth/v1/
│   │       ├── auth.pb.go
│   │       └── authv1connect/
│   │           └── auth.connect.go
│   └── ts/
│       └── auth/v1/
│           ├── auth_pb.ts
│           └── auth_connect.ts
├── buf.yaml
├── buf.gen.yaml
├── internal/auth/
│   ├── callback.go
│   └── connect_handler.go
└── web/src/
    ├── gen/                 # gen/ts からコピー
    ├── lib/
    │   └── connect-client.ts
    └── context/
        ├── AuthContext.tsx
        └── StreamingContext.tsx
```

## Proto定義（確定）

```protobuf
syntax = "proto3";

package auth.v1;

option go_package = "github.com/yacchi/backlog-cli/gen/go/auth/v1;authv1";

service AuthService {
  rpc GetConfig(GetConfigRequest) returns (GetConfigResponse);
  rpc Configure(ConfigureRequest) returns (ConfigureResponse);
  rpc SubscribeAuthEvents(SubscribeAuthEventsRequest) returns (stream AuthEvent);
}

message GetConfigRequest {}

message GetConfigResponse {
  string space = 1;
  string domain = 2;
  string relay_server = 3;
  string space_host = 4;
  bool configured = 5;
}

message ConfigureRequest {
  string space_host = 1;
  string relay_server = 2;
}

message ConfigureResponse {
  bool success = 1;
  optional string error = 2;
}

message SubscribeAuthEventsRequest {}

message AuthEvent {
  AuthStatus status = 1;
  optional string error = 2;
}

enum AuthStatus {
  AUTH_STATUS_UNSPECIFIED = 0;
  AUTH_STATUS_PENDING = 1;
  AUTH_STATUS_SUCCESS = 2;
  AUTH_STATUS_ERROR = 3;
}
```

## Buf設定（確定）

### buf.yaml

```yaml
version: v2
modules:
  - path: proto
lint:
  use:
    - STANDARD
```

### buf.gen.yaml

```yaml
version: v2
plugins:
  - remote: buf.build/protocolbuffers/go
    out: gen/go
    opt: paths=source_relative
  - remote: buf.build/connectrpc/go
    out: gen/go
    opt: paths=source_relative
  - remote: buf.build/connectrpc/es
    out: gen/ts
    opt: target=ts
```

## 実装ステップ（完全版）

### Phase 1: 生成基盤の準備

1. `.mise.toml` に `buf` を追加
2. `buf.yaml` / `buf.gen.yaml` 追加
3. `proto/auth/v1/auth.proto` を作成（上記定義）
4. `buf generate` を実行して生成物を確認
5. Go依存追加: `connectrpc.com/connect`

### Phase 2: Go側（Connectサーバー）

1. `internal/auth/connect_handler.go` を追加
    - `GetConfig`: 既存 `handleConfig` 相当のロジックを移植
    - `Configure`: 既存 `handleConfigure` 相当のロジックを移植
    - `SubscribeAuthEvents`: 既存 `handleWebSocket` の状態通知をストリームで置換
2. `internal/auth/callback.go` の `setupRoutes` に Connect ハンドラーを登録
3. 旧 `handleConfig` / `handleConfigure` / `handleWebSocket` を削除
4. WebSocket関連の依存・設定を削除（`github.com/coder/websocket` 等）
5. ストリーム切断時は `handleWSDisconnect` 相当の処理を維持（grace period）

### Phase 3: SPA側（Connectクライアント）

1. 依存追加
    - `@connectrpc/connect`
    - `@connectrpc/connect-web`
    - `@bufbuild/protobuf`
2. `web/src/lib/connect-client.ts` を追加
    - baseURL: 同一オリジン前提
    - `createConnectTransport` を使用
3. `web/src/context/AuthContext.tsx` を Connect Unary 呼び出しに置換
4. `web/src/context/StreamingContext.tsx` を追加
    - `SubscribeAuthEvents` を購読
    - 再接続方針を実装（一定間隔/回数、失敗時はclosed扱い）
5. `LoginSetup.tsx` / `LoginConfirm.tsx` を StreamingContext ベースに更新

### Phase 4: ビルド統合

1. `Makefile` に `buf-generate` / `buf-lint` を追加
2. `build-web` を `buf-generate` 依存にする
3. `web/src/gen` へ `gen/ts` をコピーするステップを追加
4. `vite.config.ts` のプロキシを Connect パスに対応させる

### Phase 5: クリーンアップ

1. `web/src/context/WebSocketContext.tsx` を削除
2. WebSocket設定値（ping/grace等）を整理・削除
3. `docs/plans/11-spa-frontend.md` を更新（WS→Connect）

## Makefile追加（確定）

```makefile
.PHONY: buf-generate buf-lint

buf-generate:
	buf generate
	rm -rf web/src/gen
	cp -r gen/ts web/src/gen

buf-lint:
	buf lint

build-web: buf-generate
	cd web && pnpm install --frozen-lockfile && pnpm build
	rm -rf internal/ui/dist
	cp -r web/dist internal/ui/dist
```

## テスト・検証

- `buf lint` / `buf generate`
- `go test ./...`
- `cd web && pnpm lint`（存在すれば）
- 手動動作確認
    - `/auth/start` と `/auth/setup` の表示
    - 設定保存 → 認証開始
    - Server Streaming による status 更新
    - ブラウザ/ポップアップ閉じた際の扱い

## リスク / 注意点

- connect-web は自動再接続を提供しないため、再接続/再購読方針が必要。
- Stream切断時の扱いを、既存の「grace period」ロジックと整合させる必要がある。
- ルーティング（Connectのパス）と Vite プロキシの不一致に注意。

## 未決事項

- ストリーム再接続の間隔・回数・失敗時のUI遷移
- 切断時に「closed」扱いとするタイミング
