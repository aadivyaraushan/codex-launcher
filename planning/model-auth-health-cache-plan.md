# Health `modelAuth` cache (no CLI in GET /v1/health)

Date: 2026-08-13

## Why

A hung `openclaw models auth list` under Termux/proot stalled `GET /v1/health` for ~2 minutes, then fail-closed to `modelAuth=missing` even when ChatGPT OAuth was already on disk. Operator showed the sign-in wall on a phone that was already signed in.

## Flow

```
  GET /v1/health
        |
        v
  return cached modelAuth now
  (default missing; pending if login in flight;
   oauth_ready sticky across list failures)
        |
        +-- background refresh (never on the handler stack)
              |
              +-- read OpenClaw on-disk store (sqlite, then json)
              |     hang-proof: read-only, 100ms busy timeout
              |     keep id/type/provider only — never tokens
              |
              +-- if no store files: CLI fallback
                    timeout/hang = keep prior cache
```

## Inputs → Outputs → Steps

1. **Inputs** — OpenClaw agent sqlite `auth_profile_store.store_json` (or legacy `auth-profiles.json`); optional CLI list; in-flight device-code flag.
2. **Outputs** — health `modelAuth`: `oauth_ready` | `missing` | `pending`. No tokens in JSON or logs.
3. **Steps**
   1. Health reads the cache only. Never waits on the CLI.
   2. Successful list with an openai oauth profile → `oauth_ready`. Later list failures leave that cache alone.
   3. Successful empty list (logged out) → `missing`.
   4. Device-code in flight and not yet oauth → `pending`.

## Out of scope

Wiping auth. `OPERATOR_ALLOW_SOFTWARE_ATTEST`. Changing the default model.
