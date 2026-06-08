/**
 * Secrets Manager rotation handler.
 *
 * Handles relay-secrets rotation:
 *   Initializes/rotates relay secrets (client_secret, JWKS, passphrase).
 *   JWKS is stored at server level (shared by all tenants and MCP).
 *   Auto-generates Ed25519 JWKS if not provided.
 *   Passphrase is per-tenant, regenerated on rotation.
 *
 * Follows the 4-step SM rotation protocol:
 * https://docs.aws.amazon.com/secretsmanager/latest/userguide/rotating-secrets-lambda-function-overview.html
 */

import { generateKeyPairSync, randomBytes } from "node:crypto";
import { hashSync } from "bcryptjs";

interface RotationEvent {
    SecretId: string;
    ClientRequestToken: string;
    Step: "createSecret" | "setSecret" | "testSecret" | "finishSecret";
}

interface AppSecret {
    client_secret: string;
}

interface TenantConfig {
    passphrase_hash?: string;
    passphrase_length?: number;
}

interface RelaySecretsValue {
    app?: AppSecret;
    server?: { jwks?: string };
    tenants: Record<
        string,
        { jwks?: string; passphrase_hash?: string; passphrase?: string }
    >;
}

async function getSmClient() {
    const { SecretsManagerClient } = await import(
        "@aws-sdk/client-secrets-manager"
    );
    return new SecretsManagerClient({});
}

function generateEdKeypairJwks(kid: string): string {
    const { privateKey } = generateKeyPairSync("ed25519");
    const jwk = privateKey.export({ format: "jwk" });
    return JSON.stringify({ keys: [{ ...jwk, kid }] });
}

function hasJwksKeys(jwksJson: string): boolean {
    try {
        const parsed = JSON.parse(jwksJson) as { keys?: unknown[] };
        return Array.isArray(parsed.keys) && parsed.keys.length > 0;
    } catch {
        return false;
    }
}

function generatePassphrase(length = 32): { passphrase: string; passphrase_hash: string } {
    const byteLength = Math.ceil((length * 3) / 4);
    const passphrase = randomBytes(byteLength).toString("base64url").slice(0, length);
    const passphrase_hash = hashSync(passphrase, 12);
    return { passphrase, passphrase_hash };
}

// ============================================================
// createSecret step — generates new secret value as AWSPENDING
// ============================================================

async function createRelaySecrets(
    secretId: string,
    clientRequestToken: string,
): Promise<void> {
    const {
        GetSecretValueCommand,
        PutSecretValueCommand,
        ResourceNotFoundException,
    } = await import("@aws-sdk/client-secrets-manager");
    const client = await getSmClient();

    try {
        await client.send(
            new GetSecretValueCommand({
                SecretId: secretId,
                VersionId: clientRequestToken,
                VersionStage: "AWSPENDING",
            }),
        );
        return;
    } catch (e) {
        if (!(e instanceof ResourceNotFoundException)) throw e;
    }

    const appSecret = JSON.parse(
        process.env.APP_SECRET ?? "{}",
    ) as AppSecret | Record<string, never>;
    const serverJwksInput = process.env.SERVER_JWKS || "";
    const tenantConfigs = JSON.parse(
        process.env.TENANT_CONFIGS ?? "{}",
    ) as Record<string, TenantConfig>;

    let existing: RelaySecretsValue | null = null;
    try {
        const resp = await client.send(
            new GetSecretValueCommand({
                SecretId: secretId,
                VersionStage: "AWSCURRENT",
            }),
        );
        if (resp.SecretString) {
            const parsed = JSON.parse(resp.SecretString);
            if (parsed.tenants || parsed.server) {
                existing = parsed as RelaySecretsValue;
            }
        }
    } catch {
        // First rotation — no structured value yet
    }

    // Server-level JWKS: use provided (if has keys) > existing server > existing tenant (migration) > auto-generate
    let jwks: string;
    const parsedInput = serverJwksInput ? JSON.parse(serverJwksInput) as { keys?: unknown[] } : null;
    if (parsedInput?.keys && parsedInput.keys.length > 0) {
        jwks = serverJwksInput;
    } else if (existing?.server?.jwks && hasJwksKeys(existing.server.jwks)) {
        jwks = existing.server.jwks;
    } else {
        // Migration: check if any existing tenant has JWKS
        const existingTenantJwks = Object.values(existing?.tenants ?? {}).find((t) => t.jwks)?.jwks;
        if (existingTenantJwks) {
            jwks = existingTenantJwks;
            console.log("Migrated JWKS from tenant to server level");
        } else {
            jwks = generateEdKeypairJwks("auto-1");
            console.log("Auto-generated server-level Ed25519 keypair (kid: auto-1)");
        }
    }

    const result: RelaySecretsValue = {
        app: "client_secret" in appSecret ? appSecret as AppSecret : undefined,
        server: { jwks },
        tenants: {},
    };

    for (const [spaceDomain, config] of Object.entries(tenantConfigs)) {
        const prev = existing?.tenants[spaceDomain];
        const entry: RelaySecretsValue["tenants"][string] = {};

        if (config.passphrase_hash) {
            entry.passphrase_hash = config.passphrase_hash;
        } else {
            const generated = generatePassphrase(config.passphrase_length);
            entry.passphrase = generated.passphrase;
            entry.passphrase_hash = generated.passphrase_hash;
            if (!prev?.passphrase_hash) {
                console.log(
                    `Auto-generated passphrase for ${spaceDomain}`,
                );
            }
        }

        result.tenants[spaceDomain] = entry;
    }

    await client.send(
        new PutSecretValueCommand({
            SecretId: secretId,
            ClientRequestToken: clientRequestToken,
            SecretString: JSON.stringify(result),
            VersionStages: ["AWSPENDING"],
        }),
    );
}

// ============================================================
// finishSecret step — promote AWSPENDING to AWSCURRENT
// ============================================================

async function finishSecret(
    secretId: string,
    clientRequestToken: string,
): Promise<void> {
    const { DescribeSecretCommand, UpdateSecretVersionStageCommand } =
        await import("@aws-sdk/client-secrets-manager");
    const client = await getSmClient();

    const metadata = await client.send(
        new DescribeSecretCommand({ SecretId: secretId }),
    );

    let currentVersionId: string | undefined;
    for (const [versionId, stages] of Object.entries(
        metadata.VersionIdsToStages ?? {},
    )) {
        if (stages.includes("AWSCURRENT")) {
            if (versionId === clientRequestToken) return;
            currentVersionId = versionId;
            break;
        }
    }

    await client.send(
        new UpdateSecretVersionStageCommand({
            SecretId: secretId,
            VersionStage: "AWSCURRENT",
            MoveToVersionId: clientRequestToken,
            RemoveFromVersionId: currentVersionId,
        }),
    );
}

// ============================================================
// Entry point
// ============================================================

export async function handler(event: RotationEvent): Promise<void> {
    switch (event.Step) {
        case "createSecret":
            await createRelaySecrets(
                event.SecretId,
                event.ClientRequestToken,
            );
            break;
        case "setSecret":
        case "testSecret":
            break;
        case "finishSecret":
            await finishSecret(event.SecretId, event.ClientRequestToken);
            break;
        default:
            throw new Error(`Unknown step: ${(event as RotationEvent).Step}`);
    }
}
