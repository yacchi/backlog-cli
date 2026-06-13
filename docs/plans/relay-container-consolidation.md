# Relay/MCP コンテナ集約 + CDK construct ライブラリ配布 計画

## 背景と目的

現状、Relay サーバーと MCP サーバーには複数の起動経路・配布形態が混在している。

- `packages/relay-docker/src/index.ts` … `RELAY_CONFIG` を読み Relay のみ起動（MCP なし）
- `packages/mcp-server/src/serve.ts` … `MCP_CONFIG` を読み MCP のみ起動（Relay なし）
- `packages/relay-aws/lib/handler.ts` … Lambda 上で Relay + MCP を統合（唯一の統合実装）
- Lambda は `NodejsFunction` + esbuild バンドル + `afterBundling` で Go CLI / Deno worker をその場でコンパイル

この構成の課題:

1. **統合実装が Lambda ハンドラにしか無い** — Relay と MCP を 1 プロセスで動かすロジック（`/auth/callback` 共有ディスパッチ等）が `handler.ts` に閉じている。
2. **Lambda ビルドが脆い** — `afterBundling` がビルドホストに Go / Deno / make を要求し、synth が重い。
3. **config.ts のクローン前提** — 組織は `packages/relay-aws/config.ts`（平文シークレット含む）をリポジトリ内で管理する必要があり、組織内運用に難がある。

### ゴール

- **単一コンテナイメージ**で Relay + MCP を提供し、`docker run`（常駐 HTTP）でも Lambda（コンテナイメージ）でも動く。
- **AWS 固有処理（SSM / Secrets Manager / JWKS ローテーション）をイメージ内に取り込む**（環境適応ローダ）。
- **クローン前提を廃止**。AWS インフラ定義は `@backlog-cli/relay-aws-cdk` を npm publish した CDK construct ライブラリとして配布。組織は数行の `app.ts` で `import` するだけにする。

## 決定事項（確定済み）

| 論点 | 決定 |
|------|------|
| イメージ形態 | 単一イメージ。環境適応で config ソースを切替（env / AWS） |
| Lambda 形態 | `NodejsFunction` を廃し `lambda.DockerImageFunction`（上記イメージ参照） |
| AWS 固有処理 | イメージ内の起動コードに SSM / Secrets Manager 読み込みを取り込む（AWS SDK は dynamic import） |
| インフラ配布 | `@backlog-cli/relay-aws-cdk` を npm publish（CDK construct library） |
| config.ts | 廃止。非機密設定は安全にコミット可能な形へ、シークレットは Secrets Manager / パラメータ注入 |

## 目標アーキテクチャ

```
┌──────────────────────────────────────────────────────────┐
│  単一イメージ ghcr.io/<org>/backlog-relay:vX              │
│                                                            │
│  unified entrypoint (Hono node-server, 常駐 HTTP)          │
│    1. ConfigSource で設定取得（環境適応）                   │
│         env  : RELAY_CONFIG (JSON, secrets inline)         │
│         aws  : SSM Parameter + Secrets Manager             │
│    2. createRelayApp()                (relay-core)         │
│    3. createMcpApp()  ※mcp_spaces があれば (mcp-server)    │
│    4. 共有 /auth/callback ディスパッチ（MCP JWT or Relay） │
│                                                            │
│  同梱: Go backlog バイナリ / Deno / Pyodide cache          │
│  同梱: aws-lambda-web-adapter（Lambda 実行時のみ有効）     │
└──────────────────────────────────────────────────────────┘
        │ docker run                       │ DockerImageFunction
        ▼                                  ▼
   任意基盤（env config）           AWS（CloudFront + SSM + Secrets + Rotation）
                                     ↑ @backlog-cli/relay-aws-cdk が構築
```

## 設計詳細

### 1. 統合エントリポイント（新規 `serve.ts`）

`relay-docker/src/index.ts` と `mcp-server/src/serve.ts` を置き換える単一の起動コードを用意する。
`relay-aws/lib/handler.ts` の mount ロジックを **プラットフォーム非依存**な形に抽出して再利用する。

責務:

1. `ConfigSource`（後述）から raw config を取得し、`relay-core` の `parseConfig` で `RelayConfig` を生成。
2. `createRelayApp({...})` を mount。
3. raw config に `mcp_spaces` があれば `buildMcpConfig` 相当で `McpServerConfig` を生成し、`createMcpApp({...})` を mount。
   - `tokenExchange` は `handler.ts` の `createDirectTokenExchange`（client_secret を使うインプロセス交換）を共通化して使う。
   - `runScript` は `spaces.length > 0` のとき `createSandboxClient` で初期化。
