# The Beeper route: what was wrong, what is built, what is left

**Date:** 2026-08-03
**Why this exists:** the owner corrected a course I had taken — *"we agreed NOT
to do a hand off for instagram remember. our goal was to use the beeper cli."*
Chasing that correction showed the plans' "messaging is blocked" verdict rests on
a spike that tested the wrong artifact. This records the disproof, the client
built on the back of it, and the one question still genuinely open.
**Who reads this:** whoever picks up messaging next.

---

## 1. The blocker was wrong

`planning/operator-complete-messaging-plan.md` and
`planning/consumer-app-implementation-plan.md` both demoted **Instagram DM,
Discord and Google Messages** to HAND-OFF, citing
`saved-results/beeper-server-phone-linux-spike.md`. That spike concluded:

> "the only Linux build Beeper publishes is the Beeper Desktop Electron/AppImage"

**That is false.** Beeper's own update feed serves a dedicated headless build:

```
GET https://api.beeper-staging.com/desktop/update-feed.json
      ?bundleID=com.automattic.beeper.server.nightly&platform=linux&channel=nightly&arch=arm64

→ {"version":"4.3.8",
   "url":"https://beeper-desktop.download.beeper.com/builds/
          beeper-server-nightly-4.3.8-linux-arm64.tar.gz",
   "download_size":"171570946",
   "pub_date":"2026-08-03T18:25:02+00:00"}
```

Downloaded and inspected:

| property | value |
|---|---|
| tarball entries | **2** — one directory, one file named `beeper-server` |
| type | ELF 64-bit aarch64 executable, 236 MB |
| interpreter | `/lib/ld-linux-aarch64.so.1` (glibc) |
| shared libraries | `libdl, libstdc++, libm, libgcc_s, libpthread, libc` — nothing else |
| Chromium/Electron | **absent** (no `chrome-sandbox`, `libffmpeg`, `CrashpadHandler`) |

An Electron bundle is a directory tree of hundreds of files. This is one
headless console binary. A string search does hit "electron" 14 times; every hit
is incidental — ICU's *electronvolt* measure unit, a Node.js source comment, and
a compression dictionary.

**Why that matters.** The spike declined to try `proot-distro` for a stated
reason: *"its known Electron/proot incompatibilities"* — Chromium's sandbox
fighting proot's ptrace-based syscall interception. That is true of Electron and
cannot apply to a binary with no Electron in it. The one lever that might have
worked was ruled out on grounds that do not hold for the artifact we want.

**Also missed entirely:** the official **`beeper-cli`** — npm `beeper-cli`
v0.6.2 (published 2026-05-18 by `bi`, the same maintainer as the official
`@beeper/desktop-api`), or `brew install beeper/tap/cli`. It installs and
supervises the headless server itself (`beeper install server`,
`targets start/stop/logs`), links each network from the shell via
`beeper accounts add` (QR/OAuth — **this answers the messaging plan's build-order
step 3**, "account link path that works without the Beeper Desktop GUI"), and
supports `targets add remote` / `targets tunnel`.

---

## 2. What is proven working

Measured read-only against the Mac's running Beeper Desktop 4.3.0, using the
`BEEPER_ACCESS_TOKEN` already in the main checkout's `.env`:

- `accounts list` → **Instagram, Discord, Google Messages, Matrix — all `connected`**
- `chats list --limit 100` → **66 threads, every one with an addressable id**
  (18 Instagram, 35 Google Messages, 11 Discord)

So the three nets the plans call blocked are, on this machine, already linked and
addressable.

---

## 3. There was no code — now there is a client

`grep -ril beeper companion/ android/` returned **one file**: a comment in
`NoAppFilterInTheReplyPathTest.kt` noting sends "were meant to go through
Beeper". The `beeper_server_localhost` route named throughout both plans had
**zero lines of implementation** behind it.

Built: `companion/internal/capability/messaging/beeper/`

- `client.go` — the API client
- `client_test.go` — 10 tests against a fake server
- `live_test.go` — 2 tests against a real Beeper, gated on `BEEPER_LIVE=1`

**Inputs → Outputs → Algorithm**

- **In:** a base URL, an access token, a person's name, message text.
- **Out:** either one resolved `Chat`, or an `AmbiguousError` carrying every
  candidate; and from a send, a `Sent{ChatID, PendingMessageID}`.
