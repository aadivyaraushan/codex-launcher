# Finish Operator consumer services on Android

**Date:** 2026-08-05
**Status:** Ready for implementation handoff; independent plan reviews PASS
**Supersedes:** the previous Mac-companion and Pixel-test version of this file
**Evidence baseline:** `saved-results/production-phone-service-usability-audit-2026-08-05.md` and `saved-results/android-vm-terminal-feasibility-2026-08-05.md`
**Scope authority:** this file overrides conflicting completion language in `consumer-app-implementation-plan.md` and `operator-complete-messaging-plan.md`

## One-minute handoff

```text
Wave 0: prove Android-local Linux, automatic restart, and no Mac fallback
   |
Wave 1: preserve credentials, connect the router, prove Todoist end to end
   |
Wave 2: Beeper messages + Instagram open + exact YouTube + Direct Reply
   |
Wave 3: Calendar, Drive, Maps, Outlook, Slack, Spotify, Notion
   |
Wave 4: crash/reboot/security regression + repair both parent-plan claims
   |
Fresh independent judge: PASS with no material gaps
```

No owner decision remains. Use the accounts and targets below, batch unavoidable
human sign-ins once, and keep the same authenticated Android VM and service data
through every wave. The owner explicitly approved publishing the static Android
App-Link association on the existing `tryoperator.net` site on 2026-08-05. A
wave advances only after its observable exit test passes; code, an HTTP success,
or an adapter registration without target-side readback does not count.

## Product shape

```text
                         NO MAC IN PRODUCTION
              NO OPERATOR BACKEND; ONE STATIC APP-LINK FILE

  Android Operator app
  +-----------------------------------------------+
  | prompt / preview / confirm / result           |
  | Android OAuth + credential broker             |
  | Android Keystore                              |
  | app intents + notification Direct Reply       |
  | runtime supervisor + status UI                |
  +----------------------+------------------------+
                         | authenticated loopback only
                         v
  Android-hosted Linux runtime
  +-----------------------------------------------+
  | phone-runtime (reused Go capability engine)   |
  | beeper-cli + headless beeper-server           |
  | restart supervisor + health endpoint          |
  +----------------------+------------------------+
                         |
        +----------------+--------------------+
        |                |                    |
        v                v                    v
  Beeper networks   official vendor APIs   Android apps
  IG / Discord /    Google / Microsoft /   Maps / YouTube /
  Google Messages   Slack / Spotify /       Spotify handoffs
                    Notion / Todoist
```

The Android app remains the only user-facing product. Linux is an internal
runtime. A normal request must never require a paired Mac, a Mac LaunchAgent,
macOS Keychain, AppleScript, or an Operator-owned relay/cloud service. The sole
exception is the approved static `tryoperator.net/.well-known/assetlinks.json`
file used for Android domain verification; it performs no request processing.

## Locked owner decisions

- Production is Android-only and continues working with the Mac shut down.
- Operator does not use an Operator-owned cloud backend. Direct calls to the
  user's chosen services and OpenAI are allowed. The owner approved one narrow
  exception on 2026-08-05: `tryoperator.net` may host the static Android
  App-Link association for Notion and Todoist. It may not exchange, store,
  proxy, or log OAuth credentials or run callback code.
- Beeper is the primary direct-send route for Instagram, Discord, and Google
  Messages. It runs on the Android device, not on macOS.
- Termux + `proot-distro` Debian is the selected production Linux runtime for
  this plan. The approved Play AVD can exercise it, and the inspected Play
  Termux build exposes the boot and persisted-job mechanisms required for the
  restart test. Do not delay production on an untestable built-in Terminal
  preference.
- Built-in Android Terminal/AVF may replace Termux only in a later plan after a
  production-capable target passes the same local-trust, real-Beeper, cold-boot,
  process-reclaim, and credential-preservation tests. Cuttlefish may supply
  portable-runtime evidence here but does not silently change the selected
  production runtime.
- Runtime recovery is automatic and invisible. A visible “Start Beeper” handoff
  is not an acceptable normal recovery path.
- Failure UI names the real state and next action. It does not collapse
  “starting,” “signed out,” “offline,” “stopped,” and “delivery unknown” into
  “app unavailable.”
- Tests run on an Android virtual device, not on the owner's Pixel. The primary
  test route is an ARM64 Android Virtual Device with Termux + `proot-distro`
  Debian. A custom Cuttlefish image may collect diagnostic AVF evidence if a
  measured Termux/proot incompatibility prevents the real server from running,
  but it cannot pass the selected Termux production route.
- The test VM uses the approved real accounts and low-stakes real actions.
- Authenticated state is preserved for the whole run. Do not reinstall apps,
  clear data, revoke tokens, unlink networks, or repeat OAuth as a debugging
  shortcut. This plan leaves the authenticated VM and checkpoint intact; their
  later deletion is a separate owner-requested destructive action.
- Evaluate every existing grant with the migration matrix before opening OAuth:
  reuse it when safe and compatible, otherwise preserve it while creating the
  already-approved Android grant. Use one persistent, authenticated VM; update
  apps with `adb install -r`; never use `-wipe-data`,
  `pm clear`, uninstall/reinstall, or a fresh AVD to recover from an ordinary
  implementation mistake.
- Browser/OAuth consent on Android is authorized. Human-only verification is
  batched into one setup pass whenever the provider requires it.
- Slack setup uses `ssdear` as approved. Existing Slack apps and their one-way
  settings remain untouched; if the current token is not safely portable,
  create one separate Android-only Operator app that can be deleted without
  changing the existing paired-computer integration.

### Account and test identities

| Service | Identity or rule |
|---|---|
| Google Calendar, Drive, Maps, YouTube | `aadivya.raushan@gmail.com`; Google Cloud project **Operator** |
| Slack | `ssdear` identity in the approved workspace |
| Microsoft Outlook | `ssdear@gmail.com` |
| Spotify | currently active signed-in Premium account |
| Notion | currently active personal workspace |
| Todoist | currently active personal account |
| Beeper | existing account and already-linked approved networks |
| OpenAI router | personal `ssdear@gmail.com` account, organization `org-oC0Cx9jwKVEEvRlRlqdQzTwE`; approved ceiling `$500/month` |
| Real message targets | the user's own account first; otherwise the already-approved Wife/Raina/Sinchana contact |

Do not put phone numbers, passwords, OAuth codes, access tokens, refresh tokens,
cookies, API keys, local pairing secrets, or message bodies in the repository,
test output, screenshots, or saved evidence.

The OpenAI boundary comes from
`saved-results/operator-agent-billing-account.md`, recorded 2026-07-31. Use only
that existing account and credential; do not switch organizations or billing
accounts. The owner approved up to `$500` in a calendar month. The recorded
prepaid balance was about `$50`, which is an observed provider limit rather than
a second approval ceiling. Offline router fixtures remain the default. Every
live run records model, request count, returned token usage, and estimated cost;
pause before a run that would put known monthly usage above `$500`, and report
insufficient balance rather than changing credentials if prepaid credit runs
out.

No paid Google Maps Platform use is approved by this plan. Offline Places and
Routes fixtures are the default. Before the first live Maps request, read the
current official SKU pricing/free-usage rules and project **Operator** billing
usage for the current billing month, then save the URLs, checked time, billing
project, current per-SKU counts, free allowance remaining, and calculation for
the whole live batch. The batch is capped at four calls total: one successful
Places lookup, one successful Routes request, and the two required package/
certificate rejection probes; automatic retries are off. Proceed only if the
current usage plus all four calls is provably inside the applicable zero-charge
allowance. If pricing, billing linkage, or sufficiently current usage cannot
prove zero charge, open no Maps endpoint and pause with the bounded four-call
cost estimate for explicit approval.

## Scope

### Must work from a normal Operator request

1. Instagram direct message through Beeper.
2. Discord direct message through Beeper.
3. Google Messages direct message through Beeper.
4. Slack read and direct send as the authenticated user.
5. Outlook read and send as `ssdear@gmail.com`.
6. Google Calendar read and write.
7. Google Drive search/read for files created by Operator or explicitly selected
   for Operator, plus creation of new files. Broad search across every existing
   Drive file is deferred because it requires the restricted `drive.readonly`
   or `drive` scope rather than the approved least-privilege `drive.file` scope.
8. Spotify search and start/transfer playback on the Android target.
9. Notion read and write in the active personal workspace.
10. Todoist read and write in the active personal account.
11. Maps place lookup and directions in Operator; navigation remains an honest
    handoff after the exact route opens in Maps.
12. YouTube search and exact selected-video playback with playback observed by
    Operator, not merely an `ACTION_VIEW` acknowledgement.
13. Android notification Direct Reply for a real reply-capable notification,
    kept separate from starting a Beeper conversation.
14. Installed-app open requests, including “open Instagram,” resolve to the
    actual installed package.

### Explicitly deferred

- WhatsApp, Facebook Messenger, Signal, Telegram, Microsoft Teams.
- Apple Notes, Apple Reminders, every iOS/macOS capability.
- Podcasts.
- Public release, Play submission, public posts, paid account changes, and the
  parent plans' services not named in this file.
