# Google broker — human unblock (OAuth testing)

**Date:** 2026-08-06T03:07Z  
**Device:** Pixel `4B230DLAQ001Z5`  
**Blocks:** `LiveGoogleAuthProofTest` → full instrumentation 0/134

## Verified blocker

Launching `GoogleAuthorizeActivity` shows Google Identity UI:

> **Access blocked: Operator has not completed the Google verification process**  
> The app is currently being tested and can only be accessed by developer-approved testers.

Silent authorize also requires UI (`hasResolution()`), so prefs alone cannot restore the grant.

## What to do

In Google Cloud project for Operator OAuth (`operator-504223` / Firebase Android client for `app.codexlauncher`):

1. OAuth consent screen → **Testing** → add test user `ssdear@gmail.com` (or the Pixel account), **or**
2. Complete verification / publish if that is the intended path.

Then on device: `adb shell am start -n app.codexlauncher/.runtime.broker.google.GoogleAuthorizeActivity` → Allow → prefs `status=granted`.

## Not the issue

- Sibling code / APK at `7b07a29`
- Store keystore (unsigned debug instrument)
- HOME chooser (already fixed for this run)


## Cleared 2026-08-06T03:45Z

Human Cloud tester unblock confirmed. Device grant completed (both Calendar + Drive). Full instrument **0/134**. This human Google todo is done.
