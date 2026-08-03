# Wave 1 OAuth redirect alignment

**Date:** 2026-08-02 (heartbeat ~38)  
**Purpose:** One place to align portal registrations, main `.env`, and proof serve listeners before browser Approve.  
**Callers:** owner Approve prep; `wave1-overnight-remaining-walls.md` soft wall #6; overnight batch redirect table cite.  
**Not secrets:** scheme/host/port/path only — never client secrets or tokens.

**Gate facts (pre-create):**
1. **Callers:** Markdown cites only — `wave1-overnight-remaining-walls.md`, `wave1-overnight-batch-and-oauth-prep.md` (Fixed redirect URIs section), `wave1-overnight-progress-snapshot.md`. No code imports this file.
2. **No duplicate:** Only this file matches `saved-results/wave1*redirect*`; overnight Fixed redirect URIs table points here for the full drift matrix.
3. **Data files:** None. Status markdown only; `.env` shapes recorded as scheme/host/port/path without secret values.
4. **User instruction (verbatim):** You are an adversarial judge with fresh context. … Artifact to judge: `saved-results/wave1-oauth-redirect-alignment.md` … If you find factual errors in the alignment file, fix them with small edits. Write verdict to `saved-results/wave1-oauth-redirect-alignment-judge.md`.

## Inputs → Outputs → Algorithm

1. **Inputs:** Overnight prep URI table; live main `.env` redirect shapes; proof defaults in `*_proof.go`.  
2. **Outputs:** Per-service “register this / env is this / proof listens here” + pick-one rule when they disagree.  
3. **Algorithm:** Prefer what live authorize *start* already used successfully; make portal + `.env` match that exact string; keep proof listen port able to catch the callback.

## Verified shapes (main `.env`, 2026-08-02)

| Env key | Live shape (no secret values) |
|---|---|
| `SLACK_REDIRECT_URI` | `https` `127.0.0.1:9192` `/oauth/slack/callback` |
| `GOOGLE_REDIRECT_URI` | `http` `127.0.0.1:9194` path **empty** (`/`) |
| `MICROSOFT_REDIRECT_URI` | `http` `localhost` (no port) `/oauth/microsoft/callback` |

Client IDs for Slack / Google / Microsoft: **set**. Telegram keys + `PODCASTS_FEED_URL`: **missing**.

## Alignment matrix

| Service | Proof listen | Overnight prep (may be stale) | Live `.env` | Pick-one before Approve |
|---|---|---|---|---|
| Slack | `127.0.0.1:9192` TLS; default `https://127.0.0.1:9192/oauth/slack/callback` | overnight table now **HTTPS** (earlier draft had HTTP — wrong) | `https://127.0.0.1:9192/oauth/slack/callback` | Use **HTTPS** URI; expect self-signed cert click-through |
| Google Cal+Drive | `127.0.0.1:9194`; default path `/oauth/google/callback` | `http://127.0.0.1:9194/oauth/google/callback` | host `127.0.0.1:9194` with **path `/`** (empty) | Console + `.env` must match **exactly**; proof also mounts `/oauth/google/callback` as alias when path omitted |
| Microsoft Outlook mail | `127.0.0.1:9195`; default `http://127.0.0.1:9195/oauth/microsoft/callback` | `http://127.0.0.1:9195/oauth/microsoft/callback` | `http://localhost/oauth/microsoft/callback` (portless) | Entra: **`localhost` port-insensitive**; **`127.0.0.1` port-exact**. Pick **one host**. Serve rebuilds portless `localhost` → `localhost:<listen-port>` |
| Microsoft work Teams chat | `127.0.0.1:9196`; default `http://127.0.0.1:9196/oauth/microsoft/callback` only if both Teams + Microsoft redirect env empty | overnight now lists `http://127.0.0.1:9196/oauth/microsoft/callback` | no `MICROSOFT_TEAMS_REDIRECT_URI`; falls back to live `MICROSOFT_REDIRECT_URI` (portless `localhost`) then rebuilds → `localhost:9196/…` | **Host choice matters:** stay on `localhost` → one Entra URI covers Outlook **and** Teams (ports ignored). Switch to `127.0.0.1` → register **9195** and **9196** as distinct exact URIs. Prefer `MICROSOFT_TEAMS_REDIRECT_URI` if you need a Teams-only string |
| Notion | hosted MCP | browser only | no local redirect key | Approve in Notion’s hosted flow |
| Discord | n/a for Wave 1 bot path | old bot checklist | hand-off deeplink; no bot OAuth overnight | Ignore Discord redirect row for Wave 1 exit |

## Owner checklist (when ready to Approve)

1. Re-pair Pixel (separate wall — not fixed by redirects).  
2. For each service above, make **portal registered URI == `.env` URI** (exact string).  
3. Run the matching `serve-*-proof`, open the printed authorize URL, Approve, let loopback catch the code.  
4. Ports to keep free: **9192** Slack, **9194** Google, **9195** Outlook, **9196** Teams chat.

## Cite

- Overnight batch: `wave1-overnight-batch-and-oauth-prep.md`  
- Per-service evidence: `wave1-slack-oauth.md`, `wave1-google-oauth.md`, `wave1-microsoft-oauth.md`, `wave1-msteams-work-oauth.md`  
- Remaining walls: `wave1-overnight-remaining-walls.md`  
- Progress snapshot: `wave1-overnight-progress-snapshot.md`
