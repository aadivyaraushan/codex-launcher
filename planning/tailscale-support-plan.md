# Restore Tailscale Support

**Status:** Approved — adversarial review PASS

**Updated:** 2026-07-19
**Purpose:** Restore Tailscale as the free, default connection path while keeping the existing Fly relay as an explicit paid option for people who need another VPN at the same time.

## System Shape

```text
                              explicit choice; never automatic fallback
                                              |
                         +--------------------+--------------------+
                         |                                         |
                 default: Tailscale                         optional: Fly relay
	                 eligible users can use                    user's own paid account
	                 Tailscale's Personal plan
                         |                                         |
Android app ---- pinned TLS ----> Mac companion     Android app ---- pinned TLS ----> relay box
                  100.64/10 or                         public DNS/IP          |
                  fd7a:115c:a1e0::/48                                        +----> Mac companion
                         |                                         |
                 Tailscale tunnel                         existing relay client
```

The phone protocol and its TLS and proof identities stay the same. Only the route to the Mac changes.

## Decisions Already Settled

- Tailscale is the default because eligible users can use its current Personal plan without payment and it has fewer billing steps.
- Fly remains available for users who need Codex Launcher while another VPN is active.
- The product never switches transports automatically.
- Choosing or changing the transport is explicit and requires re-pairing the phone. Existing Codex tasks remain intact.
- Tailscale setup uses a literal Tailscale IP, not MagicDNS, so Android can validate the route without weakening public-DNS protections.
- The existing pinned TLS identity, Ed25519 pairing proof, and encrypted app protocol remain unchanged.
- Pairing protocol version 1 and its URI fields remain unchanged. The URI carries the target, one-time secret, host-identity pin, and TLS-identity pin; the phone later signs a pairing request that binds the target and identities, which the companion verifies. The host is sufficient to classify the route, so no `transport` field is added.
- The supported launch target is macOS 15 on Apple silicon with the currently tested Android target. macOS Intel, Linux, and Windows continue to build but remain experimental until each has native setup and phone-to-computer evidence.
- Any Fly provisioning or paid verification requires separate approval naming the account that will be charged.

## Observable Definition of Done

Given a macOS 15 Apple-silicon Mac and supported Android phone signed into the same Tailscale network, the onboarding flow selects Tailscale by default, discovers and validates the Mac's Tailscale address, installs the companion, and produces a pairing offer that the physical phone can use to connect and run a Codex session without a Fly account.

Given an existing Fly-based installation, upgrading preserves the relay configuration and continues to connect through Fly with no silent transport change.

Given a user who explicitly selects Fly, setup explains that Fly needs the user's own account and card before any paid action, then uses the existing relay path.

In both modes, `doctor`, status output, logs, docs, and release checks identify the selected transport and provide mode-specific fixes.

## Current Evidence

- `companion/internal/app/config.go` requires a relay configuration and has no transport choice.
- `companion/internal/app/main.go` always creates a relay listener.
- `companion/internal/hostsetup` and `companion/internal/hostdoctor` are relay-specific.
- `android/app/src/main/kotlin/app/codexlauncher/connection/pairing/model/PairingValidation.kt` rejects Tailscale and other private addresses.
- `android/app/src/main/kotlin/app/codexlauncher/storage/pairing/PairingRecordStore.kt` repeats that public-only check when saving or loading a paired computer.
- `android/app/src/main/kotlin/app/codexlauncher/connection/security/SafePublicDns.kt` permits only public addresses, while pairing HTTP and session WebSockets create `PinnedTlsClientFactory` without a route-specific resolver.
- `release/checks/public-alpha-release.test.mjs` and `release/checks/companion-install-smoke.test.mjs` deliberately reject the old Tailscale setup.
- The parent of commit `2a0876b` contains the last direct-Tailscale listener, setup, doctor, and CLI behavior. Reuse its rules selectively; do not restore it wholesale over the relay security work.

## Configuration Contract

Move the companion config from an always-relay version 1 shape to an explicit version 2 connection choice:

