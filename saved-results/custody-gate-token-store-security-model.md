# Custody gate — the token store security model

**Date written:** 2026-07-31
**Author:** Claude (Opus 5), for Aadivya Raushan
**Status of the gate itself:** **NOT CLEARED.** This document is the *written model* the gate asks for. The gate clears when the model below is built, the key rotation has actually been run once, and someone has put their name on it. Today none of that has happened.
**What it gates:** no RT-1 / RT-2 / RT-3 adapter may hold a token belonging to anyone other than the owner (Aadivya) until this is cleared.
**Source:** `planning/consumer-app-implementation-plan.md`, the CUSTODY GATE block (lines 671–705).

---

## Why this document exists at all

The plan's own phrase for the official runtimes is "our cloud, our tokens." Spelled out, that means Operator holds live login credentials for Gmail, Calendar, Drive, Slack, Notion, Outlook, Spotify and Uber — for every user — in one place.

That store is the most valuable thing in the product to steal. A break-in there is not one bad day. It is somebody else reading dozens of services for the entire user base at once.

Everything else the plan builds protects against a *different* attacker. Consent screens protect the user from us over-reaching. Revoke proofs protect the user from us keeping data after they said stop. The kill switch protects users from a vendor turning hostile. **None of them protect anyone from the token store being breached.** That is the hole this document fills.

Two carve-outs, both deliberate:

- **The owner's own tokens are outside the gate.** Wave 0 connects Aadivya's own Notion workspace, his own Pixel and his own Mac before any of this is built, because there is no third party to protect yet. The gate binds at the first token that is not his.
- **Class B adapters are outside the gate, permanently.** WhatsApp, Instagram, Signal, iMessage and the rest run on the user's own hardware and their credentials never reach us. There is nothing for us to custody. This is the trade: the *official* routes are the convenient ones, and the price of that convenience is this document.

---

## What is actually being held

```
  WHAT WE HOLD                         WHY IT IS DANGEROUS
  ---------------------------------    ------------------------------------
  OAuth access token                   works right now, no password needed
  OAuth refresh token                  mints new access tokens for months,
                                       so stealing it is worse than
                                       stealing the access token
  the scopes that were granted         tells an attacker exactly what each
                                       stolen token can reach
  which user it belongs to             lets an attacker target a person
                                       rather than fish at random
```

Refresh tokens are the real prize. An access token dies in an hour. A refresh token keeps working until somebody revokes it, which is why the breach response below leans so hard on being able to revoke everything at once.

---

## The five things that clear the gate

### 1. Encryption at rest, with a key we can rotate — and rotate it once for real

```
  +-------------------+        +------------------+        +---------------+
  |  token, in the    |  --->  |  encrypt with a  |  --->  |  database row |
  |  clear, in memory |        |  data key        |        |  (ciphertext) |
  +-------------------+        +--------+---------+        +---------------+
                                        |
                                        | the data key itself is
                                        | encrypted by a master key
                                        v
                               +------------------+
                               |  key management  |  master key never
                               |  service (KMS)   |  leaves the KMS
                               +------------------+
```

Plain version: the token is scrambled before it is written down. The thing that unscrambles it is itself locked in a separate service that we can change the lock on without touching any of the stored rows.

**Rotation has to be exercised, not just designed.** A rotation path nobody has run is a rotation path that does not work. The one-time drill: create a second key, re-encrypt every row under it, confirm every adapter still connects, then destroy the first key. Write down how long it took and what broke. Until that drill has been run once, this item is not done.

**Decision needed from the owner:** which cloud, and therefore which key service. This is the first money-gate item inside the custody gate — a key service is a billed product, and per the money rule it needs the account named and approved before anything is created.

### 2. Per-user isolation — one adapter's bug cannot read another user's tokens

The failure this prevents: the Spotify adapter has an ordinary bug, asks for "the token" without properly scoping the question, and gets back a row belonging to somebody else.

