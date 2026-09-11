# The demo recording is blocked: the agent never calls the native connectors

Date: 2026-09-11
Device: iPhone 16 Pro simulator `7EED8360-33E3-46F0-92B3-5F5F116C6A08`, iOS 18.6
Goal: capture a screen recording of Operator completing a real errand, for the
landing page.

## Result

Not recordable yet. The node is connected and paired; the agent does not use it.

## What works

- **Recording.** `xcrun simctl io … recordVideo` produces a real file:
  1206x2622, h264, verified with ffprobe. ffmpeg is available to trim/convert.
- **Model sign-in.** The ChatGPT device-code flow completed; the "Connect
  ChatGPT" button is gone and the agent answers.
- **Seeded data.** 6 calendar events and 5 reminders inserted with
  `ios/Tools/seed-demo-simulator.py`; Apple's 6 sample contacts ship with the
  simulator already.
- **Node pairing.** From the device log:

      [location-node] connected protocol=3-4 commands=location.get,
        calendar.events,reminders.list,contacts.search,photos.latest,
        music.nowPlaying,music.search,weather.forecast,device.status,…
      [node-pairing] inspected ownPending=0 ownPaired=1
      [node-pairing] exact native surface already approved
      [location-node] own command surface verified

  All 26 commands are registered and the surface is approved. The pairing fix
  in 1bf4c3a holds across a cold boot.

## What does not work

Two prompts, both of which should have hit `reminders.list`:

| Prompt | Reply |
| --- | --- |
| "What is on my plate today?" | "From what I can currently access, **nothing is recorded for today** — no saved tasks, reminders, or deadlines. I can't verify your calendar or inbox because no logged-in browser session is connected." |
| "List my open reminders." | "You have **no open reminders**. The two active scheduled jobs are internal maintenance tasks, not personal reminders." |

Both are wrong: there are 5 seeded reminders, 4 of them open, and 6 events.

## The evidence that pins it down

**iOS never asked for permission.** EventKit cannot read reminders or calendar
without a consent prompt, and the prompt never appeared. The simulator's TCC
database confirms it:

    sqlite3 …/data/Library/TCC/TCC.db \
      "select service,client,auth_value from access where client like '%operator%'"
    -- (no rows; 36 rows exist for other clients)

No TCC row means `EKEventStore` was never asked. So this is not a permission
denial, not a seeding failure, and not a query-window bug — **the connector was
never invoked at all**.

The second reply is the useful one: "the two active **scheduled jobs**" shows
the agent did call a tool — one of openclaw's own built-ins — and reached for
the scheduler when asked about reminders. It had tools; the node's commands
were not among them.

## Conclusion

The gap is between the gateway and the agent session, not in the connectors and
not in pairing. The node registers its 26 commands with the gateway and the
gateway approves the surface, but the model is not offered those commands as
tools, so it answers from its own built-ins and from general knowledge.

This is worth more than the video: every connector proof in
`ios-connectors-mac-checklist.md` Stage 3 would fail the same way, because none
of them can be exercised through the chat surface yet.

## Next

1. Find where the agent session builds its tool list and why node commands are
   absent. Pairing approval is evidently not sufficient.
2. Re-run the two prompts above. The pass condition is visible and cheap: iOS
   shows a Reminders permission prompt, and a TCC row appears for
   `app.operator.ios`.
3. Then record. Everything else for the take is ready.

## Notes for whoever picks this up

- Simulator automation works but is fussy. `cliclick` for taps, and always
  `du:x,y` (release) before `c:x,y` — a stale drag leaves iOS showing a text
  magnifier loupe that ruins a frame. Type with `osascript … keystroke`, not
  `cliclick t:`, which drops characters at this speed.
- The Simulator window must be activated before System Events can see it, or
  window lookup fails with `-1719`.
- Calendar/AddressBook stores were backed up before seeding; the seeder refuses
  to run twice.
