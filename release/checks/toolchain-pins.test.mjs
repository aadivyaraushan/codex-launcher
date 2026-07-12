import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const read = (path) => readFileSync(new URL(`../../${path}`, import.meta.url), "utf8");

const goMod = read("go.mod");
assert.match(goMod, /^module github\.com\/codex-launcher\/codex-launcher$/m);
assert.match(goMod, /^go 1\.26\.0$/m);
assert.match(goMod, /^toolchain go1\.26\.5$/m);

const versions = read("android/gradle/libs.versions.toml");
for (const pin of [
  'agp = "9.2.1"',
  'composeBom = "2026.06.01"',
  'androidTestRunner = "1.7.0"',
  'androidTestJunit = "1.3.0"',
]) {
  assert.match(versions, new RegExp(pin.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
}

const wrapper = read("android/gradle/wrapper/gradle-wrapper.properties");
assert.match(wrapper, /gradle-9\.4\.1-bin\.zip/);
assert.match(wrapper, /distributionSha256Sum=/);

const appBuild = read("android/app/build.gradle.kts");
assert.match(appBuild, /compileSdk = 36/);
assert.match(appBuild, /targetSdk = 36/);
assert.match(appBuild, /jvmTarget\.set\(JvmTarget\.JVM_17\)/);
assert.doesNotMatch(appBuild, /org\.jetbrains\.kotlin\.android/);

console.log("toolchain pins: 13 assertions passed");
