# Change the Landing Page to Start Setup

**Status:** Approved — adversarial review PASS

**Updated:** 2026-07-19
**Purpose:** Replace the waitlist-focused landing page with a low-friction, honest path that lets a technical user hand a fixed setup prompt to Codex and finish installing Codex Launcher.

## User Journey

```text
landing page
    |
    +-- primary: Copy setup prompt -----------------------------------+
    |                                                                |
    |        paste into Codex                                        |
    |              |                                                 |
    |        verified bootstrap                                      |
    |              |                                                 |
    |        choose connection                                       |
    |          /         \                                           |
    |  Tailscale         Fly relay                                   |
    |  free/default      optional/paid                               |
    |          \         /                                           |
    |      install companion + verify APK + pair phone + doctor pass |
    |                                                                |
    +-- secondary: Download Android app                               |
    +-- tertiary: Manual setup / Use with another VPN                 |
    +-- optional: “Using it?” email + consent + ownership confirmation
```

The copy action is not the finish line. Success means a user reaches an online phone connected to a working companion.

## Launch Dependency

Do not replace the waitlist call to action until all of these are true:

- The Tailscale plan is implemented and its physical-phone default path passes.
- The optional Fly path remains supported and its upgrade regression passes.
- A fixed, versioned setup/bootstrap command is published at an anonymously accessible URL.
- Companion and Android artifacts, checksums, and attestations for that version are public and verified.
- The copied prompt has been run from a clean supported Mac through an online Android app.
- Support and rollback instructions are published.

Until that gate passes, the live site keeps the waitlist. The code can be prepared behind a deployment branch, but the public CTA must not promise an unavailable install. At launch, setup replaces the waitlist as the primary journey while a clearly optional email form remains for self-identified users who want updates and may be contacted for feedback.

## Decisions Already Settled

- Primary action: **Copy setup prompt**.
- Tailscale is the default connection path because it is free and simpler to start.
- Fly is an explicit paid option for people who need another VPN at the same time.
- No automatic network fallback and no surprise billing.
- The prompt guides Codex through a fixed installer and verification contract; it does not ask the model to invent shell commands.
- The initial prompt targets Codex, which is already required by the product. Broader agent support is a later decision.
- Email collection remains, but no longer gates setup and is not labeled a waitlist.
- The form requires an affirmative “I use Codex Launcher” statement and consent to occasional product updates and feedback requests. The address is added to the contactable user list only after its owner confirms a one-time email link.
- A verified submission is evidence that the address owner self-reported use; it is not telemetry or proof of an installed or active app.
- Existing waitlist entries stay separate from new user/feedback signups; they are not silently relabeled as users or treated as having consented to feedback outreach.
- Existing data and external services are retained until the user separately approves export, retention, or deletion.

## Observable Definition of Done

At the public landing URL, a keyboard, mouse, or touch user can copy the exact versioned setup prompt. The button confirms success, provides a manual-copy fallback on clipboard failure, and has correct focus and screen-reader behavior.

Pasting that prompt into Codex on a clean supported Mac results in:

1. prerequisites checked without changing the machine;
2. the user choosing Tailscale by default or explicitly choosing the paid Fly route;
3. verified companion installation and a passing mode-specific doctor;
4. verified Android artifact download and clear sideload steps;
5. phone pairing and an online end-to-end session check.

The page contains no waitlist gate or stale claims that conflict with the actual release, privacy model, platform support, or network architecture. It retains an optional, purpose-specific email form that succeeds or fails independently of setup.

Given a visitor who submits the optional form, the page explains before submission what will be stored and why, records a server timestamp plus the current consent/purpose version in a users-specific store, confirms storage only after the backend does, and provides a documented way to opt out or request removal. Existing waitlist rows remain distinguishable.

Given an unverified or forged address, it never appears in the contactable `Users` worksheet and never receives product or feedback outreach. At most it receives the rate-limited ownership-confirmation message. Direct calls to the Apps Script cannot forge trusted signup metadata.

## Current Evidence

- `landing/index.html` still centers a waitlist form and posts to `/api/waitlist`.
- The same page says Tailscale is the transport even though the current released companion is relay-only.
- It includes absolute privacy claims such as “Nothing about your project leaves the machine” and “there's nothing on it to lose,” which are broader than the verified design.
- The current untracked waitlist path is `landing/api/waitlist.js` → Apps Script → Google Sheet, with tests in `tests/waitlist/waitlist.test.mjs`.
- The current Apps Script stores only `[timestamp, email]` and deduplicates only by email. Reusing that row shape would make old waitlist interest indistinguishable from new self-reported use and would discard a later user signup from an email already on the waitlist.
- `.github/workflows/release.yml` builds companion archives for six OS/architecture targets and Android APK/AAB artifacts with attestations.
- `docs/compatibility/codex.md` currently marks the physical Pixel/Android path as tested and desktop platforms more narrowly; the page must mirror the evidence at launch.

