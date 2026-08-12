# Beeper Discord/Messages send: recipient aliases (Phase 4 hard-gate)

**Date:** 2026-08-12  
**For:** Pixel overnight Phase 4 hard-gate proofs — Discord/Messages send died with `ErrNoRecipient` before the Operator Approve sheet could appear.  
**Branch:** `cursor/beeper-send-recipient-aliases-09e6` against `worktree-phase2-tool-bridge`

## What it is for

First-contact Discord/Messages sends must reach Resolve + Preview so the hard-gate policy can raise `approval_required` and the launcher can show Approve. They were failing closed on an empty `subject` even when the model had named the person in `handle`, `to`, `recipient`, or `chat_id`.

## Result

Verified in this session:

- Hypothesis holds: `beepermessage.resolveSend` used only `in.Subject` (`adapter.go` around the old `recipient := strings.TrimSpace(in.Subject)` check). Empty subject → `ErrNoRecipient` during Resolve, which runs *before* Preview and the gate (`bridge.go` `handleCall`: Resolve → Preview → `Policy.Evaluate`).
- `handle` was already a first-class tool field and was forwarded by the plugin, but the adapter ignored it for send.
- Fix: accept `subject`, then `handle`, then `fields.to` / `fields.recipient` / `fields.chat_id`. Plugin also copies top-level `to` / `recipient` / `chat_id` onto `subject`. Still `ErrNoRecipient` if every alias is blank. First-contact Approve is unchanged.

## Tests (red → green)

Red (before the code change), from this session:

```
TestSendResolvesRecipientFromHandleAndFieldAliases/...  Resolve: beeper message: recipient must not be empty
TestMessagesSendAcceptsHandleAsPhoneRecipient           Resolve: beeper message: recipient must not be empty
TestManageResolvesRecipientFromHandle                   Resolve: beeper message: recipient must not be empty
TestListMessagingSchemaNamesRecipientAliases            discord schema missing "to"
plugin: discord schema missing to
plugin: handle-only call left subject undefined
```

Green (after):

```
go test -count=1 -p 1 ./companion/internal/capability/adapters/beepermessage/ ./companion/internal/phoneruntime/agentbridge/
ok  beepermessage  0.002s
ok  agentbridge    0.011s

cd agentbridge/openclaw-plugin && npm test
# tests 10, pass 10, fail 0
```

`go vet` on `agentbridge` still reports a pre-existing unkeyed `execution.Preview` literal in `handleDisconnectCall` (present on the base branch; not introduced here).

## How to re-prove safely (self / known contact only)

Do **not** message strangers. Use a chat with yourself or one known contact you already talk to.

1. Rebuild/install the operator-tools plugin so the phone agent picks up `tools.ts` (aliases + descriptions) and this companion binary.
2. Ask the agent to send a short Discord or Messages ping to **yourself** or that known contact. A typical model call that used to fail: `messages`/`discord` with `verb=send`, `handle` or `to` set, `subject` omitted, `body` a harmless test line.
3. **First contact** (this recipient never messaged through the bridge before): nothing should send. The tool result should read as waiting for owner Approve (not `recipient must not be empty`). The launcher Approve sheet should appear. Typed chat text must not release it — only Approve.
4. Tap **Decline** if you do not want the message to go out. Tap **Approve** only if you intend to send that exact preview to yourself/the known contact.
5. Repeat to a recipient already marked known and allow-listed if you want the ungated path; still keep it self/known.

Hard gates are not weakened: empty recipient still fails closed; first-contact still requires Approve.

Follow-up 2: `ToolCallRequest` now has top-level `to` / `recipient` / `chat_id` (Go + plugin wire types), so a direct `/call` matches the list schema. `TestAliasFirstContactStopsForApproval` covers `fields.to`, top-level `to`, and handle-only.

## Sibling sites checked

Searched `strings.TrimSpace(in.Subject)` under `companion/internal/capability/adapters/`. Slack, Teams, Outlook, notification-reply, maps, etc. still read only `subject`. Left them: they are not the Beeper Discord/Messages send path. The plugin now copies `handle`/`to`/`recipient`/`chat_id` onto `subject` for every tool, so those adapters get the model-side aliases without a behavior change to their Resolve. Instagram Beeper send shares `beepermessage` and is covered.

Out of scope (per request): Home Send / `new_task_options`; soft-attest flags.

## Reproduce

```
cd companion
go test -count=1 -p 1 ./internal/capability/adapters/beepermessage/ ./internal/phoneruntime/agentbridge/

cd ../agentbridge/openclaw-plugin
npm test
```
