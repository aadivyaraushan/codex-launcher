import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const html = readFileSync(
  new URL("../../outputs/codex-launcher-visual-directions.html", import.meta.url),
  "utf8",
);
const design = readFileSync(new URL("../../DESIGN.md", import.meta.url), "utf8");
const reference = readFileSync(
  new URL("../../outputs/hermes-codex-android-reference.md", import.meta.url),
  "utf8",
);

const screenCalls = html.match(/\$\{screen\(/g) ?? [];
assert.equal(screenCalls.length, 7, "exactly seven screens must render per theme");
assert.doesNotMatch(html, /screen\("(?:Attention|Completion)", "0[89]"/);

assert.match(html, /studio-mac/);
assert.match(html, /class="project-selector"[^>]*>Codex Launcher</);
assert.match(html, /aria-label="Change project folder"/);
assert.match(html, /class="send disabled"[^>]*aria-disabled="true"/);
assert.match(design, /computer is fixed[^\n]+project\/folder[^\n]+changeable/i);
assert.match(design, /sending is disabled until a project\/folder is selected/i);

assert.doesNotMatch(html, />Completed(?:<| ·)/);
assert.doesNotMatch(design, /\*\*Completed:\*\*/);
assert.match(html, />Replied(?:<| ·)/);
assert.match(reference, /Codex replied/i);

for (const action of ["Rename task", "Archive task", "Fork task"]) {
  assert.match(html, new RegExp(action));
}
for (const option of ["Model", "Reasoning", "Permission mode"]) {
  assert.match(html, new RegExp(option));
}
assert.match(html, /aria-label="Dictate prompt"/);

console.log("design contract: 19 assertions passed");
