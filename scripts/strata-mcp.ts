#!/usr/bin/env node

/**
 * Strata MCP Client
 *
 * 为 AI Agent 提供沙盒 Shell 操作能力
 *
 * 使用方式:
 *   npx tsx scripts/strata-mcp.ts
 */

import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";

const API_BASE = process.env.STRATA_API || "http://localhost:8080";

interface Session {
  user_id: string;
  session_id: string;
  created_at?: string;
}

// 缓存已创建的 session
const sessions = new Map<string, Session>();

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
// MCP Tools 定义
// ─────────────────────────────────────────────────────────

const TOOLS = [
  {
    name: "strata_create_session",
    description: "创建或复用一个新的沙盒会话。每个用户可以有多个会话，但通常一个会话对应一次任务。",
    inputSchema: {
      type: "object",
      properties: {
        user_id: { type: "string", description: "用户标识，如 'alice'" },
        session_id: { type: "string", description: "会话标识，如 'task-001'" },
      },
      required: ["user_id", "session_id"],
    },
  },
  {
    name: "strata_exec",
    description: "在指定会话中执行一条 Shell 命令，返回命令输出。",
    inputSchema: {
      type: "object",
      properties: {
        user_id: { type: "string", description: "用户标识" },
        session_id: { type: "string", description: "会话标识" },
        command: { type: "string", description: "要执行的 Shell 命令" },
        timeout_ms: { type: "number", description: "超时毫秒数，默认 30000", default: 30000 },
      },
      required: ["user_id", "session_id", "command"],
    },
  },
  {
    name: "strata_write_file",
    description: "在沙盒会话的文件系统中创建或覆写文件。",
    inputSchema: {
      type: "object",
      properties: {
        user_id: { type: "string" },
        session_id: { type: "string" },
        path: { type: "string", description: "目标文件路径，如 '/tmp/test.py'" },
        content: { type: "string", description: "文件内容" },
      },
      required: ["user_id", "session_id", "path", "content"],
    },
  },
  {
    name: "strata_read_file",
    description: "读取沙盒会话中的文件内容。",
    inputSchema: {
      type: "object",
      properties: {
        user_id: { type: "string" },
        session_id: { type: "string" },
        path: { type: "string", description: "要读取的文件路径" },
      },
      required: ["user_id", "session_id", "path"],
    },
  },
  {
    name: "strata_close_session",
    description: "关闭并清理一个沙盒会话，释放资源。",
    inputSchema: {
      type: "object",
      properties: {
        user_id: { type: "string" },
        session_id: { type: "string" },
      },
      required: ["user_id", "session_id"],
    },
  },
  {
    name: "strata_stats",
    description: "查询当前服务状态（活跃会话数等）。",
    inputSchema: {
      type: "object",
      properties: {},
    },
  },
];

// ─────────────────────────────────────────────────────────
// Server 实现
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
    const { user_id, session_id } = args;
    const key = `${user_id}:${session_id}`;

    if (!sessions.has(key)) {
      const res = await apiCall<Session>("/api/sessions", {
        user_id,
        session_id,
      });
      sessions.set(key, res);
    }

    const s = sessions.get(key)!;
    return {
      content: [
        {
          type: "text",
          text: `Session created/retrieved: ${s.user_id}/${s.session_id}`,
        },
      ],
    };
  }

  private async handleExec(args: any) {
    const { user_id, session_id, command, timeout_ms = 30000 } = args;
    const key = `${user_id}:${session_id}`;

    // 确保 session 存在
    if (!sessions.has(key)) {
      await this.handleCreateSession({ user_id, session_id });
    }

    const res = await apiCall<{ output: string; elapsed: string }>("/api/exec", {
      user_id,
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
    const { user_id, session_id, path, content } = args;
    // 用 cat heredoc 方式写入
    const escaped = content.replace(/'/g, "'\\''");
    const cmd = `cat > '${path}' << 'STRATA_EOF'\n${content}\nSTRATA_EOF`;
    return this.handleExec({ user_id, session_id, command: cmd });
  }

  private async handleReadFile(args: any) {
    const { user_id, session_id, path } = args;
    return this.handleExec({ user_id, session_id, command: `cat ${path}` });
  }

  private async handleCloseSession(args: any) {
    const { user_id, session_id } = args;
    const key = `${user_id}:${session_id}`;

    await fetch(`${API_BASE}/api/sessions/${user_id}/${session_id}`, {
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

// 启动
new StrataMCPServer().start().catch((err) => {
  console.error("Fatal:", err);
  process.exit(1);
});