- Any Mac-companion or Operator-owned cloud fallback beyond the approved static
  App-Link association file.

## Current gap map

The 2026-08-05 production audit is the baseline. Do not inherit the source
plans' optimistic headers as current truth.

| Gap proved by the audit | Required repair |
|---|---|
| Production execution lives in the Mac companion | Build a minimal Linux ARM64 `phone-runtime` from reusable Go capability packages and run it inside Android-hosted Linux. |
| Android falls back from an unmatched capability to `startNewTask` on the paired computer | Add a standalone-phone session mode. An unmatched local capability gets an honest unsupported result and never starts a Mac task. |
| Background macOS Keychain reads fail with `-25293` | Remove Keychain from the production path; Android owns credentials and supplies short-lived material through an authenticated local broker. |
| Production starts the fixed `explicit_app` router | Start the existing OpenAI stage-1 router from phone runtime and keep an explicit offline router only for unambiguous app-open commands. |
| Router emits `messaging/compose`; direct adapters require `send` | Add contract tests spanning stage 1, stage 2, and the live production registry; preserve true `send`, recipient, and body fields. |
| Instagram is absent from the active routing rules | Add installed-app open and Beeper-send coverage for Instagram. |
| Contact graph is empty and discarded | Direct adapters resolve recipients themselves. Handoff routes that truly require a person use a persisted, app-owned contact index; no empty temporary graph. |
| Beeper proof credentials never reach production | Start the existing Beeper account locally, authorize Operator through Beeper's documented localhost OAuth 2.0 + PKCE flow, and keep the bearer token in the Android broker without printing it. |
| Ten service adapters are skipped at startup | Load their approved phone-held credentials and assert every in-scope adapter is both registered and reachable. |
| Notification listener is not enabled | Add a one-time Android permission gate, verify the actual enabled-listener setting, and show a precise setup state until granted. |
| YouTube only acknowledges URL delivery | Preserve `watch_url`, dispatch the exact URL, then read Android media state/title before reporting playback complete. |
| Earlier proofs depended on Pixel or owner-only commands | Replace them with normal-request tests on the persistent Android VM and target-side readback. |

## Target architecture

### 1. Phone runtime, not a port of every adapter

Reuse the existing Go capability engine and adapters. Add a small Linux ARM64
entry point, `companion/cmd/operator-phone-runtime`, that includes:

- production routing, registry, preview/confirmation, execution, and telemetry;
- the existing Google, Microsoft, Slack, Spotify, Notion, Todoist, Maps,
  YouTube, Beeper, and notification-reply adapters, with Android-backed client
  interfaces where the phone must own a token or restricted API key;
- the existing mobile protocol server bound only to the guest loopback port;
- a local health endpoint that reports process, router, credential, Beeper, and
  adapter readiness without returning secrets;
- no desktop IPC, host installer, LaunchAgent, relaybox, or macOS Keychain code.

The build must cross-compile with `GOOS=linux GOARCH=arm64`. If the package graph
pulls in a desktop-only dependency, split that dependency behind an interface;
do not add runtime checks that leave the desktop code in the phone binary.

The first release of `phone-runtime` is intentionally capability-only. Construct
the existing mobile session with no `TaskSource`, do not advertise task/project
capabilities, and keep the normal capability request/preview/confirmation wire
contract. This removes the Codex Desktop/App Server dependency without inventing
a second general assistant in this plan.

Android must understand which session it joined:

- **standalone phone:** capability requests go to `phone-runtime`; a route miss
  stays on the phone and says no supported action matched;
- **explicit paired-computer mode:** existing task behavior may remain available,
  but it is never an automatic fallback for a phone service request.

Replace local-mode copy such as “Sending to computer…” and “the computer sent
this” with wording about Operator or the named service. A Mac that happens to be
online must not change standalone behavior.

The Android app also owns a narrow local service broker. It performs three jobs
that must not be pushed into Linux:

1. call Google's Android `AuthorizationClient` and Microsoft MSAL token cache;
2. make Maps and YouTube requests that use Android package/certificate-restricted
   keys, deriving the package and signing certificate from the installed APK
   rather than accepting caller-supplied identity headers; and
3. call the OpenAI router with the approved key without handing that key to the
   Linux process.

The Go adapters keep their existing interfaces. Production supplies Android
broker implementations over the authenticated local channel; Go unit tests use
fakes. A raw Maps web-service key must not be placed in Linux. Google states
that an Android-restricted key cannot also be used as an ordinary web-service
key, and direct Android REST calls need `X-Android-Package` and
`X-Android-Cert`; a wrong-package/wrong-certificate request must be rejected in
the connected test before this route is accepted.

### 2. Android-local transport

Add a local-runtime connection mode alongside the existing paired-host mode:

```text
Operator Android host              Linux guest / Termux Debian
---------------------              ----------------------------
127.0.0.1:9443             <---->  phone-runtime TLS/WebSocket
127.0.0.1:23373          <------  beeper-server Desktop API
127.0.0.1:9445 (temporary) <-----  Beeper OAuth loopback callback
```

- Termux/proot uses Android's host network directly.
- AVF Terminal uses its persisted localhost port-forwarding support.
- Bind only to loopback. Keep TLS and application authentication even on
  loopback so another Android app cannot impersonate the runtime.
- Port `9443` is fixed for the phone runtime. A process already holding it is an
  error; never scan for or silently choose another port.
- Port `9445` is bound by Operator only while Beeper OAuth is pending and is
  closed after one callback or expiry. A process already holding it fails the
  authorization before a page opens; never select another port silently.
- Initial local trust uses the exact bootstrap below. It is one Android setup
  action, not the Mac pairing screen, and it never repeats during restart.
- The connection state machine must distinguish runtime starting, runtime
  healthy, runtime degraded, and runtime failed.

#### First-time local trust and rotation

Reuse the existing TLS 1.3 pinning plus application-signature handshake, but do
not use trust-on-first-use. The Termux route bootstraps it as follows:

1. `phone-runtime pair-android` creates its persistent runtime signing identity
   and TLS certificate under the no-backup root, plus an ephemeral key and a
   single-use random 32-byte pairing secret. It writes a `0600` **public offer**
   containing only protocol version, offer id, expiry (10 minutes), port `9443`,
   runtime public identity, TLS SPKI pin, ephemeral public key, and challenge.
   The secret and every private key remain in runtime memory and never enter the
   offer. The runtime unlinks the offer on expiry and removes any expired offer
   during every start.
2. The command launches the explicit Android component
   `app.codexlauncher/.runtime.LocalPairImportActivity`, with package fixed to
   `app.codexlauncher`, using Android's Activity Manager and a one-time
   `content://com.termux.sharedfile` URI grant. It does not emit an implicit
   MIME intent or chooser. The public offer uses MIME type
   `application/vnd.app.codexlauncher.local-pair+json`; no secret is present if
   another app observes the launch. Do not use the clipboard, command-line
   secret extras, a world-readable file, a QR intended for another device, or
   an unauthenticated localhost discovery endpoint.
3. Operator accepts the explicit share only while its user-opened “Link local
   runtime” screen is awaiting an offer. Require one
   `content://com.termux.sharedfile` URI, resolve that provider to package
   `com.termux`, and verify its signing certificate against the official Play
   Termux signer recorded during AVD provisioning before reading the one-time
   URI grant. Reject text extras, wrong authorities, multiple files, expired
   ids, replayed ids, an implicit intent, and any launch while the setup screen
   is not waiting.
4. Operator reads the public offer into memory, validates the fixed
   version/port, and connects to `127.0.0.1:9443` with TLS 1.3 while pinning the
   offered SPKI. It creates a separate Android-Keystore P-256
   local-runtime-auth key with an attestation challenge covering the offer id,
   runtime identity, runtime ephemeral key, TLS pin, and transcript nonce.
   Operator returns the public key and complete Android Key Attestation chain;
   it still has not received the pairing secret.
5. Before releasing the secret, the runtime verifies the attestation chain to a
   pinned Android attestation root, its revocation status, challenge, security
   level, and `attestationApplicationId`. That application id must contain
   package `app.codexlauncher` and the exact final internal-release signing-
   certificate digest recorded by the clean provenance build. Production pins
   the applicable published Android attestation root. AVD/Cuttlefish tests pin
   only the recorded root for the immutable test-system image; never accept a
   root supplied by the client. A missing, self-signed, revoked, wrong-package,
   or wrong-signer chain fails closed without sending the secret or enrolling a
   key.
6. After that verification, the runtime sends the secret only inside the
   attested-key and runtime-ephemeral-key bound TLS transcript. The existing
   pairing challenge binds the package name, Operator signing-certificate
   digest, Android auth public key, runtime identity, both ephemeral keys, TLS
   pin, secret, offer id, and nonce. A malicious app that copies the MIME type,
   label, or icon cannot receive the explicit grant, obtain the secret, or
   enroll its key; a process that wins the localhost-port race also gets no
   secret.
7. On mutual success, Android stores the runtime identity/SPKI/epoch in its
   encrypted database; the runtime stores the Android public key/epoch as
   `0600`. Both delete the pairing secret and offer, Operator revokes the URI
   grant, and the runtime rejects every later bootstrap attempt until the user
   explicitly chooses “Replace local runtime.” Credential operations remain
   disabled until this acknowledgement is durable on both sides.

