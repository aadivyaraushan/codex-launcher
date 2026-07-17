# Generator State — Iteration 001

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
- N/A — first iteration.

## Self-Check Performed (Playwright, chromium)
- 1440px and 390px: zero horizontal scroll (`scrollWidth === clientWidth` at both).
- No console errors, no page errors.
- Waitlist form: fill email → submit → form hides, `#waitlist-success` becomes visible with exact text "You're on the list."
- Keyboard focus: `:focus-visible` confirmed via computed style — 2px solid `rgb(240,107,63)` (#F06B3F) outline, 3px offset, on nav link and buttons.
- `prefers-reduced-motion: reduce` confirmed collapses the activity-mark keyframe animation to ~0.
- Verified all locked copy strings render verbatim (hero eyebrow/headline/subhead/button/support line, feature heading, CTA heading/body/placeholder/button, footer mark + affiliation print) via DOM text extraction.
- Visually inspected full-page screenshots at both widths plus cropped close-ups of the approval sheet, transcript diff, composer-detail, and footer — state shapes, hairlines, and mono alignment all read clean.

## Known Issues
- Have not tested in a real (non-Chromium) browser engine (Firefox/WebKit) — only Chromium via Playwright.
- The empty space in the middle of the phone Home mockup (between the RECENT row and the composer) is intentional — it reads as the "apps stay one swipe away" dead zone referenced in copy block 1 — but worth flagging in case the Evaluator reads it as a bug rather than a deliberate echo of the product's own design system.
- No automated Lighthouse/contrast-checker run; contrast ratios for text/muted/state colors against `--bg` were computed by hand (all ≥ 6:1, comfortably AA) but not machine-verified.

## Dev Server
- No server needed — single static file.
- Open directly: `file:///Users/aadivyar/conductor/workspaces/codex-launcher/bozeman/landing/index.html`
- Status: file complete, self-checked, ready for evaluation.
