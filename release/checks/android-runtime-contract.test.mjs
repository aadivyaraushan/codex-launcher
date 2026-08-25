import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";

const read = (path) => readFileSync(new URL(`../../${path}`, import.meta.url), "utf8");
const activity = read("android/app/src/main/kotlin/app/codexlauncher/LauncherActivity.kt");
const application = read("android/app/src/main/kotlin/app/codexlauncher/LauncherApplication.kt");
const session = read("android/app/src/main/kotlin/app/codexlauncher/connection/runtime/LauncherSessionViewModel.kt");
const owner = read("android/app/src/main/kotlin/app/codexlauncher/storage/ownership/LocalStateOwner.kt");
const dictation = read("android/app/src/main/kotlin/app/codexlauncher/task/dictation/OnDevicePromptDictation.kt");
const dictationControl = read("android/app/src/main/kotlin/app/codexlauncher/task/dictation/PromptDictationControl.kt");
const storePath = "android/app/src/main/kotlin/app/codexlauncher/storage/connection/lastseen/LastConnectionStore.kt";

assert.equal(existsSync(new URL(`../../${storePath}`, import.meta.url)), true, "last successful connection must have durable storage");
assert.match(activity, /onDictate\s*=/, "Home dictation must be wired by LauncherActivity");
assert.match(activity, /dictation\.start\(draftComposerState\.text, draftComposerViewModel::update\)/, "Home dictation must start from and live-update the exact editable draft");
assert.match(activity, /dictation\.start\(sessionUiState\.followUpDraft\)/, "follow-up dictation must start from the current editable draft");
assert.doesNotMatch(activity, /RecognizerIntent|ACTION_RECOGNIZE_SPEECH/, "LauncherActivity must not use Google's speech activity");
assert.match(dictation, /MicTranscriber/, "dictation must use the local Moonshine microphone transcriber");
assert.match(dictation, /MOONSHINE_MODEL_ARCH_SMALL_STREAMING/, "dictation must use the selected English streaming model");
assert.match(dictation, /released\.close\(\)/, "explicit Stop must close the microphone capture thread");
assert.match(dictationControl, /Text\(if \(listening\) "Stop"/, "listening must end only through the visible Stop control");
assert.match(application, /onSuccessfulConnection[\s\S]*lastConnections\.record/, "application-owned session must persist successful connection time");
assert.match(session, /shouldRecordSuccessfulConnection/, "last connected must only change on a real transition into online");
assert.match(activity, /lastConnectedLabel\s*=/, "offline Home must receive its last successful connection label");
assert.match(owner, /lastConnections/, "durable connection state must be owned with the other local stores");

console.log("android runtime contract: 14 assertions passed");