```yaml
version: 2
computerName: My Mac
codexBinary: /path/to/codex
projects:
  - /approved/project
connection:
  mode: tailscale
  tailscale:
    host: 100.x.y.z
    port: 9443
```

```yaml
connection:
  mode: relay
  relay:
    boxHost: relay.example.com
    macPort: 8443
    phonePort: 443
    pinnedKey: ...
    secret: ...
```

Pure persisted-config validation rules:

- `mode` is required and is exactly `tailscale` or `relay`.
- Exactly the matching settings block is accepted; mixed or empty configurations fail with a clear message.
- A Tailscale host must be a canonical literal IPv4 address in `100.64.0.0/10` or IPv6 address in `fd7a:115c:a1e0::/48`; ports and common values must be structurally valid.
- A relay literal must pass the same public-address rules as Android. A relay hostname must pass the same label, length, and alphabet rules as Android's `PublicEndpoint` classifier. Ports, pinned relay key, and secret retain their current structural requirements.
- Secrets never appear in status or diagnostic logs.

`Config.Validate()` is pure: it performs no command execution, DNS, socket, daemon, sign-in, or local-address checks. `LoadConfig`, `doctor`, `status`, and recovery can therefore read a structurally valid config while Tailscale or the network is unavailable. Live ownership, DNS-publicness, port, and reachability checks belong to setup preflight, `serve`, and `doctor`; each reports a different error instead of turning an outage into “setup incomplete.”

### Upgrade and rollback

`install --replace NEW_ARTIFACT` continues executing inside the already-installed old process. Replacing the binary file does not replace that process's migration callback, so the upgrade must not depend on new callback code. The compatible sequence is:

```text
old binary process: install --replace NEW_ARTIFACT
  -> snapshot exact old binary + config
  -> activate verified new binary
  -> old callback validates its own version-1 config without rewriting
  -> start new service
       -> new binary loader strictly recognizes v2, relay-v1, or tailscale-v1
       -> normalize recognized legacy config to the v2 model in memory only
       -> serve and report healthy
  -> old process publishes old binary + original config as persistent rollback

new binary process: config migrate
  -> transactionally persist the already-proven in-memory v2 shape
```

The new loader returns a normalized `Config` plus `ConfigSource` (`v2`, `relay_v1`, or `tailscale_v1`) without writing. New setup always writes version 2. `status` exposes the non-secret source and `doctor` warns when an explicit persistence migration remains.

| Existing config | Version 2 result |
|---|---|
| Current version 1 with `relay` | `connection.mode: relay`; values preserved exactly only when they pass version-2 structural rules |
| Historical version 1 with `listenHost` and `listenPort` | `connection.mode: tailscale` only if the address passes the Tailscale rules |
| Unknown or mixed shape | Stop with recovery instructions; do not guess |

The only accepted legacy top-level key sets are `{version, computerName, projects, relay}` or that set plus optional `codexBinary`, and `{version, computerName, listenHost, listenPort, projects}` or that set plus optional `codexBinary`. Nested relay keys are exactly `{boxHost, macPort, phonePort, pinnedKey, secret}`. The new loader's raw decoders reject duplicate JSON keys, every other unknown/missing/mixed key set, unsafe file types/permissions, and invalid values. A recognized legacy config is converted and validated in memory but remains byte-for-byte unchanged during replacement.

A legacy relay host that the old Go validator accepted but the shared version-2 public route rules reject—such as loopback, LAN-private, metadata, malformed hostname, or reserved literal—does not normalize. The new service fails health, and the old process's existing replacement transaction restores its snapshot. The error tells the user to correct the relay address using the old version or perform an explicit fresh `setup relay`; upgrade never rewrites it or emits a pairing offer Android would reject. A syntactically valid hostname normalizes structurally, while new-binary start health/doctor must also prove that its resolved addresses are public.

