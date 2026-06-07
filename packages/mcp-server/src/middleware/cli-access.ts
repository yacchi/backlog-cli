import type { CliAccess } from "../config/schema.js";

export function checkCliAccess(args: string, access: CliAccess): boolean {
    for (const pattern of access.deny) {
        if (matchPattern(args, pattern)) {
            return false;
        }
    }

    for (const pattern of access.allow) {
        if (matchPattern(args, pattern)) {
            return true;
        }
    }

    return false;
}

const READ_ONLY_COMMANDS = new Set(["whoami"]);
const READ_ONLY_SUBCOMMANDS = new Set(["list", "view", "myself", "show"]);

export function isReadOnlyCommand(args: string): boolean {
    const parts = args.trim().split(/\s+/);
    if (parts.length === 0) return false;

    const command = parts[0];

    if (READ_ONLY_COMMANDS.has(command)) return true;

    if (command === "api") {
        const methodIdx = parts.indexOf("-X");
        if (methodIdx === -1) return true;
        const method = parts[methodIdx + 1]?.toUpperCase();
        return method === "GET" || method === "HEAD" || method === "OPTIONS";
    }

    return READ_ONLY_SUBCOMMANDS.has(parts[1] ?? "");
}

function matchPattern(args: string, pattern: string): boolean {
    const regexStr = pattern
        .split("*")
        .map(escapeRegex)
        .join(".*");
    return new RegExp(`^${regexStr}$`).test(args);
}

function escapeRegex(s: string): string {
    return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
