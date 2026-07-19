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

The final Mac companion artifact was built from commit
`a3be5661b6e448425d2b34673e3020061d16f97a`, with SHA-256
`b9d08189fab11ccbfee620f6433eabd882b8655dfd3b89d755cdb28416d3d9b4`.
The verified replacement installer completed, the LaunchAgent returned to
`running`, and the installed command's new secret-aware doctor passed 7/7.

## Physical-phone handoff

The old Pixel 9 pairing was revoked as required for the address migration. The
debug APK is at `android/app/build/outputs/apk/debug/app-debug.apk`. At the final
check, `adb devices -l` listed no device, so the only unfinished plan step is to
connect and unlock the Pixel with USB debugging enabled, install this APK,
generate a fresh five-minute pairing link, pair once, and run a real task.
