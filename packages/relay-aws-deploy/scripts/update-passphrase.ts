#!/usr/bin/env tsx
/**
 * Secrets Manager 上のテナントパスフレーズを config.ts の値で更新する。
 *
 * - config.ts に passphrase（平文）が設定されているテナントのみ対象
 * - 現在値と同一なら何もしない（bcrypt.compare で比較）
 * - passphrase_hash のみ指定の場合はハッシュ文字列の一致で比較
 * - 未設定テナント（自動生成）はスキップ
 *
 * Usage:
 *   pnpm update-passphrase            # 差分表示 (dry-run)
 *   pnpm update-passphrase --apply    # 実際に更新
 */

import {
    SecretsManagerClient,
    GetSecretValueCommand,
    PutSecretValueCommand,
} from "@aws-sdk/client-secrets-manager";
import { compareSync, hashSync } from "bcryptjs";
import { config } from "../config.js";

interface SecretsValue {
    app?: unknown;
    server?: unknown;
    tenants: Record<
        string,
        { jwks?: string; passphrase_hash?: string; passphrase?: string }
    >;
}

const secretId = `${config.parameterName}-secrets`;
const apply = process.argv.includes("--apply");

async function main() {
    if (!config.parameterValue) {
        console.error("config.ts に parameterValue が設定されていません。");
        process.exit(1);
    }
    const tenants = config.parameterValue.tenants ?? {};

    // config.ts にパスフレーズ指定があるテナントだけ収集
    const targets: Array<{
        domain: string;
        newHash: string;
        source: "plaintext" | "hash";
    }> = [];

    for (const [domain, tenant] of Object.entries(tenants)) {
        if (tenant.passphrase) {
            targets.push({
                domain,
                newHash: hashSync(tenant.passphrase, 12),
                source: "plaintext",
            });
        } else if (tenant.passphrase_hash) {
            targets.push({
                domain,
                newHash: tenant.passphrase_hash,
                source: "hash",
            });
        }
    }

    if (targets.length === 0) {
        console.log("config.ts にパスフレーズが設定されたテナントがありません。何もしません。");
        return;
    }

    const client = new SecretsManagerClient({});
    const resp = await client.send(
        new GetSecretValueCommand({ SecretId: secretId }),
    );
    if (!resp.SecretString) {
        console.error(`Secret ${secretId} が空です。先に cdk deploy を実行してください。`);
        process.exit(1);
    }

    const secrets: SecretsValue = JSON.parse(resp.SecretString);
    let updated = false;

    for (const { domain, newHash, source } of targets) {
        const current = secrets.tenants[domain];
        if (!current) {
            console.log(`[${domain}] Secrets Manager にテナントが存在しません。スキップ`);
            continue;
        }

        // 比較: plaintext なら元の平文で bcrypt.compare、hash なら文字列一致
        let same: boolean;
        if (source === "plaintext") {
            const tenant = tenants[domain];
            same = current.passphrase_hash
                ? compareSync(tenant.passphrase!, current.passphrase_hash)
                : false;
        } else {
            same = current.passphrase_hash === newHash;
        }

        if (same) {
            console.log(`[${domain}] パスフレーズは最新です。変更なし`);
            continue;
        }

        console.log(`[${domain}] パスフレーズの変更を検出`);

        if (apply) {
            current.passphrase_hash = newHash;
            delete current.passphrase;
            updated = true;
            console.log(`[${domain}] → 更新します`);
        } else {
            console.log(`[${domain}] → --apply で実際に更新されます (dry-run)`);
        }
    }

    if (!apply) {
        if (targets.some(({ domain }) => {
            const current = secrets.tenants[domain];
            if (!current) return false;
            const tenant = tenants[domain];
            if (tenant.passphrase) {
                return !current.passphrase_hash || !compareSync(tenant.passphrase, current.passphrase_hash);
            }
            if (tenant.passphrase_hash) {
                return current.passphrase_hash !== tenant.passphrase_hash;
            }
            return false;
        })) {
            console.log("\n実行するには: pnpm update-passphrase --apply");
        }
        return;
    }

    if (!updated) {
        console.log("\n更新対象がありませんでした。");
        return;
    }

    await client.send(
        new PutSecretValueCommand({
            SecretId: secretId,
            SecretString: JSON.stringify(secrets),
        }),
    );
    console.log(`\nSecrets Manager (${secretId}) を更新しました。`);
}

main().catch((err) => {
    console.error(err);
    process.exit(1);
});
