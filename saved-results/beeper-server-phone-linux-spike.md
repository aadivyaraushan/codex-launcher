# Beeper Server on phone Linux — spike result

**Date:** 2026-08-03
**What for:** Answers the one open question in `planning/operator-complete-messaging-plan.md` — can a headless Beeper Server run inside a Linux userspace on the Pixel 9 itself, serving its Desktop API on `127.0.0.1:23373`, so an app on the phone can send a chat message as the user. This is the P2 spike gating "messaging COMPLETE" in that plan.
**Who reads this:** whoever picks up `operator-complete-messaging-plan.md` next; the plan's Spike-fail exit table points here.
**Device:** Pixel 9, serial `4B230DLAQ001Z5`, model `tokay`, Android build with `ro.boot.hypervisor.vm.supported=1` (AVF-capable hardware).

---

## CORRECTION 2026-08-03 (later the same day) — this spike tested the wrong artifact

**Do not cite this file as proof that messaging is blocked.** The verdict below
is honest about what it ran, but what it ran was not the right thing, and its
central factual claim is false.

**The false claim.** "The only Linux build Beeper publishes is the Beeper Desktop
Electron/AppImage." Measured today against Beeper's own download feed, a
**dedicated headless server build exists**:

```
GET https://api.beeper-staging.com/desktop/update-feed.json
      ?bundleID=com.automattic.beeper.server.nightly&platform=linux&channel=nightly&arch=arm64
→ {"version":"4.3.8",
   "url":".../beeper-server-nightly-4.3.8-linux-arm64.tar.gz",
   "download_size":"171570946",
   "pub_date":"2026-08-03T18:25:02+00:00"}
```

That tarball was downloaded and inspected. It is **not an AppImage and not an
Electron bundle**:

| property | value |
|---|---|
| tarball contents | **2 entries** — one directory, one file named `beeper-server` |
| type | ELF 64-bit aarch64 executable, 236 MB, dynamically linked |
| interpreter | `/lib/ld-linux-aarch64.so.1` (glibc) |
| shared libs needed | `libdl, libstdc++, libm, libgcc_s, libpthread, libc` — **and nothing else** |
| Chromium / Electron | **absent** — no `chrome-sandbox`, no `libffmpeg`, no `CrashpadHandler` |

An Electron bundle is a directory tree of hundreds of files. This is one
console-only binary whose entire dependency list is glibc and libstdc++.
(A string search does hit "electron" 14 times; all are incidental — ICU's
*electronvolt* measure unit, a Node.js source comment, and a compression
dictionary.)

**Why that matters for the recommendation at the end of this file.** That
recommendation says "do not attempt proot-distro Debian inside Termux as a
substitute", for a specific reason: *"its known Electron/proot
incompatibilities"* — Chromium's sandbox conflicting with proot's ptrace-based
syscall interception. That reason is correct **for Electron** and **cannot apply
to this binary**, which has no Chromium, and needs no sandbox, no FUSE and no
display. proot-distro was ruled out on grounds that do not hold for the artifact
we actually want to run.

**Also missed:** there is an official **`beeper-cli`** (npm `beeper-cli` v0.6.2,
or `brew install beeper/tap/cli`; same maintainer as the official
`@beeper/desktop-api`). It installs and supervises that server itself
(`beeper install server`, `targets start/stop/logs`), links each network from the
shell via `beeper accounts add` (QR/OAuth — which answers this plan's *other*
open step, "account link path that works without the Beeper Desktop GUI"), and
supports `targets add remote` / `targets tunnel`, meaning **the phone may not
need a local Beeper at all.**

**Verified working, read-only, against the Mac's Beeper Desktop 4.3.0**, using
the `BEEPER_ACCESS_TOKEN` already in `.env` with `BEEPER_READONLY=1`:
`accounts list` → Instagram, Discord, Google Messages, Matrix all `connected`;
`chats list` → 66 threads, **every one with an addressable id** (18 Instagram,
35 Google Messages, 11 Discord).