The optional Cuttlefish/AVF evidence route must provide an equivalent
system-owned, package-verified one-time file/share handoff. If its public
Terminal surface cannot do that, its evidence route fails; do not weaken
bootstrap security or replace the selected Termux production route.

Normal reconnect pins the stored TLS SPKI, then requires a fresh nonce signed by
the stored Android P-256 key before credential operations are enabled. Bind each
credential request to the connection transcript, session id, request id,
adapter, and exact scope allowlist; reject replays and requests from the normal
durable action stream. A different localhost app may cause temporary port
denial, but it cannot authenticate or receive credential material.

Rotate a runtime TLS pin only over the already-authenticated connection: the
runtime identity signs the new SPKI and epoch, Android stores it atomically,
then acknowledges before the runtime activates it. Rotate the Android auth key
the same way with an old-key signature. Keep this auth key separate from the
credential-database wrapping key. If either old auth key is lost, require the
same one-time local pairing flow; do not clear or reconnect provider accounts.
A one-sided checkpoint restore must fail closed and offer local re-pairing only.

Red tests precede this implementation: fake Termux provider/signature, forged
offer, expired/replayed offer, implicit-share interception, lookalike receiver
with the same MIME type/label and a different package or signer, wrong or
untrusted attestation root, wrong attestation challenge/application id, wrong
TLS pin, hostile process holding `9443`, wrong secret, wrong Android signature,
credential request before acknowledgement, old-epoch rotation, one-sided
restore, and log/fixture secret scans. The connected lookalike test must prove
that it receives no URI grant or secret and cannot enroll a key.

Separate Beeper-auth red tests occupy `23373` with a plausible fake metadata and
approval server, occupy callback port `9445`, reuse a dead Beeper PID, change the
server binary/socket inode, and kill/swap the server after page open but before
code exchange. Before-page attacks must open no authorization page; every case
must accept/store no token and let no broker request reach the fake.

### 3. Android credential broker

Android is the credential owner:

- Long-lived refresh tokens, API keys, generated client credentials, and the
  local runtime pin/epoch live in an app-private encrypted database whose key is
  wrapped by Android Keystore. The local-runtime-auth private key remains
  non-exportable inside Android Keystore. The database and keys are excluded
  from Android backup.
- The Linux runtime receives only what it needs for the current process or
  request. It never writes secrets to logs, command lines, shell history, or
  the repository.
- Existing valid grants are tested before replacement. A bearer/refresh token
  may be imported only when it belongs to the same provider client and the
  provider permits it; migration writes directly to the encrypted database and
  never prints the value.
- Private app sessions are never copied from the Pixel. Sign into the persistent
  Play AVD once, complete all required provider consent in one batch, and keep
  that writable data partition and its current owner-only checkpoint. An implementation
  error is never fixed by starting with a clean device.
- Use the provider's native Android flow where it exists. Slack and Spotify use
  their registered Android custom schemes. Notion and Todoist, whose current
  production rules require HTTPS redirects, use verified Android App Links on
  the already-owned `tryoperator.net` domain. No callback depends on a desktop
  process or a token-handling server.
- Before changing an OAuth/API flow, the implementer must check the provider's
  current official documentation. No shared confidential client secret may be
  hardcoded into the APK.

Maps, YouTube, OpenAI, and provider-compatible OAuth registration/token records
use this one-time Android-side credential-ingestion path:

1. Before any secret is read, the internal-release Operator APK creates a
   non-exportable P-256 Keystore key for key agreement and returns only its
   public key, attestation chain, one-time import id, and 10-minute expiry. The
   local provisioning helper verifies the pinned Android attestation root,
   challenge, package `app.codexlauncher`, and the frozen internal-release
   signing digest.
2. A local setup helper, used only during AVD provisioning and never in
   production, reads only the approved records named in the migration matrix
   below directly from their existing macOS Keychain records. This is one-time test-
   VM provisioning, not a host process used by a production request. It never
   places plaintext in source files, environment dumps, arguments, standard
   output/error, shell history, the clipboard, or an `adb` command. If the
   secure store requires an
   unlock/approval, batch that one human action with the sign-in handoffs below.
3. The helper uses an ephemeral P-256 key, ECDH, HKDF-SHA-256, and AES-256-GCM
   to create a single-use envelope. Its authenticated metadata binds the import
   id, expiry, AVD serial, Operator package/signing digest, provider, and
   permitted operation. Only this ciphertext file crosses `adb` into Android;
   Operator imports it through the system document picker.
4. Operator decrypts directly into memory, validates each credential with its
   provider, writes the value to the Keystore-wrapped database, acknowledges
   only the named provider/credential id, zeroes buffers where the platform permits,
   and deletes the ciphertext document plus the one-time import key. Failed,
   expired, replayed, wrong-device, wrong-signer, or wrong-provider envelopes
   fail closed. The helper deletes its owner-only ciphertext temporary file
   after the durable acknowledgement.
5. If an approved non-OAuth key is absent from the secure store, stop at one
   Android `FLAG_SECURE`, no-autofill, clipboard-disabled secret-entry screen
   instead of exposing it to automation or chat. An OAuth grant never uses this
   manual-entry screen; use its provider's approved Android consent route. This
   is a batched owner handoff, not a reason to change provider/account or
   generate a second credential silently.

Red tests use canary secrets and scan the repository, process arguments,
stdout/stderr, host temporary paths after cleanup, `adb` command history,
Android shared storage after acknowledgement, logcat, crash reports, protocol
captures, Linux files/processes, and screenshots. A test succeeds only when the
canary exists solely in Android process memory and the encrypted database, and
the three provider health checks pass from the release-candidate APK.

Use a separate, non-journaled logical credential stream inside the authenticated
full-duplex connection on `127.0.0.1:9443`; do not open another port or put
secrets into the durable mobile session protocol. The broker responds only after
the pin/signature checks above, redacts HTTP bodies and headers from logs, and
returns values only in memory. A Go `oauth2.TokenSource` backed by this broker
lets API clients request a current access token without owning the refresh
token. Maps, YouTube, and OpenAI use broker operations rather than receiving
their API keys.

Credential-broker requests need an explicit contract:

```text
runtime -> credential_request(adapter, scopes, request_id)
Android -> credential_ready(request_id, short_lived_access, expires_at)
Android -> credential_unavailable(request_id, reason, recoverable)
```

Never include credential values in protocol debug serialization or captured
test fixtures. The request id and expiry may be logged; the response value may
not. Rewrite the current Google, Slack, and Spotify desktop flows rather than
carrying their client-secret requirements onto Android.

Provider contract, checked against the official documentation on 2026-08-05:

| Provider | Phone method and callback | Exact access ceiling | Durable owner and refresh | Reuse, disconnect, and migration rule |
|---|---|---|---|---|
| Google Calendar + Drive | Google Play Services `AuthorizationClient` with Android OAuth client for `app.codexlauncher` in project **Operator**; Play Services owns the result callback; use Google's Picker flow when the owner explicitly adds an existing Drive file to Operator's app-visible corpus | `calendar.events` and `drive.file` only | Play Services caches short-lived access; broker asks again silently when expired; no refresh token is stored on device | Desktop refresh tokens are not imported into Play Services. Reuse the signed-in Google account and prior project grant when `authorize()` returns without resolution; otherwise use the already-approved first Android-client consent once. `revokeAccess()` revokes all project scopes, so disconnect must warn and stay provider-wide. |
| Microsoft Outlook | MSAL Android public client, authorization-code + PKCE; registered `msauth` redirect for the installed package/signature | `offline_access User.Read Mail.ReadWrite Mail.Send` | MSAL's encrypted token cache; `acquireTokenSilent` before any interactive call | Raw desktop/browser refresh tokens are not imported into MSAL. Reuse the cached `ssdear@gmail.com` account and system-browser session, then use the already-approved first MSAL Android consent if silent acquisition cannot find an account. Disconnect removes only this MSAL account/cache entry and then verifies silent acquisition fails. |
| Slack | Keep every existing Slack app unchanged. Reuse one existing valid `ssdear` user token if it is safely portable; otherwise create one separate, reversible Android-only Operator Slack app with PKCE enabled from its creation, custom URI + S256, and no client secret | user scopes `chat:write channels:read channels:history groups:read groups:history im:write im:history users:read`; empty bot scopes | Imported non-rotating token or Android-app rotating access/refresh pair in Keystore; serialize refresh; custom-scheme PKCE refresh needs no client secret | Validate the source token with `auth.test` without refreshing it. Import it only if its app id, workspace, user, token type, and expiry show that Android can use it without a client secret or competing rotation. Otherwise preserve it, create the separate app under the already-authorized `ssdear` Slack setup, and complete one Android PKCE consent. Never enable the one-way setting on an existing app. `auth.revoke` disconnects only the Android-held grant. |
| Spotify | Authorization Code with PKCE, registered Android callback URI; client id + verifier, no client secret | `user-read-playback-state user-modify-playback-state`; no playlist/library write | Refresh token in Keystore; preserve the previous refresh token when Spotify omits a new one | Validate the current Premium grant in its source client without refreshing it. Import only if official metadata and client id prove the refresh grant is usable by the Android PKCE client without a client secret; otherwise preserve it and perform the already-approved one-time Android PKCE consent in the signed-in Premium account. Local disconnect clears the Android grant and opens Spotify's connected-app page because Spotify exposes no app token-revocation endpoint. |
| Notion | Hosted Notion MCP discovery, mandatory S256 PKCE, dynamic registration with `token_endpoint_auth_method=none`, verified App Link `https://tryoperator.net/oauth/android/notion` | measured tool list per workspace; read requires `notion-search`; writes only under Operator's own container page | Persist client id plus any returned client credential and the rotating token pair atomically in Keystore; one refresh at a time | Import the registered public client tuple and active personal-workspace token pair through the attested envelope only when metadata, redirect registration, workspace, and token endpoint auth method match. Never re-register merely to fix a token error. An incompatible/missing tuple uses one approved Android registration + consent. Refresh-token lifetime is at most 180 days or 30 idle days; `invalid_grant` means one reconnect, not a retry loop. |
| Todoist | RFC metadata + dynamic public-client registration + S256 PKCE; verified App Link `https://tryoperator.net/oauth/android/todoist` | `data:read_write` | client id and rotating token pair in Keystore; serialize refresh | Import the matching public client id and rotating personal-account token pair through the attested envelope only when current metadata confirms the same redirect/auth method. An incompatible/missing tuple uses one approved Android registration + consent. Todoist's confidential-only revocation means this public client performs a local disconnect and opens Integrations settings for provider-side removal; it never invents a successful RFC 7009 revoke. |
| Maps + YouTube | Android broker calls vendor APIs with separate Android package/signature-restricted keys; no OAuth for the scoped read/search calls | Maps place lookup/directions only after the zero-charge/approval gate; YouTube search only, capped at the existing 100-search product budget | Keys in Keystore; broker never returns them to Go | Import the approved project keys once through the attested encrypted-envelope flow above. Prove wrong package/certificate rejection inside the bounded Maps batch. Disconnect deletes the local key; provider revocation is key deletion in project **Operator**. |
| OpenAI router | Android broker makes the approved direct API call | stage-1 routing only; existing `ssdear@gmail.com` personal account and organization; `$500/month` ceiling | API key in Keystore; no copy in Linux | Import and validate the existing key once through the attested encrypted-envelope flow above, then reuse it. Record usage/cost, never switch account, and pause before known monthly usage would exceed `$500`. Provider revocation is dashboard key deletion; no new paid provider/account without approval. |
| Beeper | Official CLI/server sign-in and device verification inside the phone-local profile; local Desktop API OAuth 2.0 + S256 PKCE at `127.0.0.1:23373` | existing linked Instagram, Discord, and Google Messages accounts only | Beeper profile in Termux/AVF app-private, no-backup storage; Desktop API bearer token in Android's Keystore-wrapped database | Reuse account/network state. Authorize Operator once through the official local OAuth page, introspect before reuse, and never scrape stdout/private config. Never clear the whole profile to reconnect one network. |

The migration decision is provider-specific and is recorded before any consent
page opens:

| Existing source record | Safe Android disposition |
|---|---|
| Google Calendar/Drive desktop grant | Do not export it. Ask Play Services for the already-signed-in Android account; accept a silent result or use the approved first Android-client consent. |
| Microsoft desktop/browser grant | Do not export it. Ask MSAL's cache, reuse the signed-in system-browser account, or use the approved first MSAL consent. |
| Slack user token | Health-check without refresh. Envelope-import only a matching, non-competing public token; otherwise preserve it and use the separate Android app + PKCE once. |
| Spotify refresh grant | Health-check without refresh. Envelope-import only if the same client and official rules prove secretless PKCE refresh; otherwise preserve it and use Android PKCE once. |
| Notion DCR client + token pair | Envelope-import the complete atomic tuple only when current metadata, workspace, redirect, and public auth method match; otherwise register/consent once on Android. |
| Todoist DCR client + token pair | Envelope-import the complete atomic tuple only when current metadata, account, redirect, and public auth method match; otherwise register/consent once on Android. |
| Beeper account/profile | Do not copy Desktop files or a bearer token. Sign into the phone-local profile through Beeper's official device verification, then grant Operator a new localhost OAuth token. |

The source health check records only provider, account/workspace, client id,
scope set, token class, expiry, and pass/fail. It never refreshes a rotating
source token, prints a value, or deletes/revokes the source. Once an imported
rotating pair is acknowledged in Android, every Mac/desktop process that could
refresh that source stays off; the source copy is marked retired but is not
deleted. A record that cannot prove compatibility is **not** “invalid”: it stays
preserved while the already-approved Android consent creates a separate grant.

Official references the implementation agent must re-check if a provider has
changed: Google Android authorization, Google Picker with `drive.file`, and
installed-app OAuth; Microsoft auth code/PKCE; Slack OAuth + PKCE; Spotify PKCE;
Notion's custom MCP client guide;
Todoist's OAuth metadata; Android App Links/domain verification; and Google's
Maps API-key security guide. The local-trust implementation must also re-check
Android Key Attestation/application-id validation and revocation, explicit
Activity Manager component launches with one-time URI grants, and the installed
Termux provider/signer. Save the URLs and checked date in implementation
evidence.

The owner explicitly approved these two HTTPS App Links as the sole
Operator-owned cloud dependency. The existing static site serves only
`/.well-known/assetlinks.json` for `app.codexlauncher`; the two callback paths
have no page, handler, forwarding rule, analytics, or other deployed content.
The site never exchanges, stores, proxies, or intentionally handles an
authorization code or token. Derive the certificate fingerprint from the one
signing identity locked before the first authenticated install, and test the
hosted association with Android's domain-verification tools. Immediately before
every Notion or Todoist authorization launch, call
`getDomainVerificationUserState()` and require both
`hostToStateMap["tryoperator.net"] == DOMAIN_STATE_VERIFIED` and
`isLinkHandlingAllowed == true`. Also resolve the exact HTTPS callback intent
with `MATCH_DEFAULT_ONLY` and require Operator's declared callback component as
the sole default—never a browser or resolver. If any check fails, do not open
the provider authorization page; show the exact Android supported-links setting
that must be enabled. Recheck while authorization is outstanding and cancel the
local flow if handling becomes disabled.

Add manifest and connected tests for `android:autoVerify="true"`, host
`tryoperator.net`, and path prefix `/oauth/android/`; reject a wrong host/path,
wrong state, missing PKCE verifier, and a callback delivered to a differently
signed APK. The connected suite must disable “Open supported links,” prove the
authorization page is never opened and no request reaches either callback path,
then re-enable it and prove both callbacks resolve directly to Operator without
a browser/resolver. This small static association is permitted by the no-backend rule;
adding callback compute, storage, forwarding, analytics, or a token proxy is
not.

The deployed association is generated from the signing certificate frozen
before the first authenticated install and has this exact shape (the build
supplies the fingerprint; it is not typed by a person):

```json
[
  {
    "relation": ["delegate_permission/common.handle_all_urls"],
    "target": {
      "namespace": "android_app",
      "package_name": "app.codexlauncher",
      "sha256_cert_fingerprints": ["<DERIVED_INTERNAL_RELEASE_SHA256>"]
    }
  }
]
```

After deployment, save the static-site source commit/deployment id, the fetched
file's SHA-256, and `pm verify-app-links --re-verify app.codexlauncher` plus `pm
get-app-links app.codexlauncher` output. The verification must name
`tryoperator.net` as verified before either OAuth flow opens. Reuse an existing
signed-in site/deployment session; do not ask for another sign-in while it is
valid.

### 4. Beeper lifecycle

Inside the Android Linux environment:

1. Install the official `beeper-cli` and dedicated headless ARM64
   `beeper-server`; never use the Desktop AppImage.
2. Reuse the existing Beeper account and linked network state. Do not unlink
   working networks to fix another network.
3. Set `HOME`, `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, and `XDG_CACHE_HOME` for both
   Beeper processes under `/data/data/com.termux/no_backup/operator/beeper` in
   the Termux route (or the AVF equivalent). Create the root with mode `0700`.
   The Android `no_backup` directory prevents cloud backup; Android
   file-based encryption protects it at rest after the device locks.
4. During the first spike, trace file opens and fail the spike if durable auth
   or network state is written outside that root. Cache/tmp writes are allowed
   only in app-private, backup-excluded locations.
5. Add the managed server target with the official CLI, pin stable CLI/server
   versions and SHA-256 hashes in evidence, and bind its authenticated Desktop
   API to `127.0.0.1:23373` only. Never scrape a token from output or read a
   private Beeper config file directly.
