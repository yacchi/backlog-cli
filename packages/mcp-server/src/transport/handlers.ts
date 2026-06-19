import { Hono } from "hono";
import type { McpServerConfig, SpaceAccess, ScriptConfig } from "../config/schema.js";
import { matchSpacePattern } from "../config/schema.js";
import { jwtAuth, getAuthContext, resolveSpaceToken } from "../middleware/jwt-auth.js";
import { resolveBaseUrl } from "../base-url.js";
import { executeBacklogCommand } from "../tools/backlog.js";
import { materializeFiles, substituteFileRefs } from "../tools/files.js";
import { Logger, LOGGER_CONTEXT_KEY, logToolCall, type LoggingConfig } from "../logging/logger.js";
import type { TokenPayload, SigningKeys } from "../crypto/jwt.js";
import { listSpaceEntries } from "../crypto/jwt.js";

export interface ScriptFile {
    content: string;
    encoding?: "base64" | "gzip+base64" | "utf8";
    name?: string;
}

interface JsonRpcRequest {
    jsonrpc: "2.0";
    id?: string | number;
    method: string;
    params?: Record<string, unknown>;
}

interface JsonRpcResponse {
    jsonrpc: "2.0";
    id?: string | number | null;
    result?: unknown;
    error?: { code: number; message: string; data?: unknown };
}

const MCP_PROTOCOL_VERSION = "2025-03-26";

const SERVER_INSTRUCTIONS = `Backlog CLI MCP Server — provides Backlog project management via the \`backlog\` CLI, a GitHub CLI (gh) compatible command-line tool for Backlog.

## IMPORTANT: Prefer local CLI when available

If the \`backlog\` CLI is installed locally (check with \`backlog --version\`), prefer running CLI commands directly in the shell instead of using these MCP tools. The local CLI provides faster execution, richer interactive features, and access to local configuration (.backlog.yaml).

Use these MCP tools only when:
- The local CLI is not installed (e.g., Claude.ai web, non-developer environments)
- You have already confirmed the CLI is unavailable in this session

## CLI Reference (backlog_help)

Whether using the local CLI or MCP tools, call \`backlog_help\` to look up command syntax and flags. The \`backlog\` CLI follows GitHub CLI (gh) conventions — subcommand structure, \`--json\` output, and flag patterns are similar. If you know gh, you can apply the same patterns to \`backlog\`.`;

const FILES_SCHEMA = {
    type: "array" as const,
    items: {
        type: "object" as const,
        properties: {
            content: {
                type: "string" as const,
                description: "File content, encoded according to the encoding field.",
            },
            encoding: {
                type: "string" as const,
                enum: ["base64", "gzip+base64", "utf8"],
                description: "Content encoding (default: base64).",
            },
            name: {
                type: "string" as const,
                description: "Optional filename hint (e.g., 'diagram.png'). Used as the temp file name for --attach.",
            },
        },
        required: ["content"],
    },
    description:
        "File contents to pass to CLI commands. Referenced in args as $file[0], $file[1], etc. Written to temp files and substituted with actual paths before CLI execution. Use with --body-file, --attach, or any flag that accepts a file path.",
};

const HELP_TOOL = {
    name: "backlog_help",
    description:
        "Get the Backlog CLI reference — available commands, flags, output formats, and usage examples. IMPORTANT: Call this BEFORE using any other backlog tool if you have not already done so in this conversation.",
    inputSchema: {
        type: "object" as const,
        properties: {
            command: {
                type: "string" as const,
                description:
                    "Optional: a specific command to get help for (e.g., 'issue list', 'wiki', 'api'). Omit for the full reference.",
            },
        },
    },
    annotations: {
        title: "Backlog CLI Help",
        readOnlyHint: true,
        destructiveHint: false,
        openWorldHint: false,
    },
};

const CLI_REF_HINT = "Call backlog_help first to learn available commands and flags.";

