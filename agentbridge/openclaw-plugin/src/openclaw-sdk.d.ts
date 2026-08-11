// Ambient declaration for the OpenClaw plugin SDK's entry-point helper.
// The real package (openclaw/plugin-sdk/plugin-entry) only exists on the
// phone runtime, not in this workspace's node_modules, so src/index.ts
// (which is not exercised by the unit tests — see test/plugin.test.mjs)
// cannot resolve it without this stub. It declares just enough of the
// shape definePluginEntry needs to type-check register(api) below.
declare module "openclaw/plugin-sdk/plugin-entry" {
  export function definePluginEntry(definition: {
    id: string;
    name?: string;
    description?: string;
    register(api: any): void;
  }): unknown;
}
