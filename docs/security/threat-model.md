# Security and privacy model

Codex Launcher is a direct, private bridge between one Android phone and one
user-owned computer. It does not run a project cloud service, use Firebase, or
ask the project maintainers to hold ChatGPT, Codex, or Tailscale credentials.

## What is protected

- ChatGPT/Codex authentication remains in the computer's existing Codex or
  ChatGPT installation.
- Pairing uses a one-time secret, a phone signing key, a pinned computer
  identity, and TLS. Later requests must be signed by the paired phone.
- The companion listens only on an address Tailscale confirms belongs to the
  computer. It rejects public, loopback, and arbitrary LAN bind addresses.
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
  signed action + pinned TLS
        ↓
Tailscale encrypted network
        ↓
Companion paired protocol
  validates device, project, action, size, and sequence
        ↓
Local Codex adapter
  public app-server or verified same-user Desktop bridge
        ↓
Codex and the approved project folder on the computer
```

The raw ChatGPT Desktop socket and raw Codex app-server protocol are never
forwarded to the phone. The companion maps them to a smaller mobile contract.

## Sensitive data on each device

The phone stores its pairing key, pinned computer identity, selected project
ID, encrypted unfinished draft, task/action state needed for recovery, and files
selected for an in-progress upload. The computer stores its identity key,
paired-device record, approved folder list, event/action journal, and temporary
attachment data. Codex and ChatGPT keep their own task and authentication data.

Logs use feature tags, IDs, counts, state names, and fixed error classes. They
must not contain pairing links, private keys, auth tokens, prompt/response text,
attachment contents, local project paths, or raw Desktop frames.

## What this design does not protect against

- Malware or an attacker already running as the same operating-system user can
  read or control data that user can access.
- A rooted phone, compromised Android keystore, compromised Tailscale account,
  or compromised computer can defeat the local trust boundary.
- The unsupported private ChatGPT Desktop interface can change without notice.
  Compatibility checks reduce accidental misuse but cannot turn it into a
  supported API.
- Tailscale controls network membership. Review and remove lost devices in the
  Tailscale admin console as well as revoking them in Codex Launcher.
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