const SPACE_PROP = {
    type: "string" as const,
    description:
        "Target Backlog space (e.g., 'example.backlog.com'). Omit to use the primary authenticated space.",
} as const;

const QUERY_TOOLS = [
    {
        name: "who",
        description:
            "Look up Backlog users. Without query: returns the authenticated user's own profile. With query: searches users by name, userId, or email address.",
        inputSchema: {
            type: "object" as const,
            properties: {
                query: {
                    type: "string" as const,
                    description:
                        "Search query — matched against name, userId, and email (case-insensitive, partial match). Omit to get your own profile.",
                },
                space: SPACE_PROP,
            },
        },
        annotations: {
            title: "Who",
            readOnlyHint: true,
            destructiveHint: false,
            openWorldHint: false,
        },
    },
    {
        name: "backlog_query",
        description:
            `Execute a read-only backlog CLI command (similar to gh CLI). Supports list, view, and GET API calls. Use --json flag for structured output, --jq for filtering. For user lookup, use the who tool. For write operations, use backlog_mutate. ${CLI_REF_HINT}

Key flags for 'issue list': -p PROJECT (required), -L LIMIT, -S/--search KEYWORD, -s/--state open|closed|all, -a/--assignee USER|@me, --mine, -T/--type TYPE, --priority NAME, --since YYYY-MM-DD, --sort FIELD, --order asc|desc, --json=FIELDS, --count.
Key flags for 'issue view': -c/--comments (show comments, default: 20), -c N (N comments), -c all (all comments), --comments-order asc|desc, --comments-since ID.
Flags accept names (case-insensitive, fuzzy): --priority high (=高), --type bug (=バグ), --type task (=タスク).`,
        inputSchema: {
            type: "object" as const,
            properties: {
                args: {
                    type: "string" as const,
                    description:
                        "CLI arguments for read-only commands (e.g., 'issue list -p PROJ -L 20 --json=issueKey,summary,status')",
                },
                space: SPACE_PROP,
                files: FILES_SCHEMA,
            },
            required: ["args"],
        },
        annotations: {
            title: "Backlog Query",
            readOnlyHint: true,
            destructiveHint: false,
            openWorldHint: false,
        },
    },
    {
        name: "backlog_query_script",
        description:
            `Execute a read-only Python analysis script with backlog() helper. Only read commands (list, view, GET API calls) are allowed — write operations are blocked. Use for multi-step data retrieval, aggregation, filtering, and reporting across multiple API calls in one round trip. Python standard library available (json, datetime, re, collections, etc.). ${CLI_REF_HINT}

IMPORTANT: backlog() takes CLI subcommand args as a string, NOT HTTP methods. Correct: backlog('issue list -p PROJ --json=issueKey,summary'). Wrong: backlog('GET /api/v2/issues'). The return value from backlog() with --json is already a parsed dict/list — do NOT call json.loads() on it.`,
        inputSchema: {
            type: "object" as const,
            properties: {
                script: {
                    type: "string" as const,
                    description:
                        "Python code. backlog(args) runs a CLI command and returns the result (parsed JSON with --json, or text). print() output is returned to the user; if no print(), the last expression value is used as fallback. Read-only commands only.",
                },
                space: SPACE_PROP,
                files: FILES_SCHEMA,
            },
            required: ["script"],
        },
        annotations: {
            title: "Analyze Backlog Data",
            readOnlyHint: true,
            destructiveHint: false,
            openWorldHint: false,
        },
    },
];