## Page Information and Actions

```text
Nav: Product | How it works | Security | Manual setup | Get started

Hero
  Remotely control Codex sessions running on your computer
  [ Copy setup prompt ]  [ Download Android app ]
  Works best with Tailscale (free). Another VPN? Use a private Fly relay.

How setup works
  1. Copy prompt     2. Paste into Codex     3. Pair phone

Connection choice
  Tailscale — free, recommended; may conflict with another active VPN
  Fly relay — optional; your Fly account/card; show a current price as an estimate with source/date

Security and privacy
  Credentials and project files stay on the computer
  Phone stores pairing material and connection details
  Relay handles encrypted traffic and can observe connection metadata, not session contents

Compatibility / support / manual path / release verification

Using Codex Launcher?
  Get occasional product updates and volunteer for feedback.
  [ Email address ]
  [ ] I use Codex Launcher and agree to receive occasional product updates
      and requests for feedback at this address.
  [ Email me a confirmation link ]
  We add you only after you confirm that you own the address.
  We store your email, confirmation time, self-reported usage, consent, source,
  and this form's purpose version.
  Setup does not require an email. Privacy and removal link.
```

Copy must remain short enough to scan. Link to detailed security, compatibility, pricing source, and manual setup instead of hiding qualifications.

The email section belongs after the setup path, not in the hero. Its heading and submit label must not imply that submitting installs the app or verifies actual usage.

The checkbox is unchecked on every initial load and after form reset. The browser never sends a request until the visitor actively selects it, and the API requires both affirmative booleans; missing/false assertions create no pending record or email. Browser tests reload the page, prove the control starts unchecked, attempt keyboard/mouse submission without selection, and assert zero network calls. API/Apps Script tests assert zero pending state for missing or false consent.

## Immutable Release and Prompt Contract

Add one same-origin `landing/setup/release-manifest.json` generated by the release workflow and deployed with the page. This is the only version source used by the prompt loader, visible version label, Android download action, manual-setup link, and contract tests. Its schema is fixed:

```json
{
  "schema": 1,
  "version": "<exact release version>",
  "gitCommit": "<40-character commit>",
  "promptUrl": "<versioned rendered-prompt asset URL>",
  "promptSha256": "<sha256 of prompt.txt>",
  "promptAttestationUrl": "<verification URL for promptSha256>",
  "manualSetupUrl": "<versioned URL>",
  "android": {
    "apkUrl": "<versioned release-asset URL>",
    "sha256": "<APK sha256>",
    "attestationUrl": "<verification URL for the same digest>"
  },
  "companionManifestUrl": "<versioned companion manifest URL>",
  "companionManifestSha256": "<sha256 of companion manifest>",
  "companionAttestationUrl": "<verification URL for that digest>"
}
```

No manifest URL may contain a branch name, `/latest`, or an unversioned redirect. The release source contains `release/onboarding/setup-prompt.template.txt`, not the final self-referential prompt. For release source commit `S`, CI builds the artifacts, renders the prompt with exact version and `S`, computes its hash, creates the manifest containing `S` and that prompt hash, and separately attests/publishes both final prompt bytes and manifest bytes. The later landing promotion commit is different from `S`; no file claims to contain the hash of the commit that contains itself. CI fails if version, source commit, filenames, digests, or URLs disagree.

At runtime the page fetches the same-origin manifest and `landing/setup/prompt.txt`, computes the prompt SHA-256 with Web Crypto, and enables copy/download only when the schema and hash match. The Android button displays the exact version and leads only to `android.apkUrl`; the adjacent verification link exposes the matching SHA-256 and attestation instructions. Manifest, prompt, APK, manual path, or visible-version disagreement enters the disabled mismatch state instead of falling back to another release.

### Release-to-landing handoff

The release job does not write into a Vercel checkout. The handoff is an explicit promotion change after the release succeeds:

```text
release workflow
  -> build + attest + publish versioned artifacts, rendered prompt, and release manifest
  -> repository command `release/promote-landing VERSION MANIFEST_SHA256`
       downloads the public manifest by versioned URL
       verifies its supplied digest and GitHub attestation
       verifies every referenced artifact/digest/attestation
       downloads and attestation-verifies the prompt named by the manifest
       copies both assets byte-for-byte to landing/setup/
       verifies copied prompt.txt against promptSha256
  -> reviewed promotion commit/PR
  -> Vercel preview from that commit
  -> anonymous preview smoke
  -> merge to main -> production Vercel deployment
```

