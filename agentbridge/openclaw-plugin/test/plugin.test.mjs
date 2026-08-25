import test from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { createServer } from "node:https";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { createBridgeClient } from "../dist/bridge-client.js";
import { buildAgentTools, registerOperatorTools } from "../dist/tools.js";

const here = path.dirname(fileURLToPath(import.meta.url));

const fixtureDescriptors = [
  {
    name: "instagram",
    description: "Adapter instagram; verbs: read, send",
    verbs: [
      { name: "read", requiresPreview: false },
      { name: "send", requiresPreview: true },
    ],
    ceiling: "completes",
  },
  {
    name: "notes",
    description: "Adapter notes; verbs: read",
    verbs: [{ name: "read", requiresPreview: false }],
    ceiling: "completes",
  },
];

test("buildAgentTools maps descriptors honestly", () => {
  const tools = buildAgentTools(fixtureDescriptors);
  assert.equal(tools.length, 2);
  assert.deepEqual(tools.map((t) => t.name), ["instagram", "notes"]);

  const instagram = tools[0];
  // The agent must see the adapter's own description, its ceiling, and
  // which verbs pause for a preview before executing.
  assert.match(instagram.description, /Adapter instagram/);
  assert.match(instagram.description, /completes/);
  assert.match(instagram.description, /preview/i);
  assert.match(instagram.description, /send/);

  // parameters is JSON-Schema-shaped: verb is required and its allowed
  // values are exactly the declared verbs.
  const schema = JSON.parse(JSON.stringify(instagram.parameters));
  assert.ok(schema.required.includes("verb"), "verb must be required");
  const schemaText = JSON.stringify(schema);
  assert.ok(schemaText.includes('"read"'), "schema names the read verb");
  assert.ok(schemaText.includes('"send"'), "schema names the send verb");
  const notesText = JSON.stringify(JSON.parse(JSON.stringify(tools[1].parameters)));
  assert.ok(!notesText.includes('"send"'), "notes offers no send verb");
});

test("registerOperatorTools registers synchronously and forwards calls to the bridge", async () => {
  const registered = [];
  const calls = [];
  const fakeClient = {
    listTools: async () => ({ tools: fixtureDescriptors }),
    callTool: async (request) => {
      calls.push(request);
      return {
        ok: true,
        reached: "completes",
        done: true,
        detail: "sent",
        preview: { headline: "Send to Maya on Instagram" },
      };
    },
  };

  registerOperatorTools(
    { registerTool: (tool) => registered.push(tool) },
    { descriptors: fixtureDescriptors, client: fakeClient },
  );
  // Synchronous: all tools are registered by the time the call returns —
  // OpenClaw does not await register(api).
  assert.equal(registered.length, 2);
  assert.deepEqual(registered.map((t) => t.name), ["instagram", "notes"]);

  const result = await registered[0].execute("call-1", {
    verb: "send",
    subject: "Maya",
    body: "hi",
  });
  assert.deepEqual(calls, [
    { adapter: "instagram", verb: "send", subject: "Maya", body: "hi" },
  ]);
  assert.equal(result.content[0].type, "text");
  assert.match(result.content[0].text, /sent/);
  assert.match(result.content[0].text, /Send to Maya on Instagram/);
  assert.equal(result.details.ok, true);
});

test("a failed call surfaces the bridge error to the model instead of throwing", async () => {
  const registered = [];
  registerOperatorTools(
    { registerTool: (tool) => registered.push(tool) },
    {
      descriptors: fixtureDescriptors,
      client: {
        listTools: async () => ({ tools: [] }),
        callTool: async () => ({
          ok: false,
          error: { code: "verb_not_offered", message: "instagram does not offer order" },
        }),
      },
    },
  );
  const result = await registered[0].execute("call-2", { verb: "order" });
  assert.match(result.content[0].text, /verb_not_offered/);
  assert.match(result.content[0].text, /does not offer order/);
  assert.equal(result.details.ok, false);
});

test("every call is stamped with the current turn key", async () => {
  const registered = [];
  const calls = [];
  registerOperatorTools(
    { registerTool: (tool) => registered.push(tool) },
    {
      descriptors: fixtureDescriptors,
      client: {
        listTools: async () => ({ tools: [] }),
        callTool: async (request) => {
          calls.push(request);
          return { ok: true, done: true, detail: "sent" };
        },
      },
      turnKey: () => "session-abc",
    },
  );
  await registered[0].execute("call-3", { verb: "send", subject: "Maya", body: "hi" });
  // The bridge's exfiltration gate buckets reads and sends by turn; a call
  // without the key falls into one shared always-suspicious bucket.
  assert.equal(calls[0].turnKey, "session-abc");
});

