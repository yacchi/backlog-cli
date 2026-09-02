# Backlog API 認証

Backlog CLI は OAuth アクセストークン認証と API キー認証の 2 つの経路で Backlog API に接続します。
認証情報は設定された credentials backend に保存されます。

## OAuth アクセストークン

OAuth ログインでは、CLI がローカルサーバーを起動してブラウザの認可フローを開始します。CLI は
Client Secret を保持せず、Backlog OAuth Relay に認可コードの交換を依頼します。Relay から返された
OAuth アクセストークンを credentials layer に保存し、API 呼び出しではアクセストークンを
`Authorization: Bearer <access-token>` ヘッダーとして送信します。アクセストークンの更新も Relay
経由で行います。詳細なブラウザと Relay の流れは `docs/design/auth-flow.md` と
`docs/design/oauth-relay-server.md` を参照してください。

## API キー

API キーでログインした場合、CLI は API リクエストごとに次のリクエストヘッダーを送信します。

```http
Backlog-API-Key: <api-key>
```

API キーを URL のクエリパラメーターに含めないのは、キーが URL、プロキシや CDN のアクセスログ、
シェル履歴、エラーメッセージに現れる範囲を減らすためです。Backlog API サーバー側では従来の
`?apiKey=` クエリ形式も引き続き動作しますが、CLI はその形式を生成しません。

API キーの有効性は認証時に Backlog API への接続で確認され、成功したキーは credentials layer
に保存されます。秘密値の実際の保存先は `auth.credential_backend` の設定に従います。
