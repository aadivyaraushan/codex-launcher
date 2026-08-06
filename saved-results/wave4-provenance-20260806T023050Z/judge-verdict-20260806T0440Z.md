# Independent judge verdict — Waves 0–4 exit claim (second round)

**Date:** 2026-08-06 ~04:40Z
**Judge:** fresh-context LLM judge (post prior-FAIL re-grade)
**Plan:** `planning/finish-consumer-and-messaging-plan.md`
**Evidence graded:** `wave4-provenance-20260806T023050Z/RESULT.md` (all updates), `google-sha1-fix/RESULT.md`, `step6-signed-resmoke/RESULT.md`, end of `finish-consumer-overnight-status-2026-08-05.md`

## Exit standard (defined from the plan alone, before reading evidence)

For a Waves 0–4 complete claim, all of the following must be true:

1. **Substrate:** every wave test and real-account smoke runs on the persistent Play AVD `operator_android16_arm64` (Pixel 9 profile, android-36 google_apis_playstore arm64), explicitly "not on the owner's Pixel" (plan, Locked owner decisions).
2. **Signing:** the internal-release signing identity is frozen **before the first Operator install or provider sign-in**; `assetlinks.json` is generated from that certificate and Android reports `tryoperator.net` verified before Notion/Todoist OAuth.
3. **Source-to-binary:** clean sibling worktree at the final commit; every final Go/unit/instrumentation/protocol/release/privacy/security suite green **there**; Linux ARM64 runtime and all APKs built there; **only those recorded hashes installed** for the final smoke.
4. **Wave 4 step 6:** on that installed release-candidate APK, rerun the reboot/process-recovery checks **and every real-account row in the service/verb acceptance table** with target-side readback. Grant UI or HTTP 2xx does not satisfy a row.
5. **Waves 0–2 substrate proofs on the same VM:** cold boot, Termux boot receiver, child/proot death recovery, three JobScheduler process-reclaim trials ≤20 min, local-trust bootstrap red/green, route miss stays local; Beeper three-network final-event sends; measured YouTube playback; Direct Reply.
6. **No Mac/Keychain/relay in any production path**; both parent-plan headers corrected; dedup journal / delivery_unknown proofs.
7. Any corrective edit after the final commit restarts the whole clean-commit → sibling → suites → install → smoke cycle; partial carryover is not final evidence.

## Verdict: **FAIL**

What genuinely improved since the prior FAIL:
- Google authorization on the signed release APK now passes after release SHA-1/256 registration (`google-sha1-fix/RESULT.md`).
- Full Pixel instrumentation reached 0/134 on the debug APK at `7b07a29`.
- A frozen keystore now exists and sibling signed `assembleRelease` builds.

## Concrete remaining gaps

1. **Wrong test substrate (fatal).** All authenticated-wave and smoke evidence is on physical Pixel `4B230DLAQ001Z5`. The plan mandates the persistent Play AVD and forbids the Pixel. Nothing in the graded evidence shows the AVD exists, is authenticated, or ran any wave.
2. **Signer frozen after authentication.** Keystore created 2026-08-06T04:27Z, after all provider sign-ins — the plan requires it frozen before the first install/sign-in. No evidence `assetlinks.json` was regenerated from the release cert and domain verification re-proved on-device.
3. **Installed APK ≠ sibling hash.** Installed release APK `a4327a70…`; sibling-built APK `636d9cb8…`. Plan: install only the sibling-recorded hashes. Same cert is not enough.
4. **Step 6 is a thin smoke, not the matrix.** On the signed APK only: launch, Outlook grant UI, Google grant UI, one Beeper HTTP 200, shade presence. Everything else is explicitly labeled debug-APK carry-over. Zero acceptance-table rows (read/write/readback per verb) reran on the release candidate, and no reboot/recovery rerun on it.
5. **Wave 0 runtime evidence absent.** No cold-boot, Termux/proot supervisor, process-reclaim-trials, or local-trust proof appears in the graded evidence at the final commit/substrate.
6. **Mac/Keychain routes still in the Wave 3 record.** Notion Wave 3 pass ran "via normal Keychain Get" on the host; Slack/Notion proofs ran through host `go run` commands, not the phone-local runtime from a normal Android prompt.
7. Host suite matrix "not fully re-run this pass" after the signing changes; the full-cycle-repeat rule is unmet.

## Is a Waves 0–4 complete PR justified? **No.**

## Single next blocking action

Resolve the substrate contradiction before anything else: either provision and authenticate the plan-mandated Play AVD (`operator_android16_arm64`) and run the release-candidate (sibling-hash) install + Wave 0 recovery + full acceptance-table smoke there, or obtain an explicit owner amendment of the plan accepting the physical Pixel as the test substrate. Every other gap is downstream of this decision.