`release/promote-landing` refuses a dirty destination, an existing different version without explicit replacement, unversioned URLs, or any prompt/manifest/artifact digest or attestation mismatch. It does not publish or deploy. Promotion CI verifies that both checked-in landing files are byte-identical to their published, attested assets. The preview must prove copy, APK, manual, checksum, and attestation links anonymously before promotion. After production, repeat those checks at `https://codex-launcher.vercel.app` and record the Vercel deployment ID plus prompt/manifest digests. Never edit a published version in place.

## Setup Prompt Contract

Store the exact copied text at `landing/setup/prompt.txt`, and have the page load it through that one tested source. Do not keep different prompt copies in HTML, tests, and docs.

The prompt must instruct Codex to:

- explain the intended outcome and show every command before making machine changes;
- check supported OS/architecture, Codex availability, Tailscale state, existing Codex Launcher config, ports, and Android access;
- fetch one fixed public release or a signed stable channel, never an unpinned branch or `latest` without resolved version verification;
- verify checksums and GitHub artifact attestations before running downloaded code;
- use the shipped installer/setup commands rather than reconstructing configuration;
- default to Tailscale and pause for Tailscale installation or sign-in when needed;
- ask whether the user needs Codex Launcher while another VPN is active;
- if yes, explain the Fly estimate, name that the user's own Fly account/card is charged, and obtain explicit approval before any account, provisioning, or paid command;
- ask for explicit approval of each project folder exposed to the phone;
- avoid printing or storing pairing and relay secrets in chat or logs;
- run `doctor`, download and verify the Android artifact, guide sideloading, pair the phone, and finish with a no-model-call connection plus session-list/empty-state check;
- explain that starting or resuming a real Codex task may use the user's authenticated model account, name the active credential/account when available, and obtain separate explicit approval before any check that can consume model usage;
- stop with recovery instructions on unsupported platforms, failed verification, unknown existing config, or unavailable release assets;
- make no automatic Tailscale/Fly switch.

The release-rendered prompt includes the exact manifest version and release source commit `S`. It never resolves `latest`; page and CI tests fail if its bytes, hash, or embedded release identity differ from the published/checked-in manifest.

## Interaction States and Accessibility

The primary button has four tested states:

| State | Visible behavior |
|---|---|
| Ready | “Copy setup prompt” |
| Copied | “Copied — paste into Codex”; polite screen-reader announcement; returns to ready without stealing focus |
| Clipboard unavailable/denied | Prompt appears in a selected, read-only text area with “Copy manually” instructions |
| Load/version mismatch | Copy is disabled; page links to manual setup and does not copy stale text |

Requirements:

- Works with keyboard only and retains a visible focus ring.
- Has an accessible name and an `aria-live` status region.
- Does not depend on color alone.
- Honors reduced motion.
- Works at 390px and 1440px without clipped prompt, actions, or diagrams.
- The copy action makes no network request and records no prompt contents.
- Analytics, if later added, require a separate privacy decision; none is added in this change.

The optional email form has separate tested states for ready, consent missing, invalid address, requesting confirmation, confirmation sent, verified, already verified, expired/invalid confirmation, rate limited, and backend unavailable. It remains usable by keyboard and screen reader, never hides or disables setup actions, never calls a pending address “stored” or “a user,” never claims verification before the store confirms it, and does not erase the entered address after a failed submission.

## Honest Product Copy

Before writing launch copy, derive every claim from the shipped release, current compatibility document, and security model.

Required corrections:

- Say Tailscale is the recommended default and may conflict with another VPN.
- Describe Fly as optional, paid by the user, and useful when another VPN must stay active.
- Replace “nothing leaves the machine” with the narrower verified statement: project files and provider credentials stay on the computer; session content travels encrypted to the paired phone.
- State that the phone stores pairing material and connection details, so device loss still requires revocation.
- State clearly which desktop and Android versions have been tested and label the rest experimental.
- Quote no permanent Fly price. Display an estimate with “as of” date and a link to the current source, or omit the number if it cannot be maintained reliably.

## Compatibility Launch Contract