**Still genuinely untested:** whether `beeper-server` runs under proot-distro on
the Pixel. Bare Termux is bionic and has no glibc loader, so the binary will not
run there directly — proot-distro is what supplies the glibc rootfs that would
make it possible. That question is open, not answered either way.

**Status of the demotions below: withdrawn pending that test.** Instagram DM,
Discord and Google Messages must not be recorded as HAND-OFF *on the strength of
this file*.

---

## Verdict: FAILED (superseded — read the correction above first)

**One-sentence cause:** the only Linux build Beeper publishes is the Beeper Desktop Electron/AppImage (also used as "Beeper Server" per the official CLI) and it cannot execute in the Linux userspace actually reachable on this phone without a human tap — it downloaded and had exec permission, but running it produced `Could not find a PHDR: broken executable?` and aborted (`exit 134`, SIGABRT) before ever binding a port.

**Demotions per the plan's spike-fail exit table, row 1 ("P2 userspace or Beeper Server won't run on phone")**: messaging COMPLETE is blocked for v1 — Instagram DM, Google Messages, and Discord all stay at whatever their pre-spike ceiling was (i.e. do **not** mark them COMPLETE via `beeper_server_localhost`). The Owner may reopen Browserbase for Meta only via a new decision; that is not automatic.

I did not reach the send gate (step 6) — the Server never came up, so there was nothing to point Operator at, no account-link test was meaningful, and no message left the phone. Steps 1–2 of the plan's order-of-work were completed; step 3 (Linux userspace) was completed with the lab fallback; step 4 (run the Server) is where it failed.

---

## Step 1 — API contract (learned against the Mac's already-running Beeper Desktop, port 23373)

Docs fetch of `https://developers.beeper.com/desktop-api` states: *"Beeper Desktop API runs inside Beeper Desktop and requires Beeper Desktop to be running to be accessible."* The docs reference page (`/desktop-api-reference/`) 404'd; the real contract was read straight off the running instance's self-describing `/v1/info` and `/v1/spec` (OpenAPI) endpoints instead — more reliable than the docs site.

Commands run (against `127.0.0.1:23373` on the Mac, using `BEEPER_ACCESS_TOKEN` from repo `.env`):

```
curl -s http://127.0.0.1:23373/v1/info
→ {"app":{"name":"Beeper","version":"4.3.0","bundle_id":"com.automattic.beeper.desktop"},
   "platform":{"os":"darwin","arch":"arm64"},
   "server":{"status":"running","base_url":"http://127.0.0.1:23373","port":23373},
   "endpoints":{"spec":"http://127.0.0.1:23373/v1/spec", ...}}

curl -s -H "Authorization: Bearer $BEEPER_ACCESS_TOKEN" http://127.0.0.1:23373/v1/accounts
→ [{"accountID":"matrix", ...}, {"accountID":"discordgo","network":"Discord", ...},
   {"accountID":"gmessages","network":"Google Messages", ...},
   {"accountID":"instagramgo","network":"Instagram", ...}]   # all four already "connected"

curl -s -H "Authorization: Bearer $BEEPER_ACCESS_TOKEN" "http://127.0.0.1:23373/v1/chats?limit=50"
→ 25 chats returned (Instagram DMs, Google Messages threads, a Discord DM), full title/participant/capabilities shape.
```

