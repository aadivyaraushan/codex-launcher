import { Type } from "typebox";

import type { BridgeClient, ToolCallRequest, ToolDescriptor } from "./bridge-client.js";

// AgentToolResult per the OpenClaw SDK (dist/types-D0CdrmU4.d.ts): content
// is what the model reads, details keeps the raw envelope.
export interface AgentToolResult {
  content: { type: "text"; text: string }[];
  details?: unknown;
}

export interface AgentToolDefinition {
  name: string;
  description: string;
  // TypeBox schema (JSON-Schema-shaped at runtime).
  parameters: unknown;
  execute(toolCallId: string, params: Record<string, unknown>): Promise<AgentToolResult>;
}

// The one method this plugin needs from OpenClawPluginApi, structurally
// typed so unit tests can pass a plain recorder object.
export interface OperatorToolsApi {
  registerTool(tool: AgentToolDefinition): void;
}

// buildAgentTools maps bridge descriptors onto agent tool definitions
// (without execute): name verbatim from the adapter id, description carrying
// the adapter description, ceiling, and which verbs pause for a preview,
// parameters built from the declared verbs.
export function buildAgentTools(descriptors: ToolDescriptor[]): Omit<AgentToolDefinition, "execute">[] {
  return descriptors.map(buildOneAgentTool);
}

function buildOneAgentTool(descriptor: ToolDescriptor): Omit<AgentToolDefinition, "execute"> {
  const previewVerbs = descriptor.verbs.filter((verb) => verb.requiresPreview).map((verb) => verb.name);
  const previewNote =
    previewVerbs.length > 0
      ? ` Verbs ${previewVerbs.join(", ")} pause for a preview before executing.`
      : " No verbs require a preview.";
  const description = `${descriptor.description}. Ceiling: ${descriptor.ceiling}.${previewNote}`;

  const verbNames = descriptor.verbs.map((verb) => verb.name);
  const verbSchema = Type.Union(verbNames.map((name) => Type.Literal(name)));

  const parameters = Type.Object({
    verb: verbSchema,
    subject: Type.Optional(Type.String()),
    handle: Type.Optional(Type.String()),
    body: Type.Optional(Type.String()),
    fields: Type.Optional(Type.Record(Type.String(), Type.String())),
  });

  return { name: descriptor.name, description, parameters };
}

// registerOperatorTools registers one tool per descriptor, synchronously —
// OpenClaw's register(api) hook is not awaited, so nothing here may be
// async before the registerTool calls. execute forwards to the bridge.
export function registerOperatorTools(
  api: OperatorToolsApi,
  deps: { descriptors: ToolDescriptor[]; client: BridgeClient },
): void {
  const { descriptors, client } = deps;
  const definitions = buildAgentTools(descriptors);

  for (const definition of definitions) {
    api.registerTool({
      ...definition,
      execute: async (_toolCallId, params) => executeTool(client, definition.name, params),
    });
  }
}

async function executeTool(
  client: BridgeClient,
  adapter: string,
  params: Record<string, unknown>,
): Promise<AgentToolResult> {
  const request: ToolCallRequest = {
    adapter,
    verb: params.verb as string,
  };
  if (params.subject !== undefined) request.subject = params.subject as string;
  if (params.handle !== undefined) request.handle = params.handle as string;
  if (params.body !== undefined) request.body = params.body as string;
  if (params.fields !== undefined) request.fields = params.fields as Record<string, string>;

  const result = await client.callTool(request);

  const text = result.ok
    ? [result.detail, result.preview?.headline].filter((part): part is string => Boolean(part)).join(" — ")
    : `${result.error?.code ?? "error"}: ${result.error?.message ?? "unknown error"}`;

  return {
    content: [{ type: "text", text }],
    details: result,
  };
}
