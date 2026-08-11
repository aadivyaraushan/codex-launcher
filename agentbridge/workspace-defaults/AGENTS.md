# Rules for your phone agent

This file is yours to edit — plain text, any editor. The agent reads it at
the start of every turn and follows it as instructions. Uncomment a line
(delete the `<!--` and `-->`) to turn it on, or write your own rules in
ordinary sentences.

Two layers keep you in control:

1. **These rules** steer the agent's judgment. It follows them, but they are
   instructions, not physics.
2. **Hard gates** are enforced outside the agent, in the launcher itself, and
   nothing written here can loosen them. The agent always stops and asks you
   before: messaging someone it has never messaged before, revoking or
   disconnecting a service, doing anything irreversible that you haven't
   allow-listed below, and sending anything outbound in a turn where it also
   read data from a different service (the classic trick an attacker's
   message would try). Approval happens only by tapping Approve on the
   launcher's sheet — typing "yes" in chat never releases a gate.

## Tone

<!-- Reply in my voice: casual, short, no emoji. -->
<!-- Sign off messages to my family with "love you". -->

## Who you may talk to

<!-- You may reply freely to: (list people, one per line) -->
<!-- Never initiate conversation with anyone not listed above. -->

## Allow-listed irreversible actions

Anything listed here skips the "irreversible action" gate (the other gates
still apply).

<!-- Deleting my own drafts. -->

## Heartbeat

What to do when you wake up on a timer and nothing has happened.

<!-- Check for unread messages and summarize anything urgent to me. -->
<!-- Otherwise: do nothing and go back to sleep. -->

## Quiet hours

<!-- Between 23:00 and 08:00: never send messages; queue anything -->
<!-- non-urgent and tell me about it in the morning. -->
