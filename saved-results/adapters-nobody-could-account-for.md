# Three finished adapters were in neither list, and one id was in both

**Date:** 2026-08-03
**Where:** `companion/internal/capability/runtime/` and two adapter manifests, worktree `phase0-notification-probe`
**Plan item this came out of:** `planning/consumer-app-implementation-plan.md:1141` —
*"authorization gate + class H hand-off contract, with tests proving an
unofficial, unauthorized or under-scoped verb cannot execute"*

## The short version

`Inventory` describes itself as *"the honest record of what NewProduction
actually built: what went in, what was left out and why"*
(`production.go:129-133`). It was not honest in three ways:

1. **Notion was in neither list.** It declares `Auth: AuthOAuth`
   (`notion/notion.go:110`), so it can never come up unattended — but it was
   not in `Registered` and not in `Skipped`. The inventory did not mention it
   at all.
2. **Apple Notes and Apple Reminders were in neither list either**, for a
   different reason: they are macOS Automation adapters whose only
   construction anywhere is the owner-only `proveadapter` command. Two
   finished, tested adapters claiming `Ceiling: Completes` that no user can
   reach, with nothing anywhere saying so.
3. **"spotify" was in both lists at once**, with a reason that was false for
   the one that shipped.

The rule keeping OAuth adapters out was a hand-written list of seven adapter
ids (`production.go:284-294`). Nothing tied that list to the thing it tracks —
each adapter's own declared `Auth`. It was correct only as long as everyone
adding an OAuth adapter remembered to edit a list in a different package.
Notion is what happens when someone does not.

## Evidence

The failing tests, before any code changed:

```
--- FAIL: TestEveryAdapterThatNeedsASignInIsAccountedFor
    spotify is listed as both registered and skipped, and the skip reason
      does not explain that the registered one is the hand-off
    notion needs a browser sign-in but appears in neither Registered nor
      Skipped; the inventory does not account for it at all

--- FAIL: TestEveryAdapterIsRegisteredExplainedOrDeclaredUnshipped
    apple-notes is in none of the three honest states
    apple-reminders is in none of the three honest states
```

## The spotify case, which turned out not to be a bug

Two different adapters answer to the id `spotify`:

| | Auth | Ceiling | In the build? |
|---|---|---|---|
| `adapters/spotify` — the full-control API adapter | `oauth` | completes | no |
| `deeplink` Wave-1 spec (`deeplink/adapter.go:54`) | `none` | hands_off | **yes** |

The registry is keyed by id, so only one can be in it at a time. The hand-off
is what ships until the sign-in exists. **This is exactly the demotion the
plan's authorization gate describes** — *"a missing check or a separate acting
identity demotes the verb to class H hands_off"* — and it works. It had simply
never been written down or tested, and the inventory reported it as a flat
contradiction: `spotify: registered` next to `spotify: skipped, needs OAuth`.
Anyone debugging "can the user play Spotify?" got two opposite answers.

A test now pins the demotion itself: when an id carries both, whatever is
registered under it must declare `HandsOff`. It must not claim to complete
anything, because the user reads that claim as fact. It passed on the first
run — this is confirmation of behaviour that was already right, not a fix.

## Inputs → output → steps

**Input:** the production build with every credential it can accept supplied.
**Output before:** an inventory silently missing three adapters and
self-contradictory on a fourth.
**Output now:** every adapter in the repo is in exactly one of three honest
states.

The three states, which is the whole rule:

1. **Registered** — a user can reach it.
2. **In `Skipped` with a plain-English reason** — this build left it out.
3. **Its manifest declares `Unshipped`** — no build registers it.

Anything in none of the three is the defect: nobody reading the code can tell
whether it was left out deliberately or forgotten, and its passing unit tests
read exactly like a shipped adapter's.

State 3 is not new. `manifest.Unshipped` already existed, `maps_saved_places`
already used it (`maps/adapter.go:221-224`), and
`TestNothingMarkedUnshippedIsActuallyShipped` already gives it teeth by
refusing to let an adapter claim it is unshipped while production registers it.
The Apple adapters simply were not using it.

## What changed

- `production.go` — the seven-id list became an id→reason map with notion
  added, and spotify's reason now says the hand-off of the same id is
  registered in its place.
- `applenotes/notes.go`, `applereminders/reminders.go` — each now declares
  `Unshipped`, naming the real blocker: macOS refuses osascript access to
  Notes.app and Reminders.app until someone clicks Allow in the Automation
  privacy pane on the Mac, and an unattended serve has no way to get that
  click.
- `runtime/signin_accounted_for_test.go` — new; four tests plus a control.

**No behaviour changed for any user.** Nothing was registered or unregistered.
This makes what the build already does legible and keeps it that way.

## The part that cannot be fixed in code, and why the test is the fix

`NewProduction` cannot derive the skip list from the manifests, because those
adapters are never constructed in that build and an unbuilt adapter has no
manifest to ask. So the check lives in the test instead: it builds every
adapter in the repo, reads what each one declared, and fails if one that needs
a sign-in is missing from the map. Adding an eighth OAuth adapter and
forgetting the list now fails in CI instead of failing in front of a user —
which is precisely the failure `oauthReason` warns about: *"let the router
choose it, let the user confirm a preview, and only then discover — after they
had already said yes — that there was never a credential behind it."*

## Verified

Run by hand:

- `go build ./...` — ok
- `go vet ./...` — clean
- `go test -count=1 ./...` — **82 packages ok, 0 FAIL**

Four tests written before the code existed; three failed on behaviour (not on
a build error). Two controls: `TestKeyOnlyAdaptersAreStillRegistered` (an API
key is not a sign-in — refusing everything with a credential would pass every
other test and leave nothing but hand-off adapters) and the `needsSignIn == 0`
guard, which fails the test if the loop checked nothing.

`rtk proxy` is required. A bare `go test` is silently rewritten by a shell
hook into a cached path and has reported `ok` for a package with six failing
tests.

## The same bug elsewhere — the sweep

The category is *a hand-maintained list that has to stay in sync with a
declared field*.

| Candidate | Verdict |
|---|---|
| the OAuth skip list (`production.go:284`) | the bug → fixed here |
| the other three `Skipped` writes (maps, youtube, podcasts) | fine — each is set right where its own credential is checked, so it cannot drift |
| `classAddressing` (`production.go:49-62`) | fine — a class missing from it is left `AddressingUndeclared` and stage 2 refuses it rather than guessing |
| `cmd/proveadapter/main.go:236` (`applenotes.ID, notion.ID`) | an owner-only proof command; no user path |

Also checked and **not** a defect: Apple Notes and Reminders declare
`Platform: PlatformAndroid` while running on a Mac. `Platform` means which
phone the adapter serves, not which machine the code runs on
(`manifest.go:333`), and both files already carry a comment saying so.

## How to reproduce

```bash
cd companion
rtk proxy go test -count=1 -run 'SignIn|KeyOnly|HandOff|RegisteredExplained' \
  ./internal/capability/runtime/
```
