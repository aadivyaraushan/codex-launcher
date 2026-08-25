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
assert.doesNotMatch(html, /class="project-selector"/);
assert.match(html, /aria-label="Start on phone"/);
assert.match(html, /aria-label="Start on computer"/);
assert.match(design, /project\/folder pick inside that flow/i);
assert.match(design, /sending a computer task is still disabled until a project\/folder is selected/i);

assert.doesNotMatch(html, />Completed(?:<| ·)/);
assert.doesNotMatch(design, /\*\*Completed:\*\*/);
assert.match(html, />Replied(?:<| ·)/);
assert.match(reference, /Codex replied/i);

for (const action of ["Rename task", "Archive task", "Fork task"]) {
  assert.match(html, new RegExp(action));
}
console.log("design contract: 16 assertions passed");
