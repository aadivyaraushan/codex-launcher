# Relay box Fly deployment

**Date:** 2026-07-19

## Purpose

Deploy the single-tenant Codex Launcher relay described in
`planning/relay-box-build-plan.md`, then prove that a real phone protocol client
can pair and run a session through the deployed box while the box sees only
ciphertext.

## Approved billing configuration

- Authenticated Fly account: `ssdear@gmail.com`
- Fly organization: `personal`
- App: `codex-launcher-relay-ssdear`
- Region: `lax` (Los Angeles, US-West)
- Resources: one shared CPU, 256 MB machine; one dedicated IPv4; one 1 GB
  encrypted volume; high availability and autoscaling disabled
- Estimated cost approved by the user on 2026-07-19: **$4.20–$7/month**, plus
  unusual traffic

The estimate uses Fly's current listed prices for the smallest always-running
machine, a $2/month dedicated IPv4, a $0.15/GB-month volume, and normal outbound
traffic. The range allows for region pricing, traffic, and a possible memory
increase if live evidence shows 256 MB is insufficient.

## Security configuration

- Fly forwards raw TCP on public phone port 8443 and public Mac port 443; it
  terminates TLS on neither. Port 443 forwards to the box's internal port 9000.
- The box terminates its own pinned TLS only on the internal Mac door, port 9000.
- Phone-to-Mac TLS passes through the phone door, port 8443, untouched.
- `RELAYBOX_SECRET` is generated from secure random bytes and stored only in
  Fly secrets and the owner-only Mac companion configuration. It is never saved
  in this repository or this report.
- `/data/relaybox/box.pem` lives on the encrypted Fly volume so the box pin
  survives restarts.

## Reproduce and verify

## Deployed resources

- Host: `codex-launcher-relay-ssdear.fly.dev`
- Dedicated IPv4: `168.220.90.156`
- Dedicated IPv6: `2a09:8280:1::150:52e4:0`
- Machine: `d8d05eda5d3958`, one shared CPU, 256 MB, state `started`
- Deployment: `01KXVTKHWBRX1S22034N89AHVM`
- Encrypted volume: `vol_rkgw1j7jkmkxjk64`, 1 GB, attached in `lax`
- Secret state: `RELAYBOX_SECRET` is deployed; its value is not recorded here

`fly services list` on 2026-07-19 reported exactly:

```text
TCP  8443 => 8443  [PROXY_PROTO]  lax  1 machine
TCP   443 => 9000  []             lax  1 machine
```

The first allocated IPv4 (`137.66.47.4`) had a broken public route even though
Fly's internal proxy completed TLS to the machine. It was released and replaced
with `168.220.90.156`; the replacement passed the public TLS-pin and full tunnel
checks. This is deployment history, not an address the app should retain.

## Verification evidence

The final automated checks on 2026-07-19 were:

```sh
fly config validate --config fly.toml
go test ./... -race -count=1
go vet ./...
./android/gradlew -p android testDebugUnitTest lintDebug assembleDebug
./android/gradlew -p android assembleRelease
python3 release/checks/protocol/schema_test.py
node release/checks/public-alpha-release.test.mjs
```

Results: Fly config valid; the whole Go module race-clean; vet clean; Android
unit, lint, debug APK, and unsigned release APK builds green; 34 protocol frames
accepted and 38 invalid frames rejected; public release contract green.

The deployed stage-6b test requires the relay's single Mac slot to belong to the
harness. A first repeat while the installed companion was live failed safely
with `x509: certificate signed by unknown authority`: the installed companion
had reclaimed the slot, so the test phone rejected its unexpected certificate.
After briefly stopping that LaunchAgent, the exact deployed test passed:

```sh
RELAYBOX_LIVE_MAC_ADDR=codex-launcher-relay-ssdear.fly.dev:443 \
RELAYBOX_LIVE_PHONE_ADDR=codex-launcher-relay-ssdear.fly.dev:8443 \
RELAYBOX_LIVE_PIN="$(<.context/deploy/relaybox-pin)" \
RELAYBOX_LIVE_SECRET="$(<.context/deploy/relaybox-secret)" \
go test -race ./companion/internal/relaye2e -run 'TestLiveFly' -count=1
```

