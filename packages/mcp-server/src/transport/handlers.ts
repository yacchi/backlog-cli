import { Hono } from "hono";
import type { McpServerConfig, McpTenant } from "../config/schema.js";
import { jweAuth, getAuthContext } from "../middleware/jwe-auth.js";
import { executeBacklogCommand } from "../tools/backlog.js";

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

const TOOLS = [
    {
        name: "backlog",
        description:
            "Execute a backlog CLI command (similar to gh CLI). Returns command output. Use --json flag for structured output, --jq for filtering.",
        inputSchema: {
            type: "object" as const,
            properties: {
                args: {
                    type: "string" as const,
                    description:
                        "CLI arguments (e.g., 'issue list --project PROJ -L 20 --json issueKey,summary,status')",
                },
            },
            required: ["args"],
        },
    },
    {
        name: "run_script",
        description:
            "Execute Python in a sandboxed environment with backlog() helper. Use for chaining multiple CLI calls, filtering, aggregating, or computing derived data in one round trip. Python standard library available (json, datetime, re, collections, etc.). Dangerous modules (os, subprocess, socket, etc.) are blocked.",
        inputSchema: {
            type: "object" as const,
            properties: {
                script: {
                    type: "string" as const,
                    description:
                        "Python code. Available: backlog(args) returns parsed JSON (runs CLI with --json). The last expression is the return value.",
                },
            },
            required: ["script"],
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
    options?: { binPath?: string; runScript?: (script: string, token: import("../crypto/jwe.js").TokenPayload, tenant: McpTenant | undefined) => Promise<{ result: string; error?: string }> },
): Hono {
    const app = new Hono();
    const auth = jweAuth(config.token_key, config.token_key_prev);

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
        // SSE endpoint — for now return 200 with empty event stream
        // Full SSE implementation for server-initiated notifications is Phase 4
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
        if (tenant?.script?.enabled) {
            return TOOLS;
        }
        return TOOLS.filter((t) => t.name !== "run_script");
    }

    async function handleToolCall(
        req: JsonRpcRequest,
        token: import("../crypto/jwe.js").TokenPayload,
        tenant: McpTenant | undefined,
    ): Promise<JsonRpcResponse> {
        const params = req.params as { name?: string; arguments?: Record<string, unknown> } | undefined;
        const toolName = params?.name;
        const toolArgs = params?.arguments ?? {};

        switch (toolName) {
            case "backlog": {
                const args = toolArgs.args as string | undefined;
                if (!args) {
                    return jsonRpcError(req.id, -32602, "Missing 'args' parameter");
                }

                try {
                    const result = await executeBacklogCommand(
                        args,
                        token,
                        tenant,
                        options?.binPath,
                    );
                    return jsonRpcResult(req.id, {
                        content: [
                            {
                                type: "text",
                                text: result.output,
                            },
                        ],
                        isError: result.exitCode !== 0,
                    });
                } catch (err) {
                    return jsonRpcResult(req.id, {
                        content: [
                            {
                                type: "text",
                                text: `Error: ${(err as Error).message}`,
                            },
                        ],
                        isError: true,
                    });
                }
            }

            case "run_script": {
                const script = toolArgs.script as string | undefined;
                if (!script) {
                    return jsonRpcError(req.id, -32602, "Missing 'script' parameter");
                }

                if (!tenant?.script?.enabled) {
                    return jsonRpcResult(req.id, {
                        content: [
                            {
                                type: "text",
                                text: "run_script is not enabled for this tenant",
                            },
                        ],
                        isError: true,
                    });
                }

                if (options?.runScript) {
                    try {
                        const result = await options.runScript(script, token, tenant);
                        return jsonRpcResult(req.id, {
                            content: [
                                {
                                    type: "text",
                                    text: result.error
                                        ? `Error: ${result.error}`
                                        : result.result,
                                },
                            ],
                            isError: !!result.error,
                        });
                    } catch (err) {
                        return jsonRpcResult(req.id, {
                            content: [
                                {
                                    type: "text",
                                    text: `Sandbox error: ${(err as Error).message}`,
                                },
                            ],
                            isError: true,
                        });
                    }
                }

                return jsonRpcResult(req.id, {
                    content: [
                        {
                            type: "text",
                            text: "run_script sandbox is not available",
                        },
                    ],
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

You have access to the \`backlog\` tool which executes the Backlog CLI — a command-line interface similar to GitHub CLI (gh).

## Connection Info
- Space: ${space}
- Domain: ${domain}

## Common Commands

### Issues
\`\`\`
backlog issue list --project PROJ -L 20 --json issueKey,summary,status,assignee
backlog issue view PROJ-42 --json
backlog issue view PROJ-42 --comments
backlog issue create --project PROJ --summary "Title" --description "Body" --type Bug
backlog issue update PROJ-42 --status InProgress --assignee @me
backlog issue list --project PROJ --assignee @me --status Open,InProgress --json issueKey,summary,dueDate
\`\`\`

### Projects
\`\`\`
backlog project list --json projectKey,name,id
backlog project view PROJ --json
\`\`\`

### Wiki
\`\`\`
backlog wiki list --project PROJ --json id,name
backlog wiki view 12345
\`\`\`

### Notifications
\`\`\`
backlog notification list -L 10 --json
\`\`\`

### Activity
\`\`\`
backlog activity list --project PROJ -L 20 --json
\`\`\`

### Raw API Access
\`\`\`
backlog api /api/v2/issues/count -X GET -f projectId[]=12345
backlog api /api/v2/space --json
\`\`\`

## Output Flags
- \`--json field1,field2\`: Output specific fields as JSON
- \`--jq '.[] | select(.status.name == "Open")'\`: Filter with jq
- \`--format '{{.issueKey}}: {{.summary}}'\`: Go template format
- \`-L N\`: Limit results (0 = all)

## Tips
- Use \`--json\` for structured data, easier to process
- \`@me\` refers to the current user
- Commands follow the same patterns as \`gh\` (GitHub CLI)
- For complex queries combining multiple API calls, use the \`run_script\` tool with Python
`;
}