6. Before Android trusts anything on `23373`, the controller and authenticated
   `phone-runtime` prove which process owns it. Record the `runsv` child PID,
   `/proc/<pid>/stat` start time, pinned executable path/inode/SHA-256, and every
   socket inode in `/proc/<pid>/fd`. Match exactly one of those in
   `/proc/net/tcp*` to `127.0.0.1:23373` in `LISTEN`; a listener owned by any
   other PID, a reused PID/start time, a changed binary, or an extra listener
   fails closed. `phone-runtime` fetches
   `http://127.0.0.1:23373/.well-known/oauth-authorization-server`, requires an
   all-loopback issuer, authorization endpoint, token endpoint, introspection
   endpoint, S256 PKCE, and either standards-based public-client registration
   or a documented public client id, then signs the canonical metadata hash,
   child identity, socket inode, and fresh Android nonce over the already-pinned
   `9443` session. Android never fetches or accepts metadata directly from an
   unproved `23373` listener.
7. Operator verifies that signed ownership report, binds the exact loopback
   redirect `http://127.0.0.1:9445/oauth/beeper/callback` before opening a page,
   generates state plus a PKCE verifier in Android memory, and opens only the
   attested local authorization endpoint. A process already holding `9445`,
   non-loopback endpoint, missing S256 support, or missing public-client path
   fails Wave 0; do not fall back to an in-app token, stdout, or private-file
   scraping. The controller watches the child PID/start time and socket inode
   for the entire authorization window and invalidates the attempt immediately
   if any changes.
8. After the owner approves the local Beeper connection once, Operator receives
   the code on `9445`, verifies state, then sends the code and verifier on the
   non-journaled authenticated `9443` credential stream. `phone-runtime`
   re-proves the same live Beeper PID/socket/binary before exchanging the code
   and introspecting the token against that owned endpoint; a process swap
   between approval and exchange fails. It returns the token once in memory to
   Android, which stores it only in the Keystore-wrapped database. The broker
   later gives `phone-runtime` a memory-only bearer value for the current API
   call. Restart re-proves ownership and introspects the stored token before any
   send; invalid means one local OAuth reconnect, not a profile reset.
9. Start Beeper before `phone-runtime` under the controller below and expose
   process, API, account, and per-network state separately.

This follows Beeper's current official authentication contract: every Desktop
API endpoint requires a bearer token; OAuth 2.0 with PKCE is supported; RFC 8414
metadata is served from the localhost URL above; and `/oauth/introspect` reports
whether a token is active. The implementation must save the fetched metadata
and checked documentation date. The dedicated headless ARM64 server remains
unproven until it exposes this same contract in the Wave 0 spike; if it does not,
Wave 0 fails.

The live 2026-08-05 OpenAPI schema proves that `POST
/v1/chats/{chatID}/messages` accepts text/reply/attachment and returns a
`pendingMessageID`; it has no client transaction or idempotency field. Do not
invent one. Use this durable send journal instead:

```text
prepared -> confirmed -> dispatching -> submitted(pendingMessageID)
                                         |            |
                                         v            v
                                      observed      delivery_unknown
```

- Atomically write `confirmed` with action id, payload hash, account id, and
  chat id before the external call; atomically write `dispatching` immediately
  before `POST`.
- A repeated action id with the same hash returns the stored state. The same id
  with a different hash is rejected.
- After a 200 response, persist `pendingMessageID`, then poll `GET
  /v1/chats/{chatID}/messages/{messageID}`. That endpoint accepts the pending id
  and resolves it to the final message.
- Complete only when the read message matches chat/text, `isSender=true`, and
  `sendStatus.status=SUCCESS`. `PENDING` remains pending;
  `FAIL_PERMANENT` is failed. `FAIL_RETRIABLE` is shown as failed/retryable but
  is not resent automatically because the send endpoint has no idempotency.
- A crash/timeout in `dispatching` with no pending id is
  `delivery_unknown`. Search the selected thread for an exact recent match when
  possible, but never blind-resend. A timeout with a pending id resumes GET
  polling and never repeats POST.

The action journal is an Android app-private encrypted Room database, not a
Linux log. It contains identifiers, hashes, timestamps, state, and provider
receipt ids but no raw message body. Beeper's own profile remains in its
separate no-backup root.

Session operations are deliberately narrow:

- checkpoint: preserve the current authenticated AVD data partition locally
  with owner-only permissions; replace it only after a clean shutdown and never
  export it or restore an older copy to undo a code mistake;
- reconnect: act on one Beeper account/network and prove the other two stay
  connected;
- migration: use Beeper's official device verification/login, not a raw copy of
  Desktop files;
- final preservation: leave the authenticated AVD, its owner-only current
  checkpoint, and any diagnostic Cuttlefish data intact at handoff. Deletion is
  outside this plan and occurs only as a later, explicit owner-requested action.

### 5. Automatic recovery and honest status

#### Exact controller ownership

Selected production runtime and primary AVD test route: Termux + Debian.

1. Pin Google Play Termux `googleplay.2026.06.21` (`versionCode=141`) or a newer
   build only after repeating these checks. The inspected APK targets API 37,
   declares `RECEIVE_BOOT_COMPLETED`, owns `TermuxBootReceiver`, runs its service
   as foreground-service type `specialUse`, and executes sorted scripts from
   `~/.config/termux/boot` and legacy `~/.termux/boot` in the background.
2. Install one executable script,
   `~/.config/termux/boot/10-operator-runtime`. It acquires the Termux wake lock
   and `exec`s `$PREFIX/libexec/operator-runtime-watchdog`. It never opens a
   Terminal activity. Install the official `termux-api` client package from the
   same Termux package source; the inspected Play APK already contains its
   matching JobScheduler service.
3. Register the same watchdog as persisted Termux job id `7301`:
   `termux-job-scheduler --script "$PREFIX/libexec/operator-runtime-watchdog" --job-id 7301 --period-ms 900000 --network none --battery-not-low false --persisted true`.
   Android permits no shorter periodic JobScheduler interval. This job is the
   invisible repair path if Termux's foreground process is reclaimed and must
   run even while offline; boot remains owned by `TermuxBootReceiver`.
4. The outer watchdog takes an exclusive `flock`, writes a persistent JSON state
   journal under `/data/data/com.termux/no_backup/operator/runtime`, and starts
   `proot-distro login debian -- /usr/bin/runsvdir /etc/operator/services`. If
   proot or `runsvdir` exits, it restarts with
   1/2/4/8/16/30/60-second capped backoff and records exit class and attempt.
5. Debian `runsvdir` owns two `runsv` services: `beeper-server` and
   `phone-runtime`. Each has a check script, separate logs, and restart backoff.
   `phone-runtime` may start before Beeper but reports messaging as `starting`
   until Beeper health and account checks are green.
6. Operator does not keep a fake forever-service. When visible or handling a
   request it reads the local health endpoint and journal, reconnects, and shows
   exact state. Termux owns the Android foreground service and its one
   system-required persistent notification.

Recovery boundary by failure:

| Failure | Owner | Required recovery/proof |
|---|---|---|
| Beeper or phone-runtime child exits | Debian `runsv` | restart child; same action journal and sessions; green health without a tap |
| proot or `runsvdir` exits | Termux outer watchdog | restart Debian supervisor with capped backoff; no duplicate send |
| Termux process is reclaimed | persisted Termux JobScheduler job `7301` | the inspected service returns `START_NOT_STICKY`; use the same-UID kill test below, verify the package remains `stopped=false` and job `7301` remains pending, then prove natural JobScheduler execution creates a different Termux service PID, restores health with the same profile/journal, and does not duplicate a send |
| Device reboots | Termux's own `BOOT_COMPLETED` receiver | after first device unlock, boot script starts runtime; Operator later connects without launching Termux UI |
| User force-stops Termux | Android platform | Android intentionally suppresses receivers/services until the user relaunches Termux. Operator must say “Android has force-stopped Operator services” and open exact App Info; never call this an automatic-recovery success. |
| Battery/background restriction | Android setting | one-time onboarding requires notifications, wake lock, background use, and battery-unrestricted settings for Termux; status names the missing setting |

The production acceptance claim covers process death, proot death, network loss,
and reboot after unlock. Process-reclaim recovery may take Android's measured
JobScheduler interval. Run three unlocked, battery-unrestricted trials; each
must return to ready without a tap in at most 20 minutes, and evidence records
all three observed times. A slower or missed trial fails the Termux route rather
than weakening the promise. It does not falsely claim that Android permits
recovery from an explicit user force-stop. No test may substitute `adb am start`
for any of these recovery paths.

```text
BOOT / PROCESS DEATH
        |
        v
runtime_starting -- health green --> ready
        |                                |
        | timeout                        | process dies
        v                                v
runtime_start_failed <---- backoff ---- auto_restarting

Per service after runtime is ready:
connected | authorization_required | service_offline | network_offline
```

Required user-visible states:

| State | Required message and action |
|---|---|
| Linux/runtime starting | “Starting Operator services…” with progress and elapsed time; no error yet. |
| Device not yet unlocked after reboot | “Unlock once to start Operator services”; start automatically immediately after unlock. |
| Automatic restart underway | “Operator services stopped and are restarting…”; preserve the pending request without sending it twice. |
| Start timed out | Name the failed component and offer Retry plus setup diagnostics. |
| Beeper signed out | “Beeper needs sign-in”; open the exact Android setup surface. |
| One network disconnected | Name Instagram, Discord, or Google Messages; do not call the whole server unavailable. |
| Device offline | “No network connection”; retry only after connectivity returns. |
| Provider authorization expired | Name the provider and offer one reconnect flow that preserves other sessions. |
| Delivery unknown | Say the message may have been sent and provide target-thread readback; never blind-retry. |
| App absent | Name the missing Android package/app and offer installation; do not report this when the package is installed but routing is wrong. |
| Termux force-stopped | “Android has force-stopped Operator services”; open Termux App Info and explain that Android requires one relaunch after an explicit force-stop. |

Android 15+ forbids `dataSync`, camera, media playback, phone call, media
projection, and microphone foreground services from a boot receiver. Do not
mislabel the runtime as one of those. The inspected Termux build uses
`specialUse` with its Linux-environment reason. Re-check the Android 16 rules and
the installed Termux manifest before implementation evidence. Operator adds
`RECEIVE_BOOT_COMPLETED` only if its own receiver does real bounded work; Termux
or the approved AVF mechanism owns Linux startup. Do not fake recovery with
`adb am start`, an instrumentation-only hook, or a flashing activity.

Logging must use the existing structured loggers. Tag the phone runtime,
supervisor, credential broker, and each adapter; record inputs by shape, state
transitions, selected route, output state, and full caught-error context without
secret or message contents.

## Service and verb acceptance table

Every row starts from an ordinary typed Operator prompt and includes the actual
preview/confirmation. Provider receipts and generated ids are persisted before
cleanup.

| Capability / verb | Phone-local route | Required observation and cleanup |
|---|---|---|
| Instagram / send | local Beeper Server API | provider event resolves from pending id with matching chat/text, `isSender=true`, `sendStatus=SUCCESS` |
| Discord / send | local Beeper Server API | same final-event proof in the selected approved Discord conversation |
| Google Messages / send | local Beeper Server API | same final-event proof in the approved self/Wife thread; number never enters repo/evidence |
| Slack / read | Slack Web API user token | `auth.test` identity is `ssdear`; read a unique marker from the exact approved destination through `conversations.history` |
| Slack / send | `chat.postMessage` as user | first resolve a writable self/App Home conversation for `auth.test.user_id`; if Slack rejects self-messaging, call `users.list` under `users:read`, keep only non-deleted human members, and require one exact normalized display-name/real-name match for Wife, Raina, or Sinchana. Zero or multiple matches fails before preview. Show the resolved name and immutable user id before confirmation. Read the returned channel/`ts` back with exact marker and sender id equal to `auth.test.user_id`; delete only that test message if allowed |
| Outlook / read | Microsoft Graph delegated user | read one unique self-test message and show subject/from/preview returned by Graph |
| Outlook / write | Graph draft | create a self-addressed draft, read it from Drafts by returned id, then delete that id |
| Outlook / send | Graph mail | send to `ssdear@gmail.com`; read returned marker in Sent Items and Inbox, then delete only those ids |
| Calendar / read | Calendar API | read the unique event created by the write test and match id/title/time |
| Calendar / write | Calendar API | create event, persist returned id, read it, then delete that id |
| Drive / scoped search + read | Drive API under `drive.file` | create one file and explicitly select a second existing low-stakes file for Operator; find both within the app-visible corpus, fetch exact content by returned id, and show it in Operator. Confirm an unrelated unselected file is absent. Metadata-only is insufficient, and the result must not claim whole-Drive search. |
| Drive / write | Drive API | create a unique text file, persist/read returned id and content, then delete that id |
| Spotify / read | Spotify Web API | exact track id/title/artist from search appears in Operator |
| Spotify / play | Web API + Android Spotify | transfer/start the selected track on the VM; Android media session reports playing and matching title; stop playback afterward. `hands_off` or “no active device” fails this row. |
| Notion / measured connect | hosted MCP `tools/list` | save the actual tool names. Completes is allowed only when `notion-search`, `notion-fetch`, `notion-create-pages`, and `notion-update-page` are all present; otherwise this plan fails rather than silently lowering the scope. |
| Notion / read | hosted MCP | search the unique child under Operator's own container, fetch by returned id, and match exact marker |
| Notion / write | hosted MCP | create only under Operator's own container, fetch by returned id, then archive/delete that exact child if the live tool set supports it; otherwise record the remaining id for manual cleanup without deleting broader content |
| Todoist / read | official API | read the unique task created by the write test and match returned id/content |
| Todoist / write | official API | create, read by returned id, then complete/delete that id |
| Maps / place | Android broker + Places, after the zero-charge gate above | show exact place id/name/address in Operator |
| Maps / directions | Android broker + Routes, after the zero-charge gate above | show origin/destination/distance/duration; for navigation, foreground Maps with the exact destination and report `hands_off`, never “navigation complete” |
| YouTube / read | Android broker + Data API | show exact selected video id/title/channel in Operator |
| YouTube / play | exact Android watch URL | foreground YouTube with that video id; Android media session reports playing and matching title. Generic app open or `ACTION_VIEW` acknowledgement fails. |
| Direct Reply / send | Android `RemoteInput` on a real reply-capable notification | target conversation contains exactly one unique reply and the notification action/result is observed. No incoming notification means wait for the approved human handoff; this row cannot be waived. |
| Open installed app | Android package resolver | dynamically resolve launcher-visible apps and make every in-scope installed app foreground: Instagram `com.instagram.android`, Discord `com.discord`, Google Messages `com.google.android.apps.messaging`, Slack `com.Slack`, Outlook `com.microsoft.office.outlook`, Calendar `com.google.android.calendar`, Drive `com.google.android.apps.docs`, Spotify `com.spotify.music`, Notion `notion.id`, Todoist `com.todoist`, Maps `com.google.android.apps.maps`, and YouTube `com.google.android.youtube`; provisioning must confirm each recorded package id before the test |

Use unique test markers and returned object ids. Never find cleanup targets by
title alone. If cleanup fails, record the exact remaining object; do not delete
broader user data.

## Reproducible test fixtures

### Primary Play AVD

Create one persistent AVD named `operator_android16_arm64` with:

```text
device: pixel_9
system image: system-images;android-36;google_apis_playstore;arm64-v8a
system image revision: 7 (or save the replacement revision if Google removes it)
emulator: 36.6.11
platform-tools: 37.0.0
data partition: 12 GB
owner-only authenticated checkpoint after setup: operator-authenticated-v1
```

- Install every proprietary app through this AVD's Google Play Store. Record
  package name, version code/name, signer digest, and install date for Instagram,
  Discord, Google Messages, Slack, Outlook, Calendar, Drive, Spotify, Notion,
  Todoist, Maps, and YouTube. Never pull these APKs or their data from the Pixel.
- Install Play Termux `googleplay.2026.06.21`/141. Before accepting a newer
  build, repeat the manifest/boot-script proof and store its signer plus SHA-256.
  This build already contains the boot receiver; do not mix it with a
  differently signed Termux:Boot plug-in.
- Install Debian with `proot-distro`, pin the Debian snapshot/package versions,
  and install `runit`, the exact official Beeper CLI/server versions, and the
  hashed phone-runtime artifact.
- Sign in once. Keep the AVD directory and checkpoint owner-only, never export
  either,
  use `adb install -r` for Operator updates, and save a pre-test session-health
  report instead of reauthorizing healthy services.
- The writable AVD data directory, not a restored old snapshot, is the normal
  source of truth. After the one-time sign-in batch and after each completed
  wave, power the AVD off cleanly and replace one owner-only checkpoint on the
  same host. Never load an older checkpoint merely to undo a code mistake
  because it can roll rotating refresh tokens backward.
- Final reboot proof uses a cold boot with the existing writable data and no
  `-wipe-data`. The current checkpoint exists only to recover from same-host AVD
  or snapshot corruption and is replaced after successful rotating-token
  refreshes. It is not a disk-loss backup. Disk-loss recovery is explicitly
  outside this plan and may require provider sign-in again; do not claim
  otherwise or add an off-device credential copy without fresh owner approval.

### Cuttlefish + AVF diagnostic route

This diagnostic route is triggered only by saved loader/syscall proof that the
real ARM64 Beeper Server cannot run under Termux/proot. That finding immediately
fails Wave 0 and requires a new production-runtime decision; no Cuttlefish result
closes this plan. It is not triggered by an unfamiliar setup step. Cuttlefish
may isolate whether AVF changes the loader/syscall failure, but it cannot turn a
failed Termux production path into success.

1. Use an ARM64 Linux host with hardware KVM, nested virtualization exposed to
   the guest, at least 64 GB RAM and 400 GB free disk. Gate immediately on
   `test -c /dev/kvm` plus successful KVM access. A paid host requires the normal
   spending approval before provisioning; it is test infrastructure, never a
   production backend.
2. Sync and select the official ARM64 phone target, then add `VmTerminalApp` to
   that product and save the exact product-file diff:

   ```bash
   repo init -u https://android.googlesource.com/platform/manifest -b android-latest-release
   repo sync -c
   source build/envsetup.sh
   lunch aosp_cf_arm64_only_phone-userdebug
   # Add: PRODUCT_PACKAGES += VmTerminalApp to the selected product definition.
   m -j"$(nproc)" dist
   ```

