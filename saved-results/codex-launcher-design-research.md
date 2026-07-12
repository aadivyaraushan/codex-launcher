# Codex launcher design research

**Date:** 2026-07-12

## Purpose

Inform the visual design of a personal Pixel 9 launcher whose primary surface is remote Codex work rather than an app grid.

## Approved memorable idea

> My phone is a quiet interface to my work, not a grid of apps.

## References checked

- Hermes Desktop at commit `4281151ae859241351ba14d8c7682dc67ff4c126`: flat task surface, live tool state, bottom composer, approvals, files, and sessions.
- OpenAI Remote connections documentation: host/task selection, approvals, outputs, offline host behavior, and remote continuation.
- Niagara Launcher website and current Google Play screenshots: short vertical app list, one-handed alphabet access, minimal home screen.
- Olauncher GitHub and current Google Play screenshots: text before icons, very low visual noise, search as the secondary app surface.
- Before Launcher website: notification filtering, hidden apps, text-first organization, and an intentional escape from the ordinary app grid.

## Synthesis

Minimal launchers generally suppress distraction but still organize the phone around apps. This launcher should organize the home screen around intentions and active work:

```text
Apps -> hidden utility layer
Codex tasks -> primary home-screen objects
Prompt -> primary action
```

The common information layout across all visual directions is:

1. status, time, date, and selected computer;
2. active tasks, then recent tasks;
3. bottom prompt composer;
4. task transcript with compact tool activity and diffs;
5. security-first approval sheet;
6. explicit offline screen with always-available Apps and Android Settings routes.

## Visual artifact

`outputs/codex-launcher-visual-directions.html`

It records the approved Quiet Instrument system across Home, Active task, Approval, Offline, Question, All apps, and Appearance screens. Deep Charcoal is the preferred dark appearance; Warm Paper is the light appearance. Follow system is the default, with persistent Light and Dark overrides. The Appearance screen keeps only the theme selector, previews, and one note that text size, contrast, and motion follow Android settings. Needs Attention and Completion are not separate screens: attention remains visible on Home, while Codex results remain in the active task transcript.

## Verification

- Static checks confirm screens 8 and 9, their rendering functions, and their screen-specific styles are absent.
- Static checks confirm seven screens per appearance direction and the approved Android-settings note.
- A fresh visual reload in the Codex in-app browser was blocked because that browser does not permit local `file://` access. The earlier nine-screen visual measurements are therefore not claimed for this regenerated guide.
- Approval screens name the computer, project, access requested, exact command, duration choice, deny action, and allow-once action.
- Offline screens retain All apps and Android Settings access.

## Reuse

Open the comparison board directly:

```bash
open outputs/codex-launcher-visual-directions.html
```

Update the same board after feedback rather than creating disconnected mockups.
