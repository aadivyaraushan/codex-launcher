# OpenClaw gateway protocol — extracted facts

date: 2026-08-12
note: "extracted from npm openclaw@2026.7.1-2 dist types + webchat client source; phone's installed version to be re-checked on reconnect"

Base path for all quotes below:
`/private/tmp/claude-501/-Users-aadivyar-Documents-Startups-ai-native-mobile-software/2d96c312-c7af-4a85-943c-5fb399ed70b1/scratchpad/package`

Schema format note: the package uses **typebox**, not Zod, for its protocol schemas (`declare const ConnectParamsSchema: Type.TObject<{...}>` etc., in `dist/schema-DtyqV_v0.d.ts`).

---

## 1. WebSocket URL, port, path, token placement

Loopback default, confirmed in multiple docs.

`docs/network.md:17`
```
- **Loopback first**: the Gateway WS defaults to `ws://127.0.0.1:18789`.
```

`docs/gateway/index.md:178`
```
Then connect clients locally to `ws://127.0.0.1:18789`.
```

`docs/gateway/configuration-reference.md:554`
```
url: "ws://127.0.0.1:18789",
```

No path suffix (no `/ws`, `/gateway`, etc.) appears anywhere — every doc example and every client-code URL is the bare `ws://host:port` root. Confirmed by grepping the actual `new WebSocket(...)` call sites and finding no path concatenation:

`dist/control-ui/assets/gateway-CWCQz7bR.js` (control UI client):
```
new WebSocket(this.opts.url)
```
`dist/src-DZzKBMa7.js:335` (node/CLI client):
```
ws = new WebSocket(url, wsOptions);
```
where `wsOptions` is only `{maxPayload, origin?, rejectUnauthorized?, checkServerIdentity?}` (TLS-pinning fields) — no `Authorization` header, no token field.

**Token placement**: the token does NOT go in the URL query string or in headers. It goes inside the `connect` request's JSON body, `params.auth.token` (see §3). Confirmed by grepping both client files for `token=` / query-string construction near the URL — no hits — and by the `auth` object shape in `ConnectParamsSchema` (§3).

---

## 2. Frame envelope

`docs/gateway/protocol.md:27-33`
```
Frame shapes:

- Request: `{type:"req", id, method, params}`
- Response: `{type:"res", id, ok, payload|error}`
- Event: `{type:"event", event, payload, seq?, stateVersion?}`

Side-effecting methods require idempotency keys (see schema).
```

Exact typebox schema, `dist/schema-DtyqV_v0.d.ts:2789-2817`:
```
RequestFrame: import("typebox").TObject<{
  type: import("typebox").TLiteral<"req">;
  id: import("typebox").TString;
  method: import("typebox").TString;
  params: import("typebox").TOptional<import("typebox").TUnknown>;
}>;
ResponseFrame: import("typebox").TObject<{
  type: import("typebox").TLiteral<"res">;
  id: import("typebox").TString;
  ok: import("typebox").TBoolean;
  payload: import("typebox").TOptional<import("typebox").TUnknown>;
  error: import("typebox").TOptional<import("typebox").TObject<{
    code: import("typebox").TString;
    message: import("typebox").TString;
    details: import("typebox").TOptional<import("typebox").TUnknown>;
    retryable: import("typebox").TOptional<import("typebox").TBoolean>;
    retryAfterMs: import("typebox").TOptional<import("typebox").TInteger>;
  }>>;
}>;
EventFrame: import("typebox").TObject<{
  type: import("typebox").TLiteral<"event">;
  event: import("typebox").TString;
  payload: import("typebox").TOptional<import("typebox").TUnknown>;
  seq: import("typebox").TOptional<import("typebox").TInteger>;
  stateVersion: import("typebox").TOptional<import("typebox").TObject<{
    presence: import("typebox").TInteger;
    health: import("typebox").TInteger;
  }>>;
}>;
```

Error shape fields: `code` (string, required), `message` (string, required), `details` (optional, unknown), `retryable` (optional bool), `retryAfterMs` (optional int).

---

## 3. Connect handshake