After a healthy replacement, the bootstrap runs the new exact command `codex-launcher config migrate`. It re-reads and classifies the raw file; version 2 returns `already current` without consulting or changing service state. For a recognized v1 source, the command requires installed/running service status plus a current valid `StateRunning` health record before mutation; missing, malformed, failed, or stale health records refuse. Add `Manager.ReconfigureRunning`, which checks that condition inside the install mutation lock and returns `service_not_healthy` before calling the apply callback if it is false. It then snapshots the v1 bytes, stops the new service, atomically writes owner-only version 2, restarts/health-checks, and restores the v1 bytes/new service on any failure. Do not use `Manager.Reconfigure`, whose stopped-service branch applies without a transaction. This transaction does not consume, replace, or rewrite the persistent old-binary/old-config rollback published by replacement. Because effective mode, pairing target, host/TLS identities, durable pairing state, and task state do not change, config migration requires no revoke or re-pair.

Failure handling is fixed:

- New-service legacy decode, normalization, start, or health failure: the still-running old install process restores its transaction's exact old binary/config and restarts it.
- Replacement interruption after activation: the existing journal and transaction snapshot restore the old pair; no config write has occurred.
- Explicit `config migrate` with a stopped/unhealthy service: refuse before the write callback; leave v1 bytes and persistent rollback files untouched. Interruption/failure after a healthy start uses the reconfigure snapshot to restore v1 bytes while keeping the new binary/service.
- Successful replacement plus config migration: a later `rollback` restores the persistent old binary and its original version-1 config together.

Add `release/checks/legacy-upgrade-smoke.sh`, but run its native service portion only in a fresh macOS 15 arm64 VM snapshot under a dedicated unpaired test user. Before egress is disabled, prepare the VM image with pinned source archives, candidate artifact, Go/toolchain versions, and dependency caches; inject no credentials. The test then disables public egress and proves that state before running. The VM has no developer home, Keychain, launch agents, Tailscale account, Fly credentials, or route to production. The harness:

1. Builds the relay-v1 binary from `4071363e65c2c3821a53dc52f11efd468ff7988d`, historical Tailscale-v1 binary from `ae42c88f81ccf71da604905aea62e36b82b21d1c` (`2a0876b^`), and the candidate artifact.
2. Uses the dedicated user's real LaunchAgent backend and isolated home/config/state roots, so the fixed service label cannot collide with a developer installation.
3. Installs a local fake Codex executable that implements only the bounded version/app-server behavior required for health; it makes no model or network call.
4. Gives a no-egress VM interface a Tailscale-range fixture address for the historical direct listener. For relay, a local fake relay process on a separate no-egress interface uses a public-syntax fixture address, pinned test key, `CHECK`, and `REGISTER`. Product validation remains unchanged; packets cannot leave the VM.
5. Creates matching v1 config, companion identity, paired-device state, and Android-record fixtures; invokes each actual old binary as `install --replace NEW_ARTIFACT`; proves the new service starts while v1 bytes remain unchanged; runs `config migrate`; then exercises rollback and interruption recovery.
6. After every case unloads the LaunchAgent, kills fixture processes, removes the dedicated user's test root and interfaces, asserts no service/process/socket remains, and reverts the VM snapshot.

Hermetic Go tests with injected fake backends cover every fault branch on ordinary CI; only the isolated VM job may run native old-binary commands. It must refuse to start unless it proves the dedicated user, empty service label, no production credentials, and no public egress. Real Tailscale-tailnet and Fly upgrade checks are separate release evidence: name/approve the Tailscale test account first, and name the charged Fly account plus cost approval before any paid resource or call. Never point this harness at a developer's live companion, tailnet, relay, or config.

## Setup and Runtime Flow

```text
setup command
  |
  +-- `codex-launcher setup tailscale COMMON_FLAGS [--tailscale-ip IP] [--listen-port PORT]`
  |     +-- require Tailscale CLI and signed-in state
  |     +-- discover and validate one locally owned Tailscale literal
  |     +-- validate local ownership and port availability
  |     +-- write version 2 config, install/restart service, run doctor
  |
  +-- `codex-launcher setup relay COMMON_FLAGS RELAY_FLAGS`
        +-- accept already-provisioned relay values; this command never creates paid resources
        +-- collect existing relay values
        +-- write version 2 config, install/restart service, run doctor

serve
  |
  +-- tailscale -> bind pinned-TLS listener only to configured Tailscale address
  +-- relay     -> use existing authenticated relay listener
```

