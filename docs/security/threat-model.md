# Security and privacy model

Codex Launcher is a private bridge between one Android phone and one user-owned
computer through a small relay box controlled by that user. It does not run a
maintainer-owned project cloud service, use Firebase, or ask the project
maintainers to hold ChatGPT, Codex, or relay credentials.

## What is protected

- ChatGPT/Codex authentication remains in the computer's existing Codex or
  ChatGPT installation.
- Pairing uses a one-time secret, a phone signing key, a pinned Ed25519 computer
  proof identity, a separate pinned P-256 TLS identity, and TLS. The TLS key is
  derived from the saved computer identity so both pins stay stable across
  restarts. Later requests must be signed by the paired phone.
- The companion opens only outbound, box-pinned TLS connections. It never opens
  a public listener or falls back to a direct local listener when the relay is
  unavailable.
- The phone accepts only a public relay address and checks resolved addresses
  again at connection time. Loopback, private, link-local, metadata, CGNAT, and
  unsafe IPv6 ranges fail closed.
- Project choices are server-approved opaque IDs. The phone does not choose an
  arbitrary filesystem path.
- Approvals, questions, interrupts, and other actions bind to the exact pending
  request and durable action record. Missing, expired, duplicated, or replayed
  actions fail closed.
- Config, identity keys, transaction snapshots, health state, attachments, and
  service markers use private local storage and bounded file reads.
- Attachments use Android system pickers, size limits, SHA-256 checks, private
  temporary files, and cleanup after cancel, failure, or expiry.

## Trust boundaries

```text
Android app
  signed action + pinned end-to-end TLS
        ↓
Untrusted network + untrusted relay box
  forwards sealed bytes; cannot open phone content
        ↓
Companion paired protocol
  validates device, project, action, size, and sequence
        ↓
Local Codex adapter
  public app-server or verified same-user Desktop bridge
        ↓
Codex and the approved project folder on the computer
```

The phone-to-computer TLS session is sealed end to end. The relay sees source
addresses, connection timing, connection length, and byte counts, but not
pairing messages, prompts, responses, actions, attachments, or computer identity
keys. The Mac-to-box control and data connections have a separate pinned TLS
layer; the relay opens that outer layer only to route a short-lived random token,
while the inner phone-to-computer TLS remains sealed.

The raw ChatGPT Desktop socket and raw Codex app-server protocol are never
forwarded to the phone or relay. The companion maps them to a smaller mobile
contract.

## Sensitive data on each device

The phone stores its pairing key, pinned computer proof and TLS identities,
selected project ID, encrypted unfinished draft, task/action state needed for
recovery, and files selected for an in-progress upload. The computer stores its identity key,
paired-device record, approved folder list, event/action journal, and temporary
attachment data. Codex and ChatGPT keep their own task and authentication data.

Logs use feature tags, IDs, counts, state names, and fixed error classes. They
must not contain pairing links, private keys, auth tokens, prompt/response text,
attachment contents, local project paths, or raw Desktop frames.

## What this design does not protect against

- Malware or an attacker already running as the same operating-system user can
  read or control data that user can access.
- A rooted phone, compromised Android keystore, stolen relay registration
  secret, compromised relay account, or compromised computer weakens or defeats
  the corresponding trust boundary. A compromised relay can deny service and
  observe traffic shape, but cannot decrypt phone content without also
  compromising an endpoint.
- The unsupported private ChatGPT Desktop interface can change without notice.
  Compatibility checks reduce accidental misuse but cannot turn it into a
  supported API.
- The public phone door can be flooded. The relay limits connection attempts,
  caps pending phones, requires an immediate TLS preface, and runs one fixed
  machine so load cannot create surprise machines. These controls reduce denial
  of service; they do not make a public port invisible.
- Codex can modify files and run commands within its own permissions. The phone
  interface does not make those actions harmless; inspect approvals carefully.

## Release and update boundary

Android APK/AAB files are signed with a key supplied to the release workflow;
the key is not stored in this repository. Desktop archives are unsigned
technical alpha artifacts until Apple notarization and Windows code-signing
accounts and costs are explicitly approved. Every release includes SHA-256
checksums, SPDX SBOMs, dependency notices, and GitHub artifact attestations.

There is no network self-updater in V1. A local replacement is accepted only
when the executable bytes match adjacent checksum and provenance files. The
installer retains one complete binary/config rollback and uses a private,
retryable transaction journal for interrupted maintenance.

## Report a problem

Do not include pairing links, tokens, private keys, prompts, task transcripts,
project paths, or attachments in a public report. Reproduce with fixed error
codes from `./codex-launcher doctor` on macOS/Linux or
`.\codex-launcher.exe doctor` on Windows PowerShell, and redact usernames or
local paths.