Protocol version constants, `dist/schema-DtyqV_v0.d.ts:2659-2666`:
```
/** Current gateway protocol version emitted by modern clients and servers. */
declare const PROTOCOL_VERSION: 4;
/** Lowest general client protocol version accepted by the gateway. */
declare const MIN_CLIENT_PROTOCOL_VERSION: 4;
/** Lowest authenticated node protocol version accepted by the gateway. */
declare const MIN_NODE_PROTOCOL_VERSION: 3;
/** Lowest lightweight probe protocol version accepted by the gateway. */
declare const MIN_PROBE_PROTOCOL_VERSION: 3;
```
Current protocol version literal: **4**.

### connect.challenge (server → client, sent first, pre-connect)

Server emission, `dist/server-ws-runtime-CXYLcFG-.js` (near "event: \"connect.challenge\""):
```
const connectNonce = randomUUID();
send({
  type: "event",
  event: "connect.challenge",
  payload: {
    nonce: connectNonce,
    ts: Date.now()
  }
});
```

Client handling, `dist/src-DZzKBMa7.js` (near `evt.event === "connect.challenge"`):
```
if (evt.event === "connect.challenge") {
  const payload = evt.payload;
  const nonce = payload && typeof payload.nonce === "string" ? payload.nonce : null;
  if (!nonce || nonce.trim().length === 0) {
    this.notifyConnectError(new Error("gateway connect challenge missing nonce"));
    this.ws?.close(1008, "connect challenge missing nonce");
    return;
  }
  this.connectNonce = nonce.trim();
  if (this.socketOpened) this.sendConnect();
  return;
}
```
Doc example, `docs/gateway/protocol.md:39-45`:
```json
{
  "type": "event",
  "event": "connect.challenge",
  "payload": { "nonce": "…", "ts": 1737264000000 }
}
```

### ConnectParams schema

`dist/schema-DtyqV_v0.d.ts:7612-7648` (canonical `declare const` form):
```
declare const ConnectParamsSchema: Type.TObject<{
  minProtocol: Type.TInteger;
  maxProtocol: Type.TInteger;
  client: Type.TObject<{
    id: Type.TEnum<["webchat-ui", "openclaw-control-ui", "openclaw-tui", "webchat", "cli", "gateway-client", "openclaw-macos", "openclaw-ios", "openclaw-android", "node-host", "test", "fingerprint", "openclaw-probe"]>;
    displayName: Type.TOptional<Type.TString>;
    version: Type.TString;
    platform: Type.TString;
    deviceFamily: Type.TOptional<Type.TString>;
    modelIdentifier: Type.TOptional<Type.TString>;
    mode: Type.TEnum<["webchat", "cli", "test", "probe", "ui", "backend", "node"]>;
    instanceId: Type.TOptional<Type.TString>;
  }>;
  caps: Type.TOptional<Type.TArray<Type.TString>>;
  commands: Type.TOptional<Type.TArray<Type.TString>>;
  permissions: Type.TOptional<Type.TRecord<"^.*$", Type.TBoolean>>;
  pathEnv: Type.TOptional<Type.TString>;
  role: Type.TOptional<Type.TString>;
  scopes: Type.TOptional<Type.TArray<Type.TString>>;
  device: Type.TOptional<Type.TObject<{
    id: Type.TString;
    publicKey: Type.TString;
    signature: Type.TString;
    signedAt: Type.TInteger;
    nonce: Type.TString;
  }>>;
  auth: Type.TOptional<Type.TObject<{
    token: Type.TOptional<Type.TString>;
    bootstrapToken: Type.TOptional<Type.TString>;
    deviceToken: Type.TOptional<Type.TString>;
    password: Type.TOptional<Type.TString>;
    approvalRuntimeToken: Type.TOptional<Type.TString>;
    agentRuntimeIdentityToken: Type.TOptional<Type.TString>;
  }>>;
  locale: Type.TOptional<Type.TString>;
  userAgent: Type.TOptional<Type.TString>;
}>;
```

Required fields per the schema: `minProtocol`, `maxProtocol`, `client` (with required `id`, `version`, `platform`, `mode` inside it — `displayName`, `deviceFamily`, `modelIdentifier`, `instanceId` optional). Everything else at the top level (`caps`, `commands`, `permissions`, `pathEnv`, `role`, `scopes`, `device`, `auth`, `locale`, `userAgent`) is optional per the `TOptional` wrapper, though `docs/gateway/protocol.md:214-231` states `role` is one of `operator` | `node` in practice and `scopes` is drawn from the closed operator-scope set below.

