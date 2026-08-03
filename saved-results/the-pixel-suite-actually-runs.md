# The Pixel suite actually runs — 101 of 130 pass

**Date:** 2026-08-03
**Status:** Measured on the real Pixel 9. **This replaces the earlier
"owner-blocked, needs the phone" reading of the operator plan's Pixel row.**
**What this is for:** the plan said the connected suite could not run because the
lock screen was up, and that the only thing needed was one physical act from the
owner. The phone was unlocked, the suite ran, and the earlier diagnosis turns
out to have been wrong about what the failures were.

---

## The numbers

Counted from the JUnit XML
(`app/build/outputs/androidTest-results/connected/debug/TEST-Pixel 9 - 16-_app-.xml`),
not from Gradle's wording:

| run | tests | failures | errors |
|---|---|---|---|
| earlier, phone locked | 129 | **67** | 0 |
| this run, phone unlocked | 130 | **29** | 0 |

101 tests pass on a real phone. That is the first time any of them have.

Device state at the start, verified before launching: `adb devices` →
`4B230DLAQ001Z5 device`; `dumpsys window` → `isKeyguardShowing=false`,
`mKeyguardOccluded=false`, `mAwake=true`, `mScreenOnFully=true`. So the phone was
genuinely unlocked, not merely awake — which is the distinction the earlier
measurement got wrong twice.

By the end of the run the phone had re-locked (`isKeyguardShowing=true`).
`screen_off_timeout` is 1800000 (30 minutes) and the run took 4m24s, so an idle
timeout did not do it. What did is not established.

## The old explanation is dead

The plan's Pixel row said: "every class that draws a screen failed all of its
tests, every class that draws nothing passed all of its tests." That was a clean
story and it is now false. Seven Compose-drawing classes passed completely:

| class | result |
|---|---|
| `UiScenarioActivityTest` | 25 / 25 |
| `TaskScreenTest` | 15 / 15 |
| `LauncherActivityTest` | 7 / 7 |
| `PairingScreenTest` | 4 / 4 |
| `CapabilitySheetTest` | 3 / 3 |
| `AppearanceScreenTest` | 3 / 3 |
| `ProjectSelectorTest` | 3 / 3 |

Compose attaches a hierarchy on this phone. Whatever is wrong is narrower than
"the keyguard is on top of everything".

## Where the 29 failures actually are

27 of them are `java.lang.IllegalStateException: No compose hierarchies found in
the app`, and they sit in exactly three classes:

| class | fail / total |
|---|---|
| `launcher.home.HomeScreenTest` | 19 / 20 |
| `launcher.apps.AppDrawerScreenTest` | 4 / 4 |
| `decision.DecisionSheetsTest` | 4 / 4 |

**One HomeScreenTest passed.** That single test is the most informative data
point available — whatever it does differently is probably the whole answer.

The other two failures are different exceptions:

- `CodexConnectionServiceTest > serviceSurvivesScreenOffAndDozeWhileObservingTheDefaultNetwork`
  — `AssertionError: Timed out waiting for device in deep idle;
  snapshot=Snapshot(running=true, createCount=1, destroyCount=0`. The other 6
  tests in that class passed. This one forces the device into deep doze, so it
  is device-state-dependent by design.
- `InstalledAppsRepositoryInstrumentedTest > realLauncherAppsSourceEnumeratesAndLaunchesAndroidSettings`
  — `ComparisonFailure: expected:<com.android.settings> but was:<com.android.systemui>`
  (`InstalledAppsRepositoryInstrumentedTest.kt:79`). It launches Android Settings
  and asserts the resumed package. Getting `systemui` instead is consistent with
  something system-level sitting on top at that moment — plausibly the same
  thing that re-locked the phone. **Not concluded**, because I could not re-run.

## The lock-screen explanation is ruled out — from logcat

The obvious theory is that the phone re-locked partway through and the three
classes ran inside the locked window. It has a good argument behind it: the
arithmetic fits exactly (4 + 4 + 19 = 27), and two classes failing 100% with one
failing 19/20 is the signature of a contiguous time window. It is still wrong,
and logcat says so.

Establishing the clock first, because the two sources disagree by four hours:
`adb shell date` and the host `date` both read `21:17:26 +04`, so the phone and
the Mac agree. The JUnit XML stamps **UTC** — its `16:48:57` is device-local
`20:48:57`.