The exact public CLI is `setup tailscale` and `setup relay`; bare `setup` is a usage error. The product prompt and UI choose Tailscale by default but always invoke the explicit subcommand. Both modes require the current common computer, Codex binary, and repeated project flags. Tailscale accepts only `--tailscale-ip` and `--listen-port` (default `9443`); relay accepts only the existing `--box-host`, `--mac-port`, `--phone-port`, `--pinned-key`, and `--relay-secret`. Passing flags from the other mode fails before any write.

Tailscale discovery trims nonempty lines from `tailscale ip -4`. Exactly one canonical, locally owned address is selected; if there is no IPv4 result, the same rule is applied to `tailscale ip -6`. Empty, malformed, or multiple results fail without changing config and instruct the user to pass `--tailscale-ip`. An override must be a canonical address in the official Tailscale range and must exactly match one address returned by the local client; it cannot introduce an arbitrary private address.

`Config.PairingTarget()` is the only companion accessor for the phone-facing host, port, and protocol. It returns the configured Tailscale host/listen port or relay box host/phone port. `pair`, status, logs, and tests stop reading relay fields directly.

### Two-phase setup interface

Replace the write-only `cli.Setup func(context.Context, Config) error` with a coordinator that has two explicit operations:

```text
Preflight(ctx, candidate Config) -> {
  activeConfig: Config or absent,
  pairedDeviceIDs: []string,
  candidateTarget: PairingTarget
}

Apply(ctx, candidate Config) -> error
```

`Preflight` first runs pure `candidate.Validate()`, then loads the optional active config, service status, and device IDs. Extend the existing query-only `companion/internal/durablestore/inspection` path to return device IDs while retaining its safe-path, schema, identity, and read-only database checks; do not add a second inspector or construct the Codex runtime, listener, or pairing service. This wiring lives in `defaultLocalHostOperations`, so `setup` remains usable without `runWith` opening a configured runtime.

Only after that inspection does preflight run mode-specific no-write checks:

- Relay: public DNS, pinned TLS, and secret validation use the relay's existing non-mutating `CHECK <secret>` exchange. Preflight and apply revalidation are forbidden from sending `REGISTER`; only the newly started companion runtime registers after the old service has stopped. Tests hold an existing control line open throughout preflight and prove it is neither evicted nor replaced.
- Tailscale, changed target: verify CLI/daemon/sign-in and exact local ownership, then temporarily bind the different candidate address/port and close it.
- Tailscale, unchanged target with a healthy running service: do not bind. Verify the existing listener through service status and the local pinned-TLS health probe.
- Tailscale, unchanged target with no healthy service: temporarily bind and close the candidate to distinguish a free port from an unrelated owner.

Common Codex/project checks remain non-mutating. None of these branches stops/restarts the service, writes config/state, registers a relay control line, creates a pairing offer, or changes the active listener.

The CLI compares mode and `PairingTarget` using the returned state. A changed target with paired devices returns the revoke instructions before `Apply`, service stop, or config write. `Apply` re-runs every safe no-write check, again using relay `CHECK` and the target/service-aware Tailscale rule, then calls the existing `Manager.Reconfigure` transaction. Its post-stop callback performs the final Tailscale bind-availability check before `hostsetup.SaveValidated` writes the candidate. If that check or write fails, `Manager.Reconfigure` restores the old config and service. Relay `REGISTER` still occurs only when the replacement service starts. Unit tests use separate spies for `Preflight` and `Apply` and assert that every refusal calls `Apply` zero times. Setup of a fresh machine treats absent config/state as unpaired; an unsafe/unreadable existing state fails closed rather than assuming zero devices.

`doctor` checks common service, identity, config, and Codex requirements first, then:

- Tailscale: CLI installed, daemon reachable, signed in, configured address locally owned, shields-up state reported, listener bound to only that address, port/firewall readiness, and a local TLS probe.
- Relay: existing relay key, secret, public endpoint, and relay connection checks.