const MUTATION_TOOLS = [
    {
        name: "backlog_mutate",
        description:
            `Execute a backlog CLI command that modifies data (create, update, delete). For multi-line text content (issue body, comments, wiki content), use the files parameter with --body-file $file[0] instead of inline -b/--body to avoid escaping issues. For read-only commands, use backlog_query. ${CLI_REF_HINT}

Key flags for 'issue create': -p PROJECT, -t/--title TITLE, --body-file $file[0], -T/--type TYPE, --priority NAME, -a/--assignee USER.
Flags accept names (case-insensitive, fuzzy): --priority high (=高), --type bug (=バグ), --type task (=タスク).
Wiki commands require numeric IDs: use 'wiki list' first, then 'wiki view ID'.`,
        inputSchema: {
            type: "object" as const,
            properties: {
                args: {
                    type: "string" as const,
                    description:
                        "CLI arguments (e.g., 'issue create -p PROJ -t \"Title\" --body-file $file[0] --type Bug')",
                },
                space: SPACE_PROP,
                files: FILES_SCHEMA,
            },
            required: ["args"],
        },
        annotations: {
            title: "Backlog Mutate",
            readOnlyHint: false,
            destructiveHint: false,
            openWorldHint: false,
        },
    },
    {
        name: "backlog_mutate_script",
        description:
            `Execute Python in a sandboxed environment with backlog() helper. Use for chaining multiple CLI calls including write operations (create, update, delete). Python standard library available (json, datetime, re, collections, etc.). Dangerous modules (os, subprocess, socket, etc.) are blocked. For multi-line text content (issue body, comments, wiki content), use the files parameter with --body-file $file[N] instead of inline -b/--body to avoid escaping issues. ${CLI_REF_HINT}`,
        inputSchema: {
            type: "object" as const,
            properties: {
                script: {
                    type: "string" as const,
                    description:
                        "Python code. backlog(args) runs a CLI command and returns the result (parsed JSON with --json, or text). print() output is returned to the user; if no print(), the last expression value is used as fallback.",
                },
                space: SPACE_PROP,
                files: FILES_SCHEMA,
            },
            required: ["script"],
        },
        annotations: {
            title: "Run Script",
            readOnlyHint: false,
            destructiveHint: false,
            openWorldHint: false,
        },
    },
];

const SKILL_PROMPT = {
    name: "backlog-cli-reference",
    description:
        "Backlog CLI reference. Similar to gh (GitHub CLI). Use this to understand available commands, flags, and output formats before calling the backlog tool.",
    arguments: [],
};

