#!/usr/bin/env node
// Regenerates src/tools.json from a live agent bridge, and keeps
// openclaw.plugin.json's contracts.tools list (the manifest the "manifest
// contracts.tools matches the committed tool table" test checks) in sync
// with it. Run via `npm run sync-tools` after the tool table on a real
// device/bridge changes; requires `npm run build` to have produced
// dist/bridge-client.js first.
//
// Env vars (all required):
//   BRIDGE_URL  - base URL of the agent bridge, e.g. https://127.0.0.1:9443
//   TOKEN_PATH  - path to the bearer token file minted by the phone runtime
//   CERT_PATH   - path to the runtime's exported TLS certificate PEM

import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { createBridgeClient } from "../dist/bridge-client.js";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(here, "..");

function requireEnv(name) {
  const value = process.env[name];
  if (!value) {
    throw new Error(`sync-tools: missing required env var ${name}`);
  }
  return value;
}

async function main() {
  const bridgeUrl = requireEnv("BRIDGE_URL");
  const tokenPath = requireEnv("TOKEN_PATH");
  const certPath = requireEnv("CERT_PATH");

  const client = createBridgeClient({ bridgeUrl, tokenPath, certPath });

  let list;
  try {
    list = await client.listTools();
  } catch (err) {
    throw new Error(`sync-tools: could not reach bridge at ${bridgeUrl}: ${err.message}`, { cause: err });
  }

  const tools = [...list.tools].sort((a, b) => a.name.localeCompare(b.name));

  const toolsJsonPath = path.join(root, "src", "tools.json");
  writeFileSync(toolsJsonPath, `${JSON.stringify({ tools }, null, 2)}\n`);

  const manifestPath = path.join(root, "openclaw.plugin.json");
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
  manifest.contracts.tools = tools.map((tool) => tool.name);
  writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

  console.log(`sync-tools: wrote ${tools.length} tool(s) to ${toolsJsonPath} and ${manifestPath}`);
}

main().catch((err) => {
  console.error(err.message ?? err);
  process.exit(1);
});