Status and structured logs include `connection_mode`, check name, and failure class, but never relay secrets, pairing secrets, project contents, or a newly invented computer ID. Status continues to expose the existing configured computer name.

Local `doctor` explicitly reports its proof boundary: it cannot prove the Android Tailscale app is connected, that both devices are in the same Tailscale network, or that tailnet access rules and the host firewall permit the phone. Tailscale setup docs and the Android connection error screen therefore require/checklist: install and sign in on both devices, confirm the same tailnet, keep shields-up disabled on the Mac, permit the companion through the host firewall, and confirm the phone can reach the configured Tailscale IP. Only the physical-phone test closes those items; a local self-dial never claims end-to-end success.

### Fly billing boundary

`setup relay` is local configuration and never authenticates to Fly or creates resources. The separate bootstrap/manual Fly deployment step must, before `fly launch`, `fly deploy`, IP allocation, volume creation, or any paid test: identify the literal Fly account that will be charged, verify current pricing from Fly's official pricing pages, show a dated estimate, and obtain explicit approval. It then passes the resulting values to `setup relay`. No account identity or approval means no provisioning command.

Tailscale documentation says that eligible users can use its current Personal plan at no charge and links to current official terms; it does not promise that every person or organization qualifies forever.

## Android Route Policy

Keep pairing protocol version 1 and the exact pairing-URI fields unchanged; the later signed pairing request continues to bind the target and identities. Add one `EndpointRoute.classify(host)` API in the existing connection security/pairing area and use it everywhere a route enters or leaves storage:

```text
PairingOffer.parse ----------+
PairingRecordStore save/load +----> EndpointRoute.classify(host)
PinnedPairingTransport ------+             |
CompanionSessionClient ------+             +-- PublicEndpoint
                                            +-- TailscaleLiteral(parsed address)
                                            +-- reject
```

- Literal Tailscale IPv4/IPv6: allow only the two official Tailscale ranges. `PinnedTlsClientFactory.builder(pin, route)` uses a literal-only resolver that returns the already-parsed address only when OkHttp requests the exact configured host; it never calls `Dns.SYSTEM`.
- Relay hostname or public literal IP: keep `SafePublicDns` and its DNS-rebinding protections.
- Loopback, link-local, LAN-private, cloud metadata, malformed, and ambiguous addresses: reject.

`PairingOffer.parse`, `PairingRecordStore.isValid`, pairing HTTP, and session WebSocket creation all classify the same host and pass the resulting route to the TLS factory. Go and Kotlin run the same checked-in endpoint vectors so version-2 relay/Tailscale configs and Android acceptance cannot drift. Tests fail if any caller uses the default public resolver for a Tailscale route.

Network failures carry the already-classified route kind to the UI. `PairingViewModel` replaces its hardcoded relay message with “Couldn't reach your computer over Tailscale…” or “Couldn't reach the relay securely…” as appropriate. `ConnectionStateMachine` replaces `BOX_UNREACHABLE`/`BoxUnreachable` with a generic endpoint-unreachable event that retains `TAILSCALE` or `RELAY`. `HomeScreen` shows the same-tailnet/Tailscale-app/access-rule/firewall checklist for Tailscale and the existing box/companion checklist for relay. Invalid stored routes still fail closed before a connection attempt. Error copy says what to check without claiming the app can distinguish every VPN, ACL, or firewall cause.

## Transport Change and Re-pairing

There is no remote route update and no automatic switch. `setup <mode>` validates the complete candidate first, then compares `Config.PairingTarget()` and mode with the active config. If either changes while a device is paired, setup refuses before stopping the service or writing config and prints the exact `devices` and `revoke DEVICE_ID` sequence.

The supported order is:

1. Run the new `setup <mode> ...` once; it proves the candidate is locally valid, then refuses because a phone is paired.
2. Run `devices`, explicitly `revoke DEVICE_ID`, and confirm the phone will need a new pairing code.
3. Re-run the identical setup command. `Manager.Reconfigure` snapshots the old config, stops the service, writes the new config atomically, restarts, and restores the old config/service on failure.
4. Run mode-specific `doctor`; only after it passes, run `pair` to create a code containing the new `Config.PairingTarget()`.
5. On Android, explicitly remove/replace the old pairing record during the new pairing flow. Task history remains because route/pairing state and task storage are separate.

