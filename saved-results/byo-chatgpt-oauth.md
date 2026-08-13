# BYO ChatGPT OAuth gate (phone-agent)

Date: 2026-08-13

What this is for: Operator must not pay model cost. Home Send stays off until OpenClaw on the phone has the user’s ChatGPT / Codex OAuth profile (device-code). API keys do not count.

Branch: `cursor/byo-chatgpt-oauth-18d0`  
PR: https://github.com/aadivyaraushan/codex-launcher/pull/18 into `worktree-phase2-tool-bridge`

## Result

Unpaired / no-oauth users cannot send. After device-code ChatGPT OAuth, health reports `modelAuth=oauth_ready` and Home Send works. OpenClaw is told to prefer that oauth profile over any API key (`auth.order.openai`), including on reboot when the profile already exists.

`OPERATOR_ALLOW_SOFTWARE_ATTEST` was not changed. ChatGPT tokens stay in OpenClaw’s store, not in the Android app.

## Inputs → outputs → steps

1. **Inputs** — `openclaw models auth list --provider openai --json` (id/type/provider only); device-code CLI stdout; Android `GET /v1/health`.
2. **Outputs** — `modelAuth`: `oauth_ready` | `missing` | `pending`; `POST /v1/model-auth/start` `{ userCode, verificationUrl }` only; Home gated until both `taskCapable` and `oauth_ready`.
3. **Steps**
   1. Classify list: type `oauth` → ready; `api_key` only (including `keyed:true`) → missing; in-flight login → pending.
   2. Continue with ChatGPT spawns `openclaw models auth login --provider openai --device-code`.
   3. Show user code; open `https://auth.openai.com/...` via `ACTION_VIEW`.
   4. Prefer oauth ids in `openclaw models auth order set --provider openai …` and restart gateway.
   5. Poll health until `taskCapable && modelAuth=oauth_ready`, then show Home composer.

## How to reuse / verify

Do **not** set `OPERATOR_ALLOW_SOFTWARE_ATTEST` on a physical Pixel.

1. With local-pair acked and `:9443` up, but no ChatGPT OAuth in OpenClaw: Home is replaced by “Sign in with ChatGPT to use Operator”. Composer/Send are off. All apps and Android Settings still work.
2. Tap **Continue with ChatGPT**, enter the code at the ChatGPT page, wait until health is `oauth_ready` and `taskCapable=true`. Home Send should work.
3. Confirm `openclaw models auth list --provider openai` shows an oauth profile first in order, not only an API key.

## Tests run (this session)

```
go test -count=1 ./companion/internal/phoneruntime
go test -count=1 ./companion/internal/phoneruntime/modelauth/...
cd android && ANDROID_HOME=$HOME/android-sdk ./gradlew :app:testDebugUnitTest \
  --tests 'app.codexlauncher.runtime.modelauth.ModelAuthTest' \
  --tests 'app.codexlauncher.launcher.home.HomeUiStateTest' \
  --tests 'app.codexlauncher.LauncherStartupPolicyTest' \
  --tests 'app.codexlauncher.runtime.standalone.StandaloneRuntimeStatusReaderTest' \
  --tests 'app.codexlauncher.runtime.standalone.StandaloneRuntimeStatusTest' \
  --tests 'app.codexlauncher.launcher.home.HomeSendRouterTest'
bash scripts/phone-boot/test/run-tests.sh
```

All of the above passed. `android/local.properties` is gitignored and was not committed.

OpenClaw CLI shapes checked from current OpenClaw docs/source (`docs/cli/models.md`, `src/commands/models/auth-list.ts`): list `--json` uses `{ profiles: [{ id, provider, type, ... }] }`; `--device-code` is a shortcut for `--method device-code`; `models auth order set --provider openai <ids…>` is the order command.

## Health cache (Pixel hang, 2026-08-13)

After PR #18, `GET /v1/health` called `openclaw models auth list` on the handler. Under Termux/proot that CLI hung (~2 min) and ignore-cancel, then health fail-closed to `modelAuth=missing` even when ChatGPT OAuth was already on disk. Operator showed the sign-in wall.

Fix (branch `cursor/model-auth-health-cache-ad3f`, PR https://github.com/aadivyaraushan/codex-launcher/pull/19):

1. **Inputs** — OpenClaw on-disk store (`openclaw-agent.sqlite` `auth_profile_store`, else `auth-profiles.json`); CLI list only if no store files; in-flight device-code flag.
2. **Outputs** — cached `modelAuth` (`missing` until first successful list, `oauth_ready` sticky across later list failures, `pending` while device-code is in flight). No tokens in health or logs.
3. **Steps**
   1. Health returns the cache immediately. Never waits on the CLI.
   2. Open seeds the cache from a hang-proof disk read (read-only sqlite, 100ms busy timeout).
   3. Background refresh: disk first; CLI only when the store is absent. A hung/timed-out list keeps a prior `oauth_ready`. If the CLI ignores cancel, further refresh is disk-only so a later on-disk login can still lift the cache.

Verified this session:

```
go test -count=1 -timeout 120s ./companion/internal/phoneruntime/...
go test -race -count=1 -timeout 90s ./companion/internal/phoneruntime ./companion/internal/phoneruntime/modelauth/...
go vet ./companion/internal/phoneruntime ./companion/internal/phoneruntime/modelauth/...
```

All passed. Auth was not wiped. `OPERATOR_ALLOW_SOFTWARE_ATTEST` was not changed.

Sibling search: `modelAuthStatus`, `ListOpenAI(`, `models auth list`. Only `/v1/health` used `modelAuthStatus`; it no longer calls the CLI on the handler. `preferOAuthAfterLogin` still lists in the background after oauth (store-first now). Beeper health still probes with a 3s timeout inside `Health()` — different path, left alone.

## Listen before Health (Pixel Continue-with-ChatGPT, 2026-08-13)

PR #19 stopped `/v1/health` from waiting on the CLI, but `operator-phone-runtime` still called `Health()` three times in the "serve starting" log *before* `Serve()`. That can wait on Beeper (3s probe) and used to wait on `openclaw models auth list`, so `:9443` was not listening when Continue-with-ChatGPT needed it.

Fix: `saved-results/listen-before-health.md`. Boot log uses listen/mode from config. `Serve()` binds first. No software-attest change.

Searched `taskCapable`, `keyed`, `modelAuth`, `canSend`, `showComposer`. Send/composer now require `modelAuth=oauth_ready` in `HomeUiPolicy` and `StandaloneRuntimeStatus.isReady`. Android OpenAI broker remains keystore API-key for other Google/OpenAI device features and was left unchanged (requirement: do not put ChatGPT tokens in the Android app).
