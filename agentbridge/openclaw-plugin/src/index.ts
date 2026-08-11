// Plugin entry point, loaded by OpenClaw on the phone via dist/index.js
// (package.json "main"). Not covered by test/plugin.test.mjs — the SDK it
// imports only exists on-device — so this file stays a thin wire-up over
// the tested buildAgentTools/registerOperatorTools/createBridgeClient.
import { definePluginEntry, type RegisterToolContext } from "openclaw/plugin-sdk/plugin-entry";

import { createBridgeClient } from "./bridge-client.js";
import { registerOperatorTools, type AgentToolDefinition, type OperatorToolsApi } from "./tools.js";
import toolTable from "./tools.json" with { type: "json" };

interface OperatorToolsPluginApi extends OperatorToolsApi {
  pluginConfig: Record<string, unknown>;
  // registerTool also accepts a factory that OpenClaw calls fresh at the
  // start of every session (including /new and /reset); used below only to
  // refresh currentSessionId, not to register a tool of its own. Both
  // signatures are declared so this stays assignable to OperatorToolsApi.
  registerTool(tool: AgentToolDefinition): void;
  registerTool(factory: (ctx: RegisterToolContext) => AgentToolDefinition | AgentToolDefinition[] | null): void;
}

// Module-level so the turnKey closure passed to registerOperatorTools below
// always reads the session id from the most recent factory invocation.
let currentSessionId: string | undefined;

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

    // Factory form: OpenClaw invokes this at the start of each session, so
    // currentSessionId tracks /new and /reset without a plugin restart.
    api.registerTool((ctx) => {
      currentSessionId = ctx.sessionId;
      return null;
    });

    registerOperatorTools(api, {
      descriptors: toolTable.tools,
      client,
      turnKey: () => currentSessionId,
    });
  },
});

function requireConfigString(config: Record<string, unknown>, key: string): string {
  const value = config[key];
  if (typeof value !== "string" || value.length === 0) {
    throw new Error(`operator-tools plugin config is missing required key "${key}"`);
  }
  return value;
}
