// Plugin entry point, loaded by OpenClaw on the phone via dist/index.js
// (package.json "main"). Not covered by test/plugin.test.mjs — the SDK it
// imports only exists on-device — so this file stays a thin wire-up over
// the tested buildAgentTools/registerOperatorTools/createBridgeClient.
import { definePluginEntry } from "openclaw/plugin-sdk/plugin-entry";

import { createBridgeClient } from "./bridge-client.js";
import { registerOperatorTools, type OperatorToolsApi } from "./tools.js";
import toolTable from "./tools.json" with { type: "json" };

interface OperatorToolsPluginApi extends OperatorToolsApi {
  pluginConfig: Record<string, unknown>;
}

export default definePluginEntry({
  id: "operator-tools",
  name: "Operator Tools",
  description:
    "Exposes the Operator capability adapters (Beeper messaging and friends) as agent tools, calling the on-device agent bridge over pinned loopback TLS.",
  register(api: OperatorToolsPluginApi) {
    const config = api.pluginConfig ?? {};
    const bridgeUrl = requireConfigString(config, "bridgeUrl");
    const tokenPath = requireConfigString(config, "tokenPath");
    const certPath = requireConfigString(config, "certPath");

    const client = createBridgeClient({ bridgeUrl, tokenPath, certPath });
    registerOperatorTools(api, { descriptors: toolTable.tools, client });
  },
});

function requireConfigString(config: Record<string, unknown>, key: string): string {
  const value = config[key];
  if (typeof value !== "string" || value.length === 0) {
    throw new Error(`operator-tools plugin config is missing required key "${key}"`);
  }
  return value;
}