test("an approval_required answer reads as waiting for the owner, not as a failure", async () => {
  const registered = [];
  registerOperatorTools(
    { registerTool: (tool) => registered.push(tool) },
    {
      descriptors: fixtureDescriptors,
      client: {
        listTools: async () => ({ tools: [] }),
        callTool: async () => ({
          ok: false,
          gateId: "gate-1",
          preview: { headline: "Send “hi” to Maya on Instagram" },
          error: { code: "approval_required", message: "first message to this recipient needs owner approval" },
        }),
      },
    },
  );
  const result = await registered[0].execute("call-4", { verb: "send", subject: "Maya", body: "hi" });
  const text = result.content[0].text;
  // The model must learn: nothing happened, the owner was asked, do not
  // retry — approval arrives out of band on the launcher's sheet.
  assert.match(text, /owner/i);
  assert.match(text, /approval/i);
  assert.match(text, /Send “hi” to Maya on Instagram/);
  assert.doesNotMatch(text, /^approval_required:/);
  assert.equal(result.details.gateId, "gate-1");
  assert.equal(result.details.ok, false);
});

// --- bridge client against a mock TLS bridge ---

// Like the real runtime certificate: self-signed, CN only, no SAN — the
// client must pin bytes, not rely on hostname verification.
function mintCert(dir, label) {
  const key = path.join(dir, `${label}-key.pem`);
  const cert = path.join(dir, `${label}-cert.pem`);
  execFileSync("openssl", [
    "req", "-x509", "-newkey", "ec",
    "-pkeyopt", "ec_paramgen_curve:P-256",
    "-keyout", key, "-out", cert,
    "-days", "2", "-nodes",
    "-subj", "/CN=Codex Launcher Companion",
  ]);
  return { key, cert };
}

const TOKEN = "test-bridge-token-0123";

function startMockBridge(tlsFiles) {
  const seen = { authHeaders: [] };
  const server = createServer(
    { key: readFileSync(tlsFiles.key), cert: readFileSync(tlsFiles.cert) },
    (req, res) => {
      seen.authHeaders.push(req.headers.authorization ?? "");
      if (req.headers.authorization !== `Bearer ${TOKEN}`) {
        res.writeHead(401, { "content-type": "application/json" });
        res.end(JSON.stringify({ ok: false, error: { code: "unauthorized", message: "unauthorized" } }));
        return;
      }
      if (req.method === "GET" && req.url === "/v1/agent-tools/list") {
        res.writeHead(200, { "content-type": "application/json" });
        res.end(JSON.stringify({ tools: fixtureDescriptors }));
        return;
      }
      if (req.method === "POST" && req.url === "/v1/agent-tools/call") {
        let body = "";
        req.on("data", (chunk) => (body += chunk));
        req.on("end", () => {
          const request = JSON.parse(body);
          if (request.verb === "detonate") {
            res.writeHead(400, { "content-type": "application/json" });
            res.end(JSON.stringify({ ok: false, error: { code: "bad_request", message: "unknown verb" } }));
            return;
          }
          res.writeHead(200, { "content-type": "application/json" });
          res.end(JSON.stringify({ ok: true, reached: "completes", done: true, detail: "sent " + request.adapter }));
        });
        return;
      }
      res.writeHead(404);
      res.end();
    },
  );
  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => resolve({ server, seen, port: server.address().port }));
  });
}

test("bridge client authenticates, pins the certificate, and decodes envelopes", async (t) => {
  const dir = mkdtempSync(path.join(tmpdir(), "operator-tools-test-"));
  const bridgeTLS = mintCert(dir, "bridge");
  const otherTLS = mintCert(dir, "other");
  const tokenPath = path.join(dir, "agentbridge-token");
  writeFileSync(tokenPath, TOKEN + "\n");

  const { server, seen, port } = await startMockBridge(bridgeTLS);
  t.after(() => server.close());
  const bridgeUrl = `https://127.0.0.1:${port}`;

  const rejectEnvBefore = process.env.NODE_TLS_REJECT_UNAUTHORIZED;

  const client = createBridgeClient({ bridgeUrl, tokenPath, certPath: bridgeTLS.cert });
  const list = await client.listTools();
  assert.equal(list.tools.length, 2);
  assert.equal(seen.authHeaders.at(-1), `Bearer ${TOKEN}`);

  const okResult = await client.callTool({ adapter: "instagram", verb: "read" });
  assert.equal(okResult.ok, true);
  assert.equal(okResult.detail, "sent instagram");

  // The bridge sends the envelope on error statuses too; the client must
  // hand it back, not throw.
  const errResult = await client.callTool({ adapter: "instagram", verb: "detonate" });
  assert.equal(errResult.ok, false);
  assert.equal(errResult.error.code, "bad_request");

  // A peer whose certificate differs from the pinned PEM is refused.
  const wrongPin = createBridgeClient({ bridgeUrl, tokenPath, certPath: otherTLS.cert });
  await assert.rejects(() => wrongPin.listTools());

  // Pinning must never come from disabling Node's TLS verification globally.
  assert.equal(process.env.NODE_TLS_REJECT_UNAUTHORIZED, rejectEnvBefore);
});

test("manifest contracts.tools matches the committed tool table", () => {
  const manifest = JSON.parse(readFileSync(path.join(here, "..", "openclaw.plugin.json"), "utf8"));
  const table = JSON.parse(readFileSync(path.join(here, "..", "src", "tools.json"), "utf8"));
  const tableNames = table.tools.map((tool) => tool.name).sort();
  assert.deepEqual(manifest.contracts.tools, tableNames);
});
