# Scroll snap + single morphing device

Date: 2026-07-22  
File: [`landing/index.html`](../landing/index.html)

## Why

Snap is soft (`proximity`). Mobile clones a device per chapter; desktop morphs one box — two different visuals.

## Locked (founder)

- Hard snap between story beats
- One morphing `#story-device` on desktop **and** mobile
- Mobile: device pinned under nav (band A)

## Done looks like

- After a fling, each beat’s headline sits cleanly in the text area (≤8px of snap target)
- Waitlist / footer do **not** snap
- `document.querySelectorAll('.device').length === 1` at all widths
- Laptop → phone reads as one frame resizing, not two boxes

## Architecture

```
[nav sticky]
[device band]  desktop: sticky side col | mobile: fixed under nav
[chapters]     min-height 100svh, snap-align start, snap-stop always
[waitlist]     no snap
```

- Snap stays on `html` (`y mandatory`) — not a nested scroller (simpler with existing waitlist)
- Mobile: `.story__sticky-col` `position: fixed; top: var(--nav-h); height: var(--device-band-h)` + opaque bg
- `scroll-padding-top: calc(var(--nav-h) + var(--device-band-h))` on mobile; desktop: `--nav-h` only
- Delete `buildMobileDevices` + `.chapter__visual` hosts
- Active beat: `scrollend` (debounced scroll fallback) → nearest chapter; IO as backup with rootMargin subtracting device band
- Dots: `scrollIntoView({ block: 'start' })`
- Morph: keep continuous outer chrome for laptop/phone/**duo**; set `data-shape` first, crossfade screens after short delay
- `prefers-reduced-motion`: snap off; morph jumps (existing global transition kill)
- Waitlist: `.cta` is a full-viewport snap stop; mobile device band hides when `html.is-past-story`

## Judge

Adversarial review: Agree-with-fixes — fixes above baked in before coding.  
Post-ship judge: Pass-with-fixes on waitlist trap → fixed (CTA snap + unpin band).