Valid `role` values (prose, `docs/gateway/protocol.md:219-222`):
```
Roles:

- `operator`: control-plane client (CLI/UI/automation).
- `node`: capability host (camera/screen/canvas/system.run).
```

Valid operator `scopes` (prose, `docs/gateway/protocol.md:224-231`):
```
Operator scopes (`src/gateway/operator-scopes.ts`), the full closed set:

- `operator.read`
- `operator.write`
- `operator.admin`
- `operator.approvals`
- `operator.pairing`
- `operator.talk.secrets`
```

Doc example of a full `connect` request, `docs/gateway/protocol.md:47-80`:
```json
{
  "type": "req",
  "id": "…",
  "method": "connect",
  "params": {
    "minProtocol": 4,
    "maxProtocol": 4,
    "client": {
      "id": "cli",
      "version": "1.2.3",
      "platform": "macos",
      "mode": "operator"
    },
    "role": "operator",
    "scopes": ["operator.read", "operator.write"],
    "caps": [],
    "commands": [],
    "permissions": {},
    "auth": { "token": "…" },
    "locale": "en-US",
    "userAgent": "openclaw-cli/1.2.3",
    "device": {
      "id": "device_fingerprint",
      "publicKey": "…",
      "signature": "…",
      "signedAt": 1737264000000,
      "nonce": "…"
    }
  }
}
```

### HelloOk (connect response payload) schema

`dist/schema-DtyqV_v0.d.ts:7650-7730`:
```
declare const HelloOkSchema: Type.TObject<{
  type: Type.TLiteral<"hello-ok">;
  protocol: Type.TInteger;
  server: Type.TObject<{
    version: Type.TString;
    connId: Type.TString;
  }>;
  features: Type.TObject<{
    methods: Type.TArray<Type.TString>;
    events: Type.TArray<Type.TString>;
    capabilities: Type.TOptional<Type.TArray<Type.TString>>;
  }>;
  snapshot: Type.TObject<{
    presence: Type.TArray<Type.TObject<{ ... }>>;
    health: Type.TAny;
    stateVersion: Type.TObject<{ presence: Type.TInteger; health: Type.TInteger; }>;
    uptimeMs: Type.TInteger;
    configPath: Type.TOptional<Type.TString>;
    stateDir: Type.TOptional<Type.TString>;
    sessionDefaults: Type.TOptional<Type.TObject<{
      defaultAgentId: Type.TString;
      mainKey: Type.TString;
      mainSessionKey: Type.TString;
      scope: Type.TOptional<Type.TString>;
    }>>;
    authMode: Type.TOptional<Type.TUnion<[Type.TLiteral<"none">, Type.TLiteral<"token">, Type.TLiteral<"password">, Type.TLiteral<"trusted-proxy">]>>;
    updateAvailable: Type.TOptional<Type.TObject<{ currentVersion: Type.TString; latestVersion: Type.TString; channel: Type.TString; }>>;
  }>;
  controlUiTabs: Type.TOptional<Type.TArray<Type.TObject<{ ... }>>>;
  pluginSurfaceUrls: Type.TOptional<Type.TRecord<"^.*$", Type.TString>>;
  auth: Type.TObject<{
    deviceToken: Type.TOptional<Type.TString>;
    role: Type.TString;
    scopes: Type.TArray<Type.TString>;
    issuedAtMs: Type.TOptional<Type.TInteger>;
    deviceTokens: Type.TOptional<Type.TArray<Type.TObject<{
      deviceToken: Type.TString; role: Type.TString; scopes: Type.TArray<Type.TString>; issuedAtMs: Type.TInteger;
    }>>>;
  }>;
  policy: Type.TObject<{
    maxPayload: Type.TInteger;
    maxBufferedBytes: Type.TInteger;
    tickIntervalMs: Type.TInteger;
  }>;
}>;
```

Doc says all of `server`, `features`, `snapshot`, `policy`, `auth` are required (`docs/gateway/protocol.md:108-109`):
```
`server`, `features`, `snapshot`, `policy`, and `auth` are all required by
`HelloOkSchema` (`packages/gateway-protocol/src/schema/frames.ts`).
```

