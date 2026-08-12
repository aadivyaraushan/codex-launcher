import { Type } from "typebox";

import type { BridgeClient, ToolCallRequest, ToolCallResult, ToolDescriptor } from "./bridge-client.js";

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

const messagingAdapters = new Set(["discord", "messages", "instagram"]);

function buildOneAgentTool(descriptor: ToolDescriptor): Omit<AgentToolDefinition, "execute"> {
  const previewVerbs = descriptor.verbs.filter((verb) => verb.requiresPreview).map((verb) => verb.name);
  const previewNote =
    previewVerbs.length > 0
      ? ` Verbs ${previewVerbs.join(", ")} pause for a preview before executing.`
      : " No verbs require a preview.";
  let description = `${descriptor.description}. Ceiling: ${descriptor.ceiling}.${previewNote}`;
  if (messagingAdapters.has(descriptor.name)) {
    description +=
      " For send, set subject to the person, phone number, or chat (handle, to, recipient, and chat_id are aliases). First contact waits for the owner to tap Approve — do not retry.";
  }

  const verbNames = descriptor.verbs.map((verb) => verb.name);
  const verbSchema = Type.Union(verbNames.map((name) => Type.Literal(name)));
  const recipientField = (text: string) => Type.Optional(Type.String({ description: text }));
  const messaging = messagingAdapters.has(descriptor.name);

  const parameters = messaging
    ? Type.Object({
        verb: verbSchema,
        subject: recipientField("Who to message: contact name, phone number, or chat. Prefer this field for send."),
        handle: recipientField("Same as subject when you already have a phone number or chat id."),
        to: recipientField("Alias for subject: the person or chat to send to."),
        recipient: recipientField("Alias for subject: the person or chat to send to."),
        chat_id: recipientField("Alias for subject: the person or chat to send to. This is a name or handle, not a raw Beeper id."),
        body: Type.Optional(Type.String({ description: "Message text for send, or search text for read." })),
        fields: Type.Optional(Type.Record(Type.String(), Type.String())),
      })
    : Type.Object({
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
  deps: {
    descriptors: ToolDescriptor[];
    client: BridgeClient;
    // Returns the current turn's session id, or undefined between turns.
    // Stamped onto every call so the bridge's exfiltration gate can bucket
    // by turn instead of lumping unstamped calls into one suspicious pile.
    turnKey?: () => string | undefined;
  },
): void {
  const { descriptors, client, turnKey } = deps;
  const definitions = buildAgentTools(descriptors);

  for (const definition of definitions) {
    api.registerTool({
      ...definition,
      execute: async (_toolCallId, params) => executeTool(client, definition.name, params, turnKey),
    });
  }
}

async function executeTool(
  client: BridgeClient,
  adapter: string,
  params: Record<string, unknown>,
  turnKey?: () => string | undefined,
): Promise<AgentToolResult> {
  const request: ToolCallRequest = {
    adapter,
    verb: params.verb as string,
  };
  const recipient = recipientFromParams(params);
  if (recipient !== undefined) request.subject = recipient;
  const handle = filledString(params.handle);
  if (handle !== undefined) request.handle = handle;
  if (params.body !== undefined) request.body = params.body as string;
  if (params.fields !== undefined) request.fields = params.fields as Record<string, string>;
  const key = turnKey?.();
  if (key) request.turnKey = key;

  const result = await client.callTool(request);

  const text = renderResultText(result);

  return {
    content: [{ type: "text", text }],
    details: result,
  };
}

function filledString(value: unknown): string | undefined {
  if (typeof value !== "string") {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function recipientFromParams(params: Record<string, unknown>): string | undefined {
  const fields = isRecord(params.fields) ? params.fields : {};
  return (
    filledString(params.subject) ??
    filledString(params.handle) ??
    filledString(params.to) ??
    filledString(params.recipient) ??
    filledString(params.chat_id) ??
    filledString(fields.to) ??
    filledString(fields.recipient) ??
    filledString(fields.chat_id) ??
    filledString(fields.handle) ??
    filledString(fields.subject)
  );
}

// renderResultText turns a bridge envelope into the text the model reads.
// approval_required is not a failure the model should retry: nothing ran,
// the owner has been asked on the launcher, and the answer arrives out of
// band — so it gets its own wording instead of the generic "code: message"
// used for every other error.
function renderResultText(result: ToolCallResult): string {
  if (result.ok) {
    return [result.detail, result.preview?.headline].filter((part): part is string => Boolean(part)).join(" — ");
  }
  if (result.error?.code === "approval_required") {
    const headline = result.preview?.headline;
    const action = headline ? `"${headline}"` : "this action";
    return (
      `Nothing happened yet: ${action} needs the OWNER's APPROVAL on the launcher before it can run. ` +
      `Do not retry — the owner's answer will arrive on its own.`
    );
  }
  return `${result.error?.code ?? "error"}: ${result.error?.message ?? "unknown error"}`;
}
