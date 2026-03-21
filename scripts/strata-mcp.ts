#!/usr/bin/env node

/**
 * Strata API wrap as a StdIO MCP Server
 *
 * Provides sandboxed shell operations for AI Agents via MCP protocol.
 * Communicates with Strata server over HTTP.
 *
 * Environment Variables:
 *   STRATA_API    Strata server URL (default: http://localhost:2280)
 *   STRATA_UID    Default user ID (if set, owner_id param becomes optional)
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

interface Session {
  owner_id: string;
  session_id: string;
  created_at?: string;
}

// Cache created sessions
const sessions = new Map<string, Session>();

// Dynamically build tools based on STRATA_UID presence
function buildTools() {
  const userIdProp = STRATA_UID
    ? { description: `User ID (fixed to ${STRATA_UID})` }
    : { type: "string", description: "User identifier, e.g., 'alice'" };

  const baseProps = {
    owner_id: { ...userIdProp },
    session_id: { type: "string", description: "Session identifier, e.g., 'task-001'" },
  };

  const requireUserId = !STRATA_UID;

  return [
    {
      name: "strata_create_session",
      description: `Create or reuse a sandbox session${STRATA_UID ? ` (user: ${STRATA_UID})` : ""}`,
      inputSchema: {
        type: "object",
        properties: requireUserId ? baseProps : { session_id: baseProps.session_id },
        required: requireUserId ? ["owner_id", "session_id"] : ["session_id"],
      },
    },
    {
      name: "strata_exec",
      description: "Execute a shell command in the specified session and return the output.",
      inputSchema: {
        type: "object",
        properties: {
          ...(requireUserId ? { owner_id: baseProps.owner_id } : {}),
          session_id: baseProps.session_id,
          command: { type: "string", description: "Shell command to execute" },
          timeout_ms: { type: "number", description: "Timeout in milliseconds, default 30000", default: 30000 },
        },
        required: [...(requireUserId ? ["owner_id"] : []), "session_id", "command"],
      },
    },
    {
      name: "strata_write_file",
      description: "Create or overwrite a file in the sandbox session.",
      inputSchema: {
        type: "object",
        properties: {
          ...(requireUserId ? { owner_id: baseProps.owner_id } : {}),
          session_id: baseProps.session_id,
          path: { type: "string", description: "Target file path, e.g., '/tmp/test.py'" },
          content: { type: "string", description: "File content" },
        },
        required: [...(requireUserId ? ["owner_id"] : []), "session_id", "path", "content"],
      },
    },
    {
      name: "strata_read_file",
      description: "Read file content from the sandbox session.",
      inputSchema: {
        type: "object",
        properties: {
          ...(requireUserId ? { owner_id: baseProps.owner_id } : {}),
          session_id: baseProps.session_id,
          path: { type: "string", description: "File path to read" },
        },
        required: [...(requireUserId ? ["owner_id"] : []), "session_id", "path"],
      },
    },
    {
      name: "strata_close_session",
      description: "Close and cleanup a sandbox session to release resources.",
      inputSchema: {
        type: "object",
        properties: {
          ...(requireUserId ? { owner_id: baseProps.owner_id } : {}),
          session_id: baseProps.session_id,
        },
        required: [...(requireUserId ? ["owner_id"] : []), "session_id"],
      },
    },
    {
      name: "strata_stats",
      description: "Query service status (active sessions, etc.).",
      inputSchema: {
        type: "object",
        properties: {},
      },
    },
  ];
}

// Get actual owner_id (prefer args, fallback to env var)
function getUserId(argsUserId?: string): string {
  if (argsUserId && argsUserId.trim()) {
    return argsUserId;
  }
  return STRATA_UID;
}

// Dynamically generated tools list
const TOOLS = buildTools();

async function apiCall<T>(path: string, body: any): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const err = await res.text();
    throw new Error(`strata API error: ${res.status} ${err}`);
  }
  return res.json();
}

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
          case "strata_create_session":
            return this.handleCreateSession(args);
          case "strata_exec":
            return this.handleExec(args);
          case "strata_write_file":
            return this.handleWriteFile(args);
          case "strata_read_file":
            return this.handleReadFile(args);
          case "strata_close_session":
            return this.handleCloseSession(args);
          case "strata_stats":
            return this.handleStats();
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

  private async handleCreateSession(args: any) {
    const owner_id = getUserId(args.owner_id);
    const { session_id } = args;
    const key = `${owner_id}:${session_id}`;

    if (!sessions.has(key)) {
      const res = await apiCall<Session>("/api/sessions", {
        owner_id,
        session_id,
      });
      sessions.set(key, res);
    }

    const s = sessions.get(key)!;
    return {
      content: [
        {
          type: "text",
          text: `Session created/retrieved: ${s.owner_id}/${s.session_id}`,
        },
      ],
    };
  }

  private async handleExec(args: any) {
    const owner_id = getUserId(args.owner_id);
    const { session_id, command, timeout_ms = 30000 } = args;
    const key = `${owner_id}:${session_id}`;

    // Ensure session exists
    if (!sessions.has(key)) {
      await this.handleCreateSession({ owner_id, session_id });
    }

    const res = await apiCall<{ output: string; elapsed: string }>("/api/exec", {
      owner_id,
      session_id,
      command,
      timeout_ms,
    });

    return {
      content: [
        {
          type: "text",
          text: res.output || "(empty output)",
        },
      ],
    };
  }

  private async handleWriteFile(args: any) {
    const owner_id = getUserId(args.owner_id);
    const { session_id, path, content } = args;
    const cmd = `cat > '${path}' << 'STRATA_EOF'\n${content}\nSTRATA_EOF`;
    return this.handleExec({ owner_id, session_id, command: cmd });
  }

  private async handleReadFile(args: any) {
    const owner_id = getUserId(args.owner_id);
    const { session_id, path } = args;
    return this.handleExec({ owner_id, session_id, command: `cat ${path}` });
  }

  private async handleCloseSession(args: any) {
    const owner_id = getUserId(args.owner_id);
    const { session_id } = args;
    const key = `${owner_id}:${session_id}`;

    await fetch(`${API_BASE}/api/sessions/${owner_id}/${session_id}`, {
      method: "DELETE",
    });
    sessions.delete(key);

    return {
      content: [{ type: "text", text: `Session ${key} closed` }],
    };
  }

  private async handleStats() {
    const res = await fetch(`${API_BASE}/api/stats`);
    const stats = await res.json();
    return {
      content: [
        {
          type: "text",
          text: JSON.stringify(stats, null, 2),
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
