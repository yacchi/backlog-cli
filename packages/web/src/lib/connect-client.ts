import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { AuthService } from "../gen/auth/v1/auth_pb";

// 同一オリジン前提のトランスポート
// credentials: "include" でCookieを確実に送信
const transport = createConnectTransport({
  baseUrl: window.location.origin,
  fetch: (input, init) =>
    fetch(input, {
      ...init,
      credentials: "include",
    }),
});

// AuthService クライアント
export const authClient = createClient(AuthService, transport);
