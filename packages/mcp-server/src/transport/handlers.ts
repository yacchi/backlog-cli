import { Hono } from "hono";
import type { McpServerConfig, McpTenant } from "../config/schema.js";
import { jweAuth, getAuthContext } from "../middleware/jwe-auth.js";
import { isReadOnlyCommand } from "../middleware/cli-access.js";
import { executeBacklogCommand } from "../tools/backlog.js";
import { logToolCall } from "../logging/logger.js";

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
            `Execute a read-only backlog CLI command (similar to gh CLI). Supports list, view, and GET API calls. Use --json flag for structured output, --jq for filtering. For user lookup, use the who tool. For write operations, use backlog_mutate. ${CLI_REF_HINT}`,
        inputSchema: {
            type: "object" as const,
            properties: {
                args: {
                    type: "string" as const,
                    description:
                        "CLI arguments for read-only commands (e.g., 'issue list -p PROJ -L 20 --json issueKey,summary,status')",
                },
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
            `Execute a read-only Python analysis script with backlog() helper. Only read commands (list, view, GET API calls) are allowed — write operations are blocked. Use for multi-step data retrieval, aggregation, filtering, and reporting across multiple API calls in one round trip. Python standard library available (json, datetime, re, collections, etc.). ${CLI_REF_HINT}`,
        inputSchema: {
            type: "object" as const,
            properties: {
                script: {
                    type: "string" as const,
                    description:
                        "Python code. backlog(args) runs a CLI command and returns the result (parsed JSON with --json, or text). print() output is returned to the user; if no print(), the last expression value is used as fallback. Read-only commands only.",
                },
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
            `Execute a backlog CLI command that modifies data (create, update, delete). For read-only commands, use backlog_query. ${CLI_REF_HINT}`,
        inputSchema: {
            type: "object" as const,
            properties: {
                args: {
                    type: "string" as const,
                    description:
                        "CLI arguments (e.g., 'issue create -p PROJ -t \"Title\" --type Bug')",
                },
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
            `Execute Python in a sandboxed environment with backlog() helper. Use for chaining multiple CLI calls including write operations (create, update, delete). Python standard library available (json, datetime, re, collections, etc.). Dangerous modules (os, subprocess, socket, etc.) are blocked. ${CLI_REF_HINT}`,
        inputSchema: {
            type: "object" as const,
            properties: {
                script: {
                    type: "string" as const,
                    description:
                        "Python code. backlog(args) runs a CLI command and returns the result (parsed JSON with --json, or text). print() output is returned to the user; if no print(), the last expression value is used as fallback.",
                },
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
    options?: { binPath?: string; runScript?: (script: string, token: import("../crypto/jwe.js").TokenPayload, tenant: McpTenant | undefined, options?: { readOnly?: boolean }) => Promise<{ result: string; error?: string }> },
): Hono {
    const app = new Hono();
    const resourceMetadataUrl = `${config.base_url}/.well-known/oauth-protected-resource`;
    const auth = jweAuth(config.token_key, config.token_key_prev, resourceMetadataUrl);

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
        const tenantKey = `${token.space}.${token.domain}`;
        const tenant = config.tenants[tenantKey];

        const result = await handleMethod(req, token, tenant);
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

    async function handleMethod(
        req: JsonRpcRequest,
        token: import("../crypto/jwe.js").TokenPayload,
        tenant: McpTenant | undefined,
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
                });

            case "notifications/initialized":
                return jsonRpcResult(req.id, {});

            case "tools/list":
                return jsonRpcResult(req.id, {
                    tools: getAvailableTools(tenant),
                });

            case "tools/call":
                return handleToolCall(req, token, tenant);

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

    function getAvailableTools(tenant: McpTenant | undefined) {
        const tools = [HELP_TOOL, ...QUERY_TOOLS, ...MUTATION_TOOLS];
        if (tenant?.script?.enabled) {
            return tools;
        }
        return tools.filter((t) => t.name === "who" || t.name === "backlog_query" || t.name === "backlog_mutate" || t.name === "backlog_help");
    }

    async function handleToolCall(
        req: JsonRpcRequest,
        token: import("../crypto/jwe.js").TokenPayload,
        tenant: McpTenant | undefined,
    ): Promise<JsonRpcResponse> {
        const params = req.params as { name?: string; arguments?: Record<string, unknown> } | undefined;
        const toolName = params?.name;
        const toolArgs = params?.arguments ?? {};
        const tenantKey = `${token.space}.${token.domain}`;

        switch (toolName) {
            case "backlog_help": {
                const command = (toolArgs.command as string | undefined)?.trim();
                const full = buildCliReferencePrompt(token.space, token.domain);
                let text = full;
                if (command) {
                    const sections = extractCommandSection(full, command);
                    text = sections ?? `No specific help found for "${command}". Here is the full reference:\n\n${full}`;
                }
                return jsonRpcResult(req.id, {
                    content: [{ type: "text", text }],
                });
            }

            case "who": {
                const query = (toolArgs.query as string | undefined)?.trim();
                const log = logToolCall({ tool: "who", input: { query }, tenant: tenantKey });
                try {
                    if (!query) {
                        const result = await executeBacklogCommand(
                            "whoami --json",
                            token,
                            tenant,
                            options?.binPath,
                        );
                        log.finish({ output: result.output, error: result.exitCode !== 0 ? result.output : undefined });
                        return jsonRpcResult(req.id, {
                            content: [{ type: "text", text: result.output }],
                            isError: result.exitCode !== 0,
                        });
                    }

                    const result = await executeBacklogCommand(
                        "api /api/v2/users",
                        token,
                        tenant,
                        options?.binPath,
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

            case "backlog_query": {
                const args = toolArgs.args as string | undefined;
                if (!args) {
                    return jsonRpcError(req.id, -32602, "Missing 'args' parameter");
                }

                const log = logToolCall({ tool: "backlog_query", input: { args }, tenant: tenantKey });

                if (!isReadOnlyCommand(args)) {
                    log.finish({ error: "rejected: not a read-only command", category: "access_denied" });
                    return jsonRpcResult(req.id, {
                        content: [{
                            type: "text",
                            text: "backlog_query only accepts read-only commands (list, view, GET). Use backlog_mutate for write operations.",
                        }],
                        isError: true,
                    });
                }

                try {
                    const result = await executeBacklogCommand(
                        args,
                        token,
                        tenant,
                        options?.binPath,
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
                }
            }

            case "backlog_mutate": {
                const args = toolArgs.args as string | undefined;
                if (!args) {
                    return jsonRpcError(req.id, -32602, "Missing 'args' parameter");
                }

                const log = logToolCall({ tool: "backlog_mutate", input: { args }, tenant: tenantKey });

                try {
                    const result = await executeBacklogCommand(
                        args,
                        token,
                        tenant,
                        options?.binPath,
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
                }
            }

            case "backlog_query_script":
            case "backlog_mutate_script": {
                const script = toolArgs.script as string | undefined;
                if (!script) {
                    return jsonRpcError(req.id, -32602, "Missing 'script' parameter");
                }

                const log = logToolCall({ tool: toolName, input: { script }, tenant: tenantKey });

                if (!tenant?.script?.enabled) {
                    log.finish({ error: `${toolName} is not enabled for this tenant`, category: "tenant_disabled" });
                    return jsonRpcResult(req.id, {
                        content: [{ type: "text", text: `${toolName} is not enabled for this tenant` }],
                        isError: true,
                    });
                }

                const readOnly = toolName === "backlog_query_script";

                if (options?.runScript) {
                    try {
                        const result = await options.runScript(script, token, tenant, { readOnly });
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
                    }
                }

                log.finish({ error: `${toolName} sandbox is not available`, category: "sandbox_unavailable" });
                return jsonRpcResult(req.id, {
                    content: [{ type: "text", text: `${toolName} sandbox is not available` }],
                    isError: true,
                });
            }

            default:
                return jsonRpcError(req.id, -32602, `Unknown tool: ${toolName}`);
        }
    }

    async function handlePromptGet(
        req: JsonRpcRequest,
        token: import("../crypto/jwe.js").TokenPayload,
    ): Promise<JsonRpcResponse> {
        const params = req.params as { name?: string } | undefined;
        if (params?.name !== "backlog-cli-reference") {
            return jsonRpcError(req.id, -32602, `Unknown prompt: ${params?.name}`);
        }

        const prompt = buildCliReferencePrompt(token.space, token.domain);
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

function buildCliReferencePrompt(space: string, domain: string): string {
    return `# Backlog CLI Reference

You have access to backlog tools which execute the Backlog CLI — a command-line interface similar to GitHub CLI (gh).

## Connection Info
- Space: ${space}
- Domain: ${domain}

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
- **\`--json\`**: Use \`--json\` for all fields, or \`--json field1,field2\` for specific fields
- **\`@me\`** refers to the current authenticated user
- **User lookup**: Use the \`who\` tool — no args for yourself, with query to search users

## Common Commands

### Issues (Query — use \`backlog_query\`)
\`\`\`
# Basic listing and viewing
issue list -p PROJ -L 20 --json issueKey,summary,status,assignee
issue view PROJ-42 --json
issue view PROJ-42 --comments

# Filter by assignee
issue list -p all --assignee @me --status Open,InProgress --json

# My involved issues (assigned + commented + created)
# Use --involved to find issues where the user is involved, not just assigned
# Add --include-commented to also include issues where the user commented
issue list -p all --involved @me --include-commented --state all --json
issue list -p all --involved @me --include-commented --updated-since 2025-01-01 -L 50 --json

# Date-based search
issue list -p all --updated-since 2025-01-01 -L 50 --json
\`\`\`

### Issues (Mutation — use \`backlog_mutate\`)
\`\`\`
issue create -p PROJ -t "Title" -b "Body" --type Bug
issue update PROJ-42 --status InProgress --assignee @me
\`\`\`

### Projects
\`\`\`
project list --json
project view PROJ --json
\`\`\`

### Wiki
\`\`\`
wiki list -p PROJ --json
wiki view 12345 --json
\`\`\`

### Notifications
\`\`\`
notification list -L 10 --json
\`\`\`

### Activity
\`\`\`
activity list -p PROJ -L 20 --json
\`\`\`

### Raw API Access
\`\`\`
api /api/v2/issues/count -X GET -F projectId[]=12345
api /api/v2/space --json
\`\`\`

## Output Flags
- \`--json\`: Output all fields as JSON
- \`--json field1,field2\`: Output specific fields as JSON
- \`--jq '.[] | select(.status.name == "Open")'\`: Filter with jq
- \`--format '{{.issueKey}}: {{.summary}}'\`: Go template format
- \`-L N\`: Limit results (0 = all)

## Tips
- Always use \`--json\` for structured data — easier to process
- For read-only analysis combining multiple API calls, use \`backlog_query_script\`
- For scripts that include write operations, use \`backlog_mutate_script\`
`;
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
