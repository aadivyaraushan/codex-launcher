# Build plan — the self-run "meeting-point box"

**Date:** 2026-07-17 (rev 2, after adversarial review)
**Follows the decision in:** `planning/remote-connection-options-plan.md`
**In one line:** replace "phone reaches into the Mac over Tailscale" with "the
phone and the Mac both call out to a small box that passes sealed messages
between them" — without changing any of the locks that keep it private.

> **Build status (updated 2026-07-18).** The core tunnel is built and proven
> locally:
> - `companion/internal/relaybox/` — the box (both doors, tokens, registration).
> - `companion/internal/relayclient/` — the Mac's dial-out `net.Listener`.
> - `companion/internal/relaye2e/` — **step 6a passes**: a real phone client
>   pairs + runs a real session against a real companion *through the box*, and
>   the box's recorded phone-door bytes contain neither the round-tripped marker
>   nor the pairing secret and start with a TLS record — box saw ciphertext only.
>   A fresh-context adversarial judge could not construct any false-green path
>   (routing is structurally enforced, the "box can't read" check is proven
>   non-vacuous, both directions recorded, real production crypto); its one
>   concrete fix — unchecked shutdown-error channels in test cleanup — is applied.
>   Known accepted gap: the round trip proven is a `hello`→`welcome` message
>   envelope, not a literal "task" message; since the transport relays bytes
>   without parsing them, the ciphertext-only security proof fully generalizes.
> All of §11 steps 1 (incl. fail-closed), 2, 3, and 6a are green + race-clean.
> - `companion/cmd/relaybox/` + `relaybox/persist.go` — **the runnable box
>   binary is built**: persists its TLS identity (0600), prints the pinned key
>   for the operator to copy, serves both doors, shuts down on a signal. Audited;
>   whole module green under `-race`.
> - **Piece 2 (Mac companion wiring) is built, security-audited, and green.**
>   The four open decisions were confirmed with the user and implemented: the box
>   secret lives in the same owner-only config store as before (0700 dir / 0600
>   file); `ListenHost/Port` + the Tailscale-IP gate are gone, replaced by a
>   `RelayConfig` (box host, mac/phone ports, pinned key, secret) validated at the
>   boundary; relay *fully* replaces Tailscale (setup/doctor Tailscale asserts
>   removed); and the operator pastes box host + pinned key + secret at
>   `codex-launcher setup`. The Mac fails **closed** if the relay is unreachable
>   or misconfigured — there is no local-listener fallback anywhere in the tree.
>   A fresh-context security judge found **no CRITICAL** issues and confirmed the
>   core crypto is sound (exact-match key pinning with no CA fallback, secret
>   never logged, box content-blind, doctor never REGISTERs, fail-closed startup).
>   Its HIGH/MEDIUM findings — all about hardening the *new public attack surface*
>   the removed Tailscale gate used to cover — are fixed:
>   - the registration secret now must be ≥16 chars and control-char-free, checked
>     both in `relaybox.New` (`ErrWeakSecret`) and `app.Config.Validate`;
>   - the public Mac-door header read is byte-capped (`ReadSlice` over a 4096-byte
>     buffer) so a client streaming a newline-less header can't grow memory.
>   - *Deliberately not added:* a registration rate-limit / lockout — a lockout is
>     itself a denial-of-service lever against the real Mac, and a strong secret
>     already makes online brute force infeasible. *Optional future upgrade:* have
>     `setup` generate a random secret instead of accepting an operator-typed one.
> All of §11 steps 1 (incl. fail-closed), 2, 3, and 6a are green + race-clean, and
> the whole module passes `go test -race ./...` (one **pre-existing, unrelated**
> flake in `internal/app/mobilesession` — untouched by this work, passes 10/10 in
> isolation — is noted but out of scope for this plan).
>
> - **Piece 3 (Android safe-endpoint rule) is built, security-audited, and green.**
>   `isTailscaleAddress` is gone; `PairingValidation.isSafePublicEndpoint` gates
>   both link parsing (`PairingOffer`) and stored-record validation
>   (`PairingRecordStore`). It denies loopback / RFC1918 / link-local / cloud
>   metadata / CGNAT, disguised IPv4 encodings (octal/hex/integer/short), and the
>   IPv6 special ranges; it accepts a public IP or a public hostname *on shape*
>   (no DNS at scan time). The rebinding defense is a custom OkHttp `Dns`
>   (`SafePublicDns`) wired into the single `PinnedTlsClientFactory` choke point,
>   so **both** dial paths (pairing POST and session websocket) resolve once, drop
>   every deny-listed address, and fail **closed**. The byte-level deny-list lives
>   in one place (`PublicInetAddress`) shared by the pure rule and the DNS filter,
>   so they cannot disagree. Two fresh-context security-judge rounds: round 1
>   found a **CRITICAL** — IPv4-*compatible* IPv6 (`::a.b.c.d`, e.g.
>   `::169.254.169.254`) slipped both layers — now fixed by treating all of
>   `::/80` as an embedded IPv4 and applying the v4 deny-list; round 2 confirmed
>   that CRITICAL closed (no over-blocking of real public addresses) and flagged
>   small residuals, of which `ff00::/8` multicast and `fec0::/10` deprecated
>   site-local are now also denied. **Accepted residual risk:** a NAT64 gateway
>   using a *non-well-known* prefix (`64:ff9b:1::/48` etc.) can embed a private v4
>   that address inspection alone cannot detect; exploiting it needs the victim's
>   own network to run such a gateway with that exact prefix (not attacker-
>   controlled), a materially higher bar than the well-known-prefix case, which is
>   denied. Full Android JVM unit suite green (270 tests).
>
> **Fly deployment and automated proof complete 2026-07-19.** The approved
> single 256 MB machine is running in `lax`, with one dedicated IPv4 and one
> encrypted 1 GB volume in Fly organization `personal` (`ssdear@gmail.com`).
> Public services are raw TCP: Mac `443 -> 9000` with no handler; phone
> `8443 -> 8443` with PROXY protocol only. `fly config validate` passes. The
> deployed stage-6b phone harness passed under `-race` in 9.058s after the
> installed companion was briefly stopped so the single-computer relay slot
> belonged only to the harness; the LaunchAgent was restored and `doctor`
> returned 7/7 checks green. Full Go race/vet, Android unit/lint/debug/release
> builds, release contracts, and protocol schema checks are green. The old phone
> was revoked so migration could not silently keep its stale address. The
> physical step that followed is recorded below.
> A final independent review found no confidentiality flaw and its four
> completion gaps are closed: `doctor` now uses a non-evicting `CHECK` command
> to verify both pin and secret; the external-test evidence is described as a
> pin-based proof rather than a nonexistent Fly byte recording; clean deploy,
> rotation, recovery, and re-pair steps live in `docs/setup/relay-box.md`; and
> the control-blip test proves replacement acceptance by heartbeat plus the
> configured reconnect delay. The phone door now also rejects a non-TLS preface
> before consuming a Mac session slot.
>
> **Physical-phone confirmation complete 2026-07-19.** A Pixel 9 running
> Android 16 paired with the installed Mac companion through the deployed Fly
> box, selected the one approved project, started a real Codex turn, and showed
> `RELAY_BOX_PHONE_VERIFICATION_PASSED` in the phone transcript. The user
> approved that one turn on the verified ChatGPT login `aadivya@fermi.ai`;
> `OPENAI_API_KEY` was not set. The repository status and tracked diff were
> unchanged by the read-only prompt. This physical run found and closed two
> final migration bugs: an already-paired phone now has an explicit **Remove
> local data** recovery action when an obsolete Tailscale record fails closed,
> and the companion now records its sent snapshot sequence before validating
> the phone's cumulative acknowledgement. The latter fix stopped the real
> phone's repeated 40-second reconnect loop. Its regression test failed first
> because a valid acknowledgement never reached the handler, then passed under
> `-race`; the full companion race suite and Android unit/lint/debug/release
> checks are green. **All required §11 steps are now complete.**

