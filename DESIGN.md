# Design System — Quiet Instrument

## Product Context

- **What this is:** A personal Android launcher for a Pixel 9 whose primary surface is remote Codex work running on the user's computer.
- **Who it is for:** One technically capable owner using their phone to start, monitor, redirect, approve, and resume computer-based Codex tasks.
- **Product category:** Android launcher, remote agent client, and task inbox.
- **Memorable idea:** “My phone is a quiet interface to my work, not a grid of apps.”
- **Primary hierarchy:** Codex tasks first, prompt second, ordinary apps and launcher settings as a quiet utility layer.

## Aesthetic Direction

- **Name:** Quiet Instrument.
- **Direction:** Brutally minimal with a restrained technical edge.
- **Decoration:** Minimal. Type, spacing, state, and hairlines provide structure.
- **Mood:** Calm, serious, and personal. It should feel ready for work without making the phone feel like a terminal.
- **Avoid:** App-icon grids, wallpaper competing with content, nested cards, gradients, ornamental widgets, constant animation, and color without meaning.

## Appearance Modes

The product has one visual identity with two modes.

### Deep Charcoal

- Preferred everyday appearance and the first mode shown in product imagery.
- Background: `#111210`
- Raised surface: `#1A1B18`
- Primary text: `#F0EEE8`
- Muted text: `#969990` (`6.49:1` against the background)
- Hairline: `#30312D`
- Active/attention signal: `#F06B3F`
- Success: `#75A987`
- Warning: `#D49A3B`
- Error/destructive: `#E46F61`

### Warm Paper

- Daylight and long-reading appearance.
- Background: `#F4F2ED`
- Raised surface: `#EBE9E2`
- Primary text: `#191A18`
- Muted text: `#656861` (`5.06:1` against the background)
- Hairline: `#D8D7D1`
- Active/attention signal: `#E86136`
- Success: `#4D8061`
- Warning: `#B87816`
- Error/destructive: `#A33D32`

### Mode behavior

- Default selection: **Follow system**.
- Manual choices: **Light** and **Dark**.
- A manual choice persists until the user selects Follow system again.
- Android Dynamic Color does not recolor the launcher.
- Android font scaling, high-contrast preferences, and reduced-motion settings are respected.
- Color meanings remain identical between modes.

## Typography

- **Display and task titles:** Instrument Sans, width axis near normal, medium weight.
- **Body and controls:** Instrument Sans, regular through semibold.
- **Commands, paths, status, elapsed time, and tool activity:** JetBrains Mono.
- **Fallback:** Bundle both families with the app. Platform fallback is used only if a bundled font cannot load.
- **Minimum working sizes:** 15sp task titles, 13sp body and supporting text, 10sp only for short uppercase or monospaced metadata that remains above accessible contrast.
- **Hierarchy:** Task title over clock. The clock is useful context, not the home screen's main subject.

## Color Rules

- Signal orange is rare. Use it only for active work, the selected task, or something requiring attention.
- Do not use signal orange as a decorative brand wash or large background.
- Success, warning, and error colors never carry meaning alone; pair them with words and distinct shapes.
- Commands and sensitive paths use primary text, never low-contrast muted text.
- Dimmed background content behind approval/question sheets remains recognizable but cannot compete with the decision.

## Spacing and Sizing

- **Base unit:** 4dp.
- **Common spaces:** 4, 8, 12, 16, 24, and 32dp.
- **Screen side inset:** 20dp, adjusted for Android-reported safe areas.
- **Row density:** Comfortable, not spacious. Task rows generally use 12dp vertical padding.
- **Minimum touch target:** 44 × 44dp; security and task-control actions target 48dp height.
- **Corners:** 4–8dp for normal controls, 12–16dp only for bottom sheets. Avoid making every surface a pill.

## Layout

- One left-aligned vertical column.
- Active tasks appear above recent tasks.
- The new-prompt composer stays at the bottom within thumb reach.
- The computer is fixed in V1. The selected project/folder is changeable and appears directly above the Home composer.
- Sending is disabled until a project/folder is selected.
- All apps and Android Settings remain visibly reachable on Home and Offline.
- Swipe up opens the searchable text-first app list.
- Task transcripts are continuous work logs rather than alternating chat bubbles.
- Tool activity is compact by default and expands on tap.
- Diffs, command output, files, test output, and screenshots use dedicated viewers reached from the task log.
- Every screen uses Android-reported status, cutout, gesture, keyboard, and navigation insets. Mockup frame measurements are not implementation constants.

