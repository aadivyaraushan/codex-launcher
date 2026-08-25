import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const read = (path) => readFileSync(new URL(`../../${path}`, import.meta.url), "utf8");

const ignore = read(".gitignore");
for (const pattern of [
  ".gstack/",
  "work/",
  ".gradle/",
  "build/",
  "*.jks",
  "*.p12",
  "*.key",
  "*.pem",
  "*.token",
  "*.qcow2",
  "local.properties",
]) {
  assert.match(ignore, new RegExp(pattern.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
}

assert.match(read("LICENSE"), /Apache License\s+Version 2\.0, January 2004/);

const notice = read("NOTICE");
assert.match(notice, /Codex Launcher/);
assert.match(notice, /Instrument Sans/);
assert.match(notice, /JetBrains Mono/);
assert.match(notice, /SIL Open Font License 1\.1/);

const thirdPartyNotices = read("THIRD_PARTY_NOTICES.md");
assert.match(thirdPartyNotices, /Moonshine Voice/);
assert.match(thirdPartyNotices, /ONNX Runtime/);

const readme = read("README.md");
assert.match(readme, /technical alpha/i);
assert.match(readme, /Android 16/i);
assert.match(readme, /relay box/i);
assert.doesNotMatch(readme, /connects directly.*Tailscale/is);
assert.match(readme, /Computer offline/);
assert.match(readme, /does not copy.*ChatGPT.*credential/is);

console.log("repository baseline: 23 assertions passed");