Doc example response, `docs/gateway/protocol.md:84-106`:
```json
{
  "type": "res",
  "id": "…",
  "ok": true,
  "payload": {
    "type": "hello-ok",
    "protocol": 4,
    "server": { "version": "…", "connId": "…" },
    "features": { "methods": ["…"], "events": ["…"] },
    "snapshot": { "…": "…" },
    "auth": { "role": "operator", "scopes": ["operator.read", "operator.write"] },
    "policy": { "maxPayload": 26214400, "maxBufferedBytes": 52428800, "tickIntervalMs": 15000 }
  }
}
```

---

## 4. chat.send

Full params schema, `dist/schema-DtyqV_v0.d.ts:7025-7051`:
```
ChatSendParams: import("typebox").TObject<{
  sessionKey: import("typebox").TString;
  agentId: import("typebox").TOptional<import("typebox").TString>;
  sessionId: import("typebox").TOptional<import("typebox").TString>;
  message: import("typebox").TString;
  thinking: import("typebox").TOptional<import("typebox").TString>;
  fastMode: import("typebox").TOptional<import("typebox").TUnion<[import("typebox").TBoolean, import("typebox").TLiteral<"auto">]>>;
  fastAutoOnSeconds: import("typebox").TOptional<import("typebox").TInteger>;
  deliver: import("typebox").TOptional<import("typebox").TBoolean>;
  originatingChannel: import("typebox").TOptional<import("typebox").TString>;
  originatingTo: import("typebox").TOptional<import("typebox").TString>;
  originatingAccountId: import("typebox").TOptional<import("typebox").TString>;
  originatingThreadId: import("typebox").TOptional<import("typebox").TString>;
  attachments: import("typebox").TOptional<import("typebox").TArray<import("typebox").TUnknown>>;
  timeoutMs: import("typebox").TOptional<import("typebox").TInteger>;
  systemInputProvenance: import("typebox").TOptional<import("typebox").TObject<{
    kind: import("typebox").TString;
    originSessionId: import("typebox").TOptional<import("typebox").TString>;
    sourceSessionKey: import("typebox").TOptional<import("typebox").TString>;
    sourceChannel: import("typebox").TOptional<import("typebox").TString>;
    sourceTool: import("typebox").TOptional<import("typebox").TString>;
  }>>;
  systemProvenanceReceipt: import("typebox").TOptional<import("typebox").TString>;
  suppressCommandInterpretation: import("typebox").TOptional<import("typebox").TBoolean>;
  expectedSessionRoutingContract: import("typebox").TOptional<import("typebox").TString>;
  idempotencyKey: import("typebox").TString;
}>;
```

Key facts:
- The session field is literally called **`sessionKey`** (required, `TString` — not `TOptional`).
- The text/content field is literally called **`message`** (required, `TString` — not `text`).
- **`idempotencyKey` is required** (`TString`, not `TOptional`) at the `chat.send` RPC level.
- `attachments`, `agentId`, `sessionId`, `thinking`, `fastMode`, `fastAutoOnSeconds`, `deliver`, `timeoutMs`, and the `originating*`/`system*` provenance fields are all optional.

Reference client call, webchat UI's `Rh()` function, `dist/control-ui/assets/chat-page-DrPkxqJK.js` (near `async function Rh(`):
```
async function Rh(e,t){
  let n=zh(e,t),r=!!(n.sessionId&&e.reconnectResumeSessionId===n.sessionId),
  i=await e.client.request(`chat.send`,{
    sessionKey:n.sessionKey,
    ...P(n.sessionKey)&&n.selectedAgentId?{agentId:n.selectedAgentId}:{},
    ...n.sessionId?{sessionId:n.sessionId}:{},
    ...r?{__controlUiReconnectResume:!0}:{},
    message:t.message,
    deliver:!1,
    idempotencyKey:t.runId,
    attachments:Ph(t.attachments)
  });
  return r&&(e.reconnectResumeSessionId=null),Lh(i,t.runId)
}
```
Here `t.runId` is a freshly generated UUID (`v()`) used directly as the `idempotencyKey` — confirming the client always supplies one even though the field itself is documented as required by the schema either way.

Response payload: no separate `ChatSendResult` typebox declaration was found in `dist/schema-DtyqV_v0.d.ts` (searched for `ChatSend` region lines 7020-7057 — only `ChatSendParams` exists there, immediately preceded by `ChatMessageGetResult` and followed by `ChatAbortParams`). **UNKNOWN** — the ack/result shape of `chat.send` itself is not present as a named schema in this file; the actual streamed data comes through `event: "chat"` frames carrying the `ChatEvent` union (§5), which is what the reference client (`Lh(i,t.runId)` above) actually consumes for UI state.