export function createTransportHandlers(
    config: McpServerConfig,
    keys: SigningKeys,
    options?: { binPath?: string; runScript?: (script: string, token: TokenPayload, scriptConfig: ScriptConfig | undefined, options?: { readOnly?: boolean; files?: ScriptFile[] }) => Promise<{ result: string; error?: string }> },
): Hono {
    const app = new Hono();
    const { verifyKeys, encKeys } = keys;
    const loggingConfig: LoggingConfig = {
        input: config.logging?.input ?? false,
        output: config.logging?.output ?? false,
    };
    const auth = jwtAuth(
        verifyKeys,
        (c) => `${resolveBaseUrl(c, config.base_url)}/.well-known/oauth-protected-resource`,
    );
    const hasScript = !!options?.runScript;

    app.post("/mcp", auth, async (c) => {
        let req: JsonRpcRequest;
        try {
            req = await c.req.json();
        } catch {
            return c.json(jsonRpcError(null, -32700, "Parse error"));
        }

        if (req.jsonrpc !== "2.0" || !req.method) {
            return c.json(jsonRpcError(req.id ?? null, -32600, "Invalid Request"));
        }

        const { token } = getAuthContext(c);
        const reqLogger = getLogger(c);

        if (req.method === "tools/call") {
            const toolParams = req.params as { name?: string; arguments?: Record<string, unknown> } | undefined;
            reqLogger.debug({ component: "jsonrpc", method: req.method, tool: toolParams?.name });
            const requestedSpace = toolParams?.arguments?.space as string | undefined;
            if (requestedSpace && !(await resolveSpaceToken(token, encKeys, requestedSpace))) {
                c.header(
                    "WWW-Authenticate",
                    `Bearer error="insufficient_scope", resource_metadata="${resolveBaseUrl(c, config.base_url)}/.well-known/oauth-protected-resource"`,
                );
                return c.json(
                    { error: "insufficient_scope", error_description: `スペース '${requestedSpace}' は認証されていません。認証ページで追加してください。` },
                    403,
                );
            }
        } else {
            reqLogger.info({ component: "jsonrpc", method: req.method });
        }

        const spaceKey = token.space;
        const access = matchSpacePattern(spaceKey, config.spaces);
        if (!access) {
            return c.json(
                jsonRpcError(req.id ?? null, -32001, `スペース '${spaceKey}' はこのサーバーでは許可されていません`),
            );
        }

        const result = await handleMethod(c, req, token, access);
        return c.json(result);
    });

    app.get("/mcp", auth, (c) => {
        c.header("Content-Type", "text/event-stream");
        c.header("Cache-Control", "no-cache");
        c.header("Connection", "keep-alive");
        return c.body("");
    });

    app.delete("/mcp", auth, (c) => {
        return c.json({ status: "session_ended" });
    });

    function getLogger(c: import("hono").Context): Logger {
        return (c.get(LOGGER_CONTEXT_KEY) as Logger | undefined) ?? new Logger();
    }

    async function handleMethod(
        c: import("hono").Context,
        req: JsonRpcRequest,
        token: TokenPayload,
        access: SpaceAccess,
    ): Promise<JsonRpcResponse> {
        switch (req.method) {
            case "initialize":
                return jsonRpcResult(req.id, {
                    protocolVersion: MCP_PROTOCOL_VERSION,
                    capabilities: {
                        tools: { listChanged: false },
                        prompts: { listChanged: false },
                    },
                    serverInfo: {
                        name: "backlog-mcp-server",
                        version: "0.1.0",
                    },
                    instructions: SERVER_INSTRUCTIONS,
                });

            case "notifications/initialized":
                return jsonRpcResult(req.id, {});

            case "tools/list":
                return jsonRpcResult(req.id, {
                    tools: getAvailableTools(access),
                });

            case "tools/call":
                return handleToolCall(c, req, token);

            case "prompts/list":
                return jsonRpcResult(req.id, {
                    prompts: [SKILL_PROMPT],
                });

            case "prompts/get":
                return handlePromptGet(req, token);

            case "ping":
                return jsonRpcResult(req.id, {});

            default:
                return jsonRpcError(req.id, -32601, `Method not found: ${req.method}`);
        }
    }

    function getAvailableTools(access: SpaceAccess) {
        const tools = [...QUERY_TOOLS];
        if (access.writable) {
            tools.push(...MUTATION_TOOLS);
        }
        if (!hasScript) {
            return [HELP_TOOL, ...tools.filter((t) => !t.name.endsWith("_script"))];
        }
        return [HELP_TOOL, ...tools];
    }

    async function handleToolCall(
        c: import("hono").Context,
        req: JsonRpcRequest,
        token: TokenPayload,
    ): Promise<JsonRpcResponse> {
        const logger = getLogger(c);
        const params = req.params as { name?: string; arguments?: Record<string, unknown> } | undefined;
        const toolName = params?.name;
        const toolArgs = params?.arguments ?? {};
        const requestedSpace = toolArgs.space as string | undefined;
        const resolved = await resolveSpaceToken(token, encKeys, requestedSpace);
        if (!resolved) {
            const spaceEntries = listSpaceEntries(token);
            const authenticated = spaceEntries.length > 0
                ? spaceEntries.map(([domain]) => domain).join(", ")
                : token.space;
            return jsonRpcResult(req.id, {
                content: [{
                    type: "text",
                    text: `スペース '${requestedSpace}' はこのセッションで認証されていません。\n認証済みスペース: ${authenticated}\nこのMCPサーバーに再接続し、認証ページで '${requestedSpace}' を追加する必要があります。`,
                }],
                isError: true,
            });
        }

        const effectiveToken: TokenPayload = {
            ...token,
            space: resolved.space,
            bl_access_token: resolved.bl_access_token,
        };
        const spaceKey = resolved.space;
        const effectiveAccess = matchSpacePattern(spaceKey, config.spaces);
        if (!effectiveAccess) {
            return jsonRpcResult(req.id, {
                content: [{
                    type: "text",
                    text: `スペース '${spaceKey}' はこのサーバーでは許可されていません。`,
                }],
                isError: true,
            });
        }

        const isMutation = toolName === "backlog_mutate" || toolName === "backlog_mutate_script";
        if (isMutation && !effectiveAccess.writable) {
            return jsonRpcResult(req.id, {
                content: [{
                    type: "text",
                    text: `スペース '${spaceKey}' への書き込み操作は許可されていません。このスペースは読み取り専用です。`,
                }],
                isError: true,
            });
        }

        switch (toolName) {
            case "backlog_help": {
                const command = (toolArgs.command as string | undefined)?.trim();
                const log = logToolCall(logger, { tool: "backlog_help", input: { command }, tenant: spaceKey, loggingConfig });
                try {
                    const cliRef = await executeBacklogCommand(
                        'cli-ref --diff --exclude "ai,auth,config,markdown,profile,version"',
                        effectiveToken,
                        { readOnly: true, binPath: options?.binPath },
                    );
                    const full = buildCliReferencePrompt(effectiveToken.space, cliRef.output);
                    let text = full;
                    if (command) {
                        const sections = extractCommandSection(full, command);
                        text = sections ?? `No specific help found for "${command}". Here is the full reference:\n\n${full}`;
                    }
                    log.finish({ output: `${text.length} chars` });
                    return jsonRpcResult(req.id, {
                        content: [{ type: "text", text }],
                    });
                } catch (err) {
                    log.finish({ error: (err as Error).message, category: "exception" });
                    const fallback = buildCliReferencePrompt(effectiveToken.space);
                    return jsonRpcResult(req.id, {
                        content: [{ type: "text", text: fallback }],
                    });
                }
            }

            case "who": {
                const query = (toolArgs.query as string | undefined)?.trim();
                const log = logToolCall(logger, { tool: "who", input: { query }, tenant: spaceKey, loggingConfig });
                try {
                    if (!query) {
                        const result = await executeBacklogCommand(
                            "whoami --json",
                            effectiveToken,
                            { readOnly: true, binPath: options?.binPath },
                        );
                        log.finish({ output: result.output, error: result.exitCode !== 0 ? result.output : undefined });
                        return jsonRpcResult(req.id, {
                            content: [{ type: "text", text: result.output }],
                            isError: result.exitCode !== 0,
                        });
                    }

                    const result = await executeBacklogCommand(
                        "api /api/v2/users",
                        effectiveToken,
                        { readOnly: true, binPath: options?.binPath },
                    );
                    if (result.exitCode !== 0) {
                        log.finish({ error: result.output, category: "cli_error" });
                        return jsonRpcResult(req.id, {
                            content: [{ type: "text", text: result.output }],
                            isError: true,
                        });
                    }

                    const users = JSON.parse(result.output) as Array<Record<string, unknown>>;
                    const q = query.toLowerCase();
                    const matched = users.filter((u) => {
                        const name = String(u.name ?? "").toLowerCase();
                        const userId = String(u.userId ?? "").toLowerCase();
                        const mail = String(u.mailAddress ?? "").toLowerCase();
                        return name.includes(q) || userId.includes(q) || mail.includes(q);
                    });

                    if (matched.length === 0) {
                        log.finish({ output: `No users found matching "${query}"` });
                        return jsonRpcResult(req.id, {
                            content: [{ type: "text", text: `No users found matching "${query}"` }],
                        });
                    }
                    log.finish({ output: `${matched.length} users matched` });
                    return jsonRpcResult(req.id, {
                        content: [{ type: "text", text: JSON.stringify(matched, null, 2) }],
                    });
                } catch (err) {
                    log.finish({ error: (err as Error).message, category: "exception" });
                    return jsonRpcResult(req.id, {
                        content: [{ type: "text", text: `Error: ${(err as Error).message}` }],
                        isError: true,
                    });
                }
            }

            case "backlog_query":
            case "backlog_mutate": {
                const args = toolArgs.args as string | undefined;
                if (!args) {
                    return jsonRpcError(req.id, -32602, "Missing 'args' parameter");
                }
                const files = toolArgs.files as ScriptFile[] | undefined;
                const readOnly = toolName === "backlog_query" || !effectiveAccess.writable;

                const log = logToolCall(logger, { tool: toolName, input: { args }, tenant: spaceKey, loggingConfig });

                const filePaths = files?.length ? materializeFiles(files) : null;
                try {
                    const resolvedArgs = filePaths ? substituteFileRefs(args, filePaths.paths) : args;
                    const result = await executeBacklogCommand(
                        resolvedArgs,
                        effectiveToken,
                        { readOnly, binPath: options?.binPath },
                    );
                    log.finish({ output: result.output, error: result.exitCode !== 0 ? result.output : undefined, category: result.exitCode !== 0 ? "cli_error" : undefined });
                    return jsonRpcResult(req.id, {
                        content: [{ type: "text", text: result.output }],
                        isError: result.exitCode !== 0,
                    });
                } catch (err) {
                    log.finish({ error: (err as Error).message, category: "exception" });
                    return jsonRpcResult(req.id, {
                        content: [{ type: "text", text: `Error: ${(err as Error).message}` }],
                        isError: true,
                    });
                } finally {
                    filePaths?.cleanup();
                }
            }

            case "backlog_query_script":
            case "backlog_mutate_script": {
                const script = toolArgs.script as string | undefined;
                if (!script) {
                    return jsonRpcError(req.id, -32602, "Missing 'script' parameter");
                }
                const files = toolArgs.files as ScriptFile[] | undefined;

                const log = logToolCall(logger, { tool: toolName, input: { script }, tenant: spaceKey, loggingConfig });

                if (!options?.runScript) {
                    log.finish({ error: `${toolName} sandbox is not available`, category: "sandbox_unavailable" });
                    return jsonRpcResult(req.id, {
                        content: [{ type: "text", text: `${toolName} sandbox is not available` }],
                        isError: true,
                    });
                }

                const readOnly = toolName === "backlog_query_script" || !effectiveAccess.writable;

                const filePaths = files?.length ? materializeFiles(files) : null;
                try {
                    const resolvedScript = filePaths ? substituteFileRefs(script, filePaths.paths) : script;
                    const result = await options.runScript(resolvedScript, effectiveToken, config.script, { readOnly, files });
                    log.finish({
                        output: result.result,
                        error: result.error,
                        category: result.error ? "script_error" : undefined,
                    });
                    return jsonRpcResult(req.id, {
                        content: [{
                            type: "text",
                            text: result.error ? `Error: ${result.error}` : result.result,
                        }],
                        isError: !!result.error,
                    });
                } catch (err) {
                    log.finish({ error: (err as Error).message, category: "sandbox_error" });
                    return jsonRpcResult(req.id, {
                        content: [{ type: "text", text: `Sandbox error: ${(err as Error).message}` }],
                        isError: true,
                    });
                } finally {
                    filePaths?.cleanup();
                }
            }

            default:
                return jsonRpcError(req.id, -32602, `Unknown tool: ${toolName}`);
        }
    }

    async function handlePromptGet(
        req: JsonRpcRequest,
        token: TokenPayload,
    ): Promise<JsonRpcResponse> {
        const params = req.params as { name?: string } | undefined;
        if (params?.name !== "backlog-cli-reference") {
            return jsonRpcError(req.id, -32602, `Unknown prompt: ${params?.name}`);
        }

        let cliRefOutput: string | undefined;
        try {
            const cliRef = await executeBacklogCommand(
                'cli-ref --diff --exclude "ai,auth,config,markdown,profile,version"',
                token,
                { readOnly: true, binPath: options?.binPath },
            );
            cliRefOutput = cliRef.output;
        } catch {
            // fall through to static-only prompt
        }

        const prompt = buildCliReferencePrompt(token.space, cliRefOutput);
        return jsonRpcResult(req.id, {
            description: SKILL_PROMPT.description,
            messages: [
                {
                    role: "user",
                    content: {
                        type: "text",
                        text: prompt,
                    },
                },
            ],
        });
    }

    return app;
}

