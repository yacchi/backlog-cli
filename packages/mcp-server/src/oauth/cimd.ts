/**
 * Client ID Metadata Documents (CIMD).
 *
 * MCP 2026-07-28 deprecates Dynamic Client Registration in favour of clients
 * identifying themselves with an HTTPS URL that serves their client metadata.
 * The authorization server resolves that URL on demand, so no registration
 * state is kept anywhere.
 *
 * Resolving a client-supplied URL means this server performs an outbound
 * request on behalf of an unauthenticated caller, so every fetch is fenced in:
 * https only, no redirects, no private/loopback hosts, hard timeout, body size
 * cap, optional host allow-list, and a bounded cache.
 */

export interface ClientIdMetadata {
    client_id: string;
    client_name: string;
    redirect_uris: string[];
}

export class ClientMetadataError extends Error {
    constructor(message: string) {
        super(message);
        this.name = "ClientMetadataError";
    }
}

const MAX_DOCUMENT_BYTES = 32 * 1024;
const FETCH_TIMEOUT_MS = 5_000;
const DEFAULT_TTL_MS = 300_000;
const MIN_TTL_MS = 60_000;
const MAX_TTL_MS = 24 * 60 * 60 * 1_000;
const MAX_CACHE_ENTRIES = 256;

const IPV4_PATTERN = /^\d{1,3}(\.\d{1,3}){3}$/;
const BLOCKED_HOST_SUFFIXES = [".local", ".localhost", ".internal", ".home.arpa"];
const BLOCKED_HOSTS = ["localhost"];

/** True when a client_id is a CIMD-style URL rather than an opaque identifier. */
export function isClientIdUrl(clientId: string): boolean {
    return clientId.startsWith("https://") || clientId.startsWith("http://");
}

/**
 * Validate the shape and target of a client_id URL before any network access.
 * Throws ClientMetadataError with the reason; the caller maps that to
 * `invalid_client`.
 */
export function assertFetchableClientId(clientId: string, allowedHosts: string[] = []): URL {
    let url: URL;
    try {
        url = new URL(clientId);
    } catch {
        throw new ClientMetadataError("client_id is not a valid URL");
    }

    if (url.protocol !== "https:") {
        throw new ClientMetadataError("client_id URL must use the https scheme");
    }
    if (url.pathname === "" || url.pathname === "/") {
        throw new ClientMetadataError("client_id URL must contain a path component");
    }
    if (url.hash) {
        throw new ClientMetadataError("client_id URL must not contain a fragment");
    }
    if (url.username || url.password) {
        throw new ClientMetadataError("client_id URL must not contain credentials");
    }

    const host = url.hostname.toLowerCase();
    if (host.startsWith("[") || IPV4_PATTERN.test(host)) {
        throw new ClientMetadataError("client_id URL must not target an IP address");
    }
    if (BLOCKED_HOSTS.includes(host) || BLOCKED_HOST_SUFFIXES.some((s) => host.endsWith(s))) {
        throw new ClientMetadataError(`client_id URL host is not publicly resolvable: ${host}`);
    }
    if (!host.includes(".")) {
        throw new ClientMetadataError(`client_id URL host is not a public domain: ${host}`);
    }

    if (allowedHosts.length > 0 && !matchesAllowedHost(host, allowedHosts)) {
        throw new ClientMetadataError(`client_id URL host is not allowed: ${host}`);
    }

    return url;
}

function matchesAllowedHost(host: string, allowedHosts: string[]): boolean {
    for (const pattern of allowedHosts) {
        try {
            if (new RegExp(`^${pattern}$`).test(host)) return true;
        } catch {
            // invalid regex — skip
        }
    }
    return false;
}

/** Parse `Cache-Control: max-age=<n>` into a clamped TTL. */
function ttlFromCacheControl(header: string | null): number {
    if (!header) return DEFAULT_TTL_MS;
    if (/\bno-store\b|\bno-cache\b/i.test(header)) return MIN_TTL_MS;
    const match = header.match(/\bmax-age\s*=\s*(\d+)/i);
    if (!match) return DEFAULT_TTL_MS;
    const ms = Number(match[1]) * 1000;
    if (!Number.isFinite(ms)) return DEFAULT_TTL_MS;
    return Math.min(Math.max(ms, MIN_TTL_MS), MAX_TTL_MS);
}

