# BYO ChatGPT login gate (phone-agent)

Date: 2026-08-13

## Why

Operator must not pay model cost. Home Send stays off until OpenClaw has the
**user’s ChatGPT OAuth profile** (device-code). An API key is not enough.

## Flow

```
  app open
      |
      v
  silent local-pair + :9443
      |
      v
  GET /v1/health
      |
      +-- modelAuth != oauth_ready --> MODEL_AUTH destination
      |         |                         showComposer=false
      |         |                         canSend=false
      |         v
      |    Continue with ChatGPT
      |         |
      |         v
      |    POST /v1/model-auth/start
      |         |
      |         v
      |    show user code + ACTION_VIEW verification URL
      |         |
      |         v
      |    poll health until oauth_ready AND taskCapable
      |
      +-- both true --> Home composer + Send
```

OpenClaw keeps the tokens. Operator never stores ChatGPT tokens on Android.

## Inputs → Outputs → Steps

1. **Inputs** — OpenClaw `models auth list --provider openai` (id/type only);
   device-code CLI stdout; Android health poll.
2. **Outputs** — `modelAuth`: `oauth_ready` | `missing` | `pending`; start JSON
   `{ userCode, verificationUrl }` only; Home gated until both `taskCapable`
   and `oauth_ready`.
3. **Steps**
   1. Classify list: oauth type → ready; api_key only → missing; in-flight login → pending.
   2. Start spawns `openclaw models auth login --provider openai --device-code`.
   3. After oauth, prefer that profile in `auth.order.openai` and restart gateway if the order changed.
   4. Android MODEL_AUTH screen (simple authorize UI, not Mac pairing copy).

## Out of scope

`OPERATOR_ALLOW_SOFTWARE_ATTEST`. ChatGPT tokens in the Android app.
Changing the default model (keep luna/sol).
