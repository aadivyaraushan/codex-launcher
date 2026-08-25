# Personal Google Play developer account setup

**Date:** 2026-08-02  
**For:** Creating the owner's personal Google Play developer account through the in-app browser.

```text
Google's official Play Console signup page
                    |
                    v
         User signs in if required
                    |
                    v
      Verify current fields, fee, and rules
                    |
                    v
      Assistant completes safe form fields
                    |
                    v
 User reviews legal, identity, and payment steps
                    |
                    v
       Verify the account status in Console
```

## Observable done

- The in-app browser is on Google's official Play Console developer-account route.
- The account type is personal, unless the owner explicitly changes that choice.
- Every entered fact comes from the signed-in account, the owner, or the visible Google form; nothing personal is guessed.
- The owner personally handles credentials, verification codes, identity checks, legal acceptance, and payment confirmation.
- The current fee and account requirements are read from Google's live pages before payment or submission.
- After submission, the visible Play Console status is checked and reported as active, pending verification, or blocked.
- No app is uploaded, submitted, or published as part of this task.

## Work sequence

1. Connect only to the requested in-app browser and open Google's official Play Console signup route.
2. Check the page domain and read the live account-type, fee, verification, and required-field information.
3. Pause for the owner to sign in or create Google credentials if Google requests it.
4. Select a personal developer account and complete non-sensitive fields using verified information already visible or supplied by the owner.
5. Pause before any legal acceptance, identity or phone verification, payment confirmation, or final account-creation action that Google requires the owner to perform.
6. After the owner completes each gate, continue through the remaining safe fields and checks.
7. Inspect the resulting Play Console page and record the exact account status and any remaining Google review or verification step.
8. Have a separate judge check the non-sensitive result against the requested scope and confirm that no app-release work or unsupported claim was added.

## User-controlled gates

```text
Navigation and ordinary form entry ----------> assistant
Google credentials and verification codes ---> owner
Truthful personal/profile choices ------------> owner if not already visible
Legal terms and identity verification --------> owner
Payment details and final payment approval ---> owner
Final account creation, if legally binding ---> owner review and action
App upload, testing, or publication ----------> out of scope
```

## Evidence and privacy

- Use only official Google pages for account creation.
- Do not read or expose stored passwords, cookies, local storage, payment details, identity documents, or verification codes.
- Do not save personal data or screenshots containing sensitive information in the repository.
- Record only the final non-sensitive status and any next action Google displays.
- Treat any current fee or eligibility statement as unverified until it is read from Google's live page during this run.

## Stop conditions

- The domain is not an official Google domain.
- The requested personal-account route is unavailable or Google requires a materially different account type.
- The live fee is not explicitly approved by the owner before payment.
- A field requires personal information that is neither visible nor supplied by the owner.
- Google requests credentials, a verification code, identity action, legal acceptance, payment confirmation, or another step that only the owner may complete.
