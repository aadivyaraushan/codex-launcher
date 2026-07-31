# Draft-and-open — the user experience

**Date:** 2026-07-29
**Why this shape:** [phase0-ios-capability-ceiling.md](../saved-results/phase0-ios-capability-ceiling.md)
established that draft-and-open is the ceiling on iOS, so it's the floor
everywhere. Android's direct-send stays as upside on the apps that allow it.

Operator acts **only when told** — no background action. Everything below assumes
the user is present and has just asked for something.

---

## The two constraints that actually shape this

Not "can we send." Two smaller things decide how the whole thing feels:

1. **Getting context IN.** Android can read the notification, so it already knows
   Maya messaged you. iOS cannot read anything. On iOS the user has to bring the
   context.
2. **Getting text OUT.** No mainstream app accepts a pre-filled DM draft by deep
   link. Instagram will open a thread; it will not open a thread with your words
   already typed.

Constraint 1 is the bigger one, and it is the part people underestimate.

---

## Main flow

```
              +--------------------------------------+
              |  "Reply to Maya - yes to Friday"     |
              |  (voice or text, user-initiated)     |
              +------------------+-------------------+
                                 |
                                 v
                    +-------------------------+
                    |  Do we have the thread? |
                    +----+---------------+----+
                     YES |               | NO
                         |               |
        Android: read    |               |   iOS always.
        from the         |               |   Android when the app
        notification     |               |   isn't watched.
                         |               |
                         |               v
                         |     +---------------------------+
                         |     |  ASK THE USER FOR CONTEXT |
                         |     |  - they paste / say it    |
                         |     |  - or share a screenshot  |
                         |     |    into Operator          |
                         |     +-------------+-------------+
                         |                   |
                         +--------+----------+
                                  |
                                  v
                       +----------------------+
                       |   COMPOSE THE DRAFT  |     <---+
                       |   (model call)       |         |
                       +----------+-----------+         |
                                  |                     |
                                  v                     |
                       +----------------------+         |
                       |  SHOW IT. Always.    |         |  "make it
                       |  Never send unseen.  |---------+   shorter"
                       +----------+-----------+   edit
                                  |
                                  v
                    +--------------------------------+
                    |  Can we send this directly?    |
                    +----+----------------------+----+
                     YES |                      | NO
                         |                      |
          Android +      |                      |  iOS always.
          a reply box    |                      |  Instagram always.
          on the         |                      |
          notification   |                      |
                         v                      v
            +------------------------+  +------------------------+
            |  SEND IT               |  |  HAND OFF              |
            |  fire the notification |  |  1. draft -> clipboard |
            |  reply action          |  |  2. open the thread    |
            |  app never opens       |  |  3. user pastes, sends |
            +-----------+------------+  +-----------+------------+
                        |                           |
                        v                           v
            +------------------------+  +------------------------+
            |  "Sent to Maya."       |  |  "Copied. Opening      |
            |  Operator KNOWS.       |  |   Instagram - paste    |
            |                        |  |   and send."           |
            |                        |  |  Operator does NOT     |
            |                        |  |  know if it went.      |
            +------------------------+  +------------------------+
```

---

## Where each platform lands

```
                    |  CONTEXT IN         |  TEXT OUT
  ------------------+---------------------+------------------------
  Android           |  automatic          |  direct send
  + Messages        |  (notification)     |  (proven working)
  ------------------+---------------------+------------------------
  Android           |  automatic          |  hand off
  + Instagram       |  (notification)     |  (no reply action)
  ------------------+---------------------+------------------------
  Android           |  automatic          |  probably direct
  + WhatsApp        |  (notification)     |  (UNTESTED)
  ------------------+---------------------+------------------------
  iOS               |  user must supply   |  hand off
  + anything        |  it                 |
```

The best cell and the worst cell are both real, and the UI has to make it
obvious which one the user is in — otherwise they won't know whether their
message actually went out.

---

## The handoff, honestly

This is the unglamorous part, and it's where the experience is won or lost.

```
  BASELINE (works today, feels like duct tape)

    Operator                     Instagram
    +----------------+           +----------------+
    | draft shown    |           |                |
    | [Copy & Open]  |---------->| thread open,   |
    +----------------+           | empty box      |
     copies to clipboard         |                |
     opens ig.me/m/<user>        | user long-      |
                                 | presses, pastes |
                                 | user taps send  |
                                 +----------------+

    Taps after handoff: 3 (long-press, paste, send)
```

```
  BETTER (Operator ships a keyboard)

    Operator                     Instagram
    +----------------+           +--------------------+
    | draft shown    |           | thread open        |
    | [Open]         |---------->| Operator keyboard  |
    +----------------+           | shows the draft on |
                                 | a suggestion bar   |
                                 |                    |
                                 | user taps it ->    |
                                 | text inserted      |
                                 | user taps send     |
                                 +--------------------+

    Taps after handoff: 2 (insert, send)
```

A custom keyboard is the one place you're allowed to live *inside* another app,
on both platforms. It turns the handoff from "paste this yourself" into
something that looks designed. Cost: iOS keyboards need **Allow Full Access** to
talk to the host app, which is a scary-sounding permission on a product already
asking to read your messages.

---

## The confirmation problem

On the handoff path Operator goes blind at the moment of send. Three options,
none free:

```
  A. Say nothing        "Opening Instagram."
                        Honest, but the task just dangles.

  B. Ask afterwards     On return: "Did that send?"  [Yes] [No]
                        Accurate, but nags every single time.

  C. Watch for it       Android only: the outgoing message changes the
     (Android only)     thread, so the next notification can confirm it.
                        iOS has no equivalent. Nothing to watch.
```

C is the good one and it only exists on the platform that already had the good
path. So the cross-platform answer is A or B.

---

## Decisions I need from you

1. **Keyboard or clipboard?** The keyboard is clearly better UX and clearly more
   work, plus one alarming iOS permission. Clipboard ships this week.
2. **Confirmation: A or B?** Silent handoff, or ask on return.
3. **On iOS, how does context get in?** Typing it out is tedious. A screenshot
   into a vision model is one tap and reads the whole thread. Screenshot is my
   guess at the right answer but it's a real cost and product call.

---

## Needs verifying before building

- `https://ig.me/m/<username>` opening a specific DM thread — widely used for
  "message me" buttons, **not tested by us** on either platform.
- Whether Instagram's iOS URL scheme accepts any pre-filled text (assumed no).
- WhatsApp's reply action on Android — untested, needs one inbound message.
  WhatsApp *does* support `wa.me/<number>?text=` with pre-filled text, which
  would make it the one app where the handoff is genuinely one tap.
