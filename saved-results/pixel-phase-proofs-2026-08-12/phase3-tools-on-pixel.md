# Phase 3 — tools on Pixel (CLI)

- Serial: `4B230DLAQ001Z5`
- Command: `openclaw agent --session-key agent:main:main --json -m 'Do you have tools named discord and messages? Answer YES/NO and list those two if present.'`
- Timestamp (UTC): captured via agent run in evidence JSON
- Verdict: **PASS**

## Agent visible answer

```
YES — `discord`, `messages`.
```

## Criteria

PASS if answer is YES and both tool names `discord` and `messages` are listed.

## Artifacts

- `phase3-tools-agent.json` (full agent JSON)
- `bridge-health.json` (preflight `/v1/health`)