---

## 5. Streaming (from the webchat client)

Event dispatch table — the single place the webchat client routes every incoming `event` frame, `dist/control-ui/assets/chat-page-DrPkxqJK.js` (function `W_(e,t)`):
```
function W_(e,t){
  if(t.event===`chat`){Ng(e,t.payload),B_(e,t.payload),G_(e);return}
  if(t.event===`chat.side_result`){Pg(e,t.payload)&&G_(e);return}
  if(t.event===`agent`||t.event===`session.tool`){$s(e,t.payload),G_(e);return}
  if(t.event===`session.operation`){Gs(e,t.payload),G_(e);return}
  if(t.event===`chat.send_timing`){rh(e,t.payload);return}
  if(t.event===`session.message`){z_(e,t.payload),G_(e);return}
  t.event===`sessions.changed`&&(V_(e,t.payload),G_(e))
}
```

**The event the UI actually renders streaming replies from is `event: "chat"`.** Its payload is the `ChatEvent` union — a discriminated union on `state`, `dist/schema-DtyqV_v0.d.ts:7064-7150`:

```
ChatDeltaEvent: import("typebox").TObject<{
  state: import("typebox").TLiteral<"delta">;
  message: import("typebox").TOptional<import("typebox").TUnknown>;
  deltaText: import("typebox").TString;
  replace: import("typebox").TOptional<import("typebox").TBoolean>;
  usage: import("typebox").TOptional<import("typebox").TUnknown>;
  runId: import("typebox").TString;
  sessionKey: import("typebox").TString;
  agentId: import("typebox").TOptional<import("typebox").TString>;
  spawnedBy: import("typebox").TOptional<import("typebox").TString>;
  seq: import("typebox").TInteger;
}>;
ChatFinalEvent: import("typebox").TObject<{
  state: import("typebox").TLiteral<"final">;
  message: import("typebox").TOptional<import("typebox").TUnknown>;
  usage: import("typebox").TOptional<import("typebox").TUnknown>;
  stopReason: import("typebox").TOptional<import("typebox").TString>;
  runId: import("typebox").TString;
  sessionKey: import("typebox").TString;
  agentId: import("typebox").TOptional<import("typebox").TString>;
  spawnedBy: import("typebox").TOptional<import("typebox").TString>;
  seq: import("typebox").TInteger;
}>;
ChatAbortedEvent: import("typebox").TObject<{
  state: import("typebox").TLiteral<"aborted">;
  message: import("typebox").TOptional<import("typebox").TUnknown>;
  errorMessage: import("typebox").TOptional<import("typebox").TString>;
  stopReason: import("typebox").TOptional<import("typebox").TString>;
  runId: import("typebox").TString;
  sessionKey: import("typebox").TString;
  agentId: import("typebox").TOptional<import("typebox").TString>;
  spawnedBy: import("typebox").TOptional<import("typebox").TString>;
  seq: import("typebox").TInteger;
}>;
ChatErrorEvent: import("typebox").TObject<{
  state: import("typebox").TLiteral<"error">;
  message: import("typebox").TOptional<import("typebox").TUnknown>;
  errorMessage: import("typebox").TOptional<import("typebox").TString>;
  errorKind: import("typebox").TOptional<import("typebox").TUnion<[import("typebox").TLiteral<"refusal">, import("typebox").TLiteral<"timeout">, import("typebox").TLiteral<"rate_limit">, import("typebox").TLiteral<"context_length">, import("typebox").TLiteral<"unknown">]>>;
  usage: import("typebox").TOptional<import("typebox").TUnknown>;
  stopReason: import("typebox").TOptional<import("typebox").TString>;
  runId: import("typebox").TString;
  sessionKey: import("typebox").TString;
  agentId: import("typebox").TOptional<import("typebox").TString>;
  spawnedBy: import("typebox").TOptional<import("typebox").TString>;
  seq: import("typebox").TInteger;
}>;
```

