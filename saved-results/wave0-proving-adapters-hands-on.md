# Wave 0 proving adapters — the hands-on runs, and what each one actually proved

**Date:** 2026-07-31
**Author:** Claude (Opus 5), for Aadivya Raushan
**What this is for:** the plan's Wave 0 exit test asks for two things unit tests cannot give — the three proving adapters *"driven by hand on real hardware with evidence recorded"*, and *"an adapter you have actually switched off remotely"*. This file records what was driven by hand, what came back, and exactly which runs are still blocked and on whom.
**Source:** `planning/consumer-app-implementation-plan.md`, the Wave 0 block (lines 1006–1100).

> **One of the four runs finished. Three are blocked, and all three are blocked on a person, not on code.** Nothing below is claimed as proven unless its transcript is pasted underneath it.

---

## The board

```
  RUN                    STATUS     WHAT IT NEEDED           WHO UNBLOCKS IT
  --------------------   --------   ----------------------   ----------------
  kill switch            PROVEN     nothing outside the      done
                                    process
  --------------------   --------   ----------------------   ----------------
  RT-6 Apple Notes       BLOCKED    macOS to be asked        the owner, with
  on this Mac                       whether this process     one click at the
                                    may control Notes        keyboard
  --------------------   --------   ----------------------   ----------------
  RT-1 Notion MCP        BLOCKED    an OAuth sign-in in a    the owner
                                    browser
  --------------------   --------   ----------------------   ----------------
  RT-4 reply on the      BLOCKED    one inbound message      whoever messages
  Pixel 9                           from Messages or         the owner
                                    Instagram
```

The driver that produces each transcript is `companion/cmd/proveadapter`. It has three subcommands and takes no credential of any kind — there is no flag, environment variable or stdin path in it that can carry a token, key or password.

---

## PROVEN — an adapter actually switched off remotely

The plan asks for an adapter *actually* switched off, not a kill switch with unit tests behind it. This registers both real adapters, switches one off through the same `ApplyKillList` path a remote kill would use, and proves the switched-off one is unreachable through the registry rather than merely flagged.

```bash
go run ./companion/cmd/proveadapter killswitch
```

```
[1] Register both adapters and confirm both are present and routable
    apple-notes: registered and routable
    notion: registered and routable

[2] Apply a kill list that switches off "notion" only
    reason given: proveadapter killswitch: exercising the remote kill switch by hand
    adapters whose on/off state changed: [notion]

[3] Check each adapter's routability, and prove the killed one is refused
    apple-notes: still routable — untouched by the kill list
    notion: REFUSED via registry.Get — adapter disabled: notion
    notion: registry reports disabled=true, reason="proveadapter killswitch: ..."
    adapters still offering "write": apple-notes

[4] Apply an empty kill list and show the adapter comes back
    adapters whose on/off state changed: [notion]
    notion: routable again

VERDICT: kill switch proven: notion was switched off remotely and unreachable
         through the registry, then restored
```

Exit code 0. Two things worth naming: the kill is *targeted* — Apple Notes was untouched — and the switched-off adapter drops out of the "who offers `write`" list, so the router cannot reach it even by capability rather than by name. It also comes back, which matters: a kill switch you cannot undo is a delete.

---

## BLOCKED — RT-6 Apple Notes, on macOS Automation permission

The run gets two steps in and then stops:

```bash
go run ./companion/cmd/proveadapter notes
```

```
[1] Describe the adapter
    runtime:          RT-6
    consent class:    A
    auth:             local
    verbs:            read, write
    declared ceiling: completes

[2] Resolve and preview a write — nothing runs yet
    headline: Create a note in Operator
      - Folder: Operator
      - Title: Operator proving run
      - Body: Written by Operator's Wave 0 proving run for the Apple Notes adapter.
    confirm:  Create note

[3] Execute the write against the real Notes app
execute write: apple notes: creating folder "Operator": osascript create_folder: signal: killed
```

**What "signal: killed" means here.** The adapter's own 15-second budget expired and killed the `osascript` process. It did not fail — it never got an answer.

