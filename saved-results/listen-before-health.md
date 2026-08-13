# Bind :9443 before Health (Continue-with-ChatGPT)

Date: 2026-08-13

What this is for: Pixel Continue-with-ChatGPT failed because `127.0.0.1:9443` was not listening. `operator-phone-runtime` logged `runtime.Health()` three times (Mode, Process, ListenAddress) before `Serve()`. Health used to wait on hung `openclaw models auth list`, and still waits on a Beeper accounts probe (3s timeout).

Branch: `cursor/listen-before-health-60db`  
PR: https://github.com/aadivyaraushan/codex-launcher/pull/20 into `worktree-phase2-tool-bridge` (tip ~9fb9db5, after PR #19 health cache).

## Result

The process binds `:9443` without waiting on OpenClaw auth. The serve-starting log uses CLI listen + `phoneruntime.ModeStandalonePhone`. `Serve()` calls `net.Listen` immediately. `OPERATOR_ALLOW_SOFTWARE_ATTEST` was not changed. No secrets in logs or health.

## Inputs → outputs → steps

1. **Inputs** — CLI `-listen` (default `phoneruntime.ListenAddress` = `127.0.0.1:9443`); mode string `standalone_phone`.
2. **Outputs** — JSON log `serve starting` with `mode` and `listen`; TCP bind on that address before any Health work.
3. **Steps**
   1. After `Open()`, log listen/mode from config. Zero `Health()` calls before `Serve()`.
   2. `Serve()` `net.Listen`s first. Health and model-auth list run only on `/v1/health` (and background refresh from Open), never before bind.
   3. Continue-with-ChatGPT can reach `:9443` even if OpenClaw list or Beeper probe is hung.

## How to reuse / verify

Do **not** set `OPERATOR_ALLOW_SOFTWARE_ATTEST` on a physical Pixel.

## Tests run (this session)

Red: `TestMainSourceDoesNotCallHealth` failed on the old `main.go` (`Health()` ×3 before `Serve()`).

Green:

```
go test -count=1 -timeout 120s ./companion/cmd/operator-phone-runtime ./companion/internal/phoneruntime/...
go test -race -count=1 -timeout 90s ./companion/cmd/operator-phone-runtime ./companion/internal/phoneruntime ./companion/internal/phoneruntime/modelauth/...
go vet ./companion/cmd/operator-phone-runtime ./companion/internal/phoneruntime
```

All passed. `OPERATOR_ALLOW_SOFTWARE_ATTEST` was not changed.

## Sibling search

Searched `.Health()` under `companion/cmd/` (only the new main_test guard), `serve starting`, `models auth list` before bind. No other pre-Serve Health callers. `/v1/health` still calls Health after listen. Beeper probe inside Health left alone — it is no longer on the bind path.