Field names:
- delta text field: **`deltaText`** (string, present only on `state:"delta"`)
- cumulative/final text field: **`message`** (unknown/opaque shape — carried on every state, is the full cumulative snapshot per prose doc below)
- replace-not-append flag: **`replace`** (bool, delta only)
- run identifier: **`runId`**
- session identifier: **`sessionKey`**
- error signaling: **`errorMessage`** (string) and **`errorKind`** (enum: `refusal`|`timeout`|`rate_limit`|`context_length`|`unknown`), both on the `error` state only
- abort signaling: separate terminal `state:"aborted"` (with optional `errorMessage`), not a boolean flag
- ordering: **`seq`** (integer, all states)

Prose confirmation, `docs/gateway/protocol.md:504-507`:
```
- `chat`: UI chat updates such as `chat.inject` and other transcript-only chat
  events. In protocol v4, delta payloads carry `deltaText`; `message`
  remains the cumulative assistant snapshot. Non-prefix replacements set
  `replace=true` and use `deltaText` as the replacement text.
```

Client-side reconstruction of the running message text, `Eg(e,t)` in `dist/control-ui/assets/chat-page-DrPkxqJK.js`:
```
function Eg(e,t){
  let n=t.message==null?null:G(t.message);
  if(typeof t.deltaText==`string`){
    if(t.replace===!0)return t.deltaText;
    if(e===null)return typeof n==`string`?n:t.deltaText;
    if(typeof n==`string`){
      let r=n.length-t.deltaText.length;
      if(r!==e.length||n.slice(0,r)!==e)return n
    }
    return `${e}${t.deltaText}`
  }
  return typeof n==`string`?n:null
}
```

Terminal-state detector, same file, `wg(e)`:
```
function wg(e){return e===`final`||e===`aborted`||e===`error`}
```
i.e. the client knows a run finished when it receives a `chat` event whose `payload.state` is `"final"`, `"aborted"`, or `"error"` (matching `runId`/`sessionKey`).

`session.message`, `session.operation`, and `session.tool` are separate transcript/tool-stream/session-metadata event families (handled by `z_`, `Gs`, `$s` respectively) — they're consumed for sidebar/session-list/tool-call UI, not for the primary streaming reply text.

---

## 6. sessions.steer, sessions.abort, chat.abort

### sessions.steer

No dedicated `SessionsSteerParams` typebox schema exists anywhere in `dist/*.d.ts` (grepped `dist/*.d.ts` and all of `dist/` for `SessionsSteer` — zero hits). The server implementation, `dist/sessions-UcKjjh_n.js` (near `"sessions.steer": async`):
```
"sessions.steer": async ({ req, params, respond, context, client, isWebchatConnect }) => {
  await handleSessionSend({
    method: "sessions.steer",
    req, params, respond, context, client, isWebchatConnect,
    interruptIfActive: true
  });
},
```
`handleSessionSend`, same file (near `async function handleSessionSend`):
```
async function handleSessionSend(params) {
  if (!assertValidParams(params.params, validateSessionsSendParams, params.method, params.respond)) return;
  ...
}
```
It validates against **`validateSessionsSendParams`** — i.e. `sessions.steer` uses the exact same params schema as `sessions.send` (`SessionsSendParams`), the only difference being `interruptIfActive: true` passed internally, which makes it interrupt an in-flight run instead of queueing behind it. So the effective params schema for `sessions.steer` is:

`dist/schema-DtyqV_v0.d.ts:3682-3690`:
```
SessionsSendParams: import("typebox").TObject<{
  key: import("typebox").TString;
  agentId: import("typebox").TOptional<import("typebox").TString>;
  message: import("typebox").TString;
  thinking: import("typebox").TOptional<import("typebox").TString>;
  attachments: import("typebox").TOptional<import("typebox").TArray<import("typebox").TUnknown>>;
  timeoutMs: import("typebox").TOptional<import("typebox").TInteger>;
  idempotencyKey: import("typebox").TOptional<import("typebox").TString>;
}>;
```
Note: here the session field is called **`key`** (not `sessionKey`), and **`idempotencyKey` is optional** — `handleSessionSend` generates one server-side with `randomUUID()` if the caller omits it (same file):
```
const rawIdempotencyKey = p.idempotencyKey;
const explicitIdempotencyKey = typeof rawIdempotencyKey === "string" && rawIdempotencyKey.trim() ? rawIdempotencyKey.trim() : void 0;
const idempotencyKey = explicitIdempotencyKey ?? randomUUID();
```
`handleSessionSend` then internally dispatches to the `chat.send` handler, translating `key` → `sessionKey`.