If step 3 fails after revocation, the old config remains active and the user can run `pair` on the old route to recover. If step 4 fails, setup can be rerun with the old mode because there is no paired device. No step leaves two active listeners or teaches the phone a route outside a one-time pairing URI whose target and identities are then bound by the phone's signed request.

## Platform Support Boundary

| Platform | Release state for this plan | Required work/evidence |
|---|---|---|
| macOS 15 arm64 | Supported launch target | Tailscale CLI/daemon/sign-in discovery, LaunchAgent restart, macOS firewall guidance, clean-machine and physical-phone evidence |
| macOS amd64 | Experimental | Same automated adapter/install coverage and artifact smoke; native physical evidence required before promotion |
| Linux amd64/arm64 | Experimental | CLI/daemon discovery, systemd-user lifecycle, common firewall guidance, artifact smoke; native phone-to-host evidence required before promotion |
| Windows amd64/arm64 | Experimental | CLI/daemon discovery, scheduled-task lifecycle, Windows Defender Firewall guidance, artifact smoke; native phone-to-host evidence required before promotion |

All six archives must compile and expose both setup subcommands, but the landing page and copied prompt support only the exact first-launch evidence matrix: macOS 15.5 arm64 with standalone `codex-cli 0.144.1` or the separately checked ChatGPT Desktop `26.707.51957` adapter, plus Pixel 9 on Android 16. Experimental platforms or later unverified Codex/Desktop versions display a warning and link to manual setup; they are never described as verified. Platform adapters use an injected command runner in tests and never silently install Tailscale or change firewall rules.

## Planned File Work

Exact names may shift if the existing package layout provides a closer home; do not create generic `utils` or `helpers` folders.

| Area | Existing files to change | Purpose |
|---|---|---|
| Config | `companion/internal/app/config.go` and tests | Version 2 tagged connection config and strict validation |
| Legacy loading/migration | `companion/internal/app/config.go`, CLI/config command wiring, hostinstall tests, `release/checks/legacy-upgrade-smoke.sh` | New binary normalizes both strict v1 shapes in memory; explicit new-binary transaction persists v2; real old-process upgrades and rollback |
| CLI/setup | current companion command and `companion/internal/hostsetup/` | Explicit mode commands, Tailscale discovery, mode-specific setup |
| Setup state inspection | `companion/internal/cli/root.go`, `companion/cmd/codex-launcher/main.go`, `companion/internal/durablestore/inspection/` and tests | Extend the existing query-only inspection; separate no-write preflight from transactional apply |
| Runtime | `companion/internal/app/main.go` and focused listener package | Direct listener for Tailscale; retain relay listener |
| Doctor/status | `companion/internal/hostdoctor/` and status response code | Mode-aware checks and visible connection mode |
| Pairing target | `companion/internal/app/config.go`, `companion/internal/cli/commands.go`, pairing tests | One mode-aware accessor used by `pair` and status |
| Android route | `PairingValidation.kt`, `PairingOffer.kt`, `PairingRecordStore.kt`, and tests | One classifier for parsed and persisted public/Tailscale routes |
| Android network | `SafePublicDns.kt`, `PinnedTlsClientFactory.kt`, `PinnedPairingTransport.kt`, `CompanionSessionClient.kt`, and tests | Route-specific resolution for pairing HTTP and session WebSockets |
| Android recovery UI | `PairingViewModel.kt`, `ConnectionStateMachine.kt`, `HomeScreen.kt`, and tests | Carry route kind into pairing/reconnect errors and show correct Tailscale/relay help |
| Release/docs | `release/checks/`, release smoke scripts, `README.md`, `docs/compatibility/codex.md`, security docs | Ship both modes and remove relay-only assumptions |

Before editing, search every use of the version number, relay config, listener factory, setup flags, host validation, `SafePublicDns`, pairing host, doctor output, and Tailscale wording. Classify every match as changed, intentionally relay-only, or unrelated.