function validateDocument(clientId: string, raw: unknown): ClientIdMetadata {
    if (!raw || typeof raw !== "object" || Array.isArray(raw)) {
        throw new ClientMetadataError("client metadata document is not a JSON object");
    }
    const doc = raw as Record<string, unknown>;

    // The document must claim exactly the URL it was fetched from, otherwise a
    // hosted document could impersonate another client identifier.
    if (doc.client_id !== clientId) {
        throw new ClientMetadataError("client_id in metadata document does not match its URL");
    }
    if (typeof doc.client_name !== "string" || doc.client_name.trim() === "") {
        throw new ClientMetadataError("client metadata document is missing client_name");
    }
    if (!Array.isArray(doc.redirect_uris) || doc.redirect_uris.length === 0) {
        throw new ClientMetadataError("client metadata document is missing redirect_uris");
    }
    const redirectUris: string[] = [];
    for (const uri of doc.redirect_uris) {
        if (typeof uri !== "string") {
            throw new ClientMetadataError("redirect_uris must contain strings only");
        }
        try {
            new URL(uri);
        } catch {
            throw new ClientMetadataError(`invalid redirect_uri in metadata document: ${uri}`);
        }
        redirectUris.push(uri);
    }

    return {
        client_id: clientId,
        client_name: doc.client_name,
        redirect_uris: redirectUris,
    };
}

export interface ClientMetadataResolverOptions {
    /** Full-string regex patterns of permitted hosts. Empty = any public host. */
    allowedHosts?: string[];
    fetchImpl?: typeof fetch;
    now?: () => number;
}

export interface ClientMetadataResolver {
    resolve(clientId: string): Promise<ClientIdMetadata>;
}

export function createClientMetadataResolver(
    options?: ClientMetadataResolverOptions,
): ClientMetadataResolver {
    const allowedHosts = options?.allowedHosts ?? [];
    const doFetch = options?.fetchImpl ?? fetch;
    const now = options?.now ?? (() => Date.now());
    const cache = new Map<string, { value: ClientIdMetadata; expiresAt: number }>();

    return {
        async resolve(clientId: string): Promise<ClientIdMetadata> {
            const cached = cache.get(clientId);
            if (cached && cached.expiresAt > now()) {
                return cached.value;
            }
            cache.delete(clientId);

            const url = assertFetchableClientId(clientId, allowedHosts);

            let resp: Response;
            try {
                resp = await doFetch(url.toString(), {
                    headers: { Accept: "application/json" },
                    redirect: "error",
                    signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
                });
            } catch (err) {
                throw new ClientMetadataError(
                    `failed to fetch client metadata document: ${(err as Error).message}`,
                );
            }

            if (!resp.ok) {
                throw new ClientMetadataError(
                    `client metadata document returned HTTP ${resp.status}`,
                );
            }

            const contentType = resp.headers.get("content-type") ?? "";
            if (!/application\/(\w+\+)?json/i.test(contentType)) {
                throw new ClientMetadataError(
                    `client metadata document has unexpected content-type: ${contentType || "(none)"}`,
                );
            }

            const body = await resp.text();
            if (body.length > MAX_DOCUMENT_BYTES) {
                throw new ClientMetadataError("client metadata document is too large");
            }

            let parsed: unknown;
            try {
                parsed = JSON.parse(body);
            } catch {
                throw new ClientMetadataError("client metadata document is not valid JSON");
            }

            const metadata = validateDocument(clientId, parsed);

            if (cache.size >= MAX_CACHE_ENTRIES) {
                const oldest = cache.keys().next();
                if (!oldest.done) cache.delete(oldest.value);
            }
            cache.set(clientId, {
                value: metadata,
                expiresAt: now() + ttlFromCacheControl(resp.headers.get("cache-control")),
            });

            return metadata;
        },
    };
}
