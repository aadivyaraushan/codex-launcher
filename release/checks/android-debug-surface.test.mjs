import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";

const root = new URL("../../", import.meta.url);
const read = (path) => readFileSync(new URL(path, root), "utf8");
const activityPath = "android/app/src/debug/kotlin/app/codexlauncher/debug/scenarios/UiScenarioActivity.kt";
const catalogPath = "android/app/src/debug/kotlin/app/codexlauncher/debug/scenarios/ScenarioCatalog.kt";
const apkIsolationPath = "release/checks/android-debug-apk-isolation.test.mjs";
const debugManifest = read("android/app/src/debug/AndroidManifest.xml");
const mainManifest = read("android/app/src/main/AndroidManifest.xml");
const androidWorkflow = read(".github/workflows/android.yml");

assert.equal(existsSync(new URL(activityPath, root)), true, "debug scenario activity must exist only in the debug source set");
assert.equal(existsSync(new URL(catalogPath, root)), true, "fixed scenario catalog must exist only in the debug source set");
assert.equal(existsSync(new URL(apkIsolationPath, root)), true, "built APK isolation check must exist");
assert.match(debugManifest, /android:name="\.debug\.scenarios\.UiScenarioActivity"/, "debug manifest must declare the audit activity");
assert.match(debugManifest, /android:permission="android\.permission\.DUMP"/, "only an ADB shell identity may open the audit activity");
assert.doesNotMatch(debugManifest, /android\.intent\.category\.(HOME|LAUNCHER)/, "audit activity must not become a launcher entry point");
assert.doesNotMatch(mainManifest, /UiScenarioActivity|SHOW_UI_SCENARIO/, "production manifest must not know about the audit surface");
assert.match(androidWorkflow, /node release\/checks\/android-debug-surface\.test\.mjs/, "Android CI must enforce debug-surface isolation");
assert.match(androidWorkflow, /node release\/checks\/android-debug-apk-isolation\.test\.mjs/, "Android CI must inspect the built debug and release APKs");

console.log("android debug surface contract: 9 assertions passed");
