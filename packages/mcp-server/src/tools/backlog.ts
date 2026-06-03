import { execFile } from "node:child_process";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import type { McpTenant } from "../config/schema.js";
import type { TokenPayload } from "../crypto/jwe.js";
import { checkCliAccess } from "../middleware/cli-access.js";

const DEFAULT_TIMEOUT = 30_000;

export interface BacklogToolResult {
    output: string;
    exitCode: number;
}

export async function executeBacklogCommand(
    args: string,
    token: TokenPayload,
    tenant: McpTenant | undefined,
    binPath?: string,
): Promise<BacklogToolResult> {
    if (tenant && !checkCliAccess(args, tenant.cli_access)) {
        return {
            output: `Access denied: command "${args}" is not allowed by tenant policy`,
            exitCode: 1,
        };
    }

    const cliPath = binPath ?? resolveDefaultBinPath();
    const parsedArgs = parseArgs(args);

    return new Promise((resolve, reject) => {
        execFile(
            cliPath,
            parsedArgs,
            {
                env: {
                    BACKLOG_ACCESS_TOKEN: token.bl_access_token,
                    BACKLOG_SPACE: token.space,
                    BACKLOG_DOMAIN: token.domain,
                    HOME: "/tmp",
                    PATH: process.env.PATH,
                },
                timeout: DEFAULT_TIMEOUT,
                maxBuffer: 10 * 1024 * 1024,
            },
            (error, stdout, stderr) => {
                if (error && !stdout && !stderr) {
                    reject(error);
                    return;
                }
                const output = stdout || stderr || "";
                const exitCode = error?.code
                    ? typeof error.code === "number"
                        ? error.code
                        : 1
                    : 0;
                resolve({ output, exitCode });
            },
        );
    });
}

function resolveDefaultBinPath(): string {
    const __dirname = dirname(fileURLToPath(import.meta.url));
    return resolve(__dirname, "..", "bin", "backlog");
}

function parseArgs(argsString: string): string[] {
    const args: string[] = [];
    let current = "";
    let inSingle = false;
    let inDouble = false;
    let escape = false;

    for (const ch of argsString) {
        if (escape) {
            current += ch;
            escape = false;
            continue;
        }

        if (ch === "\\") {
            escape = true;
            continue;
        }

        if (ch === "'" && !inDouble) {
            inSingle = !inSingle;
            continue;
        }

        if (ch === '"' && !inSingle) {
            inDouble = !inDouble;
            continue;
        }

        if (ch === " " && !inSingle && !inDouble) {
            if (current) {
                args.push(current);
                current = "";
            }
            continue;
        }

        current += ch;
    }

    if (current) {
        args.push(current);
    }

    return args;
}

export { parseArgs };
