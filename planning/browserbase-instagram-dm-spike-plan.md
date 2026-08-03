# Browserbase Instagram DM spike

**Date:** 2026-08-02  
**Goal:** Prove one logged-in Instagram Web DM send via Browserbase; record whether we can see it land.  
**Not:** Operator product wiring, Kernel, main-account use.

## Inputs → Outputs → Algorithm

1. **Inputs:** `BROWSERBASE_API_KEY` + `BROWSERBASE_PROJECT_ID` in main `.env`; throwaway Instagram account (owner logs in via live view).  
2. **Outputs:** Evidence in `saved-results/browserbase-instagram-dm-spike.md` — session id, login ok/fail, send attempted, verify method, ban/challenge notes.  
3. **Algorithm:**
   1. Create Browserbase session (stealth on).
   2. Playwright connect over `connectUrl`.
   3. Open `instagram.com`; print live debug URL for owner login if needed.
   4. Navigate to a DM thread (pre-agreed test recipient or first thread).
   5. Type a unique probe string → Send.
   6. Read DOM / screenshot for “sent” signal.
   7. Close session; write evidence.

## Iteration cost

~10–20 min + one Browserbase session $. Fail fast at login/challenge before building send logic.

## Stop conditions

- Login challenge / hard block → stop, document.
- Send works → document selectors + verify path; decide product follow-up separately.
