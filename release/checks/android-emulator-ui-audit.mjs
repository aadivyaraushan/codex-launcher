import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { basename, dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const decodeXml = (value) =>
  value
    .replaceAll("&quot;", '"')
    .replaceAll("&apos;", "'")
    .replaceAll("&lt;", "<")
    .replaceAll("&gt;", ">")
    .replaceAll("&amp;", "&");

const parseAttributes = (source) =>
  Object.fromEntries([...source.matchAll(/([\w-]+)="([^"]*)"/g)].map((match) => [match[1], decodeXml(match[2])]));

export function parseScenarioCatalog(source) {
  return [...source.matchAll(/^\s*[A-Z0-9_]+\("([^"]+)",\s*"([^"]+)"\),?\s*$/gm)].map((match) => ({
    wireName: match[1],
    expectedText: match[2],
  }));
}

export function parseNodes(xml) {
  return [...xml.matchAll(/<node\b([^>]*)>/g)]
    .map((match) => parseAttributes(match[1]))
    .filter((attributes) => /^\[\d+,\d+\]\[\d+,\d+\]$/.test(attributes.bounds ?? ""))
    .map((attributes) => {
      const bounds = [...attributes.bounds.matchAll(/\d+/g)].map((match) => Number(match[0]));
      return {
        text: attributes.text ?? "",
        contentDesc: attributes["content-desc"] ?? "",
        enabled: attributes.enabled !== "false",
        bounds,
        center: [Math.round((bounds[0] + bounds[2]) / 2), Math.round((bounds[1] + bounds[3]) / 2)],
      };
    });
}

export const sha256 = (bytes) => createHash("sha256").update(bytes).digest("hex");

export function adbTextCommands(value, { clear = false } = {}) {
  const typeCommand = ["input", "text", value.replaceAll(" ", "%s")];
  if (!clear) return [typeCommand];
  return [
    ["input", "keyevent", "KEYCODE_MOVE_END"],
    ["input", "keyevent", ...Array(32).fill("KEYCODE_DEL")],
    typeCommand,
  ];
}

export const sourceStatusArgs = [
  "status",
  "--porcelain",
  "--untracked-files=all",
  "--",
  ".",
  ":(exclude)outputs",
  ":(exclude)saved-results",
];

export function installDebugApk(audit, debugApk) {
  audit.runAdb(["install", "-r", "-t", debugApk]);
}

export function formatBuildIdentity({ apkSha256, sourceCommit, sourceDirty, avdName, buildFingerprint }) {
  return [
    `- AVD: ${avdName}`,
    `- Build fingerprint: ${buildFingerprint}`,
    `- Debug APK SHA-256: ${apkSha256}`,
    `- Source commit: ${sourceCommit}`,
    `- Source tree dirty during audit: ${sourceDirty ? "yes" : "no"}`,
  ].join("\n");
}

export function findNode(nodes, matcher) {
  const found = nodes.find((node) =>
    (matcher.text === undefined || node.text === matcher.text) &&
    (matcher.contentDesc === undefined || node.contentDesc === matcher.contentDesc) &&
    (matcher.enabled === undefined || node.enabled === matcher.enabled));
  assert.ok(found, `UI node not found: ${JSON.stringify(matcher)}`);
  return found;
}

const findControlLineage = (xml, label) => {
  const stack = [];
  const tokens = xml.match(/<node\b[^>]*\/>|<node\b[^>]*>|<\/node>/g) ?? [];
  for (const token of tokens) {
    if (token === "</node>") {
      stack.pop();
      continue;
    }
    const attributes = parseAttributes(token);
    const lineage = [...stack, attributes];
    if (attributes["content-desc"] === label || attributes.text === label) return lineage;
    if (!token.endsWith("/>")) stack.push(attributes);
  }
  throw new Error(`control not found: ${label}`);
};

export function controlIsDisabled(xml, label) {
  const control = findControlLineage(xml, label).findLast((candidate) => candidate.clickable === "true");
  return control?.enabled === "false";
}

export function controlIsSelected(xml, label) {
  return findControlLineage(xml, label).some((candidate) => candidate.selected === "true" || candidate.checked === "true");
}

const wait = (milliseconds = 350) => Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, milliseconds);

class EmulatorAudit {
  constructor({ root, serial, outputDir }) {
    this.root = root;
    this.serial = serial;
    this.outputDir = outputDir;
    this.results = [];
    const sdkRoot = process.env.ANDROID_HOME ?? process.env.ANDROID_SDK_ROOT ?? "/opt/homebrew/share/android-commandlinetools";
    this.adb = process.env.ADB ?? join(sdkRoot, "platform-tools", "adb");
    assert.equal(existsSync(this.adb), true, `adb is missing: ${this.adb}`);
  }

  runAdb(args, options = {}) {
    const result = spawnSync(this.adb, ["-s", this.serial, ...args], {
      encoding: options.binary ? null : "utf8",
      maxBuffer: 32 * 1024 * 1024,
    });
    assert.equal(result.status, 0, `adb ${args.join(" ")} failed: ${result.stderr?.toString() ?? ""}`);
    return result.stdout;
  }

  shell(...args) {
    return this.runAdb(["shell", ...args]);
  }

  launchScenario(wireName) {
    const output = this.shell(
      "am", "start", "-W",
      "-a", "app.codexlauncher.debug.SHOW_UI_SCENARIO",
      "-n", "app.codexlauncher/.debug.scenarios.UiScenarioActivity",
      "--es", "scenario", wireName,
    );
    assert.match(output, /Status: ok/, `scenario failed to launch: ${wireName}`);
    wait();
    return this.dumpUi();
  }

  launchProduction() {
    const output = this.shell("am", "start", "-W", "-n", "app.codexlauncher/.LauncherActivity");
    assert.match(output, /Status: ok/, "production launcher failed to open");
    wait(700);
    return this.dumpUi();
  }

  pressHome() {
    this.shell("input", "keyevent", "KEYCODE_HOME");
    wait(900);
    return this.dumpUi();
  }

  waitForBoot() {
    this.runAdb(["wait-for-device"]);
    for (let attempt = 0; attempt < 90; attempt += 1) {
      if (this.shell("getprop", "sys.boot_completed").trim() === "1") {
        wait(1_000);
        return;
      }
      wait(1_000);
    }
    throw new Error("Android did not report boot completion within 90 seconds");
  }

  dumpUi() {
    this.shell("uiautomator", "dump", "/sdcard/codex-launcher-audit.xml");
    return this.runAdb(["exec-out", "cat", "/sdcard/codex-launcher-audit.xml"]);
  }

  expectText(xml, expected) {
    const visible = parseNodes(xml).flatMap((node) => [node.text, node.contentDesc]).filter(Boolean);
    assert.ok(visible.some((value) => value.includes(expected)), `expected visible text was missing: ${expected}`);
  }

  expectNoText(xml, unexpected) {
    const visible = parseNodes(xml).flatMap((node) => [node.text, node.contentDesc]).filter(Boolean);
    assert.equal(visible.some((value) => value.includes(unexpected)), false, `unexpected visible text remained: ${unexpected}`);
  }

  waitForText(expected, attempts = 5) {
    let lastError;
    for (let attempt = 0; attempt < attempts; attempt += 1) {
      try {
        const xml = this.dumpUi();
        this.expectText(xml, expected);
        return xml;
      } catch (error) {
        lastError = error;
        wait(1_000);
      }
    }
    throw lastError;
  }

  tap(matcher) {
    const node = findNode(parseNodes(this.dumpUi()), matcher);
    this.shell("input", "tap", String(node.center[0]), String(node.center[1]));
    wait();
    return this.dumpUi();
  }

  typeInto(matcher, value, { clear = false } = {}) {
    const node = findNode(parseNodes(this.dumpUi()), matcher);
    this.shell("input", "tap", String(node.center[0]), String(node.center[1]));
    wait(600);
    for (const command of adbTextCommands(value, { clear })) this.shell(...command);
    wait(600);
    let xml = this.dumpUi();
    const valueVisible = () => parseNodes(xml).some((candidate) => candidate.text.includes(value));
    if (!valueVisible()) {
      wait(800);
      xml = this.dumpUi();
    }
    if (!valueVisible()) {
      for (const command of adbTextCommands(value, { clear: true })) this.shell(...command);
      wait(600);
      xml = this.dumpUi();
    }
    return xml;
  }

  hideKeyboard() {
    this.shell("input", "keyevent", "KEYCODE_BACK");
    wait();
  }

  screenshot(name) {
    const path = join(this.outputDir, `${name}.png`);
    writeFileSync(path, this.runAdb(["exec-out", "screencap", "-p"], { binary: true }));
    return path;
  }

  check(label, action, screenshotName = null) {
    try {
      action();
      const screenshot = screenshotName ? this.screenshot(screenshotName) : null;
      this.results.push({ label, status: "PASS", screenshot });
    } catch (error) {
      const screenshot = screenshotName ? this.screenshot(`${screenshotName}-FAILED`) : null;
      this.results.push({ label, status: "FAIL", detail: error.message, screenshot });
      throw error;
    }
  }

  writeReport({ device, androidVersion, scenarios, buildIdentity }) {
    const reportPath = join(this.root, "saved-results", "android-16-hands-on-ui-audit.md");
    const passed = this.results.filter((result) => result.status === "PASS").length;
    const failed = this.results.length - passed;
    const rows = this.results.map((result) => {
      const screenshot = result.screenshot ? ` [screenshot](../${result.screenshot.slice(this.root.length + 1)})` : "";
      const detail = result.detail ? ` — ${result.detail}` : "";
      return `- ${result.status}: ${result.label}${screenshot}${detail}`;
    });
    const body = `# Android 16 hands-on UI audit

Date: ${new Date().toISOString()}

Purpose: drive the Codex Launcher on a running Android emulator using Android UI dumps plus real tap, type, Back, and screenshot commands. The fixed-state activity renders production Compose surfaces with local synthetic data; it does not prove a live Tailscale or Codex account connection.

## Environment

- Device: ${device}
- Android: ${androidVersion}
- ADB serial: ${this.serial}
${formatBuildIdentity(buildIdentity)}
- Fixed states read from the Kotlin catalog: ${scenarios}
- Checks: ${passed} passed, ${failed} failed

## Results

${rows.join("\n")}

## Reproduce

\`ANDROID_HOME=/opt/homebrew/share/android-commandlinetools node release/checks/android-emulator-ui-audit.mjs\`
`;
    writeFileSync(reportPath, body);
    return reportPath;
  }
}

function runAudit() {
  const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
  const outputDir = join(root, "outputs", "android-vm-audit");
  mkdirSync(outputDir, { recursive: true });
  mkdirSync(join(root, "saved-results"), { recursive: true });
  const serial = process.env.ANDROID_SERIAL ?? "emulator-5554";
  const audit = new EmulatorAudit({ root, serial, outputDir });
  const catalogSource = readFileSync(join(root, "android/app/src/debug/kotlin/app/codexlauncher/debug/scenarios/ScenarioCatalog.kt"), "utf8");
  const scenarios = parseScenarioCatalog(catalogSource);
  assert.ok(scenarios.length >= 40, `scenario catalog unexpectedly small: ${scenarios.length}`);

  const device = audit.shell("getprop", "ro.product.model").trim();
  const androidVersion = audit.shell("getprop", "ro.build.version.release").trim();
  assert.equal(androidVersion, "16", `expected Android 16, got ${androidVersion}`);

  const debugApk = join(root, "android/app/build/outputs/apk/debug/app-debug.apk");
  assert.equal(existsSync(debugApk), true, `debug APK is missing: ${debugApk}`);
  const gitHead = spawnSync("git", ["rev-parse", "HEAD"], { cwd: root, encoding: "utf8" });
  assert.equal(gitHead.status, 0, `git rev-parse HEAD failed: ${gitHead.stderr}`);
  const gitStatus = spawnSync("git", sourceStatusArgs, { cwd: root, encoding: "utf8" });
  assert.equal(gitStatus.status, 0, `git status --porcelain failed: ${gitStatus.stderr}`);
  const buildIdentity = {
    apkSha256: sha256(readFileSync(debugApk)),
    sourceCommit: gitHead.stdout.trim(),
    sourceDirty: gitStatus.stdout.trim().length > 0,
    avdName: audit.shell("getprop", "ro.boot.qemu.avd_name").trim() || "unknown",
    buildFingerprint: audit.shell("getprop", "ro.build.fingerprint").trim(),
  };
  installDebugApk(audit, debugApk);
  assert.match(audit.shell("pm", "clear", "app.codexlauncher"), /Success/, "test app data reset failed");

  audit.check("production first launch uses the real pairing screen", () => {
    audit.expectText(audit.launchProduction(), "Pair with your computer");
  }, "production-first-launch");

  audit.check("production launcher becomes Android Home and opens on Home key", () => {
    audit.shell("cmd", "role", "add-role-holder", "--user", "0", "android.app.role.HOME", "app.codexlauncher");
    audit.expectText(audit.pressHome(), "Pair with your computer");
  }, "production-default-home");

  audit.check("production pairing rejects an invalid manual link", () => {
    audit.launchProduction();
    audit.tap({ text: "Enter link" });
    audit.typeInto({ contentDesc: "Pairing link" }, "invalid");
    audit.hideKeyboard();
    audit.expectText(audit.tap({ text: "Pair computer" }), "That pairing link isn't valid");
  }, "production-pairing-invalid");

  audit.check("production pairing requests and receives Android camera permission", () => {
    audit.shell("am", "force-stop", "app.codexlauncher");
    audit.launchProduction();
    let xml = audit.tap({ text: "Allow camera" });
    const permissionChoice = parseNodes(xml).find((node) => ["While using the app", "Only this time"].includes(node.text));
    if (permissionChoice) {
      audit.shell("input", "tap", String(permissionChoice.center[0]), String(permissionChoice.center[1]));
      wait(800);
      xml = audit.dumpUi();
    }
    audit.expectText(xml, "Pairing QR camera");
  }, "production-camera-permission");

  audit.check("production app drawer shows no-match state and opens launcher settings", () => {
    audit.launchProduction();
    audit.tap({ text: "All apps" });
    audit.expectText(audit.typeInto({ contentDesc: "Search apps" }, "nothinginstalled"), "No matching apps");
    audit.hideKeyboard();
    audit.expectText(audit.tap({ text: "Launcher settings" }), "Appearance");
  }, "production-app-drawer");

  audit.check("production Dark mode persists across process restart", () => {
    let xml = audit.tap({ text: "Dark" });
    assert.equal(controlIsSelected(xml, "Dark"), true, "Dark should be selected after tapping it");
    audit.shell("am", "force-stop", "app.codexlauncher");
    audit.launchProduction();
    audit.tap({ text: "All apps" });
    xml = audit.tap({ text: "Launcher settings" });
    assert.equal(controlIsSelected(xml, "Dark"), true, "Dark should remain selected after process restart");
  }, "production-dark-persistence");

  audit.check("production Android Settings escape leaves the launcher", () => {
    audit.shell("am", "force-stop", "app.codexlauncher");
    audit.launchProduction();
    audit.tap({ text: "Android Settings" });
    const activities = audit.shell("dumpsys", "activity", "activities");
    assert.match(activities, /topResumedActivity=.*com\.android\.settings/, "Android Settings did not become the top activity");
  }, "production-android-settings");

  try {
    audit.check("production launcher remains usable at 1.3× Android font scale", () => {
      audit.shell("settings", "put", "system", "font_scale", "1.3");
      audit.shell("am", "force-stop", "app.codexlauncher");
      audit.pressHome();
      audit.waitForText("Pair with your computer");
    }, "production-large-font");
  } finally {
    audit.shell("settings", "put", "system", "font_scale", "1.0");
  }

  audit.check("production Home recovers after force-stop", () => {
    audit.shell("am", "force-stop", "app.codexlauncher");
    audit.expectText(audit.pressHome(), "Pair with your computer");
  }, "production-force-stop-home");

  for (const scenario of scenarios) {
    audit.check(`render ${scenario.wireName} → ${scenario.expectedText}`, () => {
      const xml = audit.launchScenario(scenario.wireName);
      audit.expectText(xml, scenario.expectedText);
    }, `state-${scenario.wireName}`);
  }

  audit.check("pairing camera permission action", () => {
    audit.launchScenario("pairing_qr_permission");
    audit.expectText(audit.tap({ text: "Allow camera" }), "Camera permission request recorded for this audit");
  }, "interaction-pairing-camera");

  audit.check("manual pairing invalid-link recovery", () => {
    audit.launchScenario("pairing_manual");
    audit.typeInto({ contentDesc: "Pairing link" }, "invalid");
    audit.expectText(audit.tap({ text: "Pair computer" }), "That pairing link isn't valid");
  }, "interaction-pairing-invalid");

  audit.check("pairing save retry", () => {
    audit.launchScenario("pairing_save_retry");
    audit.expectText(audit.tap({ text: "Retry saving" }), "Saving…");
  }, "interaction-pairing-save-retry");

  audit.check("project is required before first send", () => {
    let xml = audit.launchScenario("home_choose_project");
    assert.equal(controlIsDisabled(xml, "Send prompt"), true, "Send prompt should be disabled without a project");
    xml = audit.typeInto({ contentDesc: "Prompt" }, "do not send");
    audit.expectText(xml, "do not send");
    audit.hideKeyboard();
  }, "interaction-home-project-required");

  audit.check("Home model choice and prompt send", () => {
    audit.launchScenario("home_online");
    audit.expectText(audit.tap({ contentDesc: "Choose model" }), "Sample model B");
    audit.expectText(audit.tap({ text: "Sample model B" }), "Low");
    audit.typeInto({ contentDesc: "Prompt" }, "run sample checks");
    audit.hideKeyboard();
    audit.expectText(audit.tap({ contentDesc: "Send prompt" }), "Sample prompt sent");
  }, "interaction-home-send");

  audit.check("offline connection help", () => {
    audit.launchScenario("home_offline");
    audit.expectText(audit.tap({ text: "Connection help" }), "Check that the computer, companion, and Tailscale are online.");
  }, "interaction-offline-help");

  audit.check("working task redirect and confirmed stop", () => {
    audit.launchScenario("task_controls_working");
    audit.typeInto({ contentDesc: "Follow-up message" }, "use newer approach");
    audit.hideKeyboard();
    audit.tap({ text: "Redirect" });
    audit.expectText(audit.tap({ text: "Redirect now" }), "Current turn redirected");

    audit.launchScenario("task_controls_working");
    audit.expectText(audit.tap({ text: "Stop" }), "Stop this task?");
    audit.expectText(audit.tap({ text: "Stop task" }), "Stop confirmed");
  }, "interaction-task-control");

  audit.check("idle task dictation and attachment removal", () => {
    audit.launchScenario("task_controls_idle");
    let xml = audit.tap({ contentDesc: "Dictate follow-up" });
    audit.expectText(xml, "Spoken sample");
    audit.expectText(xml, "Dictation added");

    audit.launchScenario("task_controls_idle");
    xml = audit.tap({ text: "Attach" });
    audit.expectText(xml, "sample-follow-up.txt");
    xml = audit.tap({ contentDesc: "Remove sample-follow-up.txt" });
    audit.expectNoText(xml, "sample-follow-up.txt");
  }, "interaction-task-inputs");

  audit.check("task rename, archive, and fork menus", () => {
    audit.launchScenario("task_controls_idle");
    audit.tap({ contentDesc: "Task actions" });
    audit.tap({ text: "Rename task" });
    audit.typeInto({ contentDesc: "New task title" }, "Renamed sample", { clear: true });
    audit.hideKeyboard();
    audit.expectText(audit.tap({ text: "Save" }), "Renamed: Renamed sample");

    audit.launchScenario("task_controls_idle");
    audit.tap({ contentDesc: "Task actions" });
    audit.tap({ text: "Archive task" });
    audit.expectText(audit.tap({ text: "Archive" }), "Archived");

    audit.launchScenario("task_controls_idle");
    audit.tap({ contentDesc: "Task actions" });
    audit.expectText(audit.tap({ text: "Fork task" }), "Forked");
  }, "interaction-task-actions");

  audit.check("redacted and sending approvals stay fail-closed", () => {
    let xml = audit.launchScenario("approval_redacted");
    assert.equal(controlIsDisabled(xml, "Allow once"), true);
    assert.equal(controlIsDisabled(xml, "Allow for session"), true);
    audit.expectText(audit.tap({ text: "Deny" }), "Decision: decline");

    xml = audit.launchScenario("approval_sending");
    for (const label of ["Allow once", "Allow for session", "Deny", "Deny and stop"]) {
      assert.equal(controlIsDisabled(xml, label), true, `${label} should be disabled while sending`);
    }
  }, "interaction-approval-safety");

  audit.check("free-text, dismiss, and secret question paths", () => {
    audit.launchScenario("question_free_text");
    audit.typeInto({ contentDesc: "Answer: Details" }, "check parser");
    audit.hideKeyboard();
    audit.expectText(audit.tap({ text: "Send answer" }), "Answer sent: check parser");

    audit.launchScenario("question_choice");
    audit.expectText(audit.tap({ text: "Not now" }), "Question dismissed");

    audit.launchScenario("question_secret");
    audit.expectText(audit.tap({ text: "Answer on computer" }), "Answer on computer");
  }, "interaction-question-safety");

  audit.check("app search empty state and navigation", () => {
    audit.launchScenario("apps");
    audit.expectText(audit.typeInto({ contentDesc: "Search apps" }, "nothinginstalled"), "No matching apps");
    audit.hideKeyboard();
    audit.expectText(audit.tap({ text: "Android Settings" }), "Android Settings requested");
  }, "interaction-app-search");

  audit.check("appearance choices", () => {
    audit.launchScenario("appearance");
    let xml = audit.tap({ text: "Light" });
    audit.expectText(xml, "Light preview");
    xml = audit.tap({ text: "Dark" });
    audit.expectText(xml, "Dark preview");
    audit.tap({ text: "Follow system" });
    wait(1_000);
    audit.waitForText("Appearance");
  }, "interaction-appearance");

  audit.check("recovery and attachment-file dialog actions", () => {
    audit.launchScenario("recovery");
    audit.expectText(audit.tap({ text: "Try again" }), "Recovery retry requested");
    audit.launchScenario("dialog_attach");
    audit.expectText(audit.tap({ text: "File" }), "Attachment choice: file");
  }, "interaction-recovery-dialog");

  audit.check("production default Home recovers after Android reboot", () => {
    audit.runAdb(["reboot"]);
    audit.waitForBoot();
    audit.pressHome();
    audit.waitForText("Pair with your computer");
  }, "production-reboot-home");

  const report = audit.writeReport({ device, androidVersion, scenarios: scenarios.length, buildIdentity });
  console.log(`Android emulator UI audit: ${audit.results.length} checks passed`);
  console.log(`Report: ${report}`);
}

const invokedDirectly = process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href;
if (invokedDirectly) runAudit();
