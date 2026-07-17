# Generator State — Iteration 002

## What Was Built
- Single self-contained `landing/index.html` for Codex Launcher — Deep Charcoal "Quiet Instrument" identity, dark mode only, exact hex tokens.
- Slim top nav (mono wordmark + outline "Join the waitlist" link to `#waitlist`).
- Hero: eyebrow, huge headline (clamp up to ~92px), subhead, primary orange CTA, mono support line, and a full CSS-rendered phone Home screen mockup as the anchor visual.
- Phone Home screen: status line (green dot + "Studio · online" + clock), ACTIVE section (working row with solid orange circle + 3-bar activity mark + mono progress line; waiting row with outlined amber diamond + "Needs your answer"; replied row with outlined muted circle + check + "Replied"), RECENT section (failed row with outlined red square + X + "Failed"), and a bottom composer (project chip, prompt field, mic + send affordances).
- Mono "spec strip" divider between hairlines.
- 7 feature blocks, alternating text-left/visual-right layouts, each with a distinct rendered UI fragment (no repeated cards):
  1. Home screen is the work → "clock vs task" typographic comparison panel.
  2. Runs where your code lives → phone/computer diagram with labeled connector.
  3. Privacy → full-width, no side visual, typographic treatment with a pulled-out emphasis line (same locked text, styled bigger/bold).
  4. Approve every move → approval sheet fragment (Computer/Project/Requested access/Affected paths/exact mono command/Duration, Deny outline + Allow filled).
  5. Whole task in your hand → transcript fragment (header with state "Working" + elapsed, two collapsed tool rows, one expanded +/- diff block).
  6. Send work the way you mean it → composer detail fragment (Model/Reasoning/Permission chips, one selected in orange, project chip, field with attach/mic/send).
  7. Offline is a state → offline card (hollow dot "Offline · Studio", persisted task rows, "Last connected 2m ago").
- Final CTA band with working waitlist form (`#waitlist`), footer with required affiliation line.
- Inline JS: form submit is prevented, attempts `fetch('/api/waitlist')` (skipped under `file:` protocol to avoid a scheme console error, still attempted over http/https), then always shows "You're on the list." via `role="status"`.

## What Changed This Iteration
Coordinator scored v1 at 7.83/10 (just under the 8.0 bar) and requested one focused polish pass. Addressed all 5 priorities, same file, no redesign:
1. **Hero phone (crown jewel)**: enlarged 320px → 380px max-width, more internal padding (14→18px outer, 24/16/16→32/22/22 screen), task-row title 13px→15px/weight 600, task-row padding-block 10px→14px, state icons 16px→20px, activity-mark bars enlarged and given `flex-shrink:0` so they're never clipped, composer field/icons enlarged (icon-btn 20px→24px).
2. **Weak fragments (blocks 1 & 6)**: clock-compare and composer-detail now have a header-bar + hairline-divider at the top (matching the sheet's handle and the transcript's header pattern), bigger padding, and a bolder/bigger title in clock-compare (700 weight, up to 1.75rem) so they read as intentional panels, not thumbnails.
3. **Block 2 diagram**: replaced the plain "companion app" floating label with a distinctive tag — a small lock icon + mono "TAILSCALE" — sitting directly on the connecting hairline (background cuts the line like a flowchart node), reinforcing "one private line" ahead of block 3.
4. **Vertical rhythm**: `--section-pad` reduced from 96px to 64px desktop / 48px mobile (both valid stops on the spec's 4px scale), tightening the stack of 7 feature blocks + CTA without touching the intentionally larger hero.
5. **Mobile nav**: added a `max-width:480px` rule shrinking the wordmark and CTA link padding/font-size so they sit comfortably instead of crowding the edges.

Also fixed a real bug introduced by enlarging the phone: a CSS grid item defaults to `min-width:auto`, so once the hero phone grew, the single-column mobile grid track blew out ~10px past the viewport. Fixed with `min-width:0` on `.hero__text`/`.hero__visual` inside the `max-width:900px` query — verified with a full-page DOM overflow scan (0 elements crossing the viewport edge at 390px, down from 4).

## Self-Check Performed (Playwright, chromium) — iteration 2
- Re-ran the iteration-1 checklist (horizontal scroll, console/page errors, waitlist form success text, focus-visible outline color/width, reduced-motion collapse) — all still pass.
- Added a full DOM overflow scan (`getBoundingClientRect` on every element vs viewport) at both 1440px and 390px — 0 overflowing elements at both, after fixing the grid min-width bug above.
- Cropped screenshots of the phone, clock-compare, diagram (desktop + mobile/vertical), composer-detail, and mobile nav to confirm the specific fixes visually.
- Iterated on the diagram's node text wrap (`macOS · Windows · Linux` was breaking mid-phrase after narrowing the connector column) by widening the diagram's own max-width to 480px (still within its 556px grid column, so no overflow) — now renders on one line.

## Known Issues
- Have not tested in a real (non-Chromium) browser engine (Firefox/WebKit) — only Chromium via Playwright.
- The empty space in the middle of the phone Home mockup (between the RECENT row and the composer) is intentional — it reads as the "apps stay one swipe away" dead zone referenced in copy block 1 — flagging again since the phone is now bigger and the gap is proportionally larger too.
- No automated Lighthouse/contrast-checker run; contrast ratios for text/muted/state colors against `--bg` were computed by hand (all ≥ 6:1, comfortably AA) but not machine-verified.

## Dev Server
- No server needed — single static file.
- Open directly: `file:///Users/aadivyar/conductor/workspaces/codex-launcher/bozeman/landing/index.html`
- Status: file complete, self-checked, ready for re-evaluation.
