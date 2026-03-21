#!/usr/bin/env bun

/**
 * Strata MCP Test Script
 *
 * Tests the MCP server via HTTP JSON-RPC.
 *
 * Usage:
 *   bun run scripts/test-mcp.ts
 *
 * Environment:
 *   STRATA_MCP   MCP server URL (default: http://localhost:2280/mcp/)
 */

const MCP_URL = process.env.STRATA_MCP || "http://localhost:2280/mcp/";
const USER = "testuser";
const SESSION = `test-${Date.now()}`;

// Colors
const GREEN = "\x1b[32m";
const RED = "\x1b[31m";
const YELLOW = "\x1b[33m";
const NC = "\x1b[0m";

let passed = 0;
let failed = 0;
let nextId = 1;
let sessionId = "";

function jsonRequest(method: string, params?: any): any {
  return { jsonrpc: "2.0", id: nextId++, method, params };
}

async function mcpRequest(method: string, params?: any, headers?: Record<string, string>): Promise<any> {
  const req = jsonRequest(method, params);

  const reqHeaders: Record<string, string> = {
    "Content-Type": "application/json",
    ...headers,
  };

  if (sessionId) {
    reqHeaders["Mcp-Session-Id"] = sessionId;
  }

  const res = await fetch(MCP_URL, {
    method: "POST",
    headers: reqHeaders,
    body: JSON.stringify(req),
  });

  // Save session ID from response
  const newSessionId = res.headers.get("Mcp-Session-Id");
  if (newSessionId) {
    sessionId = newSessionId;
  }

  if (!res.ok) {
    throw new Error(`HTTP ${res.status}: ${await res.text()}`);
  }

  return res.json();
}

async function test(name: string, fn: () => Promise<void>): Promise<void> {
  process.stdout.write(`${YELLOW}[TEST]${NC} ${name}... `);
  try {
    await fn();
    console.log(`${GREEN}OK${NC}`);
    passed++;
  } catch (err: any) {
    console.log(`${RED}FAIL${NC}: ${err.message}`);
    failed++;
  }
}

async function main() {
  console.error("=== Strata MCP Test ===");
  console.error(`URL: ${MCP_URL}`);
  console.error(`User: ${USER}`);
  console.error(`Session: ${SESSION}`);
  console.error("");

  // Test 1: Initialize
  await test("initialize", async () => {
    const resp = await mcpRequest("initialize", {
      protocolVersion: "2024-11-05",
      capabilities: {},
      clientInfo: { name: "test-client", version: "1.0.0" },
    });
    if (!resp.result) {
      throw new Error("No result");
    }
  });

  // Test 2: List tools
  await test("tools/list", async () => {
    const resp = await mcpRequest("tools/list");
    if (!resp.result?.tools?.length) {
      throw new Error("No tools returned");
    }
    console.error(`  Found ${resp.result.tools.length} tools`);
  });

  // Test 3: Create session
  await test("create_session", async () => {
    const resp = await mcpRequest("tools/call", {
      name: "create_session",
      arguments: { owner_id: USER, session_id: SESSION },
    });
    if (!resp.result?.content?.[0]?.text) {
      throw new Error("No session created");
    }
    console.error(`  ${resp.result.content[0].text}`);
  });

  // Test 4: Exec pwd
  await test("exec pwd", async () => {
    const resp = await mcpRequest("tools/call", {
      name: "exec",
      arguments: { owner_id: USER, session_id: SESSION, command: "pwd" },
    });
    if (!resp.result?.content?.[0]?.text) {
      throw new Error("No output");
    }
    console.error(`  ${resp.result.content[0].text}`);
  });

  // Test 4b: Exec pwd via header-based identity (X-Owner-Id, X-Session-Id)
  await test("exec pwd via header identity", async () => {
    const resp = await mcpRequest("tools/call", {
      name: "exec",
      arguments: { command: "pwd" },
    }, {
      "X-Owner-Id": USER,
      "X-Session-Id": SESSION,
    });
    if (!resp.result?.content?.[0]?.text) {
      throw new Error("No output");
    }
    console.error(`  ${resp.result.content[0].text}`);
  });

  // Test 5: Stats
  await test("stats", async () => {
    const resp = await mcpRequest("tools/call", {
      name: "stats",
      arguments: {},
    });
    if (!resp.result?.content?.[0]?.text) {
      throw new Error("No stats");
    }
    console.error(`  ${resp.result.content[0].text}`);
  });

  // Test 6: Close session
  await test("close_session", async () => {
    const resp = await mcpRequest("tools/call", {
      name: "close_session",
      arguments: { owner_id: USER, session_id: SESSION },
    });
    if (!resp.result?.content) {
      throw new Error("Close failed");
    }
  });

  // Summary
  console.error("");
  console.error(`=== Results: ${passed} passed, ${failed} failed ===`);

  if (failed > 0) {
    process.exit(1);
  }
}

main().catch(console.error);
