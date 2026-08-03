# Operator agent billing account

**Recorded:** 2026-07-31  
**Owner:** Aadivya  
**Purpose:** Auditable approval for paid OpenAI router calls during Operator development.

## Approved account

- Provider: OpenAI
- Account email: `ssdear@gmail.com`
- Organization: `org-oC0Cx9jwKVEEvRlRlqdQzTwE`
- Account kind: personal
- Approved ceiling: $500 per month
- Practical hard stop observed by the owner: about $50 in prepaid credit
- Credential source verified on 2026-07-31: the main checkout's gitignored `.env`

The API key itself is not copied into this file or committed. A worktree that
needs a live call must read the approved credential from that existing local
source without printing it.

## Scope of this approval

This approval covers the small number of OpenAI calls needed to build and test
the Wave 1 router. Offline fixtures remain the default. Every live run records
its model, request count, token use when returned by the API, and estimated cost.

This does not approve Kernel, proxy vendors, developer-program fees, or any
other paid service. Those need a named account and separate approval before use.

## Registration and sign-in owner

Aadivya owns developer registrations and recurring test-account sign-ins. The
implementation may pause for Aadivya when a real OAuth browser approval is
needed; it must not store account passwords in the repository.