The first public setup prompt supports exactly the evidence target from the Tailscale plan: macOS 15.5 arm64, standalone `codex-cli 0.144.1` or the separately checked ChatGPT Desktop `26.707.51957` adapter, and Pixel 9 on Android 16. Before launch, the clean physical flow must pass and `docs/compatibility/codex.md` must promote that exact combination from experimental to tested with a dated evidence record. The manifest and page repeat this matrix. Later versions remain unsupported/experimental until their own evidence updates the manifest, docs, and page together.

The current local `codex --version` command fails because its packaged executable is missing, so it is not launch evidence. Implementation must repair/use a clean machine and record the exact working binary path and version rather than relying only on the historical compatibility entry.

## Email Signup Transition

Repository and backend change after the launch gate passes:

- Replace the waitlist wording and primary placement in `landing/index.html` with the optional “Using Codex Launcher?” feedback signup after the setup content.
- Add exact endpoints `/api/user-feedback-signup`, `/api/user-feedback-confirm`, and `/api/user-feedback-remove`. Signup requires normalized email, affirmative use, affirmative contact consent, and an empty bot-trap field. Confirmation handles one-time ownership links. Removal sends and verifies its own one-time link before changing data. Keep `/api/waitlist` unchanged through the durable-routes stage; remove it from the setup-page deployment only after the waitlist form is gone, with the durable-routes deployment retained as rollback.
- The Vercel endpoints enforce POST/content-type/body-size/origin checks where applicable, generate 256-bit one-time tokens, and sign a canonical server-to-server body with HMAC-SHA256 using `FEEDBACK_SIGNING_SECRET`. The body includes action, normalized email where needed, token hash/raw delivery token as appropriate, issued-at, and nonce. Secrets and raw tokens are never logged.
- The page renders an exact `consent_version` beside the assertion. Signup sends that displayed version and both affirmative booleans; Vercel accepts only the version built into the same deployment and signs all three values. Apps Script stores them in the pending record. Confirmation succeeds only when the pending version still equals Apps Script's active consent version; if wording changed meanwhile, it deletes the pending record and returns `consent_changed`, requiring the person to reload and affirm the new wording.
- The Apps Script rejects an invalid signature, an issued-at outside five minutes, or a reused nonce before sending mail or touching the sheet. At confirmation it copies the bound pending assertion/version and sets its own `verified_at` and `source = landing_user_feedback`; it never replaces the version with whichever wording happens to be current.
- Keep the existing spreadsheet but write only confirmed addresses to a separate `Users` worksheet with exact columns `[verified_at, renewed_at, email, usage_status, feedback_opt_in, consent_version, source, last_contacted_at]`. Preserve the existing waitlist worksheet unchanged. Use a separate `Suppression` worksheet with `[email_hmac, created_at, expires_at, reason]`. Pending token hashes, bound assertion/version, expiry, attempts, cooldowns, and used nonces live in Script Properties; raw tokens are never stored. A nonce is retained for 10 minutes; its signed request is valid for five minutes, and hourly cleanup removes expired nonce records.
- Deduplicate within the `Users` worksheet only. An email already present on the old waitlist can therefore register as a current self-reported user without changing the old row.
- Exact signup/confirmation success responses are `{ok:true,status:"confirmation_sent"}`, `{ok:true,status:"verified"}`, and `{ok:true,status:"already_verified"}`; duplicate confirmation does not create a second row. An explicit renewal action repeats both assertions and returns `{ok:true,status:"renewal_sent"}` then `{ok:true,status:"renewed"}` after its fresh link is confirmed. Confirmation can also return `consent_changed`. Removal request/confirmation responses are `{ok:true,status:"removal_sent"}`, `{ok:true,status:"removed"}`, and `{ok:true,status:"already_absent"}`. Errors use distinct invalid, expired, rate-limited, and unavailable statuses without revealing whether an unrelated address is registered.
- Abuse limits are enforced under the Apps Script lock: at most one confirmation per address per minute, three per address per day, five token attempts, and 50 total confirmation emails per day or the lower remaining `MailApp` quota. A direct unsigned Apps Script call, replayed nonce, full quota, bot-trap submission, or oversized request sends no email and writes no row.
- Confirmation, renewal, and removal emails link only to `https://codex-launcher.vercel.app/feedback/confirm/#action=signup&token=...`, `#action=renew&token=...`, or `#action=remove&token=...`, built from a fixed Apps Script constant rather than request headers. URL fragments are not sent in HTTP requests. Inline code at the top of that page reads the fragment, immediately removes it with `history.replaceState`, and POSTs action/token to the corresponding API; it loads no external resource before stripping. The page shows verifying, verified/renewed/removed, already complete, expired, consent-changed, rate-limited, and unavailable states. Preview and local tests use fake mail/backend links; any live preview submission still receives the fixed production-origin link.
- Create a separate feedback Apps Script deployment and URL for the signed protocol. Do not mutate the old unsigned waitlist deployment URL. Keep `WAITLIST_SHEET_URL`, `/api/waitlist`, and the old deployment working while adding `FEEDBACK_SHEET_URL` plus `FEEDBACK_SIGNING_SECRET` for the new endpoints. Create and verify the `Users`/`Suppression` worksheets, HMAC secret, fixed origin, mail permission, cleanup trigger, limits, and signed deployment before pointing any preview at it. Roll back the preview if the new contract fails; the old waitlist remains independent.
- Keep the waitlist endpoint/source/test names unchanged through stage 1. Add new feedback files under their final purpose names. The setup-page deployment removes only the now-unused `/api/waitlist` code; the old Apps Script source/deployment, worksheet, and recorded durable-routes artifact remain unchanged for rollback until separately retired.
- Keep a dated schema, purpose, retention, access, export, and removal procedure in `saved-results/`; never commit addresses or other personal data.
- Put a short privacy explanation and a working privacy/removal link next to the form. Every later outreach must identify why the recipient is being contacted and link to the private confirmed-removal flow.

