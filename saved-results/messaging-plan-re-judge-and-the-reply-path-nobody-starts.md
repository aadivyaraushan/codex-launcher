# The messaging plan, re-judged — and a send path that is finished except for its trigger

**Date:** 2026-08-03
**Plan judged:** `planning/operator-complete-messaging-plan.md`
**Why now:** the plan's own "Done when" list asks for it — *"Judge PASS — the plan
text passed; re-judge is needed now that the bet underneath it failed"*. The bet
was a headless Beeper Server running on the phone, and it failed
(`saved-results/beeper-server-phone-linux-spike.md`).

## Verdict

**PASS-WITH-FIXES, leaning FAIL.** A fresh judge with no access to my own
suspicions was given a write-the-rubric-first brief and returned six findings.
Four hold up, one I checked and disagree with, and one turned out to be much
more useful than the judge realised. I checked every one myself rather than
taking the report at face value.

## The finding that changes what to build next

The judge's cheapest recommendation was to drop the Linux-VM-and-Beeper route
for v1 and use **Android's own Direct Reply** — the reply box inside a
notification, which WhatsApp, Instagram, Messenger, Signal and SMS all put
there. One mechanism, no Linux VM, no third-party bridge, and no account-ban
risk beyond what the official app already carries. Its ceiling is real and
should be stated plainly: it can only answer inside a conversation that already
has an unread notification. It cannot start a new one.

I went looking for how much of that would have to be built, expecting most of
it. **Almost none of it.** The chain already exists, end to end, and every link
has a production caller:

```
  Mac                                        Phone
  ---                                        -----
  adapter returns DeviceWorkError   ??? ...  (nothing does this)
        |
  handOffToDevice                            handleDeviceAction
  handler.go:1062                            LauncherSessionViewModel.kt:1189
        |                                          |
   sends "device_action"  ------------------->  carryOutDeviceReply
   handler.go:1082                             LauncherApplication.kt:37
                                                   |
                                             DeviceReplyRequest.carryOut
                                             request/DeviceReplyRequest.kt:62
                                                   |
                                             ReplySender.send
                                                   |
                                             AndroidReplyDispatch
                                             RemoteInput.addResultsToIntent:73
                                                   |
                                             NotificationProbeService holds the
                                             live reply actions (:81, registered
                                             in AndroidManifest.xml:36)
```

Every box on that diagram is written, wired and unit-tested. The dotted arrow at
the top is the whole gap: **`DeviceWorkError` is never constructed anywhere in
the product.** Grepping the entire Go tree finds exactly one construction site
and it is a test fake, `internal/app/mobilesession/device_work_test.go:45`. The
type is declared at `internal/capability/adapter/device_work.go:20` and consumed
at `handler.go:999`, and in between, nothing produces one.

So the honest description of Direct Reply's status is not "an idea worth
evaluating" and not "already working". It is: **finished apart from the one
thing that starts it.** What is missing is a messaging adapter that, when asked
to send, returns `DeviceWorkError` instead of trying to do it from the Mac.

This is the fourth appearance of the house defect recorded in
`built-but-unreachable.md`, and a new sub-variant: not a class with no
constructor, but *a whole working pipeline with no first mover*. Unit tests are
no defence — the test fake supplies the missing trigger itself, so the suite is
green while the product cannot reach the code at all.

## The other findings, checked

| Judge's finding | Where it points | My check |
|---|---|---|
| No plan for the person on the *other* end, who never consented | `:147-153` covers only the sender linking their own account | **Holds.** The consent section is entirely about the sender. |
| No ban-recovery plan for the user's own account | risks table `:201-211` lists "Meta checkpoint / ToS" with mitigation "Consent; ban recovery" | **Holds.** "Ban recovery" is named as a mitigation and is written nowhere. |
| No guard against a hostile incoming message causing a send | — | **Holds.** Nothing in the plan addresses it. |
| "Invisible" setup understates the real cost | `:129-145` | **Holds.** The real chain is a Developer Options toggle, a multi-gigabyte Debian download, and a first-run installer. |
| "Paused, waiting for the owner" is too kind; call it dead for v1 | "Done when", and `beeper-server-phone-linux-spike.md:69` | **Holds.** Beeper ships one Linux build, an Electron/GTK desktop app. A real Debian VM fixes the C-library mismatch that killed Termux, but leaves three untested steps: the GTK packages have to be installed by hand, an Electron app has never been tried with no display, and Android's virtual-machine sandbox has never been tried against Electron's own sandbox. "One owner tap" hides all three. |
| The `:29` claim that the Desktop API "can agent-send IG … **Documented**" is unsupported | evidence table | **I disagree.** The cell says *Documented*, and links the vendor's documentation. It claims documentation, not proof, and the row directly beneath it labels the on-phone version **Unproven**. That is the plan being careful, not overclaiming. |

## What follows from this

1. Direct Reply is the v1 messaging path. It needs one adapter, not a new
   mechanism, and the missing piece is precisely named above.
2. Say in the plan what Direct Reply cannot do — start a conversation — rather
   than discovering it during a demo.
3. Write the three things the judge found missing: the non-consenting
   recipient, ban recovery, and the hostile-incoming-message guard.
4. Stop describing the Beeper route as paused. Record it as out of v1 with the
   three untested steps named, so the decision to revisit it is made with them
   in view.

## How to reproduce the reachability check

```bash
cd companion
rtk proxy grep -rn "DeviceWorkError" --include="*.go" .   # 6 hits, 1 constructor, in a test
```

`rtk proxy` is required — a bare `grep`/`go` command is rewritten by a shell
hook. A symbol whose only construction site sits in a `_test.go` file is not
built, however green the suite is.