Prose doc, `docs/gateway/protocol.md:449`:
```
- `sessions.steer` is the interrupt-and-steer variant for an active session.
```

### sessions.abort

`dist/schema-DtyqV_v0.d.ts:3699-3703`:
```
SessionsAbortParams: import("typebox").TObject<{
  key: import("typebox").TOptional<import("typebox").TString>;
  runId: import("typebox").TOptional<import("typebox").TString>;
  agentId: import("typebox").TOptional<import("typebox").TString>;
}>;
```
All three fields optional — prose, `docs/gateway/protocol.md:450`:
```
- `sessions.abort` aborts active work for a session. Pass `key` plus optional
  `runId`, or `runId` alone for active runs the gateway can resolve to a
  session.
```

### chat.abort (distinct method, used by the webchat UI directly instead of sessions.abort)

`dist/schema-DtyqV_v0.d.ts:7052-7057`:
```
ChatAbortParams: import("typebox").TObject<{
  sessionKey: import("typebox").TString;
  agentId: import("typebox").TOptional<import("typebox").TString>;
  runId: import("typebox").TOptional<import("typebox").TString>;
  preserveSideRuns: import("typebox").TOptional<import("typebox").TBoolean>;
}>;
```
Here `sessionKey` is required (not `key`); `agentId`, `runId`, `preserveSideRuns` optional.

