/**
 * Secrets Manager implementation of PassphraseManager.
 *
 * Reads and writes tenant passphrases in Secrets Manager.
 * The secret stores both the bcrypt hash and plaintext passphrase
 * (matching the rotation-handler's format).
 */

import {
    SecretsManagerClient,
    GetSecretValueCommand,
    PutSecretValueCommand,
} from "@aws-sdk/client-secrets-manager";
import { hashSync } from "bcryptjs";
import type { PassphraseManager, PassphraseInfo } from "@yacchi/backlog-relay-core";
import type { RelaySecrets } from "./config-source.js";

const BCRYPT_ROUNDS = 12;
const PASSPHRASE_LENGTH = 32;

export class SecretsManagerPassphraseManager implements PassphraseManager {
    private readonly client: SecretsManagerClient;
    private readonly secretName: string;
    private readonly onUpdate?: () => void;

    constructor(secretName: string, onUpdate?: () => void) {
        this.client = new SecretsManagerClient({});
        this.secretName = secretName;
        this.onUpdate = onUpdate;
    }

    async getPassphrase(tenantName: string): Promise<PassphraseInfo> {
        const secrets = await this.loadSecrets();
        const tenant = secrets.tenants?.[tenantName];
        if (!tenant?.passphrase_hash) {
            return { hasPassphrase: false };
        }
        return {
            hasPassphrase: true,
            passphrase: (tenant as Record<string, string>).passphrase,
        };
    }

    async setPassphrase(tenantName: string, passphrase: string): Promise<void> {
        const secrets = await this.loadSecrets();
        if (!secrets.tenants) {
            secrets.tenants = {};
        }
        secrets.tenants[tenantName] = {
            ...secrets.tenants[tenantName],
            passphrase_hash: hashSync(passphrase, BCRYPT_ROUNDS),
            passphrase,
        } as RelaySecrets["tenants"] extends Record<string, infer V> ? V & { passphrase: string } : never;
        await this.saveSecrets(secrets);
    }

    async generatePassphrase(tenantName: string): Promise<{ passphrase: string }> {
        const bytes = new Uint8Array(PASSPHRASE_LENGTH);
        crypto.getRandomValues(bytes);
        const passphrase = btoa(String.fromCharCode(...bytes))
            .replace(/\+/g, "-")
            .replace(/\//g, "_")
            .replace(/=/g, "");
        await this.setPassphrase(tenantName, passphrase);
        return { passphrase };
    }

    async clearPassphrase(tenantName: string): Promise<void> {
        const secrets = await this.loadSecrets();
        if (secrets.tenants?.[tenantName]) {
            delete (secrets.tenants[tenantName] as Record<string, unknown>).passphrase_hash;
            delete (secrets.tenants[tenantName] as Record<string, unknown>).passphrase;
        }
        await this.saveSecrets(secrets);
    }

    private async loadSecrets(): Promise<RelaySecrets> {
        const response = await this.client.send(
            new GetSecretValueCommand({ SecretId: this.secretName }),
        );
        if (!response.SecretString) {
            throw new Error(`Secret ${this.secretName} not found or empty`);
        }
        return JSON.parse(response.SecretString) as RelaySecrets;
    }

    private async saveSecrets(secrets: RelaySecrets): Promise<void> {
        await this.client.send(
            new PutSecretValueCommand({
                SecretId: this.secretName,
                SecretString: JSON.stringify(secrets),
            }),
        );
        this.onUpdate?.();
    }
}