## Core States

State shapes are fixed across Home, task lists, notifications, and detail screens:

- **Working:** solid signal-orange circle, paired with the written state; a three-stroke activity mark may animate beside live progress text.
- **Waiting for user:** outlined warning-color diamond plus a warning hairline on the task row; always paired with `Needs your answer` or `Approval needed`.
- **Replied:** outlined muted circle containing a check; paired with `Replied` after Codex sends a response and is no longer working.
- **Failed:** outlined error-color square containing an X; always paired with `Failed` and a recovery action.

Do not invent new state marks in later screens. Paused, queued, and interrupted states need explicit additions to this mapping before implementation.

### Home

- Computer connection is written explicitly: online or offline.
- Working, waiting, replied, and failed tasks use words plus shape.
- A waiting-for-user task receives stronger hierarchy than the clock.
- Ordinary app access is visible but secondary.
- The project/folder selector shows the fixed computer and current folder above the composer.
- New tasks expose Model, Reasoning, and Permission mode without crowding the prompt field.
- Dictation has a written accessibility label even when represented by a compact icon.

### Active task

- Header identifies task, computer, current state, and elapsed time.
- The main action is a follow-up/redirect field.
- Stop is a written, 48dp action, not an unexplained square glyph.
- Queue, redirect, and stop must be distinguishable before implementation is approved.
- The task overflow menu offers Rename task, Archive task, and Fork task.

### Approval

- The sheet names the computer, project, requested access, affected paths or `None`, exact redacted command, and permission duration.
- Only scopes offered by the computer are displayed.
- Deny is always present and never visually hidden.
- Disconnect, timeout, ambiguity, or simultaneous pending questions fail closed.

### Question

- The prompt explains why the task is waiting.
- Choices are native controls with a typed Other response where supported.
- Experimental structured-question support requires a plain-message fallback.

### All apps

- Swipe up from Home or tap `All apps` to open it.
- Search is focused immediately when the user starts typing.
- Apps appear as a text-first alphabetical list without icons by default.
- Android Settings and Launcher settings are always available.
- Hiding or renaming apps belongs in Launcher settings, not through secret gestures.

### Appearance

- Theme control offers Follow system, Light, and Dark with Follow system selected by default.
- Light and dark previews use the real Warm Paper and Deep Charcoal tokens.
- One quiet note states: “Text size, contrast, and motion follow Android settings.”
- Appearance contains no notification or other behavior controls.
- Manual appearance selection persists until the user chooses Follow system again.

### Offline

- Say that tasks remain on the computer; do not claim their latest state is safe or current while disconnected.
- Show the last successful connection time and selected computer.
- Retry and connection help are available.
- All apps and Android Settings remain available without the computer.

## Motion

- **Approach:** Minimal and functional.
- Micro transitions: 120–180ms.
- Enter: ease-out; exit: ease-in; position change: ease-in-out.
- Working state uses a quiet three-stroke motion mark rather than a spinner.
- No looping animation except active progress, and reduced motion removes it.
- Theme changes crossfade surfaces and text without moving layout.

## Accessibility

- Normal text meets WCAG AA contrast; aim for at least `4.5:1`.
- Controls expose clear accessibility labels, especially voice, attachment, send, Stop, approval scope, and Settings.
- Font scaling must not hide task state or approval details.
- Focus order follows visual order.
- State is never communicated only by color, animation, or position.

## Decisions Log

| Date | Decision | Rationale |
|---|---|---|
| 2026-07-12 | Tasks replace favorite apps on Home | Makes the launcher about intentions and active work rather than app launching. |
| 2026-07-12 | Deep Charcoal and Warm Paper form one system | The two modes share hierarchy and meaning while fitting night and daylight use. |
| 2026-07-12 | Follow system is default with persistent overrides | Matches Android expectations without surrendering the product palette to Dynamic Color. |
| 2026-07-12 | Deep Charcoal is the preferred presentation | The user selected the dark direction as the stronger everyday appearance. |
| 2026-07-12 | Direction 3 was removed | Dynamic recoloring weakened the product identity and duplicated the light direction. |
| 2026-07-12 | Ordinary notifications remain in Android's shade | Keeps the launcher focused on Codex work and avoids building a second notification center. |
| 2026-07-12 | Notification count requires optional listener access | Preserves privacy and makes the permission's limited purpose explicit. |

## Source Artifact

The approved visual comparison is `outputs/codex-launcher-visual-directions.html`.
