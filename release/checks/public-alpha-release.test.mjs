import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";

const read = (path) => readFileSync(new URL(`../../${path}`, import.meta.url), "utf8");

const requiredFiles = [
  ".github/workflows/android.yml",
  ".github/workflows/companion.yml",
  ".github/workflows/release.yml",
  "docs/setup/android.md",
  "docs/setup/companion.md",
  "docs/security/threat-model.md",
  "docs/compatibility/codex.md",
  "release/companion-package/main.go",
  "release/companion-package/main_test.go",
];
for (const path of requiredFiles) {
  assert.equal(existsSync(new URL(`../../${path}`, import.meta.url)), true, `${path} must exist`);
}

const android = read(".github/workflows/android.yml");
assert.match(android, /actions\/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7/);
assert.match(android, /actions\/setup-java@0f481fcb613427c0f801b606911222b5b6f3083a # v5/);
assert.match(android, /gradle\/actions\/setup-gradle@90ddb51e90a5fd9ba75f40cf85156b7b41bf76a3 # v6/);
assert.match(android, /reactivecircus\/android-emulator-runner@4c44018e59b437e86cdfc41da381398f93ed8808 # v2/);
assert.match(android, /api-level:\s*36/);
assert.match(android, /connectedDebugAndroidTest/);
assert.match(android, /lintDebug/);

const companion = read(".github/workflows/companion.yml");
assert.match(companion, /actions\/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6/);
assert.match(companion, /go test \.\/\.\.\. -race -count=1/);
assert.match(companion, /go vet \.\/\.\.\./);
assert.match(companion, /companion-smoke\.(sh|ps1)/);

const release = read(".github/workflows/release.yml");
for (const workflow of [android, companion, release]) {
  for (const line of workflow.split("\n").filter((value) => value.includes("uses:"))) {
    assert.match(line, /@[0-9a-f]{40}\s+#\s+v\d+/, `action must be pinned by commit: ${line.trim()}`);
  }
}
for (const expected of [
  "actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7",
  "actions/attest@f6bf1532d7d6793fce74eac584813a8eee607999 # v4",
  "anchore/scan-action@e1165082ffb1fe366ebaf02d8526e7c4989ea9d2 # v7",
  "anchore/sbom-action@e22c389904149dbc22b58101806040fa8d37a610 # v0",
  "id-token: write",
  "attestations: write",
  "CODEX_LAUNCHER_STORE_FILE",
  "CODEX_LAUNCHER_STORE_PASSWORD",
  "CODEX_LAUNCHER_KEY_ALIAS",
  "CODEX_LAUNCHER_KEY_PASSWORD",
]) {
  assert.ok(release.includes(expected), `release workflow must include ${expected}`);
}
assert.doesNotMatch(
  release,
  /codex-launcher-\$\{\{\s*inputs\.version\s*\}\}/,
  "workflow inputs must not be pasted into shell commands",
);
for (const secret of [
  "CODEX_LAUNCHER_STORE_PASSWORD",
  "CODEX_LAUNCHER_KEY_ALIAS",
  "CODEX_LAUNCHER_KEY_PASSWORD",
]) {
  assert.doesNotMatch(
    release,
    new RegExp(`echo [^\\n]*${secret}[^\\n]*GITHUB_ENV`),
    `${secret} must not persist through GITHUB_ENV`,
  );
}
assert.match(release, /Remove temporary signing key/);
assert.match(release, /THIRD_PARTY_NOTICES/);
assert.match(release, /sha256/i);
assert.match(release, /\.apk/);
assert.match(release, /\.aab/);
for (const requiredGate of [
  "validate-repository:",
  "go test ./... -race -count=1",
  "go vet ./...",
  "testDebugUnitTest lintDebug",
  "release/checks/*.test.mjs",
  "python3 -m venv .release-schema-venv",
  "pip install --requirement release/checks/protocol/requirements.txt",
  "release/checks/protocol/schema_test.py",
  "validate-android-16:",
  "connectedDebugAndroidTest",
]) {
  assert.ok(release.includes(requiredGate), `manual release must run ${requiredGate}`);
}
assert.match(release, /companion:\n\s+needs:\s*\[validate-repository, validate-android-16\]/);
assert.match(release, /android:\n\s+needs:\s*\[validate-repository, validate-android-16\]/);
assert.match(release, /git tag "v\$\{VERSION\}" "\$\{GITHUB_SHA\}"/);
assert.match(release, /--verify-tag/);
assert.match(release, /--target "\$\{GITHUB_SHA\}"/);
assert.match(release, /--prerelease/);
assert.match(release, /--latest=false/);
const companionPackager = read("release/companion-package/main.go");
for (const targetPart of ["darwin", "linux", "windows", "amd64", "arm64"]) {
  assert.match(companionPackager, new RegExp(targetPart));
}

const appBuild = read("android/app/build.gradle.kts");
for (const variable of [
  "CODEX_LAUNCHER_STORE_FILE",
  "CODEX_LAUNCHER_STORE_PASSWORD",
  "CODEX_LAUNCHER_KEY_ALIAS",
  "CODEX_LAUNCHER_KEY_PASSWORD",
]) {
  assert.ok(appBuild.includes(variable), `Android signing must read ${variable}`);
}

const readme = read("README.md");
assert.doesNotMatch(readme, /not yet a usable launcher/i);
for (const doc of [
  "docs/setup/android.md",
  "docs/setup/companion.md",
  "docs/security/threat-model.md",
  "docs/compatibility/codex.md",
]) {
  assert.ok(readme.includes(doc), `README must link ${doc}`);
}

const combinedDocs = [
  read("docs/setup/android.md"),
  read("docs/setup/companion.md"),
  read("docs/security/threat-model.md"),
  read("docs/compatibility/codex.md"),
].join("\n");
for (const phrase of [
  "Computer offline",
  "Tailscale",
  "ChatGPT",
  "unsigned technical alpha",
  "Pixel 9",
  "Android 16",
  "experimental",
  "uninstall",
]) {
  assert.ok(combinedDocs.toLowerCase().includes(phrase.toLowerCase()), `documentation must cover ${phrase}`);
}

const companionSetup = read("docs/setup/companion.md");
assert.match(companionSetup, /Get-FileHash[^\n]+SHA256/);
for (const command of [
  "setup",
  "pair",
  "devices",
  "revoke DEVICE_ID",
  "install --replace C:\\absolute\\path\\to\\new\\codex-launcher.exe",
  "rollback",
  "uninstall",
]) {
  assert.ok(
    companionSetup.includes(`.\\codex-launcher.exe ${command}`),
    `PowerShell setup must include an explicit ${command} command`,
  );
}
assert.doesNotMatch(
  combinedDocs,
  /(?<![.\\/\w-])codex-launcher(?:\.exe)?\s+(?:pair|devices|revoke|install|rollback|uninstall|status|doctor|version)\b/,
  "public docs must not assume the companion was added to PATH",
);
for (const command of [
  "./codex-launcher install --replace /absolute/path/to/new/codex-launcher",
  "./codex-launcher rollback",
  "./codex-launcher uninstall",
]) {
  assert.ok(companionSetup.includes(command), `Unix setup must use explicit command ${command}`);
}

const checkpoint = read("saved-results/task-14-public-alpha-release-checkpoint.md");
assert.match(checkpoint, /SOURCE_DATE_EPOCH=/);
assert.match(checkpoint, /-output dist\/companion/);
assert.match(checkpoint, /-version 0\.1\.0-alpha\.1/);
assert.doesNotMatch(checkpoint, /-output-dir|-source-date-epoch/);

console.log("public alpha release contract: 120 assertions passed");
