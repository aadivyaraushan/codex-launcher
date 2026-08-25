# Operator Sol Medium and Codex Auth

**Date:** 2026-08-24  
**Purpose:** Record the live Pixel change from Luna/max to GPT-5.6 Sol/medium and verify that Operator is fueled by Codex/ChatGPT OAuth rather than an API key.

## Result

The live phone configuration is:

```json
{
  "model": {
    "primary": "openai/gpt-5.6-sol"
  },
  "thinkingDefault": "medium"
}
```

OpenClaw reports:

- OpenAI runtime: `codex`
- Usable OAuth profiles: `1`
- API-key profiles: `0`
- OAuth account: `ssdear@gmail.com`
- Default and resolved model: `openai/gpt-5.6-sol`

## Test evidence

The pre-change target assertion failed as required:

```text
expected model=openai/gpt-5.6-sol, actual model=openai/gpt-5.6-luna
expected thinking=medium, actual thinking=max
AssertionError: model mismatch: openai/gpt-5.6-luna
```

After the change and service recovery, the same checks passed:

```text
PASS config model=openai/gpt-5.6-sol thinking=medium
PASS auth runtime=codex oauth=1 apiKey=0 status=usable account=openai:ssdear@gmail.com (ssdear@gmail.com)
PASS gateway startup logged Sol/medium and ready
PASS loopback port 18789 accepts connections
PASS loopback port 9443 accepts connections
PASS loopback port 23373 accepts connections
```

All three supervised services were running:

```text
openclaw-gateway pid 17078
phone-runtime pid 17080
beeper-server pid 17075
```

The Gateway startup log recorded:

```text
agent model: openai/gpt-5.6-sol (thinking=medium, fast=off)
http server listening
ready
```

## Real phone turn

A prompt was entered through the visible Operator Android screen. Android's fast text input dropped two characters and a space, producing `Relyexactly SOL MEDIUM OAUTH OK`; the model still returned the requested exact text.

```text
session=aa310aee-dd10-4264-a246-81be89e472b8
model=gpt-5.6-sol
provider=openai
stop_reason=stop
session_record_runtime_ms=22020
trajectory_event_span_ms=8775
reply=SOL MEDIUM OAUTH OK
```

The two time fields come from different OpenClaw records and measure different windows: `session_record_runtime_ms` is the session summary field, while `trajectory_event_span_ms` is the difference between the first and last stored trajectory timestamps. This single turn proves the new live route works. It is not a before/after benchmark, so neither number by itself proves a latency reduction.

## Restart incident and recovery

OpenClaw's automatic reload crashed the old Gateway process with a Node `ResetStdio` assertion and left a zombie process. Its per-service runit supervisor then failed to reap the process. The existing outer watchdog could not restart because orphaned service supervisors still held its lock.

Recovery used the existing service layout:

1. Stop the dead/orphaned OpenClaw, phone-runtime, and Beeper service supervisors.
2. Start the installed `operator-runtime-watchdog` again.
3. Wait for the existing `/etc/operator/services` definitions to restore all three services.
4. Re-run config, authentication, process, port, startup-log, and real-turn checks.

No credentials, service definitions, binaries, or repository source files were changed during recovery.

## Backup and rollback

The pre-change phone config remains at:

```text
/root/.openclaw/openclaw.json.backup-sol-medium-20260824T230840Z
```

To roll back, restore that file to `/root/.openclaw/openclaw.json`, then restart the installed Operator watchdog and repeat the verification checks above. Because the live reload crashed once during this change, do not call a rollback complete until the Gateway log says `ready` and all three loopback ports accept connections.

## Reuse

For future checks, treat `openclaw models status --json`, the Gateway startup line, and a fresh trajectory as separate proof layers:

1. Configured model and authentication route.
2. Model and reasoning level loaded by the running Gateway.
3. Model and provider used by an actual phone turn.
