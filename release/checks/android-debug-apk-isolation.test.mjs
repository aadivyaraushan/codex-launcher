import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { delimiter, join } from "node:path";
import { spawnSync } from "node:child_process";

const debugApk = process.argv[2] ?? "android/app/build/outputs/apk/debug/app-debug.apk";
const releaseApk = process.argv[3] ?? "android/app/build/outputs/apk/release/app-release-unsigned.apk";

assert.equal(existsSync(debugApk), true, `debug APK is missing: ${debugApk}`);
assert.equal(existsSync(releaseApk), true, `release APK is missing: ${releaseApk}`);

const sdkRoots = [process.env.ANDROID_HOME, process.env.ANDROID_SDK_ROOT].filter(Boolean);
const pathCandidates = (process.env.PATH ?? "")
  .split(delimiter)
  .filter(Boolean)
  .map((entry) => join(entry, "apkanalyzer"));
const analyzerCandidates = [
  ...sdkRoots.map((root) => join(root, "cmdline-tools", "latest", "bin", "apkanalyzer")),
  ...pathCandidates,
];
const apkanalyzer = analyzerCandidates.find(existsSync);
assert.ok(apkanalyzer, "apkanalyzer was not found under ANDROID_HOME, ANDROID_SDK_ROOT, or PATH");

const analyze = (...args) => {
  const result = spawnSync(apkanalyzer, args, { encoding: "utf8", maxBuffer: 64 * 1024 * 1024 });
  assert.equal(result.status, 0, `apkanalyzer ${args.slice(0, 2).join(" ")} failed: ${result.stderr}`);
  return result.stdout;
};

const debugManifest = analyze("manifest", "print", debugApk);
const releaseManifest = analyze("manifest", "print", releaseApk);
const debugDex = analyze("dex", "packages", "--defined-only", debugApk);
const releaseDex = analyze("dex", "packages", "--defined-only", releaseApk);

assert.match(debugManifest, /app\.codexlauncher\.debug\.scenarios\.UiScenarioActivity/, "debug APK must expose the shell audit activity");
assert.match(debugManifest, /android:permission="android\.permission\.DUMP"/, "debug audit activity must retain the shell-only permission");
assert.doesNotMatch(releaseManifest, /UiScenarioActivity|SHOW_UI_SCENARIO/, "release APK manifest must exclude the audit surface");
assert.match(debugDex, /app\.codexlauncher\.debug\.scenarios\.UiScenarioActivity/, "debug APK DEX must contain the audit implementation");
assert.doesNotMatch(releaseDex, /app\.codexlauncher\.debug\.scenarios|UiScenarioActivity|ScenarioCatalog/, "release APK DEX must exclude all audit implementation classes");

console.log("android built APK isolation: 7 assertions passed");