### Logging and redaction

Use tagged logs `[feedback-signup]`, `[feedback-confirm]`, `[feedback-remove]`, `[feedback-sheet]`, and `[feedback-cleanup]`. At entry log only method, bounded body shape, and action; log the validation/consent/signature/replay/quota/storage branch and a fixed outcome immediately before return; log caught errors by safe class and operation. Never log an email address, email hash/HMAC, raw token, token hash, signature, nonce, shared secret, request body, Sheet row, or confirmation URL. Tests inject recognizable marker values into every sensitive field, capture Vercel and Apps Script logs on success/failure branches, and fail if any marker or normalized email appears.

External destructive cleanup remains a separate, explicit action:

- Vercel environment variables;
- Apps Script deployment;
- Google Sheet and its collected emails.

Do not disable or delete any of those during the code rollout without the user's explicit approval and a confirmed export/retention plan. The verified owners are the Vercel Hobby account `aadivyaraushan` in scope `aadivyaraushans-projects` and Google Apps Script/Sheet account `aadivya.raushan@gmail.com`. Reconfirm those identities before deployment. The plan adds no paid provider; if existing free quotas are insufficient, stop rather than upgrading or charging an account without approval.

## Privacy, Access, Retention, and Removal

Add public `landing/privacy/index.html` and link it immediately beside the checkbox. It states the exact fields above, self-reported nature of usage, purposes (occasional updates and feedback requests), Google Sheet/Apps Script storage, confirmation-email behavior, access owner, retention, removal, and that setup does not require submission.

- Access owner: `aadivya.raushan@gmail.com`, the verified Apps Script executor and current Sheet editor. Any later sharing must be recorded and requires a plan update.
- Pending/unverified request data: confirmation token expires after 30 minutes; an hourly cleanup removes the pending record after 24 hours (normally by hour 25 at the latest).
- Verified user row: an hourly cleanup removes it once 18 months have elapsed since the later of `verified_at` or `renewed_at`. `last_contacted_at` is audit-only and never extends retention. Renewal requires the person to repeat the current affirmative use/contact consent and confirm a fresh one-time link; only that confirmation updates `renewed_at` and `consent_version`.
- Confirmed removal: delete the plaintext `Users` row immediately and write only an HMAC suppression value for 18 months so old exports are not accidentally re-contacted. A later fully verified signup removes its matching suppression entry and becomes a new consent record.
- Private removal channel: the privacy page's removal form sends the fixed-origin one-time fragment link through `/api/user-feedback-remove`; the address is not posted publicly. The shared confirmation page shows request, removed/already-absent, expired, and unavailable states.
- Outreach owner: the same maintainer account. Every message says the recipient confirmed the landing form, includes the removal link, and updates audit-only `last_contacted_at`; sending mail never renews consent or retention. Exported lists inherit the same rules.

`installFeedbackCleanupTrigger()` creates exactly one hourly Apps Script trigger for `cleanupExpiredFeedbackData` under `aadivya.raushan@gmail.com`; deployment verifies the trigger owner, function, and interval and removes duplicates. Cleanup takes the script lock, processes bounded batches, removes expired pending properties, expired verified rows, and expired suppression rows, and is safe to rerun after a partial batch. It logs counts only. On failure it leaves unprocessed data for the next hourly retry, emits `[feedback-cleanup]` error state, and relies on Apps Script's trigger-failure notification to the owning account; the deployment smoke intentionally runs the function and checks the execution result. Unit tests fake `ScriptApp`, time, properties, and sheets to cover boundary times, partial batches, retries, lock failure, and duplicate-trigger repair.