4. 共有 `/auth/callback`: state を MCP 署名鍵で `verify` できれば MCP、失敗すれば Relay にフォールバック（`handler.ts:364-379` と同じ判定）。
5. `@hono/node-server` の `serve()` で listen。Lambda 実行時は lambda-web-adapter が HTTP に橋渡しするため、`hono/aws-lambda` の `handle()` は不要。

抽出対象（`handler.ts` から共通ロジックへ）:

- `buildMcpConfig`（SSM 依存部分を ConfigSource に逃がす）
- `createDirectTokenExchange`
- `getSandbox` 相当（sandbox の遅延初期化 + キャッシュ）
- 共有 `/auth/callback` ディスパッチ

### 2. ConfigSource 抽象（環境適応ローダ）

`handler.ts` の `loadRawConfig` / `loadRelaySecrets` / `getRelayConfig` / `buildMcpConfig` に埋まっている SSM/Secrets Manager 読み込みを、プラットフォーム非依存のインターフェースに切り出す。

```ts
interface ConfigSource {
  // secrets をマージ済みの raw config を返す
  loadRawConfig(): Promise<Record<string, unknown>>;
}
```

- **EnvConfigSource** … `RELAY_CONFIG`(JSON) をそのまま返す。secrets はインライン前提。Docker / ローカル用。
- **AwsConfigSource** … `CONFIG_PARAMETER_NAME`(SSM) + `RELAY_SECRETS_NAME`(Secrets Manager) を読み、`handler.ts:125-162` の secret マージ（client_secret / server.jwks / tenants[].passphrase_hash）を実施して返す。

選択ロジック: `RELAY_CONFIG` があれば env、`CONFIG_PARAMETER_NAME` があれば aws。

#### AWS SDK の import 方針

`handler.ts` 現状は AWS SDK を dynamic import し、`relay-aws` も AWS SDK を optional peerDependency にしている。これは zip Lambda で「必要な時だけ入れる」ための措置だった。

**統合イメージでは AWS SDK を通常の static import にする。** 理由:

- コンテナイメージは依存を常に同梱するため、dynamic import の利得は env モード起動時に AWS SDK を読まずに済む数十ms 程度に留まり、コードの複雑さに見合わない。
- AWS SDK を統合イメージ（およびその起動コードを持つパッケージ）の通常依存に格上げする。`relay-aws`(CDK 側) の optional peerDependency 設定はランタイムとは別レイヤなので影響しない。

### 3. Dockerfile 統合

`packages/mcp-server/Dockerfile` が既に Go + Node + Deno + Pyodide cache + lambda-web-adapter を持つマルチステージ構成なので、これを正式な統合イメージ Dockerfile に昇格させる。

- ビルド対象を統合エントリポイント（新 `serve.ts`）に変更。
- web アセット（ポータル）を同梱（relay-docker の `loadPortalAssets` 相当を統合エントリで読む）。
- `relay-docker/Dockerfile`（distroless / Relay のみ）は撤廃 or この統合イメージへ集約。
- 環境変数: `BACKLOG_BIN_PATH` / `DENO_PATH` / `DENO_DIR` / `SANDBOX_WORKER_PATH` / `WEB_DIST_PATH` を既定値付きで設定。

> 注: distroless は Deno サブプロセス起動や lambda-adapter との相性で制約があるため、mcp-server 側の `node:slim` ベースに寄せる。

### 4. Lambda を DockerImageFunction 化（CDK 改修）

`packages/relay-aws/lib/relay-stack.ts`:

- `NodejsFunction` + `bundling.commandHooks.afterBundling`（Go/Deno コンパイル）を廃止（`relay-stack.ts:267-323`）。
- `lambda.DockerImageFunction` に置換。イメージは published タグ参照、もしくは `DockerImageCode.fromImageAsset` でローカルビルド。
- `handler.ts` は撤廃（統合エントリへ移行）。Lambda は lambda-web-adapter 経由で HTTP サーバーを起動。
- `createFunctionUrl` / CloudFront / SSM / Secrets Manager / `rotation-handler.ts` はそのまま再利用（資産価値が高い）。
- 環境変数として `CONFIG_PARAMETER_NAME` / `RELAY_SECRETS_NAME` を渡し、コンテナ側 AwsConfigSource が読む。

### 5. CDK construct ライブラリ化（配布の核心）

`@backlog-cli/relay-aws` を **publish 可能な construct ライブラリ**へ再編する（名称は `@backlog-cli/relay-aws-cdk` を想定）。

- `private: true` を外し、`publishConfig` / `files` / `exports` を整備。`RelayStack`・`types.ts`（`RelayConfig` builder 型）をエクスポート。
- `bin/app.ts` / `config.ts` / `config.example.ts` はリポジトリ内のローカルデプロイ用サンプルに格下げ（ライブラリ本体からは分離）。
- 組織側の利用イメージ:

