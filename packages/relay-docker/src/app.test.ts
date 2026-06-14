import { describe, it, expect, beforeAll, vi } from "vitest";
import { generateKeyPair, exportJWK } from "jose";
import {
  createUnifiedApp,
  buildMcpConfig,
  createDirectTokenExchange,
  restoreMcpAuthorization,
} from "./app.js";
import type { RelayConfig } from "@yacchi/backlog-relay-core";

let jwksJson: string;

beforeAll(async () => {
  const { privateKey } = await generateKeyPair("EdDSA", {
    crv: "Ed25519",
    extractable: true,
  });
  const privJwk = await exportJWK(privateKey);
  jwksJson = JSON.stringify({
    keys: [{ ...privJwk, kid: "test-key-1", kty: "OKP", crv: "Ed25519" }],
  });
});

function relayOnlyRawConfig(): Record<string, unknown> {
  return {
    server: { port: 8080, base_url: "https://relay.example.com" },
    backlog_app: { client_id: "cid", client_secret: "secret" },
    jwks: jwksJson,
    tenants: [{ name: "myspace.backlog.jp", default_space: "myspace.backlog.jp" }],
  };
}

function mcpEnabledRawConfig(): Record<string, unknown> {
  return {
    ...relayOnlyRawConfig(),
    mcp_spaces: [{ pattern: "myspace\\.backlog\\.jp", writable: true }],
    mcp_default_spaces: ["myspace.backlog.jp"],
  };
}

describe("buildMcpConfig", () => {
  it("returns null when mcp_spaces is absent", () => {
    const relayConfig = {
      server: { base_url: "https://relay.example.com", port: 8080 },
      backlog_app: { client_id: "cid", client_secret: "s" },
      jwks: jwksJson,
    } as unknown as RelayConfig;
    expect(buildMcpConfig({}, relayConfig)).toBeNull();
  });

  it("builds MCP config from mcp_spaces and relay jwks/client_id", () => {
    const relayConfig = {
      server: { base_url: "https://relay.example.com", port: 8080 },
      backlog_app: { client_id: "cid", client_secret: "s" },
      jwks: jwksJson,
    } as unknown as RelayConfig;
    const raw = {
      mcp_spaces: [{ pattern: "myspace\\.backlog\\.jp", writable: true }],
      mcp_default_spaces: ["myspace.backlog.jp"],
    };
    const mcp = buildMcpConfig(raw, relayConfig);
    expect(mcp).not.toBeNull();
    expect(mcp!.base_url).toBe("https://relay.example.com");
    expect(mcp!.backlog_app.client_id).toBe("cid");
    expect(mcp!.spaces).toHaveLength(1);
    expect(mcp!.default_spaces).toEqual(["myspace.backlog.jp"]);
  });

  it("uses baseUrlFallback when server.base_url is missing", () => {
    const relayConfig = {
      server: { port: 8080 },
      backlog_app: { client_id: "cid", client_secret: "s" },
      jwks: jwksJson,
    } as unknown as RelayConfig;
    const mcp = buildMcpConfig(
      { mcp_spaces: [{ pattern: ".*", writable: false }] },
      relayConfig,
      "https://from-request.example.com",
    );
    expect(mcp!.base_url).toBe("https://from-request.example.com");
  });

  it("builds MCP config without base_url (issuer derived at runtime)", () => {
    const relayConfig = {
      server: { port: 8080 },
      backlog_app: { client_id: "cid", client_secret: "s" },
      jwks: jwksJson,
    } as unknown as RelayConfig;
    const mcp = buildMcpConfig(
      { mcp_spaces: [{ pattern: ".*", writable: false }] },
      relayConfig,
    );
    expect(mcp).not.toBeNull();
    expect(mcp!.base_url).toBeUndefined();
    expect(mcp!.spaces).toHaveLength(1);
  });

  it("returns null when no jwks is available", () => {
    const relayConfig = {
      server: { base_url: "https://relay.example.com", port: 8080 },
      backlog_app: { client_id: "cid", client_secret: "s" },
    } as unknown as RelayConfig;
    expect(
      buildMcpConfig(
        { mcp_spaces: [{ pattern: ".*", writable: false }] },
        relayConfig,
      ),
    ).toBeNull();
  });
});

