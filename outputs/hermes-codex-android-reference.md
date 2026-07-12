# Hermes as the Reference for a Codex Android Launcher

**Date:** 2026-07-12  
**Hermes source inspected:** `NousResearch/hermes-agent` at commit `4281151ae859241351ba14d8c7682dc67ff4c126`  
**Local Codex inspected:** `codex-cli 0.142.3`, logged in with ChatGPT

## Conclusion

The product in your head is now clear:

> A Codex-native Android launcher that combines the polished task interface of Hermes Desktop with the remote-control behavior of Hermes on WhatsApp, while all real work continues to run on your computer.

That is a coherent product. It is not a custom Android OS, and it is not just a ChatGPT-looking launcher. The launcher is a **remote Codex client and task inbox** that happens to be the phone's home screen.

One correction matters: Hermes does **not** currently contain an Android or iOS graphical app. Its polished app is an Electron desktop client. Hermes can run its command line directly on Android through Termux, and its chat-like phone experience comes through messaging adapters such as WhatsApp. I searched the full 6,250-file Git tree for Android GUI, iOS, React Native, Flutter, Gradle, and mobile app projects and found none. Hermes itself documents Termux as a tested command-line path. [Hermes Android/Termux note](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/README.md#L57-L57) The two graphical references we should combine are therefore:

1. **Hermes Desktop:** full task UI, transcript, live tools, files, previews, settings, voice, and session management.
2. **Hermes WhatsApp:** use the same agent remotely, keep sessions alive, approve actions, answer questions, redirect active work, attach media, and receive progress/completion messages.

Hermes is MIT licensed at the inspected commit, so its code can be studied and reused under the license terms. We should copy the interaction model and useful implementation patterns, not ship Hermes branding as ours.

Here, **exact analogue** should mean testable functional parity for one person's remote-agent workflow. It does not mean a pixel copy, Hermes branding, multi-user WhatsApp group behavior, or every Hermes provider/business feature. Any row in the parity table below that is deferred still remains part of the full target; it is not being silently dropped.

## What Hermes Desktop actually provides

Hermes describes the desktop app as the same agent, skills, memory, configuration, and sessions as its other surfaces. Its React renderer talks over JSON-RPC/WebSocket to a headless `hermes serve` process; it does not embed a terminal UI. That separation is the most useful architectural pattern for us. [Desktop README](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/apps/desktop/README.md#L10-L18) [backend description](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/apps/desktop/README.md#L88-L90)

Verified desktop capabilities include:

- streamed chat and live tool activity;
- structured tool summaries;
- the same conversation history on every Hermes surface;
- side-by-side previews of files, pages, and tool output;
- a working-directory file browser;
- voice input/output;
- provider, model, tool, and credential settings;
- session create, list, resume, branch, rename, delete, interrupt, redirect, undo, compress, save, and usage/status calls;
- attachments for images, PDFs, files, paths, and dropped input;
- active agents and subagent interruption;
- command, skill, tool, plugin, project, process, rollback, and scheduled-task controls.

The last group is based on the registered RPC surface in [`tui_gateway/server.py`](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/tui_gateway/server.py#L5161-L5307), including task controls later in the same file such as [`session.interrupt`, `session.steer`, and `prompt.submit`](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/tui_gateway/server.py#L8114-L8420).

The current Hermes layout also closely matches your initial idea: session/task navigation at the side, recent and pinned work near the top, one quiet main surface, and a composer fixed at the bottom.

![Hermes Desktop task layout](https://raw.githubusercontent.com/NousResearch/hermes-agent/4281151ae859241351ba14d8c7682dc67ff4c126/apps/desktop/pr-assets/session-source-folders.png)

Its written design rules are also a good fit for a minimalist launcher: flat rather than nested cards, whitespace instead of many dividers, shared design tokens, and one reusable control for each job. [Hermes design rules](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/apps/desktop/DESIGN.md#L3-L22)

## What the WhatsApp integration adds

WhatsApp is not a separate reduced agent. It is a transport into the same gateway and persistent sessions. The adapter uses an unofficial Baileys WhatsApp Web bridge, which carries an account-ban risk; we should **not** use that bridge for the launcher. [WhatsApp transport and warning](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/website/docs/user-guide/messaging/whatsapp.md#L7-L31)

The useful product behaviors are:

| Hermes behavior | What it means for our launcher |
|---|---|
| Persistent pairing and reconnect | Pair the Pixel to its computer once; show `Computer offline` when the host cannot be reached; resume automatically when it returns. Multiple computers can follow after the single-host path is reliable. |
| Persistent sessions | The phone lists and resumes the computer's real Codex tasks. It does not make a second, phone-only copy. |
| Progressive replies | Stream Codex commentary, tool state, and final output into the task screen. |
| Tool progress | Show a compact activity line such as `Running tests` or `Reading file`, with details available on tap. |
| Approvals | Show the exact command or file action in a native approval sheet, with deny, allow once, and allowed longer-lived choices when Codex offers them. |
| Clarifying questions | Turn Codex questions into native choices with an optional typed answer; send the answer back to the blocked task. |
| Busy-task controls | Let a new message queue for later, redirect the current turn, or stop it. |
| Background work | Let the user leave the task and receive a notification when it finishes or needs attention. |
| Attachments and voice | Send a photo, document, or dictated prompt from the phone to the computer task. |
| Security controls | Pair, list paired devices, revoke a device, protect credentials, and never copy the computer's service credentials onto the phone. |

After Codex replied and the task is idle, the launcher labels the row `Replied`. This describes an observed response without implying that the broader task is permanently complete.

Evidence for these behaviors:

- The WhatsApp login survives restart and temporary transport disconnects reconnect automatically. [WhatsApp credential-session persistence and reconnect](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/website/docs/user-guide/messaging/whatsapp.md#L145-L166)
- Conversation routing and transcripts persist separately in Hermes's SQLite state database. [`SessionStore` persistence](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/gateway/session.py#L986-L990) [routing database description](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/gateway/session.py#L1266-L1274)
- Voice notes, streamed edits, and real-time tool progress are supported. [Voice and delivery](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/website/docs/user-guide/messaging/whatsapp.md#L170-L210)
- The adapter sends images, video, voice, documents, typing state, native polls, and location. [Media code](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/plugins/platforms/whatsapp/adapter.py#L1040-L1187)
- The gateway can block an agent on a question and continue after the response. [Clarification callback](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/gateway/run.py#L18457-L18529)
- Plain `yes`, `approve`, `no`, or `deny` responses are routed around the normal busy queue so a waiting approval cannot deadlock. [Approval routing](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/gateway/run.py#L5319-L5392)
- Hermes exposes separate `queue`, `steer`, `stop`, background, agent/task, branch, undo, rollback, goal, model, reasoning, voice, and session commands. [Command registry](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/hermes_cli/commands.py#L64-L170)

WhatsApp-specific workarounds we should leave behind:

- the unofficial WhatsApp bridge and phone-number access rules;
- splitting replies at 4,096 characters;
- converting Markdown into WhatsApp syntax;
- waiting five seconds to combine rapid message fragments;
- group-chat mention and thread rules;
- WhatsApp-specific locations and polls where ordinary Android UI is better.

## The Codex side is more feasible than it first appeared

OpenAI now has an official **Codex Remote** feature in the ChatGPT mobile and desktop apps. According to OpenAI's current documentation, rather than the local protocol schemas, it can start or continue Codex tasks on a paired computer, send follow-ups, approve actions, inspect diffs/tests/terminal output/screenshots, notify the user, switch hosts, and report when a host is unavailable. The host must remain awake and online. [OpenAI Remote connections](https://learn.chatgpt.com/docs/remote-connections)

This is important for two reasons:

1. You can use the official Remote experience immediately to test whether this way of working fits your daily life before we write the launcher.
2. It validates the product shape, but it does **not** give us the custom Hermes-like launcher UI you want.

I also generated the protocol schemas from the locally installed `codex-cli 0.142.3`. The stable app-server surface contains the building blocks for a custom client:

- task start, list, read, resume, fork, archive, delete, rollback, and command context;
- turn start, redirect, and interrupt;
- model, skills, apps, plugins, permissions, review, and file access;
- streamed task status, item start/completion, command output, patches, diffs, plans, and final completion;
- requests for command approval, file-change approval, and added permissions.

The non-experimental schema output also contains a user-question request, but the type itself is explicitly labelled `EXPERIMENTAL`; we cannot treat native question cards as a stable contract yet. The local schema also contains first-party Remote pairing/revocation, live audio, and raw process controls only when the schema is generated with experimental APIs enabled. That is verified locally, not promised as a public third-party mobile-client contract.

Therefore, I would **not** build our app by pretending to be the official ChatGPT Remote client or by exposing Codex app-server directly to the internet. OpenAI's own documentation warns against exposing app-server directly. The safe, maintainable design is a small adapter on your computer that talks to Codex locally and presents our own narrow, versioned mobile API.

## Exact Hermes-to-Codex parity map

| Hermes capability | Codex support verified here | What our computer adapter or Android app must add | Full parity target |
|---|---|---|---|
| New, list, resume, rename, fork, archive, delete tasks | Stable task methods cover all except a direct save/export equivalent | Mobile task list, local search index, grouping, and export if wanted | Required |
| Retry, undo, compress, rollback | Rollback exists; the other Hermes meanings do not map one-for-one | Define safe equivalents using task fork/rollback/new turns; never label unlike actions as identical | Required, after semantics are designed |
| Streamed answer and tool progress | Stable item start/complete, text, command-output, patch, plan, diff, status, and completion events | Render a compact activity view and detailed event sheet | Required |
| Follow-up while busy | Stable redirect (`turn/steer`) and stop (`turn/interrupt`) | **Queue is not a stable Codex method.** The adapter must persist queued prompts and submit them after completion | Required |
| Risky-command and file approvals | Stable structured approval requests | Security sheet showing host, project, paths, exact redacted command, scope, and duration; handle timeout/deny safely | Required |
| Clarifying questions | Present in stable schema output but explicitly marked experimental | Capability check plus a plain-message fallback until stable | Required with fallback |
| Background prompts and completion | Tasks can keep running on the host | Durable adapter state plus Android notification delivery | Required |
| Active agents/tasks and subagents | Task state is stable; complete subagent controls were not proven stable in this check | Show what Codex exposes; do not invent Hermes's agent tree | Target, exact limits need a later protocol check |
| Projects, repositories, and working folders | Task start/resume and file methods provide the base | Project/worktree grouping and safe folder selection UI | Required |
| Files, previews, diffs, terminal output | Files, patches, diffs, and command output are stable | Mobile viewers. Tests and screenshots have no dedicated proven contract; show them when they arrive as command/tool/file items | Required |
| Voice, images, video, audio, documents, captions | Files/images can be passed through; live audio is experimental | Android dictation first; upload/copy media to the host; later add richer audio | Required, staged by media type |
| Models, reasoning, permissions, skills, plugins, apps | Relevant stable list/config methods exist, with some individual operations still needing capability checks | Mobile settings surfaces driven by host capabilities | Required |
| Goals, scheduled tasks, memory, profiles, handoff | Not all have direct, stable Codex equivalents | Use Codex-native features where available; otherwise mark unsupported rather than fake parity | Full target requires separate decisions |
| Usage/context and session status | Status and usage notifications exist | Small task metadata/status panel | Required |
| Updates and onboarding | Not a task protocol concern | Pairing, host-adapter update, compatibility, and recovery UI | Required |
| Multi-user/group/phone-number rules | Not relevant to a personal launcher | None | Deliberately excluded |

Hermes-specific details that the parity tests must cover include `/new`, `/resume`, `/sessions`, `/retry`, branching, background work, active tasks, goals, approval scopes, denial, stop, redirect, queue order, question timeout, media/captions, and reconnect. The source registry for those behaviors is [`hermes_cli/commands.py`](https://github.com/NousResearch/hermes-agent/blob/4281151ae859241351ba14d8c7682dc67ff4c126/hermes_cli/commands.py#L64-L170).

## Proposed product shape

```text
Pixel 9, set as the Android home app
┌──────────────────────────────────────────┐
│ Recent and active Codex tasks            │
│ Task transcript + live activity          │
│ Native questions and approval sheets     │
│ Files, diffs, screenshots, test output   │
│ Composer, attachment, voice              │
│ Settings + hidden full app drawer        │
└───────────────────┬──────────────────────┘
                    │ encrypted, paired connection
                    ▼
Always-on computer adapter
┌──────────────────────────────────────────┐
│ Pair / revoke phone                      │
│ Offline / reconnect state                │
│ Notification delivery                    │
│ Stable mobile contract                   │
│ Translate to current Codex protocol      │
└───────────────────┬──────────────────────┘
                    │ local connection only
                    ▼
Codex app-server + the computer's files,
projects, credentials, permissions, tools,
and browser/computer-use capabilities
```

The phone should hold only hardware-backed pairing keys, cached task metadata, drafts, and user preferences. Your ChatGPT/Codex login and all provider or service credentials remain on the computer.

The connection choice still needs a product decision before implementation:

- **Private mesh network:** simplest personal build and no public host port. It needs careful Android background handling, and notifications may be delayed while the app sleeps.
- **End-to-end encrypted relay plus push notifications:** smoother away-from-home behavior, but it adds a hosted service, key rotation, relay privacy rules, and more security work.

Whichever path we choose must define device identity, QR pairing, key rotation/revocation, replay protection, protocol version negotiation, and a reconnect cursor. The adapter must store queued prompts durably, assign each action an ID so retries cannot execute it twice, and reconcile actions taken from desktop and phone. Push notifications should contain only minimal task state, never commands, prompts, file contents, or credentials.

## Feature boundary for the first real version

The first version should feel complete for normal Codex work, not reproduce every Hermes setting.

### Build first

- launcher home with the last few active/recent tasks above a blank prompt;
- create, resume, switch, rename, archive, and fork tasks;
- stream assistant text and structured tool activity;
- redirect, adapter-backed durable queue, stop, and send follow-up messages;
- native approval screens and question screens with a fallback while Codex's question contract remains experimental;
- select the computer, project/folder, model, reasoning level, and permission mode;
- show diffs, changed files, command output, and useful attachments; recognize tests and screenshots when they arrive through ordinary tool/command/file items;
- notifications for completed, failed, waiting-for-approval, and waiting-for-answer states;
- a clear `Computer offline` state with retry and host switching;
- an always-available escape gesture/button to Android Settings and the full app drawer, including when the computer is offline.

### Add after the main loop works

- richer voice conversation;
- subagent tree and active-agent controls;
- skill, plugin, and app management;
- scheduled tasks and long-running goals;
- interactive terminal sessions;
- multiple computers and cross-device handoff polish.

The later list is still part of the full parity target. It is staged because parts of the current Codex protocol, especially direct Remote pairing, live audio, raw process control, and user-question UI, are marked experimental in the locally generated schema, while some Hermes concepts do not have one-to-one Codex meanings.

Approval behavior is a first-version safety requirement, not polish. The phone must show the computer, project, affected paths, command after host-side secret redaction, requested permission, and whether approval lasts once, for the task, or longer. A timeout, disconnect, ambiguous response, or simultaneous pending question must fail closed and leave a clear recoverable state.

## What this means for UI design

We should now design the UI as a **functional Hermes-for-Codex adaptation**, not as a generic ChatGPT clone:

- Hermes's task-focused information layout;
- WhatsApp's ability to control an agent from anywhere;
- Codex-native concepts such as diffs, approvals, project folders, tool activity, tests, and computer state;
- Android launcher duties such as Home, app drawer, Settings access, notifications, and offline behavior.

The next design step should produce visual options for four screens before any implementation plan is approved:

1. launcher home / recent tasks / new prompt;
2. active task with live tool work;
3. approval or question waiting for you;
4. computer offline / reconnect.

## Reproduce the research

```bash
git clone --filter=blob:none --sparse https://github.com/NousResearch/hermes-agent.git
cd hermes-agent
git checkout 4281151ae859241351ba14d8c7682dc67ff4c126
git sparse-checkout set apps/desktop apps/shared gateway plugins/platforms/whatsapp \
  scripts/whatsapp-bridge tui_gateway tests/gateway tests/tools \
  website/docs/user-guide/messaging hermes_cli

codex --version
codex login status
codex app-server generate-json-schema --out /tmp/codex-schema-stable
codex app-server generate-json-schema --experimental --out /tmp/codex-schema-experimental
```

No paid API call was made for this research.