3. Take the `*-img-*.zip` and `cvd-host_package.tar.gz` from that one build;
   never mix host tools and images from different build ids. Launch them with:

   ```bash
   mkdir -p cvd-run/images
   tar -xzf cvd-host_package.tar.gz -C cvd-run
   unzip '*-img-*.zip' -d cvd-run/images
   cd cvd-run
   HOME="$PWD" ./bin/launch_cvd --daemon --system_image_dir="$PWD/images"
   ```

   Record the build id and SHA-256 hashes for the product diff, image archive,
   and host package.
4. Before Operator work, run `atest MicrodroidHostTestCases MicrodroidTestApp`.
   Failure means AVF is not usable on that host and is recorded as a real
   environment failure, not waved away.
5. Install the same hashed Operator release-candidate APK used on the Play AVD.
   Configure Terminal's persisted localhost forwards for the Operator runtime
   ports `9443` and `23373`, boot Debian, and install the same hashed phone
   runtime and Beeper versions.
6. Do not repeat Beeper or provider sign-ins in Cuttlefish. Run only the
   loader/syscall, process-health, localhost-forward, and fake-contract checks
   needed to explain the Termux failure. If the server cannot expose a useful
   signed-out health state without login, record that exact limit and stop.

No Cuttlefish test passes a production acceptance row, Wave 0, or the final
production-restart claim. Those pass only through Termux on the persistent Play
AVD. A host tunnel, Mac Beeper process, fake server, or raw `adb`-issued action
is diagnostic evidence at most, never production proof.

## Source-to-binary evidence

The current directory is already the isolated
`phase0-notification-probe` worktree. Do not create a nested worktree that drops
its uncommitted implementation. Before edits, save:

- `git rev-parse HEAD`, branch name, `git status --short`, and SHA-256 of a
  binary diff of tracked changes;
- a path inventory and hashes for untracked source artifacts that are actually
  inputs, without adding caches, binaries, secrets, or another agent's files;
- Go, Java, Gradle, Android SDK/NDK, emulator, Termux, Debian, Beeper, and AOSP
  versions.

After the last implementation or test edit, commit every in-scope source, test,
plan, and static App-Link association file. Caches, generated binaries, secrets,
and unrelated user-owned files remain untracked and are recorded as exclusions.
Require `git diff --quiet` and `git diff --cached --quiet` for tracked files,
record the final commit id, then create a clean sibling verification worktree at
that exact commit. Run every final Go, Android unit, Android instrumentation,
protocol, release, privacy, and security suite there, then build the Linux ARM64
runtime and Android debug, instrumentation, and internal-release APKs there.
Targeted red/green runs in the implementation worktree are development evidence,
not the final green claim. A dirty diff hash is a baseline aid, not final
provenance.

Save the final commit, submodule ids, dependency locks, build commands, signing
certificate digest, and SHA-256 hashes together. Instrumentation may use the
debug APK, but every real-account and system smoke uses the internal
release-candidate APK. Install that same release hash on the Play AVD and
Cuttlefish, and verify the installed package/signing digest. Never call a host
binary, an uncommitted build, or a differently built APK production proof.
Run the final VM/system and every real-account smoke only after those exact
clean-worktree hashes are installed. If any final suite, build, or smoke causes
an edit, return to the implementation worktree, add the regression test/fix,
create a new commit, then create a new clean sibling at that exact commit (or
move the existing sibling only after proving it has no changes). Repeat **all**
final suites, builds, installs, and smokes. Partial carryover
from the prior commit is not final evidence.

The signing identity is frozen before the first Operator install or provider
sign-in. Reuse the existing owner-held internal-release key if its certificate
matches the installed package; otherwise create or select the owner-held key
once, outside the repository, before provisioning the persistent AVD. Record
only its public certificate SHA-256 in evidence. Every authenticated-wave and
final release-candidate APK is signed by that same key, and every update uses
`adb install -r` only after its signer matches. A signer mismatch stops before
uninstall, data clearing, or OAuth. Generate `assetlinks.json` from this same
certificate; never introduce a new “final” signer after authentication begins.

## Implementation waves

### Wave 0 — isolate, freeze truth, and prove the runtime substrate

1. Confirm this existing isolated worktree and save the source-to-binary baseline
   above. Lock and record the owner-held internal-release certificate before the
   first Operator install. Do not create a nested clean worktree that omits
   current changes.
2. Preserve all existing untracked files; do not move or delete another agent's
   `companion/codex-launcher`, `spikes/`, protocol cache, or saved audit.
3. Record baseline Go, Android unit, Android instrumentation, and protocol tests.
   Build tests first and retain the expected red failures for local mode,
   runtime health, restart, and no-Mac fallback.
4. Build the smallest Linux ARM64 `phone-runtime` that answers health and accepts
   one authenticated capability-only local Android session.
5. Provision the exact persistent Play AVD above and install the named packages
   from Play. Do not use the earlier temporary Google-APIs-only AVD.
6. Install Play Termux, `proot-distro`, Debian, `runit`, `phone-runtime`, the
   official Beeper CLI, and the real headless Beeper Server. Use Termux's built-in
   boot receiver, not a separately signed Termux:Boot plug-in.
7. Write the local-trust tests red, then prove the package-verified Termux share,
   pinned `9443` TLS connection, two-sided application handshake, durable
   acknowledgement, replay rejection, rotation, and local re-pair behavior
   above. A fake localhost server or forged share must never enable one broker
   operation or observe a pairing/credential secret.
8. Prove `9443` and the PID/socket-owned `23373` are reachable from the Android
   app and remain loopback-only. During Beeper OAuth, prove `9445` exists only
   for the single pending callback, rejects a second callback, and closes on
   success, failure, or expiry.
9. Prove process death triggers automatic restart and the UI walks through
   `starting -> ready` without a tap. First statically confirm the installed
   Termux service's `onStartCommand` return mode. For the measured
   `START_NOT_STICKY` build, register persisted job `7301` and record its pending
   entry plus the Termux service PID. Trigger a test-only native helper from
   inside Termux; because it has the same app UID, it enumerates `/proc`, sends
   `SIGKILL` to every process with that UID, and kills itself last. It must not
   call ActivityManager, `am force-stop`, `pm clear`, or any exported Termux
   component. Immediately verify `dumpsys package com.termux` still reports
   `stopped=false` and `dumpsys jobscheduler com.termux` still contains job
   `7301`. Observe natural JobScheduler execution, a different Termux service
   PID, restored child/runtime health, unchanged Beeper account/profile, and no
   second provider send. Run three trials within the 20-minute bound. A
   separately forced job may diagnose scheduling but does not satisfy this
   process-reclaim acceptance proof. Keep the helper under test sources and do
   not ship it in the release APK or production runtime.
10. Prove an unmatched local request does not emit `new_task_start`, connect to a
    relay, or show “Sending to computer.”
11. Kill each child and then the proot supervisor separately. Prove `runsv` and
    the outer watchdog recover with the documented state and no duplicate action.
12. Cold-reboot the VM through emulator control, with no `adb am start`, and
    prove Termux's real boot receiver starts the script after unlock while
    Operator later reconnects without showing Terminal.
13. If and only if the diagnostic trigger fires, mark Wave 0 failed, then execute
    the bounded Cuttlefish recipe above to isolate the incompatibility; do not
    treat its result as a fallback pass.

**Wave 0 exit:** the persistent Android VM cold-boots, starts both real processes
without a tap, connects Operator through the pinned two-sided local identity,
reports exact health, survives child and proot death, and does not contact a Mac
or Operator relay. A forged offer/local server cannot authenticate or reach the
broker. A route miss also remains local and honest. An explicit user force-stop
is tested and reported as the documented Android exception, not counted as
invisible recovery.

### Wave 1 — credentials, routing, and one proving adapter

1. Add the Keystore-backed credential/service broker, Google
   `AuthorizationClient`, Microsoft MSAL, and the provider-specific callback
   routes in the auth table. Publish only the static `tryoperator.net`
   `assetlinks.json` association, verify the internal-release certificate, and
   fail closed before Notion/Todoist OAuth if Android does not report the domain
   verified.
2. Apply the provider-by-provider migration matrix above: validate each source
   record without refreshing it, envelope-import only a proven-compatible
   record, and preserve every incompatible source while creating its approved
   Android grant. Complete all unavoidable VM account sign-ins/consents once,
   consecutively, then save
   `operator-authenticated-v1`. Every later test starts with a non-secret session
   health check and skips interactive OAuth when healthy.
3. Start the OpenAI router through the Android broker. Keep the API key in
   Android secure storage, use only the recorded personal account/organization,
   and enforce the approved `$500/month` ceiling with per-run usage/cost records.
4. Replace the fixed production rule-list dependency. Add a contract test that
   every registered in-scope adapter is reachable by at least one ordinary prompt.
5. Correct `send`, recipient, body, exact app id, and exact YouTube URL fields at
   the routing boundary.
6. Make Todoist the first proving adapter because its existing dynamic
   registration and read/write proof offer the smallest end-to-end slice.

