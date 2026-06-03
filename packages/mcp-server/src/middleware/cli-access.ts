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
