# Wave 1 Todoist adapter-only run record

**Date:** 2026-07-31  
**Purpose:** Retain the exact non-secret evidence from the first confirmed
Todoist create/read/revoke run.

## Inputs

- Command: `go run ./companion/cmd/proveadapter todoist`
- Task title: `Operator Wave 1 Todoist proof 20260731T163811.894573000Z`
- Description: `Created by the confirmed Wave 1 RT-2 proving run`
- OAuth authorization URL: omitted because its state and PKCE verifier were
  single-use authentication material.

## Reconstructed run summary

The lines below combine the proof command's printed result lines with the
before/after counts observed in the Todoist client logs. They are a
secret-free reconstruction, not a byte-for-byte terminal transcript.

```text
Todoist tasks before write: 31
Exact title matches before write: 0
WRITE: reached=completes done=true detail=created Todoist task 6h9crRHgqHGjgxp8
Todoist tasks after write: 32
Exact title matches after write: 1
READ-BACK: reached=completes done=true
REVOKE: in-memory access token cleared and adapter removed from the registry
VERDICT: todoist RT-2 proven by confirmed create, read-back, and revoke
```

## Result

Exactly one matching task was present after the write. Its Todoist id was
`6h9crRHgqHGjgxp8`. The adapter-only checkpoint passed; this record does not
claim that the physical Pixel path ran.

## Reuse

Run the same command from the isolated worktree. The proof first lists Todoist
for the exact unique title: zero matches allows one create, one match recovers
without another create, and more than one match stops the run.