**Why.** macOS gates one app controlling another behind a per-pair permission, and it has never been asked about this pair. The permission database has no row at all for this process and Notes:

```bash
sqlite3 ~/Library/Application\ Support/com.apple.TCC/TCC.db \
  "select indirect_object_identifier from access
   where service='kTCCServiceAppleEvents' and client='com.anthropic.claude-code';"
```

```
com.apple.finder
com.openai.sky.CUAService
```

Finder and one other target are allowed. Notes is absent — neither allowed nor denied, which is macOS's way of saying "I would ask a human about this." Two checks rule out the alternatives: plain `osascript -e 'return 1+1'` returns `2` immediately, so AppleScript itself is fine, and any script that addresses Notes hangs until it is killed, so the block is the permission and not the adapter. A screen capture taken during the hang showed a wallpaper with no dialog on it — the prompt cannot be raised from a background session, so the request simply waits forever.

**What unblocks it: one click, by the owner, at the keyboard.** Run the command above from a normal Terminal window while sitting at the Mac. macOS will ask whether the app may control Notes; approve it and the run completes. I did not and will not grant this myself — changing a system security setting, and clicking through a security prompt on someone's behalf, are both off-limits.

**What the run will do once approved**, so there are no surprises: create a folder called `Operator` in Notes, write one note in it titled "Operator proving run", read the folder back, and then *attempt* a write into a folder the adapter does not own (`Personal` by default) purely to show it is refused. It touches nothing outside the `Operator` folder. Deleting that folder undoes the whole run.

**What is already proven about this adapter without the live run.** The property that would do real damage if wrong is that note text can never become script. The AppleScript templates are fixed strings beginning `on run argv` that index `item N of argv` (`companion/internal/capability/adapters/applenotes/notes.go:59`), and the runner passes user text after `--` as genuine process arguments (`runner.go:48`). A test feeds the adapter a body containing a quote, a newline, `end tell` and `do shell script "rm -rf ~"`, then asserts the text arrives byte-for-byte in the arguments and appears nowhere in the script source. The live run does not test this; it tests that Notes exists and answers.

---

## BLOCKED — RT-1 Notion, on the owner's OAuth sign-in

```bash
go run ./companion/cmd/proveadapter notion
```

```
[1] What a real Notion proving run would need
    Notion's hosted MCP server: https://mcp.notion.com/mcp
    that server requires user-based OAuth; it refuses static API keys entirely
    (notion.NewWithAPIKey in this codebase always returns notion.ErrBearerNotSupported)
    the sign-in belongs to the repo owner's own Notion account, not to this command
    this command accepts no flag, env var, or stdin input that could carry a token,
    key, or password

[2] Stop here
    a real run needs a browser sign-in by the owner before any adapter call can be made

VERDICT: blocked: needs the owner's OAuth sign-in
```

Exit code 1, deliberately — a blocked run must not read as a pass.

This is the good kind of blocked. Notion's hosted MCP takes OAuth only and refuses bearer tokens outright, which means there is no long-lived secret for Operator to hold and therefore nothing here for the custody gate to protect. `NewWithAPIKey` exists solely to return `ErrBearerNotSupported`, so nobody can quietly create a credential we would then have to look after.

**What unblocks it:** the owner signing in to their own Notion workspace in a browser. Entering a password or authorising an account is not something I do on anyone's behalf.

---

## BLOCKED — RT-4 reply on the Pixel 9, on an inbound message

The phone is connected and the probe is live. Current device state, read straight off the phone:

```bash
adb devices -l
adb shell "run-as app.codexlauncher cat files/notification-reply-probe.txt"
```

