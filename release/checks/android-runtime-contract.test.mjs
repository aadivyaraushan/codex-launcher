import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";

const read = (path) => readFileSync(new URL(`../../${path}`, import.meta.url), "utf8");
const activity = read("android/app/src/main/kotlin/app/codexlauncher/LauncherActivity.kt");
const application = read("android/app/src/main/kotlin/app/codexlauncher/LauncherApplication.kt");
const session = read("android/app/src/main/kotlin/app/codexlauncher/connection/runtime/LauncherSessionViewModel.kt");
const owner = read("android/app/src/main/kotlin/app/codexlauncher/storage/ownership/LocalStateOwner.kt");
const storePath = "android/app/src/main/kotlin/app/codexlauncher/storage/connection/lastseen/LastConnectionStore.kt";

assert.equal(existsSync(new URL(`../../${storePath}`, import.meta.url)), true, "last successful connection must have durable storage");
assert.match(activity, /onDictate\s*=/, "Home dictation must be wired by LauncherActivity");
assert.match(activity, /beginDictation/, "Home dictation must capture the exact editable draft before speech starts");
assert.match(activity, /applyDictation/, "recognized speech must use the draft ViewModel's guarded update");
assert.match(activity, /home dictation result handled/, "dictation logs must not claim rejected speech was applied");
assert.doesNotMatch(activity, /RecognizerIntent|PromptDictationContract|ACTION_RECOGNIZE_SPEECH/, "Home dictation must not launch Google speech recognition");
assert.match(read("android/app/src/main/AndroidManifest.xml"), /RECORD_AUDIO/, "in-app dictation must declare the microphone permission");
assert.doesNotMatch(read("android/app/src/main/kotlin/app/codexlauncher/task/control/PromptDictation.kt"), /RecognizerIntent/, "follow-up dictation must not use Google speech recognition");
assert.match(application, /onSuccessfulConnection[\s\S]*lastConnections\.record/, "application-owned session must persist successful connection time");
assert.match(session, /shouldRecordSuccessfulConnection/, "last connected must only change on a real transition into online");
assert.match(activity, /lastConnectedLabel\s*=/, "offline Home must receive its last successful connection label");
assert.match(owner, /lastConnections/, "durable connection state must be owned with the other local stores");

console.log("android runtime contract: 12 assertions passed");
