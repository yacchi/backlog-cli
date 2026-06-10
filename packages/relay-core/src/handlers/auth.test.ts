import { describe, it, expect } from "vitest";
import { createAuthHandlers } from "./auth.js";
import { NoopAuditLogger } from "../middleware/audit.js";
import type { RelayConfig } from "../config/types.js";

function makeConfig(): RelayConfig {
  return {
    backlog_app: { client_id: "test-client", client_secret: "test-secret" },
    server: { port: 8787 },
  } as unknown as RelayConfig;
}

describe("GET /auth/callback error page", () => {
  it("escapes attacker-controlled error_description (no reflected XSS)", async () => {
    const app = createAuthHandlers(makeConfig(), new NoopAuditLogger());

    const payload = "<script>alert(document.domain)</script>";
    const res = await app.request(
      `/auth/callback?error=access_denied&error_description=${encodeURIComponent(payload)}`,
    );

    expect(res.status).toBe(400);
    const html = await res.text();
    // The raw script tag must NOT appear; it must be HTML-escaped.
    expect(html).not.toContain("<script>alert(document.domain)</script>");
    expect(html).toContain("&lt;script&gt;alert(document.domain)&lt;/script&gt;");
  });
});
