# Generator State — Iteration 003

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
- Inline JS: form submit validates the email, checks the fetch response, and only shows success when the request actually succeeds (see iteration 3 below for the honest-failure rework).
- `<head>` carries a meta description plus full Open Graph and Twitter Card tags describing the product accurately (added in iteration 3).

## What Changed This Iteration (003)
An independent judge scored iteration 2 at 7/10 (borderline) — visual craft was strong but flagged concrete functional gaps. Fixed exactly the 4 things raised, no redesign, no copy/token changes:

1. **Waitlist form was faking success.** It had `novalidate`, never validated the email, never checked the fetch response, and always showed "You're on the list." even on a blank field or a failed request. Reworked the whole flow:
   - JS now validates the email (non-empty, matches a standard email regex) before doing anything. An invalid/empty email shows an inline error ("Enter a valid email address.") right under the field, marks the input `is-invalid`/`aria-invalid="true"`, and does NOT show success. The error text is linked to the input via `aria-describedby` and uses `role="alert"` so screen readers announce it immediately.
   - On a valid email, the code POSTs JSON to `/api/waitlist` and checks `response.ok` — success only shows when the response is actually ok.
   - A failed request, a non-ok response, or the local `file://` case (where `/api/waitlist` can't exist yet) now shows a distinct `--error`-colored status message ("Something went wrong — try again.") instead of silently faking success. Both the success and error status lines sit in an `aria-live="polite"` region so either gets announced.
   - Kept `novalidate` on the `<form>` deliberately (rather than removing it) so the browser's native validation tooltip doesn't fight our on-brand inline error — the JS validation now does that job instead.

2. **SEO/social meta tags.** Added `<meta name="description">`, and full Open Graph (`og:type`, `og:title`, `og:description`, `og:image=og.png`, `og:url=https://codexlauncher.app`) and Twitter Card (`twitter:card=summary_large_image`, `twitter:title`, `twitter:description`, `twitter:image=og.png`) tags. Description wording is pulled from the spec's own plain-language product description ("Codex Launcher turns your Android phone into the remote control for Codex coding tasks running on your own computer, connected privately over Tailscale.") rather than invented marketing language.

3. **Mobile nav CTA was fine print.** The `max-width:480px` rule was shrinking the "Join the waitlist" link to 10.5px text / 7px padding. Bumped to 13px text / 8px-12px padding so it reads as a real button; also tightened the gap between wordmark and link slightly (12px→10px) so the bigger link still doesn't crowd the wordmark.

4. **Pressed states.** The primary button already had `:active{ transform: translateY(1px) }`. Added the matching `:active` treatment (border brightens + 1px press) to `.btn--outline` and `.nav__link` so all three interactive controls give the same tactile feedback.

## What Changed in Iteration 002 (for reference)
Coordinator scored v1 at 7.83/10 (just under the 8.0 bar) and requested one focused polish pass. Addressed all 5 priorities, same file, no redesign:
1. **Hero phone (crown jewel)**: enlarged 320px → 380px max-width, more internal padding (14→18px outer, 24/16/16→32/22/22 screen), task-row title 13px→15px/weight 600, task-row padding-block 10px→14px, state icons 16px→20px, activity-mark bars enlarged and given `flex-shrink:0` so they're never clipped, composer field/icons enlarged (icon-btn 20px→24px).
2. **Weak fragments (blocks 1 & 6)**: clock-compare and composer-detail now have a header-bar + hairline-divider at the top (matching the sheet's handle and the transcript's header pattern), bigger padding, and a bolder/bigger title in clock-compare (700 weight, up to 1.75rem) so they read as intentional panels, not thumbnails.
3. **Block 2 diagram**: replaced the plain "companion app" floating label with a distinctive tag — a small lock icon + mono "TAILSCALE" — sitting directly on the connecting hairline (background cuts the line like a flowchart node), reinforcing "one private line" ahead of block 3.
4. **Vertical rhythm**: `--section-pad` reduced from 96px to 64px desktop / 48px mobile (both valid stops on the spec's 4px scale), tightening the stack of 7 feature blocks + CTA without touching the intentionally larger hero.
5. **Mobile nav**: added a `max-width:480px` rule shrinking the wordmark and CTA link padding/font-size so they sit comfortably instead of crowding the edges.

Also fixed a real bug introduced by enlarging the phone: a CSS grid item defaults to `min-width:auto`, so once the hero phone grew, the single-column mobile grid track blew out ~10px past the viewport. Fixed with `min-width:0` on `.hero__text`/`.hero__visual` inside the `max-width:900px` query — verified with a full-page DOM overflow scan (0 elements crossing the viewport edge at 390px, down from 4).

## Self-Check Performed (Playwright, chromium) — iteration 3
- Re-ran horizontal-scroll + full-DOM overflow scan at 1440px and 390px — 0 overflowing elements at both, same as iteration 2.
- Console/page errors: none at either width.
- Form behavior, specifically verified in this order on desktop:
  - Empty email + submit → inline error visible, `#waitlist-success` NOT visible, input gets `is-invalid`.
  - Malformed email ("not-an-email") + submit → inline error visible, success NOT visible.
  - Valid email + submit (no backend available locally) → inline field error clears, but `#waitlist-error` ("Something went wrong — try again.") becomes visible, `#waitlist-success` stays hidden, and the form stays visible/usable (not replaced by a fake success state).
- Focus states: a programmatic `.focus()` call does NOT trigger `:focus-visible` in Chromium (expected browser behavior, not a bug) — retested with a real `Tab` keypress instead, which correctly shows a 2px solid `rgb(240,107,63)` (`--signal`) outline on the focused link.
- `prefers-reduced-motion: reduce` still collapses transition durations to ~0.01ms.
- Confirmed via `grep` that the meta-description string appears identically in all three of `<meta name="description">`, `og:description`, and `twitter:description` (single source edited once, no drift).

## Self-Check Performed (Playwright, chromium) — iteration 2
- Re-ran the iteration-1 checklist (horizontal scroll, console/page errors, waitlist form success text, focus-visible outline color/width, reduced-motion collapse) — all still pass.
- Added a full DOM overflow scan (`getBoundingClientRect` on every element vs viewport) at both 1440px and 390px — 0 overflowing elements at both, after fixing the grid min-width bug above.
- Cropped screenshots of the phone, clock-compare, diagram (desktop + mobile/vertical), composer-detail, and mobile nav to confirm the specific fixes visually.
- Iterated on the diagram's node text wrap (`macOS · Windows · Linux` was breaking mid-phrase after narrowing the connector column) by widening the diagram's own max-width to 480px (still within its 556px grid column, so no overflow) — now renders on one line.

## Known Issues
- Have not tested in a real (non-Chromium) browser engine (Firefox/WebKit) — only Chromium via Playwright.
- `og:image`/`twitter:image` point at a relative `og.png` that does not exist in `landing/` yet — per the coordinator's message, they will place that file themselves; the tags are wired correctly and will resolve once it's added.
- The empty space in the middle of the phone Home mockup (between the RECENT row and the composer) is intentional — it reads as the "apps stay one swipe away" dead zone referenced in copy block 1.
- No automated Lighthouse/contrast-checker run; contrast ratios for text/muted/state colors against `--bg` were computed by hand (all ≥ 6:1, comfortably AA) but not machine-verified.

## Dev Server
- No server needed — single static file.
- Open directly: `file:///Users/aadivyar/conductor/workspaces/codex-launcher/bozeman/landing/index.html`
- Status: file complete, self-checked, ready for re-evaluation. Note: opening as a static `file://` URL will always show the honest "Something went wrong" error state on waitlist submit (by design — `/api/waitlist` doesn't exist until deploy); this is expected, not a bug.
