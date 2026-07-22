# Scroll snap + single morphing device

Date: 2026-07-22

## What this is for

Make the AgentOS landing story snap cleanly between beats, and show **one** device that morphs shape (laptop → phone → duo → phone) on desktop and mobile — not a separate box per chapter.

## Locked choices

- Hard snap (`mandatory`) on story chapters; waitlist does not snap
- One `#story-device` everywhere
- Mobile: device pinned under nav (fixed band)

## Result (verified)

Preview: `http://127.0.0.1:5173/` (worktree `codex-launcher-landing-outcome`, branch `landing/outcome-copy`)

| Check | Evidence |
|---|---|
| One device node | `document.querySelectorAll('.device').length === 1` |
| Snap type | `scroll-snap-type: y mandatory` |
| Beat-2 snap align (390×844) | `snapDelta: 0` (chapter top vs scroll-padding) |
| Morph laptop→phone | shape `phone` after beat-2 dot |
| Morph duo | shape `duo` after beat-4 dot |
| Mobile center | device center X = viewport center (195) |
| No per-chapter clones | `.chapter__visual` count `0` |
| Waitlist reachable | CTA is a snap stop (`min-height: 100svh`); mobile device band hides via `html.is-past-story` |

## Judge follow-up

First judge: Pass-with-fixes — waitlist trapped by document mandatory snap.  
Fix applied: waitlist `.cta` snaps; mobile band unpins past story. Re-check: nav → waitlist from beat 7 shows email in viewport; sticky `visibility: hidden`.

## How it works

1. **Inputs:** scroll position / story-dot click  
2. **Outputs:** snapped chapter + single device `data-shape` + active screen  
3. **Algorithm:** nearest chapter vs `scroll-padding-top` → `setActiveState` (resize frame, then crossfade screen ~180ms)

Mobile band: `--device-band-h: min(32vh, 260px)` fixed under `--nav-h: 76px`.

## Files

- `landing/index.html` (CSS + morph/scroll JS)
- Plan: `planning/scroll-snap-morph-plan.md`

## Reproduce

```sh
cd ".../codex-launcher-landing-outcome/landing"
# serve static on :5173, open phone + desktop widths
# click story dots 2 and 4; confirm one device morphs and chapters lock
```