> How to read this: Sections 0–3 are the whole idea, skimmable in a minute.
> Everything after is detail for completeness (and for the engineer and the
> adversarial judge). Plain language throughout; the boxed **"For the engineer"**
> notes hold exact code spots and can be skipped by everyone else.

---

## 0. The one assumption everything rests on: one box, one computer

**Each person runs their *own* box, and that box serves exactly *their one*
computer.** This is the same shape as Tailscale (you run your own), and V1
already fixes the setup to a single computer.

This matters because it deletes the hardest problem a shared relay would have:
"when a phone connects, which of the many computers do I send it to?" On a
single-computer box there is no such question — **any phone that connects wants
the one computer this box knows.** The box never has to look inside anything to
figure out where to route. (This is the gap the first draft hand-waved; making
the box single-computer removes it entirely.)

---

## 1. The idea, in a picture

```
   TODAY (breaks when the Mac is on a VPN)
   ────────────────────────────────────────
   Phone ──knock──►  Mac        the VPN grabs the door, the knock never lands


   NEW (survives the VPN)
   ──────────────────────
   Phone ──calls out──►  ┌──────────────┐  ◄──calls out── Mac
                         │ your own box │
                         │  on Fly.io   │   carries sealed envelopes;
                         └──────────────┘   never opens them
```

