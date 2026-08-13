# Deepgram tap-to-talk dictation

**Date:** 2026-08-13  
**Why:** Home and task follow-up still open Google’s speech activity. Operator wants in-app recording + Deepgram REST, with the same composer merge as today, and no API key in git.

```
  tap mic  -->  RECORD_AUDIO?
       |            no --> Unavailable (permission)
       |            yes
       v
  start clip (2nd tap or ~12s timeout stops)
       |
       v
  POST audio --> Deepgram /v1/listen  (debug key from local.properties or app-private file)
       |
       v
  Recognized text --> mergePromptDictation / applyDictation (unchanged)
  fail --> Unavailable | Failed | Cancelled (existing kinds, clearer messages)
```

HomeScreen UI stays the same. Google `RecognizerIntent` is removed. Key never committed; debug APK on Pixel reads a gitignored BuildConfig field and/or `files/deepgram_api_key` via `adb`.
