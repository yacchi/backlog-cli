import { describe, it, expect, vi } from "vitest";
import {
    isClientIdUrl,
    assertFetchableClientId,
    createClientMetadataResolver,
    ClientMetadataError,
} from "./cimd.js";

const CLIENT_ID = "https://app.example.com/oauth/client.json";

function jsonResponse(body: unknown, init?: { headers?: Record<string, string>; status?: number }): Response {
    return new Response(typeof body === "string" ? body : JSON.stringify(body), {
        status: init?.status ?? 200,
        headers: { "Content-Type": "application/json", ...init?.headers },
    });
}

function validDocument(overrides?: Record<string, unknown>) {
    return {
        client_id: CLIENT_ID,
        client_name: "Example MCP Client",
        redirect_uris: ["http://127.0.0.1:3000/callback", "https://app.example.com/cb"],
        ...overrides,
    };
}

describe("isClientIdUrl", () => {
    it("distinguishes URL client_ids from opaque ones", () => {
        expect(isClientIdUrl(CLIENT_ID)).toBe(true);
        expect(isClientIdUrl("eyJhbGciOiJFZERTQSJ9.e30.sig")).toBe(false);
    });
});

describe("assertFetchableClientId", () => {
    it("accepts an https URL with a path", () => {
        expect(assertFetchableClientId(CLIENT_ID).hostname).toBe("app.example.com");
    });

    it.each([
        ["http://app.example.com/client.json", "https"],
        ["https://app.example.com", "path"],
        ["https://app.example.com/", "path"],
        ["https://app.example.com/client.json#frag", "fragment"],
        ["https://user:pw@app.example.com/client.json", "credentials"],
        ["https://127.0.0.1/client.json", "IP address"],
        ["https://[::1]/client.json", "IP address"],
        ["https://localhost/client.json", "resolvable"],
        ["https://internal-box/client.json", "public domain"],
        ["https://box.internal/client.json", "resolvable"],
        ["not-a-url", "valid URL"],
    ])("rejects %s", (clientId, reason) => {
        expect(() => assertFetchableClientId(clientId)).toThrow(ClientMetadataError);
        expect(() => assertFetchableClientId(clientId)).toThrow(new RegExp(reason));
    });

    it("enforces the host allow-list when configured", () => {
        expect(() => assertFetchableClientId(CLIENT_ID, ["claude\\.ai"])).toThrow(/not allowed/);
        expect(assertFetchableClientId(CLIENT_ID, ["claude\\.ai", ".*\\.example\\.com"])).toBeInstanceOf(URL);
    });
});

describe("createClientMetadataResolver", () => {
    it("fetches and validates a metadata document", async () => {
        const fetchImpl = vi.fn().mockResolvedValue(jsonResponse(validDocument()));
        const resolver = createClientMetadataResolver({ fetchImpl: fetchImpl as unknown as typeof fetch });

        const metadata = await resolver.resolve(CLIENT_ID);
        expect(metadata.client_name).toBe("Example MCP Client");
        expect(metadata.redirect_uris).toContain("http://127.0.0.1:3000/callback");

        const [, init] = fetchImpl.mock.calls[0];
        expect(init.redirect).toBe("error");
    });

    it("rejects a document whose client_id does not match the URL", async () => {
        const fetchImpl = vi.fn().mockResolvedValue(
            jsonResponse(validDocument({ client_id: "https://evil.example.com/client.json" })),
        );
        const resolver = createClientMetadataResolver({ fetchImpl: fetchImpl as unknown as typeof fetch });
        await expect(resolver.resolve(CLIENT_ID)).rejects.toThrow(/does not match its URL/);
    });

    it.each([
        ["missing client_name", { ...validDocument(), client_name: undefined }, /client_name/],
        ["empty redirect_uris", validDocument({ redirect_uris: [] }), /redirect_uris/],
        ["malformed redirect_uri", validDocument({ redirect_uris: ["not a uri"] }), /invalid redirect_uri/],
        ["non-string redirect_uri", validDocument({ redirect_uris: [42] }), /strings only/],
        ["a JSON array", [validDocument()], /not a JSON object/],
    ])("rejects a document with %s", async (_label, doc, expected) => {
        const fetchImpl = vi.fn().mockResolvedValue(jsonResponse(doc));
        const resolver = createClientMetadataResolver({ fetchImpl: fetchImpl as unknown as typeof fetch });
        await expect(resolver.resolve(CLIENT_ID)).rejects.toThrow(expected);
    });

    it("rejects non-JSON content types and non-2xx responses", async () => {
        const html = new Response("<html></html>", { headers: { "Content-Type": "text/html" } });
        const resolverHtml = createClientMetadataResolver({
            fetchImpl: (async () => html) as unknown as typeof fetch,
        });
        await expect(resolverHtml.resolve(CLIENT_ID)).rejects.toThrow(/content-type/);

        const resolver404 = createClientMetadataResolver({
            fetchImpl: (async () => jsonResponse(validDocument(), { status: 404 })) as unknown as typeof fetch,
        });
        await expect(resolver404.resolve(CLIENT_ID)).rejects.toThrow(/HTTP 404/);
    });

    it("rejects an oversized document", async () => {
        const padded = JSON.stringify({ ...validDocument(), pad: "x".repeat(40 * 1024) });
        const resolver = createClientMetadataResolver({
            fetchImpl: (async () => jsonResponse(padded)) as unknown as typeof fetch,
        });
        await expect(resolver.resolve(CLIENT_ID)).rejects.toThrow(/too large/);
    });

    it("caches within the Cache-Control lifetime and refetches after it", async () => {
        let clock = 0;
        const fetchImpl = vi.fn().mockImplementation(async () =>
            jsonResponse(validDocument(), { headers: { "Cache-Control": "public, max-age=600" } }),
        );
        const resolver = createClientMetadataResolver({
            fetchImpl: fetchImpl as unknown as typeof fetch,
            now: () => clock,
        });

        await resolver.resolve(CLIENT_ID);
        await resolver.resolve(CLIENT_ID);
        expect(fetchImpl).toHaveBeenCalledTimes(1);

        clock = 601_000;
        await resolver.resolve(CLIENT_ID);
        expect(fetchImpl).toHaveBeenCalledTimes(2);
    });

    it("never fetches a blocked target", async () => {
        const fetchImpl = vi.fn();
        const resolver = createClientMetadataResolver({ fetchImpl: fetchImpl as unknown as typeof fetch });
        await expect(resolver.resolve("https://169.254.169.254/latest/meta-data")).rejects.toThrow(
            ClientMetadataError,
        );
        expect(fetchImpl).not.toHaveBeenCalled();
    });
});