```ts
// org-infra/bin/app.ts（組織リポジトリ、数行）
import * as cdk from "aws-cdk-lib";
import { RelayStack } from "@backlog-cli/relay-aws-cdk";

const app = new cdk.App();
new RelayStack(app, "BacklogRelay", {
  config: {
    parameterName: "/backlog-relay/config",
    parameterValue: {
      backlog_app: {
        client_id: process.env.BACKLOG_CLIENT_ID!,
        client_secret: process.env.BACKLOG_CLIENT_SECRET!, // 環境/SM 由来。コードに平文を置かない
      },
      tenants: { "myspace.backlog.jp": { default_space: "myspace.backlog.jp" } },
    },
    mcp: { spaces: [{ pattern: "myspace\\.backlog\\.jp", writable: true }] },
    cloudFront: { enabled: true, customDomain: { /* ... */ } },
  },
  // image は省略可。省略時は construct 既定の公式 GHCR タグを組織 ECR へ自動コピーする（§9）
});
```

- これで **クローン不要**。組織は npm 依存 + 数行の構成 + シークレット注入だけで運用でき、config.ts のリポジトリ内管理は不要になる。
- construct はイメージ取得（GHCR→private ECR コピー）まで内部で面倒を見る（§9）。組織は `cdk deploy` だけでよい。

### 6. config.ts 問題の解消

- **非機密設定**: 組織リポジトリに安全にコミット可（client_id / tenants / mcp_spaces / cloudFront 等）。
- **シークレット**: `client_secret` 等は環境変数 / Secrets Manager 参照で注入。CDK synth 時に平文をコードへ書かない。
- 既存の「CDK synth で passphrase を bcrypt 化 → Secrets Manager 保存」「JWKS 自動生成 + ローテーション」は維持。

### 7. publish / CI

- 既存の `.github/workflows/release.yml`（GoReleaser + Homebrew）に並行して:
  - 統合 Docker イメージのビルド & レジストリ（GHCR）push を追加。タグは `version.txt` 連動。
  - `@backlog-cli/relay-core` / `@backlog-cli/mcp-server` / `@backlog-cli/relay-aws-cdk` の npm publish を追加（construct ライブラリは relay-core/mcp-server に依存するため、これらも publish 対象にする必要がある）。
- npm scope `@backlog-cli` の publish 設定（registry / access）を決める。

### 8. ローカル起動・試験

統合イメージ化の副次効果として、Relay と MCP を **AWS なしでローカル一括検証**できるようになる。README に手順を提示する。

ローカル起動（env config モード）:

```bash
# 統合イメージをビルド
docker build -t backlog-relay -f packages/mcp-server/Dockerfile .

# RELAY_CONFIG を渡して起動（secrets インライン）
docker run --rm -p 8080:8080 \
  -e RELAY_CONFIG='{"server":{"port":8080,"base_url":"http://localhost:8080"},
    "backlog_app":{"client_id":"...","client_secret":"..."},
    "jwks":"<署名鍵JSON>",
    "tenants":[{"name":"myspace.backlog.jp","default_space":"myspace.backlog.jp","passphrase_hash":"$2a$..."}],
    "mcp_spaces":[{"pattern":"myspace\\.backlog\\.jp","writable":true}]}' \
  backlog-relay
```

イメージを使わない直接起動（開発ループ用、ローカルに Go/Deno が要る）:

```bash
RELAY_CONFIG='{...}' BACKLOG_BIN_PATH=./bin/backlog \
  node packages/mcp-server/dist/serve.js   # 統合エントリ
```

検証観点:

- Relay: `GET /v1/relay/tenants/:domain/info` / `bundle`、ポータル UI、`/auth/start`→`/auth/callback` フロー。
- MCP: MCP Inspector で `POST/GET/DELETE /mcp`（Streamable HTTP）、OAuth AS（DCR / authorize / token）、`backlog` ツール、`run_script`(sandbox)。
- 共有 `/auth/callback` が MCP state / Relay state を正しく振り分けるか。

ローカル検証では実シークレットを使わずに済むよう、ダミー JWKS と passphrase_hash を生成する補助スクリプト（or `make` ターゲット）の提供も検討する。

> README 反映タスク: ルート README もしくは `packages/<統合>/README.md` に「ローカル起動・試験」節を追加する。

### 9. コンテナイメージ配布と Lambda への持ち込み

#### パブリック配布（セルフホスト / docker run 用）

- 正本レジストリは **GHCR**（`ghcr.io/yacchi/backlog-relay`）。GitHub Actions からネイティブに push でき、public は無料、Homebrew tap も既に GitHub(yacchi) にある。
- タグは `version.txt` 連動（`vX.Y.Z` + `latest`）。
- セルフホスト/オンプレ利用者はこの公開イメージをそのまま `docker run`（env config モード）で使える。

#### Lambda への持ち込み（重要な制約）

Lambda の `DockerImageFunction` には次の制約がある（2026年6月時点で確認済み）:

1. **pull-through cache 非対応** — PTC はリポジトリを初回 pull 時に遅延生成する仕組みで、Lambda の「関数作成時点で ECR に直接 push 済みであること」要件と噛み合わない。ECR-to-ECR PTC（2025-03）も PTC リポジトリなので Lambda からは使えない。
2. **同一リージョン必須** — クロスリージョンの ECR は不可。
3. **クロスアカウントは可** — リソースポリシーを付ければ別アカウントの同一リージョン ECR は使える。

→ 結論: イメージは **組織の同一リージョン private ECR に直接 push 済み**である必要がある。

#### 採用方式: construct による自動コピー

CDK construct（`@backlog-cli/relay-aws-cdk`）が以下を内部で行う:

1. private ECR リポジトリをデプロイ先リージョンに作成。
2. **custom resource（`cdk-ecr-deployment` 相当）で、公式 GHCR イメージを作成した private ECR へ直接コピー（push）**。
3. `DockerImageFunction` をその private ECR イメージに向ける。

これにより組織は `cdk deploy` だけで完結し、ミラー運用やレジストリ知識が不要になる（no-clone ゴールに最適）。

検討事項:

- `cdk-ecr-deployment` はサードパーティ construct + コピー用 Lambda(custom resource) を伴う。依存とデプロイ複雑度の増分を許容する。
- コピー元 GHCR は public のため認証不要で pull できる想定。custom resource からの GHCR pull が不安定な場合は、**ECR Public(`public.ecr.aws`) を代替コピー元**として併用（GitHub→ECR Public の push には AWS 認証が要る点に注意）。
- イメージタグの固定（digest 参照）でデプロイ再現性を担保。construct はライブラリ版に対応する既定タグを持ち、`image` 引数で上書き可能にする。

## 移行フェーズ

| Phase | 内容 | 成果物 |
|-------|------|--------|
| 0 | 共通 mount ロジック抽出（`buildMcpConfig` / `createDirectTokenExchange` / callback dispatch をプラットフォーム非依存化） | 共有モジュール |
| 1 | ConfigSource 抽象（Env / Aws）実装 + 統合エントリポイント `serve.ts` | 単一プロセスで Relay+MCP 起動可 |
| 2 | Dockerfile 統合・イメージビルド検証（docker run で Relay+MCP+run_script 動作確認） | 統合イメージ |
| 3 | CDK を DockerImageFunction 化、`handler.ts` 撤廃、既存 CloudFront/Secrets/Rotation 維持 | コンテナ Lambda |
| 4 | `@backlog-cli/relay-aws-cdk` の publish 化（private 解除、exports 整備、依存 publish） | npm construct library |
| 5 | CI に Docker push + npm publish 追加、ドキュメント更新（`docs/design/` へ反映） | リリースパイプライン |

## リスク・要検討

- **イメージサイズ / コールドスタート**: Go + Deno + Pyodide cache + Node を 1 イメージに同梱するため肥大化する。Lambda コールドスタートへの影響を計測。MCP を使わない構成では sandbox 同梱を省くオプションも検討。
- **lambda-web-adapter のタイムアウト/ストリーミング**: 既存 MCP の Streamable HTTP（GET/POST/DELETE `/mcp`）が adapter 経由で正しく動くか E2E 検証。
- **DockerImageFunction とイメージ参照**: §9 のとおり construct 自動コピー（GHCR→private ECR）で確定。`cdk-ecr-deployment` の custom resource 信頼性、GHCR からの認証なし pull、ECR Public フォールバックの要否を検証する。
- **後方互換**: 既存デプロイ（NodejsFunction + config.ts）からの移行手順。SSM/Secrets のスキーマは不変なので、Lambda 差し替えとイメージ参照追加で移行できる想定。
- **relay-cloudflare**: 同じ統合エントリ思想を Cloudflare Workers に広げるかは本計画スコープ外（別途）。

## 関連ファイル

- `packages/relay-aws/lib/handler.ts` — 抽出元の統合ロジック（mount / token exchange / callback dispatch）
- `packages/relay-aws/lib/relay-stack.ts` — Lambda 定義（`NodejsFunction`→`DockerImageFunction`）、CloudFront/SSM/Secrets
- `packages/relay-aws/lib/rotation-handler.ts` — JWKS/passphrase ローテーション（維持）
- `packages/relay-aws/lib/types.ts` — CDK 設定型（construct ライブラリの公開 API 候補）
- `packages/mcp-server/Dockerfile` — 統合イメージの母体
- `packages/mcp-server/src/serve.ts` / `packages/relay-docker/src/index.ts` — 統合エントリへ置換
- `packages/relay-core/src/config/schema.ts` / `packages/mcp-server/src/config/schema.ts` — 設定スキーマ（publish 対象）
</content>
</invoke>