```
Pixel 9 (tokay), serial 4B230DLAQ001Z5, connected over USB
listener enabled: app.codexlauncher/...NotificationProbeService

whatsapp    com.whatsapp                      CAN_REPLY     6 sightings
                                              shade action "Reply",
                                              field direct_reply_input
messages    com.google.android.apps.messaging NOT_MEASURED  0 sightings
instagram   com.instagram.android             NOT_MEASURED  0 sightings
messenger   com.facebook.orca                 NOT_MEASURED  0 sightings
messenger   com.facebook.mlite                NOT_MEASURED  0 sightings
signal      org.thoughtcrime.securesms        NOT_MEASURED  0 sightings
whatsapp    com.whatsapp.w4b                  NOT_MEASURED  0 sightings
```

I forced a reconnect, which re-sweeps the whole notification shade — the cheapest way to re-measure without waiting. Nothing changed, so the shade currently holds no message from any of the unmeasured apps. (Sightings moved 7 → 6 across the reconnect because the count is per listener session; the verdict is what carries over, and a yes is never taken back.)

**Which apps are even installed**, checked on the device, because "not installed" and "installed but silent" are different blockers:

```
com.google.android.apps.messaging   INSTALLED
com.whatsapp                        INSTALLED
com.instagram.android               INSTALLED
com.facebook.orca                   not installed
com.facebook.mlite                  not installed
org.thoughtcrime.securesms          not installed
com.whatsapp.w4b                    not installed
```

### UPDATE, later on 2026-07-31 — Instagram answered itself

Re-read the ledger and it had moved without anyone touching it:

```
instagram   com.instagram.android             CAN_REPLY    15 sightings
                                              shade action "Reply",
                                              field DirectNotificationConstants.DirectReply
                                              template MessagingStyle, canned_only=0
```

Real DMs arrived on the phone, the listener swept them on its own, and the verdict flipped. Nothing was staged, no code changed, and no message content was read — the ledger holds package, verdict, count, action label, RemoteInput key and template, and nothing else.

`canned_only=0` is the part that matters: this is a real free-form box, not the suggested-phrases menu Android offers, which the probe counts as a no.

**What is still blocked, and it is narrower than it was.** The reply box is now *proven to exist* on two apps. What has not happened is *firing one* — and firing one sends a real message to a real person from the owner's account, which needs the owner's explicit say-so in so many words. That is not something to infer from "the probe is installed". So RT-4 stays blocked, but on a different thing than before: not on evidence, on permission.

So the remaining blockers are:

- **Messages** is installed and has simply never posted a notification during any probe window. One real inbound SMS settles it. I cannot cause it: Android's inbound SMS broadcast is protected, so it cannot be simulated, and asking someone to text the phone is the owner's call.
- **Messenger and Signal** are not installed. Installing them needs the owner's Play account. Side-loading them is not an option — downloading and running executables from an untrusted source is off-limits, and it would also measure a build Google did not ship, which is not the thing the plan is asking about.

**What the adapter does in the meantime, and this is the point of it.** `NOT_MEASURED` is not a no, but it is emphatically not a yes. The RT-4 adapter turns any verdict other than `CAN_REPLY` into a hand-off — it opens the app rather than claiming a send — because the worst failure this product can produce is telling someone their message went when it did not. So the unmeasured apps are safe to ship *today*; they are just capped at `hands_off` until a real notification measures them.

---

## How to redo any of this

From the worktree root:

```bash
go run ./companion/cmd/proveadapter killswitch
go run ./companion/cmd/proveadapter notion
go run ./companion/cmd/proveadapter notes
```

The first two are safe anywhere. The third writes to the real Notes app and needs the permission click described above — run it from a Terminal window while at the Mac.

For the phone, re-sweeping the shade without waiting for a new message:

```bash
adb shell cmd notification disallow_listener app.codexlauncher/app.codexlauncher.capability.notifications.NotificationProbeService
adb shell cmd notification allow_listener app.codexlauncher/app.codexlauncher.capability.notifications.NotificationProbeService
adb shell "run-as app.codexlauncher cat files/notification-reply-probe.txt"
```

Related files: `saved-results/wave0-notification-reply-probe.md` (the probe's own result and its three rules), `saved-results/wave0-gate-status.md` (the seven gates), `saved-results/custody-gate-token-store-security-model.md`.
