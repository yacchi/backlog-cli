/**
 * Secrets Manager rotation handler.
 *
 * Handles two secret types (via SECRET_TYPE env var):
 *
 * - "mcp-token-key": Generates a random base64url-encoded 32-byte AES-256 key.
 * - "relay-secrets": Initializes/rotates relay secrets (client_secret, JWKS, passphrase).
 *   Auto-generates Ed25519 JWKS and passphrase for tenants that don't provide them.
 *   JWKS is always preserved across rotations; passphrase is regenerated.
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
    kid: string;
    jwks?: string;
    passphrase_hash?: string;
    passphrase_length?: number;
}

interface RelaySecretsValue {
    apps: Record<string, AppSecret>;
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

function generatePassphrase(length = 32): { passphrase: string; passphrase_hash: string } {
    const byteLength = Math.ceil((length * 3) / 4);
    const passphrase = randomBytes(byteLength).toString("base64url").slice(0, length);
    const passphrase_hash = hashSync(passphrase, 12);
    return { passphrase, passphrase_hash };
}

// ============================================================
// createSecret step — generates new secret value as AWSPENDING
// ============================================================

async function createMcpTokenKey(
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

    const key = randomBytes(32).toString("base64url");
    await client.send(
        new PutSecretValueCommand({
            SecretId: secretId,
            ClientRequestToken: clientRequestToken,
            SecretString: key,
            VersionStages: ["AWSPENDING"],
        }),
    );
}

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

    const appsSecrets = JSON.parse(
        process.env.APPS_SECRETS ?? "{}",
    ) as Record<string, AppSecret>;
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
            if (parsed.apps && parsed.tenants) {
                existing = parsed as RelaySecretsValue;
            }
        }
    } catch {
        // First rotation — no structured value yet
    }

    const result: RelaySecretsValue = { apps: appsSecrets, tenants: {} };

    for (const [spaceDomain, config] of Object.entries(tenantConfigs)) {
        const prev = existing?.tenants[spaceDomain];
        const entry: RelaySecretsValue["tenants"][string] = {};

        if (config.jwks) {
            entry.jwks = config.jwks;
        } else if (prev?.jwks) {
            entry.jwks = prev.jwks;
        } else {
            entry.jwks = generateEdKeypairJwks(config.kid);
            console.log(
                `Auto-generated Ed25519 keypair for ${spaceDomain} (kid: ${config.kid})`,
            );
        }

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
    const secretType = process.env.SECRET_TYPE ?? "mcp-token-key";

    switch (event.Step) {
        case "createSecret":
            if (secretType === "relay-secrets") {
                await createRelaySecrets(
                    event.SecretId,
                    event.ClientRequestToken,
                );
            } else {
                await createMcpTokenKey(
                    event.SecretId,
                    event.ClientRequestToken,
                );
            }
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