The fix is not "be careful." It is that adapter code never queries the token store at all. It receives the one token it needs, already fetched, already decrypted, for the one user whose request is being served — and it has no way to ask for a second one.

```
  request for user U, adapter A
        |
        v
  +-------------------------------+
  | token broker                  |  the ONLY code that talks to the store
  |  - looks up exactly (U, A)    |
  |  - decrypts exactly that row  |
  |  - hands back one token       |
  +---------------+---------------+
                  |
                  v
        adapter A's execute(), holding one token,
        with no database handle of its own
```

Test that proves it: an adapter asked to act for user U, while user V's token also exists, can produce no code path that returns V's token. This is the same shape as the existing "a switched-off adapter is never routed to" test — a door that is closed structurally, not by remembering to check.

### 3. Least scope, always — and read-only really means read-only

Every adapter's manifest already declares its verbs. The scopes it requests at OAuth time must be derivable from those verbs, not hand-written per adapter, because hand-written scope lists drift upward and never drift back down.

Concretely: a Gmail adapter declaring only `read` requests Gmail's read-only scope. If somebody later adds `send` to its verb list, the scope request changes with it and the user is re-prompted. A scope that no declared verb needs is a bug, and should fail the adapter's own validation rather than reaching a consent screen.

This one is cheap and should be built with the first RT-2 adapter, not deferred.

### 4. An access path with an audit trail, and no standing production access

Nobody — including the owner — has ambient permission to read the token store. Getting at it requires a deliberate, logged, time-limited grant, and the log records who, when, which rows and why.

This sounds like process theatre until the breach happens, at which point it is the only way to answer "was this us or them." The rule that makes it real: **if a human can read a production token without leaving a record, the gate is not cleared,** regardless of what the encryption looks like.

### 5. A written breach response — and the revoke-at-scale path actually built

Three questions, answered in advance:

**How we detect it.** Alert on the shapes a break-in makes rather than on the break-in itself: token reads far above the normal rate, reads for many different users from one place, decryption attempts that fail, any access outside the audited path in item 4.

**How we mass-revoke.** This is the one to build, because most of it already exists. The per-user revoke path is proven by test today — it deletes the tokens, deletes the local state, then *re-reads both stores* and fails if anything survived. Revoke-at-scale is that same proven path run over everyone, which is exactly how it is built in `companion/internal/capability/consent`: `RevokeAll` loops the per-adapter path and returns one proof per adapter, and the test asserts every proof is complete and both stores are empty afterwards. Building it as a loop over the proven path — rather than as a faster bulk delete — is what stops the emergency path being the untested one.

What is still missing for a real breach: revoking at the *vendor* as well as locally. Deleting our copy of a refresh token stops us using it; it does not stop a thief who already has a copy. Each vendor's token-revocation endpoint has to be called too, and that is per-vendor work not yet done.

**How we tell people.** Who is notified, in what order, how fast, and what we tell them to do (change the password, check the service's own "recent activity" page). Drafted before it is needed, because it will not be written well at 3am.

---

## What "cleared" looks like

The gate clears when a dated file in `saved-results/` says all five are done, with a name on it, and specifically:

- [ ] Encryption at rest live, and the rotation drill run once with the result written down
- [ ] The token broker exists, adapters have no store access, and the isolation test passes
- [ ] Scopes derived from declared verbs, with a validation failure when they diverge
- [ ] Audited access path live, with no standing production access for anyone
- [ ] Breach response written; `RevokeAll` proven by test (**done** — see the consent package) and vendor-side revocation built (**not done**)

"We think it's probably fine" does not clear this gate, the same as the legal and distribution gates.

---

## How to redo or check this

The model above is a design, not a run — there is nothing to reproduce yet. To check the one piece that *is* built:

```bash
go test ./companion/internal/capability/consent/... -run Revoke -v
```

from the worktree root. That exercises the per-user revoke, the proof-by-re-read (including the case where a delete reports success and leaves the data behind), and the mass-revoke loop.
