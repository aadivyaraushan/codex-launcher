# operator-tools (OpenClaw plugin)

OpenClaw plugin that exposes Operator capability adapters as agent tools by
calling the on-device phone-runtime bridge over pinned loopback TLS.

## Config (paths only)

Required fields (see `openclaw.plugin.json`):

| Field | Example shape |
|---|---|
| `bridgeUrl` | `https://127.0.0.1:9443` |
| `tokenPath` | path to the bridge bearer token file |
| `certPath` | path to the runtime TLS certificate PEM |

Never put token or key **values** in this repo or in committed config.

## Develop

```sh
npm test
npm run sync-tools   # refresh tools.json from a live bridge when intentional
```

Broader on-phone setup: [docs/setup/phone-agent.md](../../docs/setup/phone-agent.md).