Reference client confirms the webchat UI calls `chat.abort` (not `sessions.abort`) — from the list of `.request(...)` calls found in `dist/control-ui/assets/chat-page-DrPkxqJK.js`: `request(\`chat.abort\`` is present, `request(\`sessions.steer\`` / `request(\`sessions.abort\`` are NOT present in that file (the webchat UI is a `chat.send`/`chat.abort` client; `sessions.*` methods appear to be used by other clients, e.g. CLI/SDK, per the doc's "Session control" accordion).

---

## 7. Session identification and creation

### sessionKey format

`docs/reference/session-management-compaction.md` (section "Session keys (`sessionKey`)"):
```
A `sessionKey` identifies which conversation bucket you are in (routing + isolation). Canonical rules: [/concepts/session](/concepts/session).

| Pattern                      | Example                                                     |
| ----------------------------- | ------------------------------------------------------------ |
| Main/direct chat (per agent) | `agent:<agentId>:<mainKey>` (default `main`)                |
| Group                        | `agent:<agentId>:<channel>:group:<id>`                      |
| Room/channel (Discord/Slack) | `agent:<agentId>:<channel>:channel:<id>` or `...:room:<id>` |
| Cron                         | `cron:<job.id>`                                             |
| Webhook                      | `hook:<uuid>` (unless overridden)                           |
```

`docs/concepts/multi-agent.md:57` (single-agent default):
```
- Sessions key as `agent:main:<mainKey>` (default `mainKey` is `main`).
```
So `"main"` is **not itself the raw sessionKey** in the default single-agent setup — the full key is `agent:main:main` (agentId `main`, mainKey `main`), i.e. `agent:<agentId>:<mainKey>` — agent-qualified as the task description hypothesized, but with the literal prefix `agent:` and both an agentId and a mainKey segment, not simply `"agentId:main"`.

There is also a distinct special value `"global"` used at the API/UI layer (not part of the `agent:...` on-disk key grammar) — seen in the reference client, `dist/control-ui/assets/chat-page-DrPkxqJK.js` (function `Fg`):
```
function Fg(e){
  let t=D(e.sessionKey)?.toLowerCase(),
  n=t===`global`?null:ce(e.sessionKey),
  r=ft(e,e.sessionKey).agentId,
  i=L(e.assistantAgentId??e.agentsList?.defaultId??e.agentsList?.agents?.[0]?.id??`main`);
  return t===`global`?r??i:n??r??i
}
```
and in the server's `sessions.abort` handler (`dist/sessions-UcKjjh_n.js`), which special-cases `scopedRequestedKey?.toLowerCase() === "global"`. Exact full semantics of `"global"` beyond "resolve to the caller's default/effective agent" were not pinned down further — see UNKNOWN below.

### Session creation: explicit vs lazy/implicit

Doc lists `sessions.create` as one explicit method among the "Session control" RPCs, `docs/gateway/protocol.md:447`:
```
- `sessions.create` creates a new session entry.
```
`SessionsCreateParams` / `SessionsCreateResult` schema, `dist/schema-DtyqV_v0.d.ts:3658-3681`:
```
SessionsCreateParams: import("typebox").TObject<{
  key: import("typebox").TOptional<import("typebox").TString>;
  agentId: import("typebox").TOptional<import("typebox").TString>;
  label: import("typebox").TOptional<import("typebox").TString>;
  model: import("typebox").TOptional<import("typebox").TString>;
  parentSessionKey: import("typebox").TOptional<import("typebox").TString>;
  fork: import("typebox").TOptional<import("typebox").TBoolean>;
  emitCommandHooks: import("typebox").TOptional<import("typebox").TBoolean>;
  task: import("typebox").TOptional<import("typebox").TString>;
  message: import("typebox").TOptional<import("typebox").TString>;
  worktree: import("typebox").TOptional<import("typebox").TBoolean>;
}>;
SessionsCreateResult: import("typebox").TObject<{
  ok: import("typebox").TLiteral<true>;
  key: import("typebox").TString;
  sessionId: import("typebox").TOptional<import("typebox").TString>;
  entry: import("typebox").TOptional<import("typebox").TRecord<"^.*$", import("typebox").TUnknown>>;
  runStarted: import("typebox").TOptional<import("typebox").TBoolean>;
  worktree: import("typebox").TOptional<import("typebox").TObject<{ id: import("typebox").TString; path: import("typebox").TString; branch: import("typebox").TString; }>>;
}>;
```
But `chat.send` itself does **not** require a prior `sessions.create` call — its server handler resolves the session directly via `loadSessionEntry`, `dist/chat-pg-BxhF6.js` (inside the `"chat.send": async` handler):
```
const sessionLoadResult = measureDiagnosticsTimelineSpanSync("gateway.chat_send.load_session", () => loadSessionEntry(rawSessionKey, sessionLoadOptions), {...});
```
i.e. it loads-or-creates by key in one step; there is no branch in the `chat.send` handler that calls or requires `sessions.create` first. The reference webchat client (`Rh()` in `dist/control-ui/assets/chat-page-DrPkxqJK.js`, quoted in §4) also never calls `sessions.create` — it calls `chat.send` directly with a `sessionKey` string, confirming session creation on first send is **implicit/lazy** in normal webchat usage; `sessions.create` exists as an explicit RPC for callers that want up-front control (e.g. picking a `model`, forking (`fork`/`parentSessionKey`), or a dedicated `worktree`).

---

## UNKNOWN / not found

- **Exact `chat.send` response/ack payload schema** (the `res` frame's `payload` for a successful `chat.send` request, as opposed to the `chat` event stream). Searched `dist/schema-DtyqV_v0.d.ts` lines 7020-7057 (the region containing `ChatSendParams`) and found no adjacent `ChatSendResult`/`ChatSendResponse` schema; grepped the whole file for `ChatSend` (case-sensitive) and got only `ChatSendParams` and the `ChatSendParamsSchema` `declare const` at line 7828. The client-side code path (`Rh()` → `Lh(i, t.runId)`) treats the RPC response mainly as an ack/error carrier and drives actual UI state off the `event:"chat"` stream instead, so the exact ack shape may simply not be a separately named/exported typebox schema in this build.
- **Full semantics of the `"global"` sessionKey special value** — confirmed it exists and is treated specially in both client (`Fg()`) and server (`sessions.abort` handler) code, but no doc page in `docs/gateway/protocol.md`, `docs/concepts/session.md`, or `docs/concepts/multi-agent.md` defines it in prose; only inferred from code that it resolves to "the caller's default/effective agent" rather than a literal `agent:<agentId>:<mainKey>` string.
- **Whether a raw token ever appears in a URL query string for any non-webchat client** (e.g. legacy bridge/mobile deep links) — only checked the two WebSocket constructor call sites (`dist/control-ui/assets/gateway-CWCQz7bR.js`, `dist/src-DZzKBMa7.js:335`) plus `docs/network.md`/`docs/gateway/*.md`; did not exhaustively check `dist/extensions/browser/chrome-extension/background.js` or the macOS/iOS/Android native app bridging code (not present as JS/TS source in this npm package — those are native binaries), so a URL-embedded token for a mobile-specific transport can't be ruled out from this package alone.
