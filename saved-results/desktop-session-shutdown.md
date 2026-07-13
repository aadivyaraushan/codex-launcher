# Desktop Session Shutdown

**Date:** 2026-07-13

## Purpose

The verified ChatGPT Desktop connector already failed closed and reconnected
after an unplanned disconnect, but the future companion runtime had no public
way to stop a healthy session it owned.

This checkpoint adds the minimum owner lifecycle:

- `Client.Close()` uses the same one-time teardown gate as failure handling;
- planned close records `ErrDisconnected` for blocked callers, closes the
  verified local socket, and closes the session completion channel;
- repeated close is safe and returns the stored socket-close result;
- every request checks the stopped state immediately after taking the request
  lock, before it registers state or starts a socket write;
- queued request writes and automatic discovery replies check the stopped state
  again while holding the socket-write lock;
- planned close logs `decision=owner_closed` at info level rather than an error;
  and
- `SessionConnector.Close()` closes and removes the cached client, so the next
  `Connect` must create and initialize a fresh session.

No raw Desktop frame, task title, path, prompt, reply, or credential is added to
the shutdown logs.

## TDD evidence

The first focused test run failed to compile because neither `Client.Close` nor
`SessionConnector.Close` existed. After implementation, both lifecycle tests
passed under the race detector:

```text
TestClientCloseIsRepeatableAndReportsPlannedShutdown: PASS
TestClosedClientReturnsStoredCloseErrorAndNeverWritesAgain: PASS
TestRequestQueuedForSocketWriteStopsBeforeWritingAfterClose: PASS
TestDiscoveryResponseNeverWritesAfterClose: PASS
TestBlockedRequestWakesWhenOwnerClosesClient: PASS
TestConcurrentFailureAndOwnerCloseTearDownExactlyOnce: PASS
TestConnectorCloseForcesTheNextConnectToUseAFreshSession: PASS
```

The tests use `net.Pipe` plus a close-tracking wrapper. They prove socket close,
session completion, repeatable close, planned logging, cached-client removal,
and a second fresh initialization after owner shutdown. A connection whose
`Close` returns an error but still accepts `Write` proves that future actions
return `not_sent` and write zero bytes. Additional race tests prove a blocked
request wakes on close and concurrent failure versus owner close tears down the
socket exactly once.

The same-bug search covered every direct and tracked Desktop `writeFrame` call.
It found two sibling paths: a request already queued for the socket-write lock
and the automatic Desktop discovery rejection. Their red tests initially
returned `outcome_unknown` or wrote a reply after close. Both now skip the
write. An atomic `write began` marker reports `not_sent` only when shutdown won
before writing; it preserves `outcome_unknown` once a write could have crossed
the socket boundary.

The first independent review found the post-shutdown write gap and the missing
hard lifecycle tests. The new regression test failed with `outcome_unknown`
before the request-lock stopped-state check was added, then all five focused
shutdown tests passed under the race detector.

Final verification:

```text
go test -race ./companion/...: PASS
go vet ./companion/...: PASS
git diff --check: PASS
Independent seven-test race run, 100 repetitions: PASS
```

The final independent review found no P1, P2, or P3 issues and marked the
checkpoint ready.

## Still pending

- The runnable companion still needs to compose this connector with the owned
  local app-server process and close both when its service context ends.
- Windows still needs its production verified named-pipe connector; the current
  production Desktop connector is macOS-only.
- No live Desktop write or model call was made in this checkpoint.