Launch tests fetch the public privacy page, exercise verified opt-in and confirmed removal, inspect the installed cleanup trigger, run cleanup at fixed boundary times, and prove expired/unverified addresses are absent from `Users` and cannot be selected for outreach.

## Planned File Work

| Area | Files | Purpose |
|---|---|---|
| Page | `landing/index.html` | Replace waitlist as the main journey, correct copy, add setup actions, and retain optional feedback signup |
| Prompt template | `release/onboarding/setup-prompt.template.txt` | Release-time source rendered with version and source commit before attestation |
| Prompt source | `landing/setup/prompt.txt` | Exact versioned prompt copied by the page |
| Release manifest | `landing/setup/release-manifest.json`, release generator/checks | One immutable identity for prompt, APK, manual setup, digests, and attestation |
| Email endpoints | retain `landing/api/waitlist.js` for stage 1; add `user-feedback-signup.js`, `user-feedback-confirm.js`, `user-feedback-remove.js` | Keep the live waitlist while validating/limiting signed feedback actions; `api/` is the established Vercel framework directory |
| Apps Script | keep `waitlist-sheet.gs` unchanged; add a purpose-named feedback Apps Script source/deployment | Preserve the old waitlist protocol while the separate signed script manages confirmation, cleanup, `Users`, and `Suppression` |
| Privacy | `landing/privacy/index.html` | Public data, consent, retention, access, and private removal contract |
| Confirmation page | `landing/feedback/confirm/index.html` | Strip fragment token before external loads, POST confirmation/removal, and show exact states |
| Contract tests | `tests/landing/landing-contract.test.mjs` | Manifest/prompt/download match, API and Apps Script trust/data contracts, privacy and old-row separation |
| Browser tests | `tests/landing/landing-browser.spec.mjs`, `tests/landing/server.mjs`, `playwright.config.mjs` | Reproducible Chromium UI, clipboard, email, accessibility, and viewport tests against a local fake backend |
| Browser dependencies | root `package.json` and committed `package-lock.json` | Exact (no range) `@playwright/test` and `@axe-core/playwright` versions plus stable commands |
| Browser CI | `.github/workflows/landing.yml` | `npm ci`, pinned Chromium install, contract tests, browser tests, and screenshot artifacts |
| Release contract | `.github/workflows/release.yml`, `release/checks/`, release docs only where needed | Ensure the prompt points to public, attested artifacts and stable commands |
| Release promotion | purpose-named `release/promote-landing` command and tests | Verify a published manifest/attestation and stage the exact Vercel source update |
| Metadata/assets | landing meta/OG text and `landing/og.png` if the message changes materially | Consistent social preview and page promise |
| Transition record | `saved-results/landing-launch-transition.md` | Launch version, verification evidence, signup data contract and retention, rollback |

Follow the established single-file landing framework. The purpose-named `setup/`, `privacy/`, and `tests/landing/` groups keep at most three hand-written files. `landing/api/` is the Vercel framework directory and temporarily contains four endpoints during stage 1; the setup-page deployment removes the old waitlist endpoint and returns it to three.

Exact local/CI commands:

```sh
npm ci
npm run test:landing:contract
npm run test:landing:browser
```

`playwright.config.mjs` starts `node tests/landing/server.mjs` on `127.0.0.1:4173`. That bounded server serves `landing/` and deterministic fake signup/confirmation/removal responses; it does not call Google. Browser coverage uses pinned Chromium, 1440×900 and 390×844 projects, clipboard permission granted/denied cases, `prefers-reduced-motion`, keyboard traversal, horizontal-overflow assertions, and `@axe-core/playwright`. The public preview smoke is separate and uses test records that are removed afterward.

### Isolated-worktree source preservation

The current waitlist endpoint, Apps Script, test, landing assets, deployment record, and both new plan files are untracked user files, so a new worktree will not contain them. The explicit import set is `landing/.gitignore`, `landing/api/waitlist.js`, `landing/og.png`, `waitlist-sheet.gs`, `tests/waitlist/waitlist.test.mjs`, `saved-results/landing-waitlist-google-sheet.md`, `planning/google-sheet-waitlist-plan.md`, `planning/tailscale-support-plan.md`, and this plan. Before creating the implementation worktree, perform a read-only inventory and write SHA-256 hashes for those paths to `.context/landing-source.sha256`. After creating the worktree as the first implementation setup change, copy those paths with relative structure and “destination must not exist” behavior, then verify every destination hash against the inventory. Abort on any missing source, existing destination, or mismatch. Do not remove, modify, stage, or overwrite the source-workspace copies. Record source/destination paths and both hash results in the transition result; include no email data or deployment secrets.