describe("restoreMcpAuthorization", () => {
  it("copies x-mcp-authorization into authorization when absent", () => {
    const req = new Request("https://x/mcp", {
      headers: { "x-mcp-authorization": "Bearer tok" },
    });
    const out = restoreMcpAuthorization(req);
    expect(out.headers.get("authorization")).toBe("Bearer tok");
  });

  it("overrides a non-Bearer authorization (SigV4 from OAC)", () => {
    const req = new Request("https://x/mcp", {
      headers: {
        authorization: "AWS4-HMAC-SHA256 Credential=...",
        "x-mcp-authorization": "Bearer tok",
      },
    });
    const out = restoreMcpAuthorization(req);
    expect(out.headers.get("authorization")).toBe("Bearer tok");
  });

  it("keeps an existing Bearer authorization", () => {
    const req = new Request("https://x/mcp", {
      headers: {
        authorization: "Bearer real",
        "x-mcp-authorization": "Bearer other",
      },
    });
    const out = restoreMcpAuthorization(req);
    expect(out.headers.get("authorization")).toBe("Bearer real");
  });

  it("is a no-op without x-mcp-authorization", () => {
    const req = new Request("https://x/mcp", {
      headers: { authorization: "Bearer real" },
    });
    const out = restoreMcpAuthorization(req);
    expect(out).toBe(req);
    expect(out.headers.get("authorization")).toBe("Bearer real");
  });
});

describe("createDirectTokenExchange", () => {
  it("builds an exchange that posts to the space token endpoint", async () => {
    const calls: Array<{ url: string; body: string }> = [];
    const originalFetch = globalThis.fetch;
    globalThis.fetch = (async (url: string, init?: RequestInit) => {
      calls.push({ url: String(url), body: String(init?.body) });
      return new Response(
        JSON.stringify({
          access_token: "at",
          token_type: "Bearer",
          expires_in: 3600,
          refresh_token: "rt",
        }),
        { status: 200 },
      );
    }) as typeof fetch;

    try {
      const ex = createDirectTokenExchange({
        backlog_app: { client_id: "cid", client_secret: "sec" },
      } as unknown as RelayConfig);
      const result = await ex.exchangeCode(
        "myspace.backlog.com",
        "the-code",
        "https://relay.example.com/auth/callback",
      );
      expect(result.access_token).toBe("at");
      expect(calls[0].url).toBe(
        "https://myspace.backlog.com/api/v2/oauth2/token",
      );
      expect(calls[0].body).toContain("grant_type=authorization_code");
      expect(calls[0].body).toContain("code=the-code");
      expect(calls[0].body).toContain("client_secret=sec");
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});

describe("createUnifiedApp (relay only)", () => {
  it("serves relay endpoints and does NOT mount MCP", async () => {
    const app = await createUnifiedApp({
      rawConfig: relayOnlyRawConfig(),
    });

    const health = await app.request("/health");
    expect(health.status).toBe(200);

    // Relay-specific discovery present.
    const relayWk = await app.request("/.well-known/backlog-oauth-relay");
    expect(relayWk.status).toBe(200);

    // MCP-specific discovery absent (not mounted).
    const mcpWk = await app.request("/.well-known/oauth-authorization-server");
    expect(mcpWk.status).toBe(404);
  });
});

describe("requestId propagation into audit logs", () => {
  it("injects requestId from Logger bindings into audit events", async () => {
    const lines: string[] = [];
    const writeSpy = vi.spyOn(process.stdout, "write").mockImplementation((chunk) => {
      lines.push(String(chunk));
      return true;
    });
    vi.spyOn(process.stderr, "write").mockImplementation((chunk) => {
      lines.push(String(chunk));
      return true;
    });

    try {
      const app = await createUnifiedApp({
        rawConfig: relayOnlyRawConfig(),
      });

      // /auth/callback without code/state triggers an audit error event
      const res = await app.request("/auth/callback");
      expect(res.status).toBe(400);

      const auditLines = lines
        .map((l) => { try { return JSON.parse(l.trim()); } catch { return null; } })
        .filter((e) => e?.component === "audit");

      expect(auditLines.length).toBeGreaterThan(0);

      const event = auditLines[0];
      expect(event.requestId).toBeDefined();
      expect(typeof event.requestId).toBe("string");
      expect(event.requestId.length).toBeGreaterThan(0);
      expect(event.level).toBeDefined();
      expect(event.ts).toBeDefined();
    } finally {
      writeSpy.mockRestore();
    }
  });
});

describe("createUnifiedApp (relay + MCP)", () => {
  it("mounts both relay and MCP endpoints", async () => {
    const app = await createUnifiedApp({
      rawConfig: mcpEnabledRawConfig(),
    });

    const relayWk = await app.request("/.well-known/backlog-oauth-relay");
    expect(relayWk.status).toBe(200);

    const mcpWk = await app.request("/.well-known/oauth-authorization-server");
    expect(mcpWk.status).toBe(200);
  });

  it("dispatches /auth/callback to relay when state is not an MCP JWT", async () => {
    const app = await createUnifiedApp({
      rawConfig: mcpEnabledRawConfig(),
    });

    // A non-MCP state must fall through to the relay callback handler
    // (i.e. it must NOT be handled as an MCP authorize callback / 404).
    const res = await app.request(
      "/auth/callback?code=abc&state=not-an-mcp-jwt",
    );
    expect(res.status).not.toBe(404);
  });
});