Result after the final hardening deploy: `ok`, 9.058 seconds. The test used the
real phone protocol, real pinned
TLS and pairing handshake, and a real companion session through the public Fly
route. The external test cannot record bytes inside Fly. Instead, it proves the
public phone route preserved the end-to-end TLS session by completing a
handshake pinned to the harness Mac's fresh key; Fly TLS termination or routing
to a different computer fails that pin. The separate local stage-6a test is the
one that records both phone-door directions and proves the marker and pairing
secret are absent from the box-visible bytes.

The LaunchAgent was then restored. `codex-launcher status` reported installed
and running, and `doctor` returned 7/7 checks green: Codex 0.144.1, relay pin,
service, phone door, schema, and identity all passed. The saved historical
`last_error` value remains `service_stopped_unexpectedly`, but the check is green
because the service is currently running.

The final relay adds a non-evicting `CHECK` command. The new doctor used it
against the live box and returned `box reachable, pinned key and registration
secret match` while the service retained its control slot. The phone door also
requires the opening TLS record bytes (`16 03`) before it asks the Mac for a
data line. A post-deploy pin check during Fly's rolling restart failed closed;
the same check passed after the machine reached `started`, confirming the
encrypted volume preserved the box identity.

The earlier post-deployment hardening artifact was built from commit
`a3be5661b6e448425d2b34673e3020061d16f97a`, with SHA-256
`b9d08189fab11ccbfee620f6433eabd882b8655dfd3b89d755cdb28416d3d9b4`.
It was the artifact used for the deployed stage-6b harness and the initial
physical pairing. The later physical run found the acknowledgement bug recorded
below, so this is deployment history rather than the final installed build.

## Physical-phone handoff

Completed on 2026-07-19 with physical device `4B230DLAQ001Z5`, a Pixel 9 running
Android 16:

1. Installed `android/app/build/outputs/apk/debug/app-debug.apk` over USB.
2. Generated a fresh one-time pairing link and transferred it directly into the
   phone field. Its value is not stored in this repository or report. The
   installed companion changed from zero to one paired device.
3. The phone completed pinned TLS, device proof, `hello`, snapshot, project
   selection, action results, and cumulative acknowledgements through the public
   Fly route.
4. The one approved project was selected.
5. After explicit approval, the phone started one real read-only Codex turn using
   the ChatGPT login `aadivya@fermi.ai`. `OPENAI_API_KEY` was not set, so the
   credential source was the existing ChatGPT login. Expected added charge was
   $0 under the existing plan, with normal plan usage consumed.
6. The phone transcript showed the original prompt and the reply
   `RELAY_BOX_PHONE_VERIFICATION_PASSED`. The model omitted the requested final
   period, but the complete prompt and response round trip succeeded.
7. Git status and the complete tracked diff had identical hashes immediately
   before and after the turn, proving the read-only task changed no repository
   files.

The physical run exposed two final bugs that automated harnesses had missed:

- A stale stored Tailscale pairing record correctly failed the new public-address
  rule, but the recovery screen offered no way to remove it. The screen now has
  **Remove local data**, wired to the same complete wipe used by normal unpairing.
  Its device UI test failed before the control existed and passed after the fix.
- The companion validated a phone acknowledgement without first recording the
  snapshot sequence it had sent. Every valid acknowledgement was rejected as an
  invalid text frame, causing a reconnect roughly every 40 seconds. The transport
  now records outgoing protocol frames under the same lock used for incoming
  frames and attachment state. The regression test failed because the valid
  acknowledgement never reached the handler, then passed with `-race`; the full
  companion race suite and `go vet` also passed. From the repaired companion's
  14:26 start through the completed task, its log contained 11 accepted phone
  acknowledgements and zero rejected transport frames.

Final operator checks after installing the repaired companion: `status` reported
one paired device and a running LaunchAgent; `doctor` passed all 7 checks. The
final reproducible companion was built from commit
`09cb1d9e4ae7009dea8adbcdd6cbb4379864630a`, with SHA-256
`52bfbb01011ad2eef2d9aea9ed77274d02adf23fde8242583fe9e865d5cbd07b`.
The adjacent provenance file names that exact source commit. After verified
replacement, the installed binary and rebuilt artifact had the same SHA-256,
the LaunchAgent was running, the paired-device count remained one, and doctor
again passed 7/7.