## TDD Execution Order

1. Create an isolated git worktree from the current branch and record its path.
2. Write observable tests first and run them red:
   - config v2 performs only pure structural checks, uses shared endpoint vectors, and still loads for doctor when Tailscale/DNS is unavailable;
   - the new loader strictly normalizes relay-v1 and Tailscale-v1 in memory without writing, while new setup writes only v2;
   - both real old binaries can execute `install --replace NEW_ARTIFACT`, reach healthy new service with byte-identical v1 config, then the new `config migrate` transaction persists v2;
   - replacement/migration interruption and persistent rollback restore the matching byte-exact old config and binary;
   - `config migrate` on v2 is a no-op even when stopped; on legacy v1 it refuses stopped/unhealthy service before apply, succeeds only through `ReconfigureRunning`, restores after interruption, and leaves all persistent rollback file hashes unchanged on every path;
   - the legacy native-smoke guard refuses non-VM/developer homes, existing service labels, public egress, or production/Tailscale/Fly credentials, and teardown asserts no fixture remains;
   - the two exact setup subcommands reject cross-mode flags; Tailscale discovery handles zero, one, multiple, malformed, IPv4, IPv6, and verified override results;
   - runtime chooses only the configured listener and never falls back;
   - setup preflight performs no writes, obtains active config and paired IDs without opening Codex runtime, and refuses a route/mode change while paired before apply;
   - relay preflight/revalidation use `CHECK`, never `REGISTER`, and leave an existing registered control line connected;
   - Tailscale preflight verifies an unchanged healthy listener without rebinding, probes changed/stopped targets, and performs the final bind check only inside the stopped-service transaction;
   - doctor runs the correct checks for each mode and does not overclaim phone reachability;
   - one Android classifier accepts only literal Tailscale ranges as the private route at parse, store, pairing HTTP, and WebSocket boundaries;
   - Tailscale routes never invoke system/public DNS while relay routes retain one-shot public-DNS filtering;
   - pairing and reconnect UI receives route kind and contains no incorrect relay-only help on Tailscale failures;
   - released companion archives expose both explicit setup modes;
   - an existing Fly pairing still connects after upgrade;
   - a historical Tailscale config, companion identity, paired-device state, Android record, and unchanged route continue without re-pairing.
3. Implement the smallest config, setup, runtime, doctor, and Android changes that make those tests pass.
4. Add integration tests for installer migration/rollback, service restart, pairing, reconnect, and transport changes.
5. Re-run the focused tests, then the complete Go, Android, release-contract, and install-smoke suites.
6. Search for sibling relay-only assumptions and document why each remaining one is valid.
7. Perform the real-device and real-network verification below.

## Verification Matrix

| Scenario | Required evidence |
|---|---|
| Fresh default setup | Clean macOS 15 arm64 Mac; product chooses explicit `setup tailscale`; doctor passes without Fly prompts |
| Physical-phone Tailscale flow | Both devices signed into same tailnet; install verified APK, pair, Android shows online, and list sessions/empty state without a model call |
| Current Fly upgrade | Actual relay-v1 binary replaces itself; new service runs from in-memory normalization; explicit migration persists v2; existing phone reconnects through Fly; rollback restores relay-v1 pair |
| Historical Tailscale upgrade | Actual pre-`2a0876b` binary replaces itself; host/port, companion identity, paired-device state, and Android record continue through new service with no re-pair; explicit migration and rollback both pass |
| Explicit Fly setup | External deployment identifies the paying account and current dated estimate before account/card/provisioning; local `setup relay` makes no paid calls |
| Another VPN conflict | Product warning is shown; test at least one available conflicting-VPN case without claiming universal coverage |
| Tailscale disconnect/reconnect | Clear offline reason; recovery after reconnect; no fallback to Fly |
| Transport switch | Candidate validates, paired switch refuses, explicit revoke succeeds, atomic reconfigure and new pairing work; failure recovery returns to old route; task history retained |
| Security regression | Invalid ranges, mixed configs, DNS rebinding, changed pins, wrong pairing proof, and leaked secrets are rejected |
| Platform boundary | Native macOS 15 arm64 clean setup passes; six companion artifacts build/smoke; all other platforms remain visibly experimental |
| Release artifacts | Six companion targets and Android artifacts build; checksums/attestations and install smoke pass |

