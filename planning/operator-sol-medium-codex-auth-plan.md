# Operator Sol Medium and Codex Auth Plan

**Date:** 2026-08-24  
**Purpose:** Move the live phone agent to GPT-5.6 Sol with medium reasoning while keeping Codex/ChatGPT OAuth as its only OpenAI authentication route.

```text
Operator app
    |
    v
OpenClaw Gateway on Pixel
    |
    +-- model: openai/gpt-5.6-luna  --> openai/gpt-5.6-sol
    +-- thinking: max               --> medium
    +-- auth: Codex OAuth           --> unchanged
```

## Verified starting state

- Pixel `4B230DLAQ001Z5` is authorized over USB.
- OpenClaw Gateway PID `6191` and Operator runtime PID `6306` are running.
- `openclaw models status --json` reports default and resolved model `openai/gpt-5.6-luna`.
- `/root/.openclaw/openclaw.json` sets `thinkingDefault` to `max`.
- OpenClaw reports one usable OpenAI OAuth profile for `ssdear@gmail.com`, zero API-key profiles, and runtime `codex`.

## Done means

| Check | Expected result |
|---|---|
| Before-change assertion | Fails because the phone is Luna/max |
| Config after change | Primary model is `openai/gpt-5.6-sol`; default thinking is `medium` |
| Authentication | OpenAI OAuth remains usable; API-key count remains zero; runtime remains `codex` |
| Restart | Gateway restarts and listens on loopback port 18789 |
| Real turn | A new Operator turn records model `gpt-5.6-sol` and completes successfully |
| Regression | Operator runtime remains healthy on loopback port 9443 |
| Independent check | A fresh judge reviews the final config and live evidence against the request |

## Change

1. Make a timestamped backup of `/root/.openclaw/openclaw.json` on the phone.
2. Run the target-state assertion and record the expected failure.
3. Change the existing primary model and thinking setting in place; do not add a second behavior path.
4. Restart only the OpenClaw Gateway through its existing supervisor.
5. Re-run the assertions, inspect the auth route, and send one harmless phone-agent test turn.
6. Save the result and rollback command under `saved-results/`.
7. Have a separate judge check the evidence.

## Rollback

Restore the timestamped phone backup, restart the Gateway, and repeat the same status and health checks.