Unauthenticated request to any `/v1/*` path returns `404 {"message":"Not found","code":"not_found"}` — the earlier assumption of a 401 on missing token was wrong for this build; missing/invalid auth reads as a route-not-found, not an auth error. (Correction to the task brief's assumption, confirmed by curl.)

Send endpoint, from `GET /v1/spec` (OpenAPI 775 KB doc):

```
POST /v1/chats/{chatID}/messages
Header: Authorization: Bearer <BEEPER_ACCESS_TOKEN>
Body:   {"text": "<plain text or markdown>", "replyToMessageID"?: "...", "attachment"?: {...}}
```

Exact curl that would send (not executed — see below):

```
curl -X POST -H "Authorization: Bearer $BEEPER_ACCESS_TOKEN" -H "Content-Type: application/json" \
  -d '{"text":"..."}' \
  "http://127.0.0.1:23373/v1/chats/<chatID>/messages"
```

**Did not execute a real send.** I chose a low-stakes target (a chat with the owner's own second Instagram handle) and attempted it once for real, as the task asked ("record the exact working curl for a send") — the harness's auto-mode classifier blocked the outbound POST as an irreversible action outside the read-only probing this step needed. I did not try to route around that block. The endpoint, auth, and body shape above are confirmed correct by the OpenAPI spec plus the fact that GET calls with the identical header worked; only the actual firing of a message is unverified from the Mac, and per the task's own instructions a Mac-side send would not have proven anything about the phone anyway.

---

## Step 2 — arm64 Linux build: exists

```
curl -sI https://api.beeper.com/desktop/download/linux/arm64/stable/com.automattic.beeper.desktop
→ HTTP/2 302, location: https://beeper-desktop.download.beeper.com/builds/Beeper-4.3.0-arm64.AppImage
```

Source: `api.beeper.com` redirect (Beeper's own download API), version 4.3.0, format AppImage (Electron). The Beeper CLI docs (`developers.beeper.com/desktop-api-reference/cli`, via search cache) describe `beeper setup --server --install` as installing "a local Beeper Server" for headless use, and the community AUR package (`aur.archlinux.org` PKGBUILD for `beeper-v4-bin`) confirms this is literally the same Electron AppImage — extracted with `--appimage-extract`, wrapped in an `AppRun` shell script, depending on `libappindicator-gtk3 libnotify libsecret hicolor-icon-theme`. **There is no separate lightweight "Beeper Server" binary** — Server and Desktop are the same GTK/Electron build; "server" only means "run it without opening a window."

No dedicated `/server/download/...` endpoint exists (`curl -sI https://api.beeper.com/server/download/linux/arm64/stable/...` → `404`).

---

## Step 3 — Linux userspace on this Pixel 9

**3a. AVF Linux Terminal (the product-path P2 candidate): present but not startable without a human.**

```
./scripts/pixel-lock.sh 120 adb -s 4B230DLAQ001Z5 shell "pm list packages | grep -i -E 'terminal|virt|termux|linux'"
→ package:com.termux
  package:com.google.android.virtualmachine.res
  package:com.android.virtualization.terminal
  package:com.virtusan.application

./scripts/pixel-lock.sh ... shell "dumpsys package com.android.virtualization.terminal | grep -E 'enabled=|flags='"
→ enabled=0 (component-level, not the package's own default)
  Activity Resolver Table shows LauncherActivity / InstallerActivity / MainActivity all registered

adb shell am start -n com.android.virtualization.terminal/.LauncherActivity
→ Error type 3: Activity class ... does not exist.

adb shell pm enable com.android.virtualization.terminal
→ "new state: enabled" (package itself)
adb shell pm enable com.android.virtualization.terminal/.LauncherActivity
→ SecurityException: Shell cannot change component state ... (component gated, not a plain pm-enable target)

adb shell am start -a android.virtualization.VM_TERMINAL
adb shell am start -n com.android.virtualization.terminal/.InstallerActivity
→ SecurityException: Permission Denial: ... not exported from uid 10344
```

Conclusion: the AVF Linux Terminal app ships on this Pixel but its entry points are gated behind Android's Developer Options "Linux Terminal" toggle (a Settings UI switch), which shell/adb cannot flip — this is an OS-level protection, not a missing app. **Enabling it requires the owner to open Settings → System → Developer options and turn the feature on, then walk the Installer app's first-run flow** (which itself downloads a multi-GB Debian rootfs). This is a genuine human-tap requirement, matching the plan's "needs human step" category, not a hard block — but it took it off the table for this spike's time budget.

**3b. Termux — used as the lab fallback (explicitly allowed by the plan, not the product path).**

Termux (`com.termux`) was already installed and fully usable — launched via `adb shell am start -n com.termux/.app.TermuxActivity`, driven by `adb shell input text` / `input keyevent` (typed commands, screenshots to verify each step — no owner interaction). Confirmed real Linux userland:

```
uname -a
→ Linux localhost 6.1.145-android14-11-gfa1d6308d1fe-ab14691759 #1 SMP PREEMPT ... aarch64 Android
which curl bash
→ /data/data/com.termux/files/usr/bin/curl
  /data/data/com.termux/files/usr/bin/bash
df -h $HOME
→ 109G total, 30G available
ls /lib64/ld-linux-aarch64.so.1
→ No such file or directory   # confirms Termux is Android bionic userland, not glibc
```

---

## Step 4 — running the Server: this is where it failed

```
curl -sL -o beeper.AppImage https://beeper-desktop.download.beeper.com/builds/Beeper-4.3.0-arm64.AppImage
→ 273,410,824 bytes, matches Content-Length exactly (fast download over the phone's own Wi-Fi)

chmod 755 beeper.AppImage
./beeper.AppImage --appimage-extract-and-run --no-sandbox --headless > run.log 2>&1
→ Aborted (shell); echo $? → 134 (SIGABRT)

cat run.log
→ Could not find a PHDR: broken executable?
```

`--appimage-extract-and-run` bypasses the need for FUSE (which Termux also lacks — no `/dev/fuse` access without root), so the AppImage's own bundled extraction ran, but the underlying binary's loader could not locate its program headers under Android's process-exec model. This is the same failure widely reported for AppImages run directly on Android/Termux: Android's exec/mmap path for a foreign-loader ELF differs enough from a normal Linux kernel that AppImage's static runtime can't bootstrap it. The Server never printed anything about binding a port; `curl 127.0.0.1:23373` from inside Termux was never worth trying because the process was dead before any listener could exist.

I did not additionally try `proot-distro install debian` (a further-nested glibc chroot inside Termux) — that is a real next lever, but it is a multi-step, multi-hundred-MB additional install whose own known failure mode (Chromium/Electron's sandbox conflicting with proot's ptrace-based syscall interception) is itself well documented as unreliable, and pursuing it would have pushed well past the plan's ~90-minute spike budget without a strong reason to expect a different outcome for an Electron/GTK app specifically.

---

## Note on a shared-phone interruption (not caused by this spike)

Mid-spike, an unrelated screen (an Instagram login page, later an Android home screen) appeared on the phone between my own commands. The coordinator confirmed separately that this was the owner installing/logging into Instagram on the device concurrently — expected on a phone shared between agents per the task's ground rules, not something this spike's commands triggered. I did not interact with those screens (no text entered, no buttons tapped) and returned to Termux each time via `am start` before continuing.

Also per the coordinator: Instagram, Discord, Google Messages, WhatsApp, and the Beeper Android app are all installed on this Pixel. The Beeper Android app (`com.beeper.android`) is **not** the same thing as Beeper Server and was not used as the agent-send path here, consistent with the plan explicitly ruling out the Content Provider route — it was not touched in this spike.

---

## What a human would need to do to unblock this

1. **Fastest unblock attempt (not guaranteed):** on the Pixel, go to Settings → System → Developer options → enable the "Linux Terminal" (AVF) toggle, open the Android Virtualization Framework Terminal app, and walk its first-run installer (downloads a Debian image, several GB, needs Wi-Fi). If that succeeds, re-run this spike's steps 4–6 inside that real Debian VM instead of Termux — a genuine glibc Linux kernel there has a real chance of running the Beeper AppImage where Termux's bionic userland could not.
2. **If (1) still fails or the owner does not want to hand-hold that setup:** treat "Beeper Server on phone" as not viable for v1 and apply the plan's spike-fail exit row 1 as final — do not attempt proot-distro Debian inside Termux as a substitute; its known Electron/proot incompatibilities make it a low-odds, high-effort path.
3. **Either way:** re-run the plan's Policy supersession section (`planning/consumer-app-implementation-plan.md`) to keep Instagram DM / Google Messages / Discord at their pre-spike ceiling (not COMPLETE via `beeper_server_localhost`) until a green re-run of this spike exists.
4. **If the Owner wants to keep messaging COMPLETE moving without a phone-Linux Server:** that requires a new decision (per the plan's own exit table) to reopen Browserbase for Meta nets only — not something this spike can authorize.