The run therefore occupied **20:48:57 → ~20:53:21** (4m24s). And from
`adb logcat -b events`:

```
20:48:25.958  wm_set_keyguard_shown [0,0,0,1,0,setKeyguardShown]   <- keyguard DOWN
20:53:21      (run ends)
21:07:26.193  screen_toggled 0                                      <- screen off
21:07:27.545  wm_set_keyguard_shown [0,1,1,0,0,setKeyguardShown]   <- keyguard UP
```

The keyguard came down 32 seconds before the suite started and did not come back
up until **fourteen minutes after it finished**. The whole run sits inside the
unlocked window.

**So these 27 failures are a real defect, not a device-state artifact.** That is
the opposite of what every previous measurement of this suite concluded, and it
is the first time the question has been settled with a timestamp rather than an
inference.

## What the failures actually say

The full exception, from the XML rather than its first line:

> `IllegalStateException: No compose hierarchies found in the app. Possible
> reasons include: (1) the Activity that calls setContent did not launch;
> (2) setContent was not called; (3) setContent was called before the
> ComposeTestRule ran.`

Reason (2) is ruled out by reading the tests: the one passing HomeScreenTest
(`HomeScreenTest.kt:297-311`) and its 19 failing siblings all call
`compose.setContent { … }` in the same shape, and all three failing classes use
the same rule — `androidx.compose.ui.test.junit4.v2.createComposeRule()` — as
five of the classes that passed completely. So the rule type and the call
pattern are identical on both sides of the split.

That leaves reason (1): the `ComponentActivity` the rule launches did not start.
**Why it starts for `TaskScreenTest` and not for `HomeScreenTest` is not yet
established**, and guessing at it here would be the same error as the lock-screen
theory. It needs one more unlocked run scoped to the three classes.

## What I could not determine

~~Whether the three failing classes failed *because* the phone re-locked partway
through.~~ **Answered above from logcat: they did not — the phone was unlocked
throughout.** The XML stamps every suite with the same `timestamp`, so the
suites still cannot be ordered from the results file; logcat was the way in.

Still open: why the rule's `ComponentActivity` launches for some classes and not
others.

## What this changes for the plan

The row was filed as "owner-blocked — needs the phone, and nothing in the
codebase is in the way." Both halves of that need revising:

- The owner act it was waiting on **has now happened**, and the suite ran.
- "Nothing in the codebase is in the way" is no longer supported. Three specific
  test classes fail while seven comparable ones pass, which points at something
  those three share — a test-side or app-state problem that is agent-diagnosable
  without the phone.

The remaining ask of the owner is smaller and more specific than before: keep
the phone unlocked for the length of one 5-minute run so the three classes can
be re-measured in a known-good state.

## Amended later the same day — one test was turning the screen off

A second full run was done, and it came out **130 / 51 / 0** — nearly double the
first run's 29 failures, from the same build. `TaskScreenTest` flipped from 15/15
passing to 15/15 failing. The same three classes that failed in run 1, run on
their own, passed **28 of 28**. So the result depends on what ran before what,
which is the signature of one test changing something the next test needs.

It is `CodexConnectionServiceTest.serviceSurvivesScreenOffAndDozeWhileObservingTheDefaultNetwork`.
It sends `input keyevent KEYCODE_SLEEP` to turn the screen off. Its cleanup
cannot undo that on this phone: `KEYCODE_WAKEUP` turns the screen back on but
leaves it locked, and `wm dismiss-keyguard` does nothing to a secure keyguard —
already measured here and written into the operator plan's Pixel row.

logcat caught it happening at the start of run 2:

```
21:34:33.935  screen_toggled 0
21:34:35.264  wm_set_keyguard_shown [0,1,1,0,0,setKeyguardShown]   <- keyguard UP
```

and `isKeyguardShowing=true` afterwards. From that point the lock screen sat on
top of every activity, so each Compose test after it failed with "No compose
hierarchies found in the app".

**The test could never have passed.** It fails with "Timed out waiting for device
in deep idle" in every run on record. Measured directly on the device today:

```
dumpsys battery unplug ; cmd deviceidle force-idle
  -> Unable to go deep idle; stopped at INACTIVE
```

Android will not enter deep or light doze while the screen is on — unplugging is
not enough. So there is no version of this test that both checks doze and leaves
the screen alone. It is now marked as not-run, with the reason written into it,
and a JVM guard test (`NoTestMayLockThePhoneTest`) fails the build if any
instrumented test reintroduces `KEYCODE_SLEEP` or `wm dismiss-keyguard`.