Both sides make **outgoing** calls. A VPN carries outgoing calls happily (it only
blocks incoming knocks), so the VPN stops mattering.

---

## 2. Why this is safe — and why misconfiguring it fails *loudly*, not silently

We checked how the phone and Mac secure their talk today. The security rests on
one fact that the box can't touch:

> **The phone refuses to talk to anything that can't prove it's *your* computer.**
> At pairing, the phone memorizes your computer's cryptographic identity. On every
> connection it checks the far end presents *exactly* that identity — and it
> **ignores the address** it dialed. It has no idea, and doesn't care, whether it
> reached the Mac directly or through a box.

Two consequences, both good:

1. **The box can't read your messages even if it tried.** The sealed channel
   (TLS 1.3) can only be opened with keys that live on the phone and the Mac —
   never on the box. The box holds none of them.
2. **If the box (or the cloud platform) ever tried to open the envelope, the
   connection simply fails — it does not leak.** To open the envelope, something
   in the middle would have to present a computer identity to the phone. It can't
   — it doesn't have your computer's keys — so the phone's identity check rejects
   it and **refuses to connect.** There is no "silently reads your traffic"
   failure mode; the worst case is "it doesn't work," which you'd notice
   instantly.

So the box's job is purely to **carry sealed bytes untouched.** Getting that
right is a *functionality* requirement (so the connection works at all);
*confidentiality* is guaranteed separately by the phone's identity check, which
the box cannot defeat.

> **For the engineer.** Confirmed against the code (all citations verified by
> review):
> - Phone TLS trust is pure key-pinning that ignores hostname —
>   `PinnedTlsClientFactory.kt:12-30` (custom hostname verifier), cert-key match
>   in the `PinnedTlsTrustManager` class (`HostIdentityPin.kt:88-99`) — note that's
>   the TLS cert pin, distinct from the `HostIdentityPin` class's app-layer Ed25519
>   pin in the same file. A TLS-terminating middlebox presents the wrong
>   key → `checkServerTrusted` throws → connection refused. **Fail-closed.**
> - Endpoint auth is app-layer Ed25519 challenge/response over a server nonce,
>   transport-independent, replay-guarded — `pairing/service.go:294-369`,
>   `sessionReplays` at `service.go:341-361`.
> - Ongoing content confidentiality rides on TLS 1.3 (JSON is plaintext *inside*
>   TLS — `contract/validation.go`), so **the box must forward the phone↔Mac
>   stream raw and never terminate its TLS** (Section 5). Pinning backstops this,
>   but we still configure passthrough so it *works*.

---

## 3. How it works, start to finish

```
  (A) Mac starts up
      Mac ──dials out, over a secured line──► box:  "I'm your computer, here's my
                                                     registration secret. Hold my
                                                     control line open."   [control line]
                          box checks the secret, keeps the line, waits.

  (B) You pair the phone (one time)
      Pairing link now carries the BOX's address + port (instead of the Mac's
      Tailscale IP). Phone stores it. Identity pin is unchanged.

  (C) Phone wants to talk
      Phone ──dials out──► box (phone port).  box, on the control line, tells the
                          Mac "a phone is here, token = X." The Mac dials a FRESH
                          data line to the box and presents token X.

  (D) Box glues the two lines and steps back
      Phone ⇄ [box copies raw bytes] ⇄ Mac.  The phone↔Mac sealed TLS handshake
                          and the existing signature handshake run end to end over
                          the glued path. The box only shuffles sealed bytes.
```

Plain version: the Mac keeps one signalling line open to the box. When your phone
calls the box, the box tells the Mac "someone's here," the Mac opens a fresh line
for that call, and the box connects the two and copies bytes without listening.
It's a switchboard operator who connects two callers and never hears the call.

**Why a fresh line per call (not one shared pipe):** if the single control line
hiccups (network blip, box redeploy), calls already connected keep working — only
*new* calls wait a few seconds for the Mac to reconnect its control line. This
avoids "one blip drops every session at once."

---

## 4. The three pieces, and what changes in each

### Piece 1 — the box (new, small, runs on your Fly.io account)

A small always-on program with two doors, secured **differently on purpose**:

- **Mac door** (encrypted, and the box proves who it is): the Mac's one line to
  the box. This door **is** encrypted by the box, and the box proves its identity
  to the Mac the same way the phone proves the Mac's — **the Mac pins the box's
  key.** At deploy the box generates its own keypair and prints its public
  fingerprint; the Mac's config holds the box's address *and* that pinned
  fingerprint. So on this door:
  - The **box** proves it's the real box (Mac's pin check) — a network attacker
    can't impersonate the box without its private key.
  - The **Mac** proves it's the owner (registration secret), sent *inside* this
    encrypted, box-pinned channel — so the secret never travels in the clear and
    can't be sniffed and replayed to hijack the box's single slot.
  - This door carries only signalling, the registration secret, and per-session
    tokens — **never the phone's content.** Terminating TLS here is correct and
    required (it's not the phone-content path).
- **Phone door** (raw passthrough, box holds nothing): a plain TCP port. Any
  connection here is a phone wanting the Mac. The box immediately asks the Mac
  (over the control line) for a data line and then **copies raw bytes between
  phone and Mac, untouched.** The box **never terminates the phone↔Mac TLS on this
  door.** (The "no TLS handler" Fly rule in Section 5 applies to **this door
  only** — see the scoping note there.)
- Caps and rate limits so being public can't be abused or run up cost (Section 7).

> **For the engineer — the Mac-door client (this is new code, no prior art to
> copy, so build it as strictly as the phone's pin).** The Mac dials the box over
> TLS but must trust it by *pinned key only, with system CA trust turned off* —
> mirror Android's `PinnedTlsTrustManager` (in `HostIdentityPin.kt:88-99`) in Go:
> `tls.Config{InsecureSkipVerify: true}` **plus** a `VerifyConnection` callback
> that compares the presented leaf's `SubjectPublicKeyInfo` (DER) to the pinned
> box key and returns an error otherwise. Do **not** fall back to
> `x509.SystemCertPool` / ordinary `tls.Dial` against a Fly-issued cert — that
> compiles and smoke-tests fine but silently widens trust from "only my box" to
> "anyone with a valid cert for that name" (a mis-issued or CA-compromise cert
> would pass). This is the one hop where the easy path is the insecure path, so
> the test in Section 5 (test 5) asserts an impostor box is rejected. Set
> `MinVersion: tls.VersionTLS13` on **both ends** of the Mac door (box server and
> Mac client), matching the phone-content path's floor (`server.go:107`) — don't
> leave it at Go's default lower floor.
>
> **Data lines use the same door, and this is nested TLS on the Mac hop — say so
> plainly.** The per-session data line is a *new* TCP connection to the **same
> box-pinned Mac-door port**, so it opens the outer box-pinned TLS first; the
> `REDEEM <token>` travels as application data *inside* that outer TLS — never in
> the clear, so a network attacker can't read or race the token. (First-message
> framing on the outer TLS tells the box `REGISTER <secret>` from `REDEEM <token>`.)
> After a valid `REDEEM` the box replies `OK` and then, for the rest of the
> connection, **relays the bytes it receives without decrypting them further**: on
> the phone door it terminates *nothing*, so the phone's own TLS bytes arrive
> sealed; the box copies those sealed bytes into this outer tunnel as payload. The
> Mac's relay client peels the **outer** box TLS back off and hands the recovered
> inner byte stream to `Serve()`. So the phone↔Mac content is wrapped in **two**
> layers on the Mac hop (outer box-pinned TLS + inner phone↔Mac TLS); **the box
> terminates only the outer layer and never the inner** — that's why it can't read
> content. "Passthrough" here means "stops parsing box messages and relays the
> already-outer-decrypted inner bytes," **not** "drops to plaintext." See Piece 2.

Recommended: a **tiny custom program we write** (Go, a few hundred lines) — it
lets us *guarantee* the passthrough rule and keep the box holding zero key
material. Alternative: reuse a self-host tunnel tool (`frp` etc.) — less code to
own, but more config to get exactly right and we'd have to verify it passes TLS
through rather than terminating it. **Resolved decision O1 (Section 10).**

### Piece 2 — the Mac companion (dial out instead of wait)

Today the Mac *waits* for an incoming connection. Now it *calls out* to the box,
registers, and holds the control line. The trick that keeps this small: we wrap
each fresh data line so that, to the rest of the Mac app, **it looks exactly like
a phone knocking on the door did before.** Everything above it — the sealed-
envelope server, the signature handshake, the message handling — runs untouched.

> **For the engineer.** Replace the inbound bind (`main.go:345` `net.Listen`) with
> a relay client that implements `net.Listener`. It holds the control line (the
> box-pinned encrypted Mac door). Each time the box signals a session, in
> `Accept()` the client dials a data line to the box, **writes the one-time token
> as a short header inside the outer box-pinned TLS, and waits for the box's `ok`;
> the box consumes that header and then relays the recovered inner bytes.**
> `Accept()` returns, as a `net.Conn`, the **inner** byte stream — the data line
> with the outer box TLS already peeled off, positioned after `OK`. Layering,
> stated exactly so nobody miscounts: on the Mac hop the content is **nested —
> outer box-pinned TLS (the box terminates this) wrapping the inner phone↔Mac TLS
> (only phone and Mac ever terminate this).** The box decrypts only the outer
> layer; the inner phone↔Mac session is the one and only session that carries
> content, and its keys live solely on the phone and Mac.
> `mobileapi/transport/server.go:97-131` (`Serve`) wraps the returned inner
> `net.Conn` in `tls.NewListener` exactly as today (that's the inner handshake), so
> phone↔Mac TLS terminates only at the phone and the Mac. `Serve` runs
> **unchanged** — same
> `tls.NewListener`, same `/v1/pair` + `/v1/session` routes, same 8-connection
> cap. No stream-mux library needed (one real TCP connection per session). One
> detail to get right: the box signals new sessions concurrently, but
> `http.Server.Serve()` calls `Accept()` one at a time — so the adapter needs a
> small internal buffered queue of pending session tokens that `Accept()` drains,
> not a 1:1 "signal directly drives Accept" coupling (the standard Go listener-
> adapter pattern). The
> Tailscale-address gate (`app/config.go:26-29,163-170`) is replaced by a relay-
> endpoint config; the Tailscale asserts/probes in `hostsetup/setup.go:90,94` and
> `hostdoctor/doctor.go:98-127` become "can I register with the box" checks.

### Piece 3 — the phone (genuinely just a different address)

Because the box has a single dedicated phone port and does raw passthrough, the
phone honestly only changes *which address it dials*: the box's address+port
instead of the Mac's Tailscale IP. It opens the connection the same way, does the
same sealed TLS to that address, and the box forwards the handshake straight to
the Mac — so the **identity pin and signature handshake are unchanged.** No SNI
tricks, no custom DNS, no dropping the HTTP library. The only real edit is
replacing the "address must be a Tailscale address" rule with a strict
"address must be a safe public endpoint" rule (Section 6).

> **For the engineer.** This is the claim the first draft over-hand-waved and the
> single-box model now makes literally true: `CompanionSessionClient.kt:305-308`
> still does `wss://<host>:<port>/v1/session?...` via OkHttp — `host`/`port` now
> point at the box. Because the box does TCP passthrough on the phone port, the
> TLS session is end-to-end phone↔Mac; OkHttp's SNI (= box host) is irrelevant
> since pinning ignores hostname. Pairing URI: `host`/`port` become the box
> endpoint (`pairing/service.go:215-239`); no separate rendezvous name is needed
> in the URI. Replace `PairingValidation.isTailscaleAddress`
> (`PairingValidation.kt:8-29`) with the Section 6 safe-endpoint rule at **both**
> call sites — miss either and box pairing silently fails:
> - `PairingOffer.kt:51` — gates accepting a scanned pairing offer.
> - `PairingRecordStore.kt:136` (inside `isValid`, which gates **both** `save()`
>   at line 71 *and* `readRecord()` at line 132) — with the old check left here,
>   a box-address record fails validation, so it is **never written to disk**, and
>   even a written one is **discarded on every app start**. Section 5 test 6 must
>   exercise this stored-record path, not only the offer path.
> - `LocalStateWiperInstrumentedTest.kt:72` (test) — asserts
>   `isTailscaleAddress(paired.host)` on a `100.64.0.10` fixture
>   (`LocalStateWiperInstrumentedTest.kt:128`). When the validator changes this
>   breaks the test build; update the fixture host to a box-address form and the
>   assertion to the new safe-endpoint check.
>
> (Confirmed the full set with `grep -rn isTailscaleAddress android/`: one
> definition + these three uses, no others.)

---

## 5. The rule that must never break: the box forwards, never opens (with a real test)

The security keystone, its own section and its own tests.

- **Rule:** on the phone door, the box does **raw byte passthrough only** — it
  must never open, unseal, or re-seal the phone↔Mac stream (never terminate its
  TLS).
- **This includes the cloud platform's own front door.** Fly.io's *default* web
  setup (`http_service`) terminates TLS at Fly's edge before handing your app
  plaintext — normal for websites, **forbidden on the phone door.** The **phone
  door's** `fly.toml` service must be a **raw TCP service with no `tls` handler**,
  so Fly forwards untouched bytes. This is an explicit, checked constraint, not an
  assumption. (Verify the exact `fly.toml` syntax against current Fly docs at
  build time; the test in step 2 below is what actually proves it.)
  - **The clean rule, stated once so it can't be split-read: Fly terminates TLS
    on NEITHER port.** Both the phone-door and Mac-door Fly services are raw TCP
    with no `tls` handler — Fly always forwards untouched bytes. The **box's own Go
    program** is the *only* thing that ever terminates TLS, and it does so on the
    **Mac door only** (with its own pinned key, Section 4), never on the phone
    door. So the "opposite treatment" is at the *box-program* level (phone door =
    pure passthrough; Mac door = box terminates its own encrypted line), **not** at
    the Fly level, where both are identical raw-TCP. Do not enable Fly's edge TLS
    on the Mac port thinking it "keeps encryption" — that would stop the box's own
    pinned handshake from ever running.
  - **Why the Mac door must stay encrypted (at the box-program level):** an
    unencrypted Mac door would expose the registration secret on the wire and let
    an attacker seize the box's single slot (a real denial-of-service — Section 7).
- **Fail-closed safety net:** even if this were misconfigured, the phone's
  identity pin would reject the wrong certificate and refuse to connect (Section
  2). So misconfiguration breaks the connection *loudly*; it does not silently
  leak. We still require and test the passthrough config so it *works*.

**What the box (and the network path) can still see, and we accept:** that your
phone and Mac are talking, when, how often, roughly how much data, and the network
addresses involved. **Contents stay sealed.** Note this is visible to anyone on
the network path, not only the box; the threat-model doc will say so plainly.

**Tests (real, not promises):**
1. **Local passthrough proof.** Stand up the box locally, run a real sealed
   session through it, assert (a) the session round-trips, and (b) every byte the
   box handled on the phone door was unreadable ciphertext — if the box could read
   one plaintext message, the test fails.
2. **Real-deployment proof.** Run the same ciphertext-only + round-trip check
   against the **actual deployed Fly box**, catching a TLS-terminating `fly.toml`
   that a local test can't.
3. **Fail-closed proof.** Point the phone at a deliberately TLS-terminating
   endpoint (wrong cert) and assert it **refuses** to connect — proving misconfig
   can't leak.
4. **Structural check.** The box binary holds no phone/Mac identity or session
   keys by construction — grep/deps check that it never links the pinning/identity
   material. "Can't read" rests on both no-keys *and* no-terminate.
5. **Mac door is encrypted and hijack-proof — control *and* data lines.** Assert
   the registration secret is never observable on the wire (Mac door is box-pinned
   TLS); a registrant with a wrong/empty secret is rejected; an impostor box (wrong
   key) is rejected by the Mac's pin *and* is not accepted via a system-CA cert
   (no CA fallback); and a data-line `REDEEM` with a forged/expired/reused token is
   refused. Since data lines share the pinned Mac-door port, token redemption is
   never in the clear. This is the door the phone-pin backstop does *not* cover.
6. **Deny-list holds.** Assert the phone rejects a pairing link pointing at each
   dangerous form (loopback, private, link-local, metadata, and the tricky
   encodings in Section 6), checked against the *resolved* address, not just the
   string.

---

## 6. Names, secrets, and safe addresses

Because one box serves one Mac, there is **no secret rendezvous name traveling on
the wire** (the first draft wrongly implied one). What exists:

- **Registration secret (stays secret, on the encrypted Mac door).** One secret
  for this box, held by your Mac, set when you deploy the box. The Mac presents it
  over the box-pinned control line (never in the clear) to register.
- **Per-session data-line tokens (short-lived).** For each phone that arrives, the
  box mints a fresh token and hands it to the Mac *over the encrypted control
  line*; the Mac echoes it back on the new data line so the box knows which
  pending phone to glue it to. Concretely: at least 128 bits of randomness,
  **single-use** (dropped the instant it's redeemed), **expires in seconds** if no
  data line shows up, and each pending phone waits on its own token so two phones
  connecting at once can't be crossed. Orphaned tokens (box restart mid-handshake)
  simply expire; the phone reconnects. A network attacker can't forge or replay a
  token — it's minted with good randomness and only ever seen inside the encrypted
  control line.
- **Box address (public, not secret).** Just an address in the pairing link. It
  doesn't need to be secret — a stranger who dials your box hits your Mac's
  identity+signature wall and fails (and, if they somehow registered a fake Mac,
  the phone's pin rejects it). Being public only invites noise, handled by caps.

**Safe-address rule (replacing the Tailscale allowlist).** The phone accepts a box
endpoint only if it is a **public hostname or public IP + port**, and **rejects**
loopback, link-local, private ranges, and cloud-metadata addresses
(`127.0.0.0/8`, `10/8`, `172.16/12`, `192.168/16`, `169.254/16`,
`169.254.169.254`, `::1`, `fc00::/7`, `fe80::/10`, plus the tidy-up ranges
`0.0.0.0/8`, `100.64/10` CGNAT, `192.0.0/24`, IPv6 documentation `2001:db8::/32`,
and `64:ff9b::/96` NAT64 / `2002::/16` 6to4 which can wrap a private IPv4). Two
bypasses a naive list misses, and we close them:
- **Sneaky encodings of the same address** — IPv4-mapped IPv6 (`::ffff:127.0.0.1`)
  and non-dotted-decimal IP forms (octal/hex/integer). Fix: **canonicalize the
  address to its normal form first, then check the ranges.**
- **Bait-and-switch by DNS (rebinding)** — a hostname that looks public when the
  link is checked but resolves to a private/metadata address when the phone
  actually dials. Checking "the resolved IP" is not enough on its own: if the
  validator resolves once and the network library resolves *again* at dial time,
  the second answer can differ (a time-of-check/time-of-use gap). Fix: **resolve
  exactly once, filter, and force the connection to use that same vetted IP** — a
  custom OkHttp `Dns` that resolves, drops any address hitting the deny-list, and
  returns only the survivors, so OkHttp never re-resolves to a fresh (poisoned)
  answer.

"Well-formed" alone is not enough. The point is to keep a malicious pairing link
from turning the phone into a network-probing tool against internal services
(blind SSRF). Content can't leak this way regardless — the phone only speaks
sealed, pinned TLS to whatever it dials — but the probing risk is real, so the
deny-list stays and gets its own test (Section 5, test 6).

The registration secret is **auto-generated at deploy** with the same entropy
floor as the tokens (≥128 bits — never hand-typed, so it can't be brute-forced on
the public port). It is **rotatable**: change it on the box (a `fly secrets`
update) and in the Mac's relay config. A `fly secrets` update **redeploys/restarts
the box, which drops all live connections** — that restart is what evicts a
squatter holding the slot open; the box does not otherwise re-check the secret on
an already-open connection. Note this is **new plumbing**, not the existing
`revoke` command — today's `revoke` (`cli/commands.go`) cuts off a *paired phone*,
a different thing from re-keying the box. Phone revocation stays exactly as it is;
box re-keying is a small new pair of steps we'll document.

> **For the engineer.** Bind the per-session data-line tokens and the registration
> secret to the existing pairing/identity machinery in `pairing/service.go` rather
> than inventing a parallel trust path. One registration secret per box (single-
> tenant); the box refuses a *wrong-secret* registrant. A **correct-secret**
> re-registration (the real Mac reconnecting after a blip) **evicts the stale slot
> immediately** and takes over — otherwise the owner would be locked out behind a
> zombie connection until its heartbeat times out, making reconnect slow.

---

## 7. When things go wrong (and the honest downsides)

- **Box down → everything down.** With one path (your choice), the box is the
  single lifeline. The Mac auto-reconnects with backoff; the phone shows a clear
  **"can't reach the box"** state, distinct from **"computer offline,"** so you
  know which broke.
- **Control line drops.** Calls already glued keep working (fresh-line design,
  Section 3); only new calls wait for the Mac to re-register. Heartbeats on the
  control line trigger fast reconnect.
- **Idle timeouts / NAT.** Heartbeats keep the control line alive; the phone
  reconnects as it does today.
- **Public phone door flooded (a new risk — the port is now internet-facing).**
  Under Tailscale only tailnet members could reach the port; now anyone can, and
  every phone-door connection would otherwise make the Mac dial a data line and
  burn one of its fixed 8 slots (`transport/server.go:27`) for the whole handshake
  window (~10s). A modest distributed trickle — a few hosts, each under any
  per-IP rate limit — could keep all 8 slots busy and lock out the real phone.
  So per-IP rate limits **alone are not enough.** Concrete defenses, all on the
  box: (a) cap the number of *pending, not-yet-authenticated* phone connections
  the box will ask the Mac to service at **≤ the Mac's slot budget**, queueing or
  dropping the rest so genuine phones aren't starved; (b) give each pending phone
  a **short pre-handshake deadline** so a stalled connection frees its slot in a
  second or two, not ten; (c) per-IP and global connection rate limits on top.
  Fly.io runs **one small fixed machine with autoscaling off**, so a flood can
  make the box busy but **can't quietly run up your bill.**
- **Registration-slot takeover (why the Mac door must stay encrypted).** The box
  has one registration slot. If the registration secret ever leaked — the main way
  that happens is running the Mac door without encryption — an attacker could grab
  the slot and lock your real Mac out (a denial-of-service; they still can't read
  content, thanks to the phone's pin). The box-pinned encrypted Mac door
  (Section 4) prevents the leak, and the secret is rotatable if you ever suspect
  it. This is the one attack the phone-pin backstop does *not* cover, so we defend
  it directly.
- **Box restarts (deploy/crash).** Mac and phone reconnect automatically; no
  re-pairing needed.

---

## 8. Blast radius — separating the two kinds honestly

- **Security blast radius: tiny (unchanged).** The locks — identity pin, Ed25519
  sign/verify + replay guard, the sealed TLS channel, the one-time expiring
  pairing secret — are **not touched.** The change is confined to how sockets are
  obtained, plus config/validation/docs.
- **Availability blast radius: bigger, and that's real.** You're adding a new
  always-on dependency (the box). If it's down, you're down — the price of "one
  path, not two." The fresh-line design limits it so a brief blip doesn't nuke
  active calls, but the box being a new single point of failure is a genuine cost
  we're accepting, not hiding.

---

## 9. Moving over (migration)

Switching transport makes the stored address on already-paired phones stale. For
this alpha that's fine: **re-pair once** after the change; the setup docs will say
so. Nothing is lost on the computer side.

---

## 10. Resolved decisions

- **O1 — Build our own tiny box.** Implemented as the `relaybox` Go command so
  raw passthrough and the zero-content-key boundary are covered by repository
  tests instead of depending on a general tunnel tool's configuration.
- **O2 — Use the free Fly address.** The deployed host is
  `codex-launcher-relay-ssdear.fly.dev`; no custom domain is needed for V1.

Everything else is implementation and is mine to decide.

---

## 11. How we'll build it (test-first)

TDD — failing test first, then code:

1. **Box forwards, never reads** — Section 5 tests 1 (local) *and* 2 (real Fly
   deploy) *and* 3 (fail-closed) *and* 4 (no keys in the box). Red first (no box).
2. **Mac dials out; data lines look inbound.** Red: test the `net.Listener`
   adapter yields a working connection from a fake box; then build so the existing
   `Serve()` stack runs unchanged on top.
3. **Encrypted Mac door + registration + tokens.** Red: Mac door is box-pinned
   TLS (impostor box rejected, secret never on the wire); right secret registers,
   wrong/empty rejected; forged/replayed/expired data-line token refused; a second
   registrant can't seize the slot.
4. **Phone dials the box; identity check unchanged.** Red: safe-address rule
   accepts a public endpoint and rejects the deny-list ranges *including* the
   tricky encodings and a rebinding hostname (checked on the resolved IP); all
   pinning and handshake tests stay green.
5. **Failure behavior.** Red: reconnect/backoff; the two distinct "box
   unreachable" vs "computer offline" states; a control-line blip does not drop an
   active call.
6. **Self-driven end-to-end verification loop (not just "tests pass").** Build a
   harness that *plays the phone* — the real pinned-TLS + Ed25519 handshake and a
   real session — against a **real running Mac companion through the box**, and
   assert a full task round-trips AND the box only ever saw ciphertext. Run it,
   read the failure, change code, re-run — **iterate until it genuinely works**,
   not until a unit test goes green. Two stages:
   - **6a — local box (free, runs first):** the whole path on localhost. No Fly,
     no money. This is the cheap loop we iterate on.
   - **6b — deployed Fly box (after approval of the estimated $4.20–$7/month):** the same harness
     against the actual deployed box, which is the only thing that can catch a
     TLS-terminating `fly.toml` a local run can't.
   - *Optional extra layer:* drive the phone-side Kotlin in an **Android emulator**
     (scripted/instrumented) — the closest thing to "computer use" for the app,
     since the box and companion have no GUI to click.
7. **Final confirmation on your physical phone — complete.** Installed the
   debug build on a Pixel 9 running Android 16, paired once through Fly, selected
   the sole approved project, and ran a real read-only Codex task. The phone
   transcript showed `RELAY_BOX_PHONE_VERIFICATION_PASSED`, the companion kept
   one paired device, and the repository did not change.

Each step: see it fail, make it pass, show the green run. Steps 6–7 are the "does
it actually work" gate — we do not call this done on green unit tests alone.

---

## 12. Cost & ops recap

- Fly account `ssdear@gmail.com`, organization `personal`: estimated
  $4.20–$7/month for one always-running 256 MB machine, one dedicated IPv4,
  one 1 GB volume, and ordinary traffic. Autoscaling and high availability are
  off, so load cannot create extra machines.
- The user approved that account and estimate on 2026-07-19 before any paid
  resource was created.
- Docs updated: the README, companion setup, Android setup, compatibility guide,
  and threat model now describe re-pairing, Fly raw-TCP forwarding, the separate
  box-pinned Mac layer, and sealed TLS through an untrusted box. Release checks
  reject the removed Tailscale setup flags and stale operational instructions.
