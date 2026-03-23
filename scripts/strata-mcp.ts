#!/usr/bin/env node

/**
 * Strata API wrap as a StdIO MCP Server
 *
 * Provides sandboxed shell operations for AI Agents via MCP protocol.
 * Communicates with Strata server over HTTP.
 *
 * Environment Variables:
 *   STRATA_API    Strata server URL (default: http://localhost:2280)
 *   STRATA_UID    Default user ID (optional, identity from header or args)
 *
 * Usage:
 *   npx tsx scripts/strata-mcp.ts
 *   # or with fixed user:
 *   STRATA_UID=alice npx tsx scripts/strata-mcp.ts
 *
 * Protocol: STDIO (MCP standard)
 *   The server communicates with the AI agent via stdin/stdout.
 */

import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";

const API_BASE = process.env.STRATA_API || "http://localhost:2280";
const STRATA_UID = process.env.STRATA_UID?.trim() || "";

// Dynamically build tools based on STRATA_UID presence
function buildTools() {
  const ownerIdProp = STRATA_UID
    ? { description: `User ID (fixed to ${STRATA_UID})` }
    : { type: "string", description: "User identifier" };

  return [
    {
      name: "strata_exec",
      description: "Execute a shell command in sandbox session (auto-creates session)",
      inputSchema: {
        type: "object",
        properties: {
          owner_id: { ...ownerIdProp },
          session_id: { type: "string", description: "Session identifier" },
          command: { type: "string", description: "Shell command to execute" },
          timeout_ms: { type: "number", description: "Timeout in milliseconds", default: 30000 },
        },
        required: ["command"],
      },
    },
    {
      name: "strata_stats",
      description: "Query service status (active sessions, etc.)",
      inputSchema: {
        type: "object",
        properties: {},
      },
    },
  ];
}

async function apiCall<T>(path: string, body?: any): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    method: body ? "POST" : "GET",
    headers: { "Content-Type": "application/json" },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const err = await res.text();
    throw new Error(`strata API error: ${res.status} ${err}`);
  }
  return res.json();
}

// Dynamically generated tools list
const TOOLS = buildTools();

// ─────────────────────────────────────────────────────────
// Server Implementation
// ─────────────────────────────────────────────────────────

class StrataMCPServer {
  private server: Server;

  constructor() {
    this.server = new Server(
      { name: "strata", version: "0.1.0" },
      { capabilities: { tools: {} } }
    );

    this.server.setRequestHandler(ListToolsRequestSchema, async () => ({
      tools: TOOLS,
    }));

    this.server.setRequestHandler(CallToolRequestSchema, async (request) => {
      const { name, arguments: args } = request.params;

      try {
        switch (name) {
          case "strata_exec":
            return this.handleExec(args);
          default:
            throw new Error(`Unknown tool: ${name}`);
        }
      } catch (err: any) {
        return {
          content: [{ type: "text", text: `Error: ${err.message}` }],
          isError: true,
        };
      }
    });
  }

  async start() {
    const transport = new StdioServerTransport();
    await this.server.connect(transport);
    console.warn("Strata MCP server started");
  }

  // ─────────────────────────────────────────
  // Tool Handlers
  // ─────────────────────────────────────────

  private async handleExec(args: any) {
    const owner_id = args.owner_id || STRATA_UID;
    const session_id = args.session_id;
    const { command, timeout_ms = 30000 } = args;

    if (!owner_id || !session_id) {
      throw new Error("owner_id and session_id are required");
    }

    const res = await apiCall<{ output: string; elapsed: string }>(
      `/api/sessions/${owner_id}/${session_id}/exec`,
      { command, timeout_ms }
    );

    return {
      content: [
        {
          type: "text",
          text: res.output || "(empty output)",
        },
      ],
    };
  }

}

// Start the server
new StrataMCPServer().start().catch((err) => {
  console.error("Fatal:", err);
  process.exit(1);
});