- **Steps:** (1) `GET /v1/chats/search?query=` finds candidates; (2) `ResolveOne`
  returns the single match, `ErrNoMatch` for none, or `AmbiguousError` for
  several — **it never guesses between people**; (3) `POST
  /v1/chats/{chatID}/messages` with `{"text":…}` sends.

The contract was read off a live instance's own OpenAPI document (`GET /v1/spec`,
67 paths) rather than the docs site, whose reference page 404s.

**Design points worth keeping:**

- **Where Beeper runs is a deployment choice, not a code one.** The same client
  works against Beeper Desktop on a paired Mac, a self-hosted Beeper Server, or a
  remote one over a tunnel. Only `baseURL` changes. The plans treated
  "phone-local" as load-bearing; it is not.
- **`ReadOnly()` returns a copy** that refuses every send without opening a
  connection, so the client can be pointed at a live account while building.
- **`ResolveOne` never picks a person.** `AmbiguousError.Error()` names each
  candidate's network, because the question string is the only thing that
  reaches the user.
- **Logs carry counts, lengths and ids only** — never message text, never the
  token. A test asserts exactly that by capturing the log and searching it.

### The live test earned its place immediately

Every unit test pointed at an `httptest` server, so all 9 passed while the client
was broken. Pointed at a real Beeper, it failed:

```
live_test.go:47: live Beeper rejected the search:
                 beeper: GET /v1/chats/search returned status 400
```

Cause: an empty search sent `?query=`, and a real Beeper rejects it —
`VALIDATION_ERROR`, *"String must contain at least 1 character(s)"*. The fake
server accepted the blank happily. Fixed by omitting the parameter when empty,
with a regression test that was confirmed failing first.

This is the same lesson as
`saved-results/what-oauth-can-be-tested-without-the-owner.md`: a test that has
never been seen to fail proves nothing.

### Verified runs (counted from raw `go test` output, not a wrapper summary)

| run | result |
|---|---|
| unit, before implementation | build failed, `undefined: NewClient` — red |
| empty-query regression, before fix | 1 FAIL — red |
| unit, after | **12 run, 10 passed, 0 failed, 2 skipped** (the 2 live ones skip without the gate) |
| live, `BEEPER_LIVE=1` | **2 run, 2 passed, 0 failed** — 20 chats across Discord, Instagram, Google Messages |
| full companion suite | **85 packages ok, 0 failures**, 8 no-test-files |

Reproduce:

```sh
cd companion
go test ./internal/capability/messaging/beeper/ -v            # fake-server tests

set -a; . ../.env; set +a                                     # from the main checkout
BEEPER_LIVE=1 go test ./internal/capability/messaging/beeper/ -run Live -v
```

**Same-bug-elsewhere check:** grepped `url.Values` across the companion — 30
non-test uses. The one structurally identical case,
`internal/capability/adapters/spotify/client.go:89` (`url.Values{"q": {query}}`),
is guarded upstream at `adapters/spotify/adapter.go:71` with `ErrEmptyQuery`. The
OAuth flows all build fixed-key forms with required values. No other instance.

---

## 4. What is still open

1. **Nothing has sent a message.** `Send` is covered by tests but has never
   fired at a real person. That is the owner's call, not an agent's.
2. **Which deployment to use** — three options, cheapest first:

   | path | phone Linux? | status |
   |---|---|---|
   | Companion → Beeper Desktop on the paired Mac | none | **working today**, read-only verified |
   | `targets add remote` / `tunnel` → Beeper elsewhere | none | untested, nothing installed |
   | `beeper-server` under proot-distro on the Pixel | ~700 MB, removable | untested |

3. **Whether `beeper-server` runs under proot-distro on the Pixel is unmeasured.**
   Bare Termux is bionic and has no glibc loader, so the binary will not run
   there directly; proot-distro supplies the glibc rootfs that would make it
   possible. Open in both directions — not known-good, not known-bad.
4. **Wiring into an adapter.** The existing `adapters/instagram` is a hand-off
   whose test asserts `Send` is *not* allowed. Per the owner's correction that
   adapter's ceiling is wrong, but changing it should wait until a path is chosen
   and one real send has been seen.

## 5. Cost and safety

Nothing here spent money. The npm package, the GitHub release downloads, and
Beeper's update feed are all free and unauthenticated. Every call to the live
Beeper was a read; the client used was `ReadOnly()`. No message was sent, no
account was created or modified, and no token value appears in any file, log or
test output.