Use the real UI on a physical phone and the live companion wherever available. Automated tests alone do not close the plan.

Starting, resuming, or interrupting a real Codex task is optional follow-on evidence because it may consume the user's authenticated model allowance. Before that check, name the active credential/account and obtain separate approval; pairing, online state, and session listing are the required no-model-call completion evidence.

## Failure and Recovery Rules

- Missing or signed-out Tailscale: stop before changing the service; give install/sign-in steps.
- Address changes: doctor reports the mismatch and setup can explicitly refresh the route; never bind broadly to `0.0.0.0` or `::`.
- Port occupied: report the owning process when the operating system permits; leave the old service/config working.
- Replacement failure/interruption: the executing old process uses its existing journal snapshot to restore the byte-exact old config and matching old binary. Explicit config-migration failure restores legacy bytes under the new binary; it never strands a v2 file under the old binary.
- Fly unavailable: report relay failure; do not move the user to Tailscale automatically.
- Tailscale unavailable while another VPN is active: explain the conflict and offer the explicit paid-relay setup path.

## Out of Scope

- Building a new relay service or changing Fly pricing.
- Automatically creating or charging a Fly account.
- Supporting arbitrary private LAN or third-party VPN addresses.
- Replacing the existing application protocol, TLS pins, pairing proof, or task storage.
- Claiming compatibility with every VPN product without direct evidence.

## Adversarial Review Gate

The plan is not approved until an independent reviewer first defines the bar for a safe, low-friction two-transport launch and returns `PASS` with no unresolved critical or major findings. Each review round and resulting revision will be recorded here.

### Review history

- Round 1 — **FAIL**: no critical findings; major gaps in Android route coverage, installer migration/rollback ordering, frozen CLI/protocol contracts, safe transport switching, platform scope, and phone-side Tailscale readiness. Revision 2 addresses each item and clarifies Fly's paid boundary and address discovery.
- Round 2 — **FAIL**: no critical findings; remaining major gaps in pure-versus-live config validation, no-write access to active pairing state, Go/Android relay-host agreement, and route-aware Android recovery copy. Revision 3 freezes those interfaces and resolves the legacy-key, logging-identifier, and free-plan wording details.
- Round 3 — **FAIL**: no critical findings; remaining major hazards were relay preflight evicting the active control line and Tailscale port probing colliding with the active companion. Revision 4 requires non-mutating relay `CHECK`, target/service-aware probing, post-stop final bind validation, and reuse of the existing query-only inspector.
- Round 4 — **FAIL**: one critical flaw showed that an old process cannot execute a newly installed migration callback, plus missing historical Tailscale continuity evidence. Revision 5 makes new-binary loading backward-compatible without replacement-time writes, adds an explicit new-binary persistence transaction, and tests both actual old-binary replacement paths and pairing continuity.
- Round 5 — **FAIL**: no critical findings; remaining major gaps were stopped-service migration safety and isolation of real old-binary/native-network smoke. Revision 6 adds a running-and-healthy-only transaction and a guarded, no-egress VM harness with local fixtures and destructive teardown checks.
- Round 6 — **PASS**: zero critical and zero major findings. The reviewer confirmed actual old-process replacement, in-memory legacy loading, safe explicit persistence, persistent rollback, and isolated native verification are executable. Final minor notes on URI wording, preloaded no-egress inputs, and exact health-state refusal were resolved editorially.

## GSTACK REVIEW REPORT

**Final verdict:** PASS

**Adversarial rounds:** 6

**Unresolved critical findings:** 0
**Unresolved major findings:** 0

The independent reviewer derived its standard before inspecting the plan and verified the default/paid boundary, config and CLI contracts, real legacy upgrades, fail-closed Android routing, transport switching, platform claims, billing gates, and real-device evidence.
