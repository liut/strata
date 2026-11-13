#!/usr/bin/env bun

/**
 * Strata MCP Test Script
 *
 * Tests the MCP server via HTTP JSON-RPC.
 *
 * Usage:
 *   bun run scripts/test-mcp.ts
 *   STRATA_MCP_LEGACY=1 bun run scripts/test-mcp.ts
 *
 * Environment:
 *   STRATA_MCP   MCP server URL (default: http://localhost:2280/mcp/)
 *   STRATA_MCP_LEGACY  Use the legacy initialize/session flow (default: false)
 */

const MCP_URL = process.env.STRATA_MCP || "http://localhost:2280/mcp/";
const LEGACY = process.env.STRATA_MCP_LEGACY === "1";
const MODERN_PROTOCOL_VERSION = "2026-07-28";
const LEGACY_PROTOCOL_VERSION = "2025-11-25";
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
let legacySessionId = "";

function jsonRequest(method: string, params?: any): any {
  return { jsonrpc: "2.0", id: nextId++, method, params };
}

function modernMeta(): Record<string, any> {
  return {
    "io.modelcontextprotocol/protocolVersion": MODERN_PROTOCOL_VERSION,
    "io.modelcontextprotocol/clientInfo": {
      name: "strata-smoke-test",
      version: "1.0.0",
    },
    "io.modelcontextprotocol/clientCapabilities": {},
  };
}

async function mcpRequest(method: string, params?: any, headers?: Record<string, string>): Promise<any> {
  const requestParams = params ? structuredClone(params) : {};

  const reqHeaders: Record<string, string> = {
    "Content-Type": "application/json",
    Accept: "application/json, text/event-stream",
    ...headers,
  };

  if (LEGACY) {
    if (legacySessionId) {
      reqHeaders["MCP-Session-ID"] = legacySessionId;
    }
  } else {
    requestParams._meta = modernMeta();
    reqHeaders["MCP-Protocol-Version"] = MODERN_PROTOCOL_VERSION;
    reqHeaders["Mcp-Method"] = method;
    if (method === "tools/call" && typeof requestParams.name === "string") {
      reqHeaders["Mcp-Name"] = requestParams.name;
    }
  }

  const req = jsonRequest(method, requestParams);
  const res = await fetch(MCP_URL, {
    method: "POST",
    headers: reqHeaders,
    body: JSON.stringify(req),
  });

  const newSessionId = res.headers.get("MCP-Session-ID");
  if (LEGACY && newSessionId) {
    legacySessionId = newSessionId;
  }
  if (!LEGACY && newSessionId) {
    throw new Error(`Modern response minted MCP-Session-ID: ${newSessionId}`);
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
  console.warn(`Mode: ${LEGACY ? `legacy ${LEGACY_PROTOCOL_VERSION}` : `modern ${MODERN_PROTOCOL_VERSION}`}`);
  console.warn("");

  if (LEGACY) {
    await test("legacy initialize", async () => {
      const resp = await mcpRequest("initialize", {
        protocolVersion: LEGACY_PROTOCOL_VERSION,
        capabilities: {},
        clientInfo: { name: "test-client", version: "1.0.0" },
      });
      if (resp.result?.protocolVersion !== LEGACY_PROTOCOL_VERSION) {
        throw new Error(`Unexpected protocol version: ${resp.result?.protocolVersion}`);
      }
      if (!legacySessionId) {
        throw new Error("Legacy initialize did not return MCP-Session-ID");
      }
    });
  }

  await test("tools/list", async () => {
    const resp = await mcpRequest("tools/list");
    const names = (resp.result?.tools || []).map((t: any) => t.name).sort();
    if (names.join(",") !== "edit,exec,read,write") {
      throw new Error(`Unexpected tools: ${names.join(", ")}`);
    }
    if (!LEGACY && resp.result?.resultType !== "complete") {
      throw new Error(`Unexpected resultType: ${resp.result?.resultType}`);
    }
    if (!LEGACY && resp.result?.ttlMs === undefined) {
      throw new Error("tools/list is missing ttlMs");
    }
    console.error(`  Found ${names.length} tools: ${names.join(", ")}`);
  });

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

  await test("write rejects missing content as a tool error", async () => {
    const resp = await mcpRequest("tools/call", {
      name: "write",
      arguments: {
        owner_id: USER,
        session_id: SESSION,
        path: `invalid-${FILE_PATH}`,
      },
    });
    if (!resp.result?.isError || !String(resp.result?.content?.[0]?.text).includes("content must be a string")) {
      throw new Error("Missing write content did not return a tool error");
    }
  });

  await test("write creates a file (header identity)", async () => {
    const resp = await mcpRequest("tools/call", {
      name: "write",
      arguments: {
        path: FILE_PATH,
        content: "- first\n- second\n",
      },
    }, {
      "X-Owner-Id": USER,
      "X-Session-Id": SESSION,
    });
    if (resp.result?.isError || resp.result?.structuredContent?.created !== true) {
      throw new Error(resp.result?.content?.[0]?.text || "write failed");
    }
  });

  let readText = "";
  await test("read returns exact text (args identity)", async () => {
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
