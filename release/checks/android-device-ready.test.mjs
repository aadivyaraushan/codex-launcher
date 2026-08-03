import assert from "node:assert/strict";
import test from "node:test";
import { keyguardVerdict } from "./android-device-ready.mjs";

// Why this check exists, measured on a Pixel 9 on 2026-08-03.
//
// The connected suite does not work on a locked phone, and it does not say so.
// Every Compose test waits for a hierarchy that cannot attach while the lock
// screen is on top, so a locked phone produces a wall of five-second timeouts
// that read exactly like flaky tests. Same build, same class, one difference:
//
//   phone unlocked   UiScenarioActivityTest   23 tests / 0 failures  (x3)
//   phone locked     UiScenarioActivityTest   23 tests / 22 failures
//
// Inside the full 128-test run the phone locks part way through, so the
// failures start at a different test every time and look like load. They are
// not load. Nobody can debug that from the output, so the run should stop at
// the door and say which of the two things is wrong.

const lockedDumpsys = `
  WINDOW MANAGER POLICY STATE
    mSensor=null
    KeyguardServiceDelegate
      showing=true
      inputRestricted=true
      occluded=false
`;

const unlockedDumpsys = `
  WINDOW MANAGER POLICY STATE
    mSensor=null
    KeyguardServiceDelegate
      showing=false
      inputRestricted=false
      occluded=false
`;

test("a locked phone is reported as locked, with the reason a person can act on", () => {
  const verdict = keyguardVerdict(lockedDumpsys);
  assert.equal(verdict.state, "locked");
  assert.match(verdict.message, /unlock/i);
});

test("an unlocked phone is cleared to run", () => {
  assert.equal(keyguardVerdict(unlockedDumpsys).state, "unlocked");
});

test("a phone showing an activity over the lock screen is still locked", () => {
  // showWhenLocked lets the debug scenario activity draw over the keyguard, so
  // occluded flips to true while the keyguard is still up. That helps exactly
  // one test class and nothing else, so it must not read as unlocked.
  const occluded = lockedDumpsys.replace("occluded=false", "occluded=true");
  assert.equal(keyguardVerdict(occluded).state, "locked");
});

test("an unreadable dump is unknown, never a pass", () => {
  // The house rule: don't-know is its own answer. Reporting "unlocked" because
  // the text could not be parsed is how a gate stops being a gate.
  for (const text of ["", "adb: device offline", "WINDOW MANAGER POLICY STATE\n  mSensor=null\n"]) {
    const verdict = keyguardVerdict(text);
    assert.equal(verdict.state, "unknown", JSON.stringify(text));
    assert.match(verdict.message, /could not tell|unknown/i);
  }
});

test("only the keyguard's own state counts, not every line with the word showing", () => {
  // `dumpsys window` says "showing" in several unrelated places. Reading the
  // first one found would make this check answer a different question than the
  // one it was asked.
  const noisy = `
  mShowingDream=false mDreamingLockscreen=true
  Window{abc} isShowing=true
  KeyguardServiceDelegate
    showing=false
    occluded=false
`;
  assert.equal(keyguardVerdict(noisy).state, "unlocked");

  const noisyLocked = noisy.replace("showing=false", "showing=true");
  assert.equal(keyguardVerdict(noisyLocked).state, "locked");
});

test("the scan stops at the end of the keyguard's own block", () => {
  // The case above only put noise *before* the block, which a scan that never
  // stops would still pass. This is the other side: a keyguard block that says
  // nothing about showing, followed by a later block that does. Reading on
  // would answer with some other component's flag and call a locked phone
  // ready — the exact failure this check exists to prevent.
  const dump = `
  KeyguardServiceDelegate
    inputRestricted=true
    occluded=false
  SomeOtherService
    showing=false
`;
  assert.equal(keyguardVerdict(dump).state, "unknown");
});
