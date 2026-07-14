import assert from "node:assert/strict";
import test from "node:test";
import {
  adbTextCommands,
  formatBuildIdentity,
  installDebugApk,
  controlIsDisabled,
  controlIsSelected,
  findNode,
  parseNodes,
  parseScenarioCatalog,
  sha256,
  sourceStatusArgs,
} from "./android-emulator-ui-audit.mjs";

test("typing retry replaces the field instead of appending the same text twice", () => {
  const commands = adbTextCommands("run sample checks", { clear: true });
  assert.deepEqual(commands[0], ["input", "keyevent", "KEYCODE_MOVE_END"]);
  assert.equal(commands[1][0], "input");
  assert.equal(commands[1][1], "keyevent");
  assert.deepEqual(commands.at(-1), ["input", "text", "run%ssample%schecks"]);
  assert.equal(commands.filter((command) => command[1] === "text").length, 1);
});

test("audit always installs the exact APK it is about to exercise", () => {
  const calls = [];
  installDebugApk({ runAdb: (args) => calls.push(args) }, "/tmp/current-debug.apk");
  assert.deepEqual(calls, [["install", "-r", "-t", "/tmp/current-debug.apk"]]);
});

test("audit records a reproducible APK and source identity", () => {
  assert.equal(sha256(Buffer.from("current apk")), "6b54857b74ed0443123e04f31bc11e00e2e4f170ce5b771e44d183080b40884a");
  assert.equal(
    formatBuildIdentity({
      apkSha256: "abc123",
      sourceCommit: "def456",
      sourceDirty: true,
      avdName: "pixel_9_api_36",
      buildFingerprint: "google/sdk/fingerprint",
    }),
    [
      "- AVD: pixel_9_api_36",
      "- Build fingerprint: google/sdk/fingerprint",
      "- Debug APK SHA-256: abc123",
      "- Source commit: def456",
      "- Source tree dirty during audit: yes",
    ].join("\n"),
  );
  assert.deepEqual(sourceStatusArgs, [
    "status",
    "--porcelain",
    "--untracked-files=all",
    "--",
    ".",
    ":(exclude)outputs",
    ":(exclude)saved-results",
  ]);
});

test("scenario catalog parser keeps the Kotlin catalog as the source of truth", () => {
  const source = `enum class ScenarioId {\nHOME_ONLINE("home_online", "Sample task"),\nDIALOG_ATTACH("dialog_attach", "Attach"),\n}`;
  assert.deepEqual(parseScenarioCatalog(source), [
    { wireName: "home_online", expectedText: "Sample task" },
    { wireName: "dialog_attach", expectedText: "Attach" },
  ]);
});

test("UI parser decodes labels and calculates tappable centers", () => {
  const xml = `<hierarchy><node text="Allow &amp; continue" content-desc="Allow camera" enabled="true" bounds="[10,20][110,220]" /></hierarchy>`;
  const nodes = parseNodes(xml);
  assert.deepEqual(findNode(nodes, { contentDesc: "Allow camera" }), {
    text: "Allow & continue",
    contentDesc: "Allow camera",
    enabled: true,
    bounds: [10, 20, 110, 220],
    center: [60, 120],
  });
});

test("disabled control check follows the clickable parent around a Compose label", () => {
  const xml = `<node clickable="true" enabled="false" bounds="[0,0][100,100]"><node content-desc="Send prompt" enabled="true" bounds="[0,0][100,100]" /></node>`;
  assert.equal(controlIsDisabled(xml, "Send prompt"), true);
  assert.equal(controlIsDisabled(xml.replace('enabled="false"', 'enabled="true"'), "Send prompt"), false);

  const textLabel = `<node clickable="true" enabled="false" bounds="[0,0][100,100]"><node text="Allow once" enabled="true" bounds="[0,0][100,100]" /></node>`;
  assert.equal(controlIsDisabled(textLabel, "Allow once"), true);
});

test("selected control check follows the selectable parent around its label", () => {
  const xml = `<node clickable="true" selected="true" bounds="[0,0][100,100]"><node text="Dark" selected="false" bounds="[0,0][100,100]" /></node>`;
  assert.equal(controlIsSelected(xml, "Dark"), true);
  assert.equal(controlIsSelected(xml.replace('selected="true"', 'selected="false"'), "Dark"), false);

  const radio = `<node checkable="true" checked="true" bounds="[0,0][100,100]"><node text="Dark" bounds="[0,0][100,100]" /></node>`;
  assert.equal(controlIsSelected(radio, "Dark"), true);
});
