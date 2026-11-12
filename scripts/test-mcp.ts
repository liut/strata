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
const FILE_PATH = `mcp-file-${Date.now()}.txt`;
const DUPLICATE_FILE_PATH = `mcp-duplicate-${Date.now()}.txt`;

// Colors
const GREEN = "\x1b[32m";
const RED = "\x1b[31m";
const YELLOW = "\x1b[33m";
const NC = "\x1b[0m";

let passed = 0;
let failed = 0;
let nextId = 1;
let mcpSessionId = "";

function jsonRequest(method: string, params?: any): any {
  return { jsonrpc: "2.0", id: nextId++, method, params };
}

async function mcpRequest(method: string, params?: any, headers?: Record<string, string>): Promise<any> {
  const req = jsonRequest(method, params);

  const reqHeaders: Record<string, string> = {
    "Content-Type": "application/json",
    ...headers,
  };

  // MCP session handling
  if (mcpSessionId) {
    reqHeaders["MCP-Session-ID"] = mcpSessionId;
  }

  const res = await fetch(MCP_URL, {
    method: "POST",
    headers: reqHeaders,
    body: JSON.stringify(req),
  });

  // Save MCP session ID from response
  const newSessionId = res.headers.get("MCP-Session-ID");
  if (newSessionId) {
    mcpSessionId = newSessionId;
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
  console.warn("=== Strata MCP Test ===");
  console.warn(`URL: ${MCP_URL}`);
  console.warn(`User: ${USER}`);
  console.warn(`Session: ${SESSION}`);
  console.warn("");

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
    const names = (resp.result?.tools || []).map((t: any) => t.name).sort();
    if (names.join(",") !== "edit,exec,read,write") {
      throw new Error(`Unexpected tools: ${names.join(", ")}`);
    }
    console.error(`  Found ${names.length} tools: ${names.join(", ")}`);
  });

  // Test 3: Exec pwd (with identity in args)
  await test("exec pwd (args identity)", async () => {
    const resp = await mcpRequest("tools/call", {
      name: "exec",
      arguments: { owner_id: USER, session_id: SESSION, command: "pwd" },
    });
    if (!resp.result?.content?.[0]?.text) {
      throw new Error("No output");
    }
    console.error(`  ${resp.result.content[0].text}`);
  });

  // Test 4: Exec pwd via header-based identity
  await test("exec pwd (header identity)", async () => {
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

  // Test 5: Exec with timeout
  await test("exec with timeout", async () => {
    const resp = await mcpRequest("tools/call", {
      name: "exec",
      arguments: { owner_id: USER, session_id: SESSION, command: "echo hello", timeout_ms: 5000 },
    });
    if (!resp.result?.content?.[0]?.text) {
      throw new Error("No output");
    }
    console.error(`  ${resp.result.content[0].text}`);
  });

  await test("write creates a file", async () => {
    const resp = await mcpRequest("tools/call", {
      name: "write",
      arguments: {
        owner_id: USER,
        session_id: SESSION,
        path: FILE_PATH,
        content: "- first\n- second\n",
      },
    });
    if (resp.result?.isError || resp.result?.structuredContent?.created !== true) {
      throw new Error(resp.result?.content?.[0]?.text || "write failed");
    }
  });

  let readText = "";
  await test("read returns exact text", async () => {
    const resp = await mcpRequest("tools/call", {
      name: "read",
      arguments: { owner_id: USER, session_id: SESSION, path: FILE_PATH },
    });
    readText = resp.result?.content?.[0]?.text || "";
    if (resp.result?.isError || readText !== "- first\n- second\n") {
      throw new Error(`Unexpected read result: ${readText}`);
    }
  });

  await test("edit applies read text", async () => {
    const resp = await mcpRequest("tools/call", {
      name: "edit",
      arguments: {
        owner_id: USER,
        session_id: SESSION,
        path: `/root/${FILE_PATH}`,
        edits: [{ oldText: readText, newText: "- first\n- second\n- third\n" }],
      },
    });
    if (resp.result?.isError || !resp.result?.structuredContent?.patch) {
      throw new Error(resp.result?.content?.[0]?.text || "edit failed");
    }
  });

  await test("read returns edited text", async () => {
    const resp = await mcpRequest("tools/call", {
      name: "read",
      arguments: { owner_id: USER, session_id: SESSION, path: FILE_PATH },
    });
    if (resp.result?.isError || resp.result?.content?.[0]?.text !== "- first\n- second\n- third\n") {
      throw new Error(`Unexpected edited text: ${resp.result?.content?.[0]?.text}`);
    }
  });

  await test("file tools reject traversal", async () => {
    const resp = await mcpRequest("tools/call", {
      name: "read",
      arguments: { owner_id: USER, session_id: SESSION, path: "../outside.txt" },
    });
    if (!resp.result?.isError || !String(resp.result?.content?.[0]?.text).includes("Path escapes session root")) {
      throw new Error("Traversal was not rejected");
    }
  });

  await test("edit rejects duplicate matches", async () => {
    const writeResp = await mcpRequest("tools/call", {
      name: "write",
      arguments: {
        owner_id: USER,
        session_id: SESSION,
        path: DUPLICATE_FILE_PATH,
        content: "same\nsame\n",
      },
    });
    if (writeResp.result?.isError) {
      throw new Error(writeResp.result?.content?.[0]?.text || "write failed");
    }
    const resp = await mcpRequest("tools/call", {
      name: "edit",
      arguments: {
        owner_id: USER,
        session_id: SESSION,
        path: DUPLICATE_FILE_PATH,
        edits: [{ oldText: "same", newText: "changed" }],
      },
    });
    if (!resp.result?.isError || !String(resp.result?.content?.[0]?.text).includes("Matching locations")) {
      throw new Error("Duplicate match was not rejected");
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