**Wave 1 exit:** “create a test task in Todoist” travels through the normal
Android prompt, route, preview, confirmation, local runtime, official API, and
target readback, with no Mac and no repeated sign-in.

### Wave 2 — requested messaging plus YouTube

1. Wire Beeper's three production adapters into the same live phone registry and
   router used by normal requests.
2. Prove one real approved send per Instagram, Discord, and Google Messages with
   the pending-id/final-event state machine above.
3. Preserve exact-recipient resolution and refuse ambiguous names before preview.
4. Add Instagram installed-app open routing independently of Beeper state.
5. Preserve `watch_url` across protocol serialization and make Android report
   measured media state/title before YouTube returns complete.
6. Enable and verify notification-listener access, then prove one real Direct
   Reply without presenting it as a new-conversation route. If no reply-capable
   notification exists, request the single approved inbound notification and
   wait; do not waive the test.

**Wave 2 exit:** all three Beeper networks have final provider-event readback;
exact YouTube playback is measured inside the request; Instagram opens when
requested; and Direct Reply appears exactly once in the target conversation.

### Wave 3 — remaining official services

Implement and prove in this order so shared OAuth work is reused:

1. Google Calendar + Drive, then Maps only after the saved zero-charge evidence
   or explicit bounded Maps spending approval.
2. Microsoft Outlook.
3. Slack.
4. Spotify.
5. Notion.

Each verb in the service/verb table needs a normal-prompt test plus the stated
real-account readback and cleanup. Notion must record and meet the four-tool
ceiling; Drive read must fetch content; Outlook must pass read/draft/send; Slack
must pass read/send as the verified user; Spotify cannot demote play to a
handoff. An OAuth page, HTTP 2xx, registered adapter, or owner-only proof command
does not satisfy the exit.

**Wave 3 exit:** every in-scope row in the service/verb acceptance table has one fresh VM
evidence record and remains connected after runtime and Android restarts.

### Wave 4 — recovery, regression, and production claim repair

1. Kill Beeper, phone runtime, proot/AVF runtime, and network in controlled tests.
   Expire a short-lived access token while preserving its valid refresh grant;
   use fakes for revoked-token states. Do not revoke a healthy real grant merely
   to test UI. Verify each exact state and recovery action.
2. Reboot the Android VM and prove automatic invisible recovery with all durable
   credentials still present.
3. Submit the same action id twice and prove the second request returns the
   journaled result without another external call. Prove submitted-with-receipt
   resumes readback and dispatching-without-receipt becomes `delivery_unknown`
   without an automatic resend.
4. Search for every Mac, Keychain, LaunchAgent, relay, Pixel-only, cloud-backend,
   and stale “COMPLETE” assumption in production code and plans. Remove or demote
   each production dependency and correct both parent-plan headers.
5. Commit every in-scope source, test, plan, and static association change. At
   that exact commit, create the clean sibling and run every final suite and
   build listed in the provenance section; do not use implementation-worktree
   output for the final claim.
6. Install only the resulting recorded APK/runtime hashes, rerun the final
   reboot/process-recovery checks and **every** real-account/service row, and
   repeat the whole clean-commit cycle after any corrective edit.
7. Save compact evidence that records commit and artifact hashes, versions,
   request/action ids,
   target-side observations, test counts, and failures without private content.
8. Leave the authenticated VM and current owner-only checkpoint intact. Record
   their paths without secret contents; do not delete them during this plan.

**Wave 4 exit:** Every Mac companion, relay, tunnel, and host-side
Operator/Beeper process is off. The computer may run only the Android VM and its
standard emulator support; no production request leaves Android. After a fresh
VM reboot, every claimed route passes through the production Android flow, all
failures are specific, full suites are green, and source plans match measured
reality.

## TDD and verification contract

For every behavior change:

1. State the observable input and output.
2. Add the narrow unit tests plus the integration/connected tests capable of
   catching the production failure.
3. Run them before implementation and retain the expected failure.
4. Implement the smallest direct replacement; delete the old production path
   rather than placing the fix behind an optional flag.
5. Re-run the targeted tests and then the relevant full suite.
6. Search the repository for the same assumption and inspect every caller whose
   contract changed.

Required test layers:

| Layer | Must prove |
|---|---|
| Go unit | adapter, routing, retry classification, action-journal deduplication, pairing-offer expiry/single use, transcript binding, Beeper PID/start-time/socket ownership, token redaction, health-state behavior |
| Go integration | ordinary prompt -> production registry -> real/fake provider contract; every registered adapter reachable; fake `23373` and Beeper process-swap rejection |
| Kotlin unit | supervisor state machine, Termux provider/signer validation, pin/epoch rotation, credential broker, status copy, action/result mapping |
| Android instrumentation | package-verified local-pair share, hostile `9443`/`23373`/`9445` rejection, single-use callback handling, broker auth, notification permission, exact intents, media observation, app foreground, force-stop copy |
| VM system test | actual ARM64 binaries, real Termux share/bootstrap, Beeper child PID/socket/binary ownership, loopback ports, child/proot/Termux process death, cold reboot, Termux boot script, saved auth state |
| Real-account smoke | every row/verb in the acceptance table; approved low-stakes targets only; release-candidate APK |
| Provenance | installed APK/runtime hashes match the recorded build; no host binary or Mac route receives a production request |

The Android instrumentation suite must also assert that standalone mode never
falls through to `startNewTask`, never says it is sending to a computer, and does
not advertise task/project controls the phone runtime cannot serve.

Fakes are required for deterministic failure coverage but never replace the
real-account smoke. A test that passed before the intended implementation change
does not count as the red TDD step unless it is explicitly a regression guard.

## Evidence and claim rules

A service is **production usable** only when all seven are true:

```text
normal Android request
  -> correct route and fields
  -> real preview and confirmation
  -> phone-local runtime or Android device action
  -> target-side observed result
  -> restart keeps the connection usable
  -> saved evidence and plan claim agree
```

Use these states consistently:

- **complete:** target-side effect observed and read back.
- **hands_off:** Operator prepared/opened the exact destination but another app or
  the user owns completion.
- **unverified:** implementation exists but the required VM/target observation is
  missing.
- **deferred:** owner removed it from this plan.
- **failed:** a current run produced a specific reproducible failure.
- **delivery_unknown:** submission may have happened but readback cannot decide.

Never convert “not yet attempted,” “needs one sign-in,” or “requires setup” into
“technically blocked.” Record exactly what was attempted and what evidence exists.

## Human handoffs to batch once

The implementation agent may pause only for an unavoidable human-only step it
cannot complete safely:

- password, passkey, MFA, CAPTCHA, or QR/account-link confirmation;
- one approved inbound reply-capable notification if no such notification is
  already present in the authenticated VM when Direct Reply is tested;
- consent screen whose requested scopes or identity differ from this plan;
- a provider suspension or account-verification screen;
- a Slack workspace/admin acceptance screen for the separate Android-only app;
- a planned OpenAI run that would put known usage above the approved
  `$500/month` ceiling, or any request to change the approved account or
  organization;
- any live Google Maps batch that cannot be proved zero-charge under the current
  project **Operator** billing usage and official per-SKU allowance; report the
  bounded four-call estimate and wait for explicit spending approval;
- paid ARM64/KVM test-host provisioning, but only if the measured Cuttlefish
  diagnostic trigger fires and no already-available host passes its gate;
- one batched secure-store unlock/approval if an approved migration-matrix
  record or Maps, YouTube, or OpenAI key cannot be read non-interactively; or
  one Android `FLAG_SECURE` entry only for a missing non-OAuth Maps, YouTube, or
  OpenAI key. OAuth records always use migration or provider consent, never
  manual secret entry.

Open every known consent surface in one setup session, explain the expected
identity beside it, and resume without asking the owner to repeat completed
sign-ins. Before opening any authorization page after that batch, save evidence
that the provider health check failed, name the exact terminal error (revoked,
expired, wrong scope, wrong identity), and reconnect only that provider. Never
ask for a password or token in chat. Never clear Play Services, browser, app,
Keystore, Termux, Beeper, or AVD data to fix code.

Publishing the approved static `assetlinks.json` is already cleared. Reuse the
current `tryoperator.net` deployment login; pause only if that session has truly
expired and the provider requires a password, passkey, MFA, or CAPTCHA. Do not
create a new site or backend.

The production-runtime decision is closed for this plan: use Termux + Debian.
The current Play Termux build's integrated boot receiver is the selected boot
mechanism; a separate Termux:Boot package is only considered if the installed
Termux build lacks that receiver and the signatures match.

## Final independent gate

**Plan-only gate result (2026-08-05): PASS.** Two independent final reviews found
no remaining material blocker or high-severity planning gap. This certifies the
handoff plan only; implementation is unstarted and no service is newly claimed
production-proven by this document.

A fresh judge must first define what a strong result for this exact plan requires,
then inspect the implementation, tests, real-account evidence, service states,
credential handling, Android-only architecture, and both parent-plan headers.
Every material finding is fixed and re-judged. The work is complete only after a
fresh pass finds no remaining material gaps.
