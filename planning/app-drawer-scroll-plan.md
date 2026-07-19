# App Drawer Scroll Fix

## Behavior

```text
Home task scrolling -> stays on Home
Explicit All apps tap -> opens app drawer
Android launcher activities -> complete, sorted app drawer list -> last app remains reachable
```

## Done means

- A long upward drag over Home content never calls the All Apps action.
- The explicit **All apps** control still opens the drawer.
- The drawer includes every enabled launchable activity Android reports, except Codex Launcher itself.
- Automated tests first reproduce both failures, then pass.
- The final APK is installed and both flows are driven on the connected Pixel 9.
- A separate judge reviews the final diff and evidence.

## Steps

- [x] Capture both failures on the current Pixel build.
- [x] Add failing gesture and real-device app-discovery tests.
- [x] Remove the screen-wide swipe shortcut and declare narrowly scoped launcher-app visibility.
- [x] Run focused tests and lint; search for the same gesture and visibility assumptions elsewhere.
- [x] Install the final APK and verify scrolling, explicit opening, full-list count, and deep-list reachability on Pixel 9.
- [ ] Save evidence, obtain an independent review, commit, and apply the commit to the main workspace.
