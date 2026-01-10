# 設計書（現状実装の説明）

`docs/design/` は「現状の実装」を説明する設計書の置き場です（参照用のソースオブトゥルース）。

- 実装が変わったら、対応する設計書も必ず更新します。
- 「これから実装する」ための作業計画は `docs/plans/` に置き、実装完了後は削除します（必要な内容は本ディレクトリに移して残します）。

## ドキュメント一覧

- `cli.md`: CLIのコマンド体系と主要パッケージ
- `oauth-relay-server.md`: OAuth中継サーバー（Backlog OAuth Relay）の仕様/フロー
- `auth-flow.md`: CLIローカルサーバー + SPA（Connect RPC）による認証フロー
- `config.md`: 設定レイヤー、プロファイル、資格情報、信頼バンドルの扱い
- `relay-config-bundle.md`: Relay Config Bundle 仕様（信頼の起点）
- `backlog-gfm-conversion.md`: Backlog記法→GFM変換仕様
