# Operator (OpenClaw phone agent)

**A persistent OpenClaw agent that runs on your phone**, with this repo’s Go
capability adapters as tools and the Android launcher as the chat / approval UI.

The brain lives in Termux + proot Debian on the device. Approvals and
conversation stay on the home screen. Optional Codex-on-your-computer remains
available as a second lane through the companion + relay path.

## Why

Phones already hold the apps, accounts, and attention. Operator puts the agent
**next to those apps** instead of parking it on a laptop that must stay awake:

- **Phone brain** — OpenClaw gateway + agent session on-device
- **Tool adapters** — the same Go capability inventory, reached over a loopback
  bridge (`/v1/agent-tools`)
- **Chat-first approvals** — previews and gates surface in the launcher thread,
  not as a separate “capability sheet” product

Sensitive material stays as **file paths in config** (bridge token, TLS cert),
never as committed values. Logs use ids, not message text or people’s names.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Android phone                                              │
│                                                             │
│  ┌──────────────────┐     chat / approvals                  │
│  │  Launcher (UI)   │◄──────────────────────────────────┐   │
│  └────────┬─────────┘                                   │   │
│           │ device work / turns                         │   │
│  ┌────────▼──────────────────────────────────────────┐  │   │
│  │  Termux + proot Debian                            │  │   │
│  │  ┌─────────────┐   loopback TLS :9443             │  │   │
│  │  │ OpenClaw    │──► operator-phone-runtime        │  │   │
│  │  │ + plugin    │    /v1/agent-tools + gates       │──┘   │
│  │  └─────────────┘    (adapters → apps / brokers)   │      │
│  │         ▲                                         │      │
│  │         │ boot / watchdog (scripts/phone-boot)    │      │
│  └─────────┴─────────────────────────────────────────┘      │
└─────────────────────────────────────────────────────────────┘

Optional second lane (unchanged):
  Launcher ──sealed TLS──► relay box ──► computer companion (Codex)
```

| Piece | Role |
|---|---|
| OpenClaw on phone | Persistent agent / gateway (e.g. `:18789`) |
| `operator-tools` plugin | Registers adapters as agent tools; reads token/cert **paths** |
| Phone runtime bridge | Loopback HTTPS `:9443` — list/call tools, hard gates |
| Android launcher | Chat-first UI and approval surface |
| `scripts/phone-boot/` | Bring gateway + bridge back after reboot / Termux death |

## Start here

**Phone agent (primary path)**

- [OpenClaw on-phone setup](docs/setup/phone-agent.md)
- [Boot persistence](scripts/phone-boot/README.md) — Termux:Boot, watchdog, status
- [OpenClaw plugin](agentbridge/openclaw-plugin/) — `operator-tools` source
- [Android launcher install / pair](docs/setup/android.md)

**Computer Codex lane (still supported)**

- [Computer companion](docs/setup/companion.md)
- [Fly relay box](docs/setup/relay-box.md)
- [Security model](docs/security/threat-model.md)
- [Codex compatibility](docs/compatibility/codex.md)

Prefer a **Pixel-like AVD** (API 36 / Android 16 when available) for device
proofs. This README does not claim AVD screenshot proofs that were not run in
the packaging environment.

## Project status

| Phase | What | State |
|---|---|---|
| 1 | OpenClaw alive in phone Debian | **Done** |
| 2 | Tool bridge (`/v1/agent-tools`) | **Done** |
| 3–7 | Plugin, gates, turn proxy, chat UI, triggers | **Mac-side done**; on-device / AVD proofs still open |
| 8 | Delete predetermined-function pipeline | **Done** |
| 9 | Boot persistence (`scripts/phone-boot/`) | **Code-complete**; AVD reboot + WebChat UI proof open |
| 10 | Open-source packaging + README | **This PR** |
| 11 | End-to-end judged proof + publish push | **Open** |

## Build checks

From the repo root (use `-p 1` for a stable full companion matrix):

```bash
go test -count=1 -p 1 ./companion/...
bash scripts/phone-boot/test/run-tests.sh
```

Android (from `android/`):

```bash
./gradlew :app:testDebugUnitTest
./gradlew :app:compileDebugAndroidTestKotlin
```

Plugin package (optional, from `agentbridge/openclaw-plugin/`):

```bash
npm test
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the short contributor loop.

## License

Apache License 2.0. See [LICENSE](LICENSE), [NOTICE](NOTICE), and
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