## TDD Execution Order

1. Inventory and hash the untracked source named above, create an isolated git worktree, copy only those files without overwrite, verify hashes, and record both paths.
2. Write tests first and run them red:
   - primary CTA is “Copy setup prompt,” not waitlist;
   - copied text exactly matches the one prompt source and includes the current version marker;
   - manifest, prompt hash, visible version, APK URL/digest/attestation, and manual URL agree; stale or unpinned values disable copy/download;
   - success, clipboard-denied, and stale-prompt states behave correctly;
   - keyboard, accessible status, focus, and reduced-motion requirements hold;
   - the old waitlist gate and wording are gone while setup works without an email;
   - affirmative usage and feedback consent plus confirmed email ownership are required before a `Users` row exists;
   - consent begins unchecked; keyboard/mouse submission without a fresh selection makes zero requests and creates no pending state;
   - Vercel signs calls; Apps Script rejects direct forgeries/replays and creates authoritative source/time/consent values;
   - old waitlist rows remain unchanged and separate; an old waitlist email can confirm into `Users`;
   - confirmation, already-verified, expiry, invalid, removal, quotas, backend failure, and concurrent submissions behave honestly;
   - renewal requires a fresh current-version assertion and link; outbound contact alone never updates `renewed_at` or retention;
   - displayed consent/assertion are bound into pending state; a deployment change forces renewed consent rather than recording unseen wording;
   - fixed-origin fragment links strip token material before any external load and map every API status to an honest UI state;
   - cleanup trigger exists once, is owned correctly, retries safely, and enforces pending/user/suppression expiry;
   - tagged logs cover decisions/errors and contain none of the injected personal or secret markers;
   - public privacy/removal links work and retention cleanup excludes expired records;
   - product copy describes Tailscale/Fly, privacy, cost, and compatibility accurately;
   - prompt and manual path resolve anonymously to verified public release assets.
3. Implement the smallest page, prompt, manifest/promotion, Vercel API, Apps Script/cleanup trigger, privacy/confirmation route, tests, CI, and documentation changes that pass the focused tests.
4. Add browser-level tests for desktop and mobile copy/manual-copy flows.
5. Run the prompt through Codex on the exact compatibility target and record every pause, command, artifact verification, doctor result, pairing step, recovery path, and the no-model-call online/session-list result.
6. Use a physical Android phone to verify online and session-list/empty-state behavior. Run no model task without separately naming the active account and obtaining cost approval.
7. Run all landing, release-contract, accessibility, and relevant repository tests.
8. Search the repository and deployed page for stale waitlist URLs/wording, stale Tailscale/relay claims, old CTA labels, old release versions, and absolute privacy wording. Preserve only intentional historical waitlist data/docs and classify every result.

## Visual and Runtime Verification

Use Codex Computer Use against the local page and the deployment preview when available:

| Check | Required evidence |
|---|---|
| Desktop | 1440px screenshot; hero/action hierarchy, connection comparison, footer, no clipping |
| Mobile | 390px screenshot; thumb-friendly buttons, wrapped copy, no horizontal scroll |
| Keyboard | Tab order, Enter/Space copy, visible focus, manual-copy selection |
| Screen reader semantics | Button names, headings, status announcement, links |
| Failure | Clipboard denied and prompt/version unavailable both remain usable |
| Reduced motion | No required information depends on animation |
| Runtime | No console errors or failed assets; no email request until form submission; stored response is honest |
| Email independence | Setup remains fully available when the form is empty, invalid, submitting, or unavailable |
| Email ownership | Unconfirmed address creates no `Users` row; confirmation creates one; direct forgery/replay and expired token create none |
| Data separation | Confirmed preview submission appears in `Users` with Apps-Script-owned metadata; old waitlist worksheet is unchanged |
| Removal | Verified private removal deletes/suppresses the contact as documented and prevents outreach selection |
| Retention | Hourly trigger exists; fixed-time cleanup purges each expired data class; failure is visible and the next run retries |
| Link safety | Email uses the fixed production fragment URL; token is stripped before requests/assets/history and never logged |
| Public preview | Anonymous browser can load page, prompt, manual docs, APK, checksums, and attestations |
| Full setup | Clean Mac to online physical Android session, using the exact copied prompt |