Checked for the same problem elsewhere: grepped all instrumented tests for
`executeShellCommand`, `settings put`, `input keyevent`, `deviceidle`, `wm ` and
`dumpsys battery` — 5 call sites, this was the only one that changes device-wide
state. The other four are `cmd package query-activities` (reads only), two
`am start` calls launching this app's own activities, and `KEYCODE_BACK` for
in-app navigation.

### Confirmed on the phone, with the phone still locked

The rest of `CodexConnectionServiceTest` draws no screen, so the class can be run
on its own against a locked phone. After the change:

| | before | after |
|---|---|---|
| tests / failures / errors | 7 / 1 / 0 | **7 / 0 / 0** (2 skipped) |
| `isKeyguardShowing` before run | — | true |
| `isKeyguardShowing` after run | — | **true (unchanged)** |

The lock state being identical either side is the whole point — this class no
longer moves it. The 2 skips reconcile exactly against the only two skip
mechanisms in the file: the new `@Ignore`, and a pre-existing `assumeTrue` at
`CodexConnectionServiceTest.kt:158` gating an airplane-mode test on an external
cycle, which predates this work.

One trap worth recording: Gradle printed **"BUILD SUCCESSFUL in 10s … 66
up-to-date"**, which is exactly what a cached non-run looks like. It really did
execute — the XML `timestamp` proves it. Read the XML, never the wording.

### What this does and does not explain

**Run 2 is settled.** logcat puts the keyguard up for that whole run.

**Run 1 is probably the same cause, one step earlier — but that is a prediction,
not a measurement.** The section above ("Where the 29 failures actually are")
treats those 27 failures as a defect belonging to the three classes. That reading
does not survive the isolation run: **those same three classes, run on their own
against an unlocked phone, were 28 tests / 0 failures / 0 errors.** A class that
passes alone and fails in company is not broken — something ahead of it is. And
the screen-off test ran in run 1 too; that is what its "timed out waiting for
deep idle" failure in run 1 *is*.

The loose end is that logcat showed no keyguard event inside run 1's window. Two
ordinary explanations cover it without inventing a second cause:

1. **The screen being off is enough by itself.** An activity does not resume with
   the display off, and "the Activity that calls setContent did not launch" is
   exactly what all 27 failures report. The keyguard never has to rise.
2. **The run-1 logcat query was `tail`-truncated**, so it could not have shown a
   `screen_toggled 0` in the middle of the window even if one happened.

Add that the phone does not hold itself awake here — an earlier measurement in
the plan found it back to `mAwake=false` by the very next command — so it goes
dark again shortly after any wake.

| | run 1 (29 failures) | run 2 (51 failures) | three classes alone |
|---|---|---|---|
| keyguard up during run? | no (logcat) | yes (logcat) | no |
| screen-off test ran? | yes (it timed out) | yes (it timed out) | **no** |
| failures in those 3 classes | 27 | — | **0** |

The bottom row is the one that matters: remove the screen-off test from the
picture and the failures go away.

**The prediction to test, stated so it can be wrong:** with the screen-off test no
longer running, a full unlocked suite should match the isolated result, and
`HomeScreenTest`, `AppDrawerScreenTest` and `DecisionSheetsTest` should pass
*inside the full run*. If they still fail, the "real defect in three classes"
reading was right after all and this section is wrong. That run has not happened
— the phone re-locked (`isKeyguardShowing=true`) before it could.

## How to re-check

```bash
cd android
./scripts/../../scripts/pixel-lock.sh 60 adb shell dumpsys window | grep isKeyguardShowing   # must be false
rm -rf app/build/outputs/androidTest-results
./gradlew :app:connectedDebugAndroidTest --rerun-tasks
python3 - <<'PY'
import re,collections
s=open('app/build/outputs/androidTest-results/connected/debug/TEST-Pixel 9 - 16-_app-.xml').read()
print(re.search(r'tests="(\d+)"\s+failures="(\d+)"\s+errors="(\d+)"',s).groups())
c=collections.Counter()
for m in re.finditer(r'<failure[^>]*>(.*?)</failure>',s,re.S):
    c[[l.strip() for l in m.group(1).split('\n') if l.strip()][0][:120]]+=1
for k,n in c.most_common(): print(n,k)
PY
```
