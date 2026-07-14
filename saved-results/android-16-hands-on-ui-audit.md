# Android 16 hands-on UI audit

Date: 2026-07-14T13:34:40.389Z

Purpose: drive the Codex Launcher on a running Android emulator using Android UI dumps plus real tap, type, Back, and screenshot commands. The fixed-state activity renders production Compose surfaces with local synthetic data; it does not prove a live Tailscale or Codex account connection.

## Environment

- Device: sdk_gphone64_arm64
- Android: 16
- ADB serial: emulator-5554
- AVD: codex_launcher_pixel_9_api_36
- Build fingerprint: google/sdk_gphone64_arm64/emu64a:16/BE2A.250530.026.F3/13894323:userdebug/dev-keys
- Debug APK SHA-256: 024d29db62ee80f4abfe77937241ec66c1c9aca7fd5ee03f2985910ab1e30f61
- Source commit: b02108088ca7f3d9bc2882eb351472f324ac026b
- Source tree dirty during audit: no
- Fixed states read from the Kotlin catalog: 42
- Checks: 73 passed, 0 failed

## Results

- PASS: production first launch uses the real pairing screen [screenshot](../outputs/android-vm-audit/production-first-launch.png)
- PASS: production launcher becomes Android Home and opens on Home key [screenshot](../outputs/android-vm-audit/production-default-home.png)
- PASS: production pairing rejects an invalid manual link [screenshot](../outputs/android-vm-audit/production-pairing-invalid.png)
- PASS: production pairing requests and receives Android camera permission [screenshot](../outputs/android-vm-audit/production-camera-permission.png)
- PASS: production app drawer shows no-match state and opens launcher settings [screenshot](../outputs/android-vm-audit/production-app-drawer.png)
- PASS: production Dark mode persists across process restart [screenshot](../outputs/android-vm-audit/production-dark-persistence.png)
- PASS: production Android Settings escape leaves the launcher [screenshot](../outputs/android-vm-audit/production-android-settings.png)
- PASS: production launcher remains usable at 1.3× Android font scale [screenshot](../outputs/android-vm-audit/production-large-font.png)
- PASS: production Home recovers after force-stop [screenshot](../outputs/android-vm-audit/production-force-stop-home.png)
- PASS: render catalog → UI audit scenarios [screenshot](../outputs/android-vm-audit/state-catalog.png)
- PASS: render pairing_qr_permission → Camera access is used only to read the pairing code. [screenshot](../outputs/android-vm-audit/state-pairing_qr_permission.png)
- PASS: render pairing_manual → Pair computer [screenshot](../outputs/android-vm-audit/state-pairing_manual.png)
- PASS: render pairing_manual_error → That pairing link isn't valid [screenshot](../outputs/android-vm-audit/state-pairing_manual_error.png)
- PASS: render pairing_save_retry → Retry saving [screenshot](../outputs/android-vm-audit/state-pairing_save_retry.png)
- PASS: render home_connecting → Connecting [screenshot](../outputs/android-vm-audit/state-home_connecting.png)
- PASS: render home_syncing → Syncing [screenshot](../outputs/android-vm-audit/state-home_syncing.png)
- PASS: render home_offline → Computer offline [screenshot](../outputs/android-vm-audit/state-home_offline.png)
- PASS: render home_incompatible → Desktop integration needs an update [screenshot](../outputs/android-vm-audit/state-home_incompatible.png)
- PASS: render home_revoked → Pairing revoked [screenshot](../outputs/android-vm-audit/state-home_revoked.png)
- PASS: render home_online → Sample task [screenshot](../outputs/android-vm-audit/state-home_online.png)
- PASS: render home_choose_project → Choose project [screenshot](../outputs/android-vm-audit/state-home_choose_project.png)
- PASS: render home_draft_error → Draft could not be saved [screenshot](../outputs/android-vm-audit/state-home_draft_error.png)
- PASS: render home_new_task_review → Outcome unknown [screenshot](../outputs/android-vm-audit/state-home_new_task_review.png)
- PASS: render home_new_task_error → The computer could not start this task [screenshot](../outputs/android-vm-audit/state-home_new_task_error.png)
- PASS: render home_attachments → sample-notes.txt [screenshot](../outputs/android-vm-audit/state-home_attachments.png)
- PASS: render project_choices → Choose where Codex works [screenshot](../outputs/android-vm-audit/state-project_choices.png)
- PASS: render project_empty → No approved projects [screenshot](../outputs/android-vm-audit/state-project_empty.png)
- PASS: render project_save_retry → Retry saving [screenshot](../outputs/android-vm-audit/state-project_save_retry.png)
- PASS: render task_loading → Loading task [screenshot](../outputs/android-vm-audit/state-task_loading.png)
- PASS: render task_unavailable → Task unavailable [screenshot](../outputs/android-vm-audit/state-task_unavailable.png)
- PASS: render task_transcript → Sample task [screenshot](../outputs/android-vm-audit/state-task_transcript.png)
- PASS: render task_controls_working → Queue follow-up [screenshot](../outputs/android-vm-audit/state-task_controls_working.png)
- PASS: render task_controls_idle → Send follow-up [screenshot](../outputs/android-vm-audit/state-task_controls_idle.png)
- PASS: render task_control_unknown → Sample task [screenshot](../outputs/android-vm-audit/state-task_control_unknown.png)
- PASS: render task_fork_unknown → Previous fork unconfirmed [screenshot](../outputs/android-vm-audit/state-task_fork_unknown.png)
- PASS: render task_command_detail → Command output [screenshot](../outputs/android-vm-audit/state-task_command_detail.png)
- PASS: render task_file_detail → File change [screenshot](../outputs/android-vm-audit/state-task_file_detail.png)
- PASS: render approval_full → Approval needed [screenshot](../outputs/android-vm-audit/state-approval_full.png)
- PASS: render approval_redacted → Some command details could not be shown safely [screenshot](../outputs/android-vm-audit/state-approval_redacted.png)
- PASS: render approval_sending → Approval needed [screenshot](../outputs/android-vm-audit/state-approval_sending.png)
- PASS: render question_choice → Codex needs your answer [screenshot](../outputs/android-vm-audit/state-question_choice.png)
- PASS: render question_free_text → What should Codex check? [screenshot](../outputs/android-vm-audit/state-question_free_text.png)
- PASS: render question_secret → Answer on computer [screenshot](../outputs/android-vm-audit/state-question_secret.png)
- PASS: render question_sending → Codex needs your answer [screenshot](../outputs/android-vm-audit/state-question_sending.png)
- PASS: render apps → All apps [screenshot](../outputs/android-vm-audit/state-apps.png)
- PASS: render apps_launch_error → App could not be opened [screenshot](../outputs/android-vm-audit/state-apps_launch_error.png)
- PASS: render appearance → Appearance [screenshot](../outputs/android-vm-audit/state-appearance.png)
- PASS: render recovery → Finishing private data cleanup [screenshot](../outputs/android-vm-audit/state-recovery.png)
- PASS: render dialog_attach → Attach [screenshot](../outputs/android-vm-audit/state-dialog_attach.png)
- PASS: render dialog_background_warning → Background connection unavailable [screenshot](../outputs/android-vm-audit/state-dialog_background_warning.png)
- PASS: render dialog_unpair → Remove computer [screenshot](../outputs/android-vm-audit/state-dialog_unpair.png)
- PASS: pairing camera permission action [screenshot](../outputs/android-vm-audit/interaction-pairing-camera.png)
- PASS: manual pairing invalid-link recovery [screenshot](../outputs/android-vm-audit/interaction-pairing-invalid.png)
- PASS: pairing save retry [screenshot](../outputs/android-vm-audit/interaction-pairing-save-retry.png)
- PASS: project is required before first send [screenshot](../outputs/android-vm-audit/interaction-home-project-required.png)
- PASS: Home model choice and prompt send [screenshot](../outputs/android-vm-audit/interaction-home-send.png)
- PASS: new-task review, retry, and attachment removal [screenshot](../outputs/android-vm-audit/interaction-home-review-retry-attachment.png)
- PASS: project selection and save retry [screenshot](../outputs/android-vm-audit/interaction-project-selection.png)
- PASS: offline connection help [screenshot](../outputs/android-vm-audit/interaction-offline-help.png)
- PASS: working task redirect and confirmed stop [screenshot](../outputs/android-vm-audit/interaction-task-control.png)
- PASS: idle task dictation and attachment removal [screenshot](../outputs/android-vm-audit/interaction-task-inputs.png)
- PASS: transcript detail navigation [screenshot](../outputs/android-vm-audit/interaction-transcript-details.png)
- PASS: task rename, archive, and fork menus [screenshot](../outputs/android-vm-audit/interaction-task-actions.png)
- PASS: uncertain task controls require explicit review [screenshot](../outputs/android-vm-audit/interaction-uncertain-task-review.png)
- PASS: redacted and sending approvals stay fail-closed [screenshot](../outputs/android-vm-audit/interaction-approval-safety.png)
- PASS: full approval actions [screenshot](../outputs/android-vm-audit/interaction-approval-full.png)
- PASS: free-text, dismiss, and secret question paths [screenshot](../outputs/android-vm-audit/interaction-question-safety.png)
- PASS: sending question disables actions [screenshot](../outputs/android-vm-audit/interaction-question-sending.png)
- PASS: app search empty state and navigation [screenshot](../outputs/android-vm-audit/interaction-app-search.png)
- PASS: app launch failure and dismissal dialogs [screenshot](../outputs/android-vm-audit/interaction-app-error-dialogs.png)
- PASS: appearance choices [screenshot](../outputs/android-vm-audit/interaction-appearance.png)
- PASS: recovery and attachment-file dialog actions [screenshot](../outputs/android-vm-audit/interaction-recovery-dialog.png)
- PASS: production default Home recovers after Android reboot [screenshot](../outputs/android-vm-audit/production-reboot-home.png)

## Reproduce

`ANDROID_HOME=/opt/homebrew/share/android-commandlinetools node release/checks/android-emulator-ui-audit.mjs`