If Computer Use is unavailable, use Playwright screenshots and automated accessibility checks and state that limitation in the launch record.

## Rollout and Rollback

Use two production stages so links issued during preview and after rollback remain valid:

1. **Durable routes stage:** deploy the privacy page, confirmation/removal page, three new API endpoints, separate signed feedback Apps Script deployment, cleanup trigger, and fixed-origin links to production while keeping the existing waitlist hero/form, `/api/waitlist`, old Apps Script URL, and `WAITLIST_SHEET_URL` unchanged. Add separate `FEEDBACK_SHEET_URL` and `FEEDBACK_SIGNING_SECRET`. Record this Vercel deployment ID as the rollback floor.
2. Verify in the same production deployment that a unique old waitlist submission still reaches the old worksheet once and that the new privacy/removal pages, signed APIs, confirmation fragment, and new worksheets work. No new user-feedback form is public yet.
3. Confirm the referenced release prompt/manifest/artifacts remain public, immutable, digest-matched, and attested.
4. Deploy the new setup-focused page as a preview. A live preview email points to the already-live production confirmation route; confirm it creates and removes a test `Users` row. Complete every other preview check.
5. Promote the setup page to production while retaining the durable routes and feedback backend from stage 1. This deployment removes the waitlist form and `/api/waitlist` code but does not delete the old Apps Script deployment, environment value, or worksheet; the recorded durable-routes deployment can restore the endpoint immediately.
6. Re-run anonymous copy, download, setup-link, confirmation, duplicate, renewal, removal, abuse-limit, retention, and backend-failure smoke checks.
7. If the setup page, release identity, or prompt fails, deploy/restore the recorded **durable routes stage**, not the original pre-route deployment. That rollback restores the working waitlist hero/endpoint and also keeps privacy, confirmation, removal, signed feedback APIs, separate Apps Script deployment, and pending links working. Re-run one old waitlist submission plus one new confirmation/removal smoke after rollback.
8. Never roll back or delete the durable routes while any `Users`, `Suppression`, or unexpired pending records exist. Keep the old waitlist worksheet separate; its later bulk retention/deletion requires separate approval.

## Out of Scope

- Building an in-browser installer or executing commands from the webpage.
- Supporting arbitrary coding agents in the initial prompt.
- Automatically creating or charging a Fly account.
- Automatically detecting another VPN from the landing page.
- Bulk deletion of the legacy waitlist, whole user store, or external services. Confirmed individual removal and scheduled retention cleanup are required in scope.
- Adding analytics, tracking pixels, accounts, or a new storage provider.

## Adversarial Review Gate

The plan is not approved until an independent reviewer first defines the bar for a truthful, low-friction, recoverable onboarding launch and returns `PASS` with no unresolved critical or major findings. Each review round and resulting revision will be recorded here.

### Review history

- Round 1 — **FAIL**: no critical findings; major gaps in meaningful/verified usage consent, Apps Script trust, immutable prompt/APK versioning, reproducible browser tests, no-cost completion checks, and resolved privacy/retention/removal ownership. Revision 2 addresses each and fixes duplicate and exact compatibility contracts.
- Round 2 — **FAIL**: no critical findings; remaining major gaps in release-to-Vercel promotion, consent-version binding, untracked-source preservation, scheduled retention enforcement, executable confirmation/removal routes, and redacted diagnostic logs. Revision 3 freezes each contract and expands implementation/removal details.
- Round 3 — **FAIL**: no critical findings; remaining major gaps were the prompt/commit self-reference, preview/rollback link durability, and outreach extending retention. Revision 4 uses release-generated attested prompt bytes, a durable-routes-first deployment, and user-confirmed renewal only.
- Round 4 — **FAIL**: no critical findings; remaining major gaps were pre-checked consent and breaking the old waitlist during durable-route rollout. Revision 5 makes consent a fresh unchecked action and runs the old unsigned waitlist and new signed feedback deployments side by side through cutover/rollback.
- Round 5 — **PASS**: zero critical and zero major findings. The reviewer confirmed ungated setup, realizable attested release identity, affirmative/verified email consent, separate old/new data contracts, enforceable removal/retention, reproducible tests, and durable preview/production/rollback routes.

## GSTACK REVIEW REPORT

**Final verdict:** PASS

**Adversarial rounds:** 5

**Unresolved critical findings:** 0
**Unresolved major findings:** 0

The independent reviewer derived its standard before inspecting the plan and verified the setup journey, immutable prompt/APK identity, cost approvals, accessibility and recovery, email consent/ownership, privacy lifecycle, source preservation, and two-stage deployment/rollback contract.