function jsonRpcResult(
    id: string | number | null | undefined,
    result: unknown,
): JsonRpcResponse {
    return { jsonrpc: "2.0", id: id ?? null, result };
}

function jsonRpcError(
    id: string | number | null | undefined,
    code: number,
    message: string,
    data?: unknown,
): JsonRpcResponse {
    return { jsonrpc: "2.0", id: id ?? null, error: { code, message, data } };
}

function buildCliReferencePrompt(space: string, cliRefOutput?: string): string {
    const mcpContext = `# Backlog CLI Reference

You have access to backlog tools which execute the Backlog CLI — a command-line interface similar to GitHub CLI (gh).

## Connection Info
- Space: ${space}

## Tools

| Tool | Purpose | Commands |
|------|---------|----------|
| \`who\` | User lookup | No query = yourself, with query = search by name/userId/email |
| \`backlog_query\` | Read-only CLI | list, view, api GET |
| \`backlog_mutate\` | Write CLI | create, update, delete, api POST/PUT/DELETE |
| \`backlog_query_script\` | Read-only Python script | backlog() helper with list/view only |
| \`backlog_mutate_script\` | Read+write Python script | backlog() helper with all commands |

## Important Rules
- **\`issue list\` requires \`-p\` (project)**: Use \`-p PROJ\` for a specific project, or \`-p all\` to search across all projects
- **\`--json\` with fields requires \`=\`**: Use \`--json=issueKey,summary\` (not \`--json issueKey,summary\`)
- **\`@me\`** refers to the current authenticated user
- **User lookup**: Use the \`who\` tool — no args for yourself, with query to search users
- **Status/Priority are numeric IDs**: Use \`project view PROJ --json=statuses\` and \`priority list --json\` to look up IDs first
- **Before creating an issue**, resolve: issue type (\`project view PROJ --json=issueTypes\`), priority (\`priority list --json\`), assignee (\`who\` tool)
- **\`issue view\` with comments**: Use \`-c default\` or \`-c all\` (not bare \`--comments\`)
- **Newlines in text**: Use \`\\n\` inside double quotes (e.g., \`-b "line1\\nline2"\`)
- For read-only analysis combining multiple API calls, use \`backlog_query_script\`
- For scripts that include write operations, use \`backlog_mutate_script\`
`;

    if (cliRefOutput) {
        return mcpContext + "\n" + cliRefOutput;
    }

    return mcpContext;
}

function extractCommandSection(reference: string, command: string): string | null {
    const q = command.toLowerCase();
    const lines = reference.split("\n");
    const sections: string[] = [];
    let capture = false;
    let depth = 0;

    for (const line of lines) {
        const headingMatch = line.match(/^(#{2,3})\s+(.+)/);
        if (headingMatch) {
            const level = headingMatch[1].length;
            const title = headingMatch[2].toLowerCase();
            if (title.includes(q)) {
                capture = true;
                depth = level;
                sections.push(line);
                continue;
            }
            if (capture && level <= depth) {
                capture = false;
            }
        }
        if (capture) {
            sections.push(line);
        }
    }

    return sections.length > 0 ? sections.join("\n").trim() : null;
}
