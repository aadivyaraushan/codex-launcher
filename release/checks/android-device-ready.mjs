import { spawnSync } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

// `dumpsys window` says the word "showing" in several places that have
// nothing to do with the lock screen (a dream overlay, some random window's
// isShowing flag). The only line that actually answers "is the phone
// locked" lives inside the KeyguardServiceDelegate block, so we have to find
// that block first and only read showing= from inside it.
export function keyguardVerdict(dumpsysWindowText) {
  const lines = (dumpsysWindowText ?? "").split("\n");
  const indentOf = (line) => line.match(/^\s*/)[0].length;

  const unknown = {
    state: "unknown",
    message:
      "Could not tell whether the phone is locked: dumpsys window did not report a KeyguardServiceDelegate block. Check the connection and look at the phone yourself before continuing.",
  };

  const headerIndex = lines.findIndex((line) => /^\s*KeyguardServiceDelegate\b/.test(line));
  if (headerIndex === -1) return unknown;

  // The block ends where the indentation comes back out to the header's own
  // level (or shallower). Blank lines don't count either way.
  const headerIndent = indentOf(lines[headerIndex]);
  let showing = null;
  for (let i = headerIndex + 1; i < lines.length; i += 1) {
    const line = lines[i];
    if (line.trim().length > 0 && indentOf(line) <= headerIndent) break;
    const match = line.match(/^\s*showing=(true|false)/);
    if (match) {
      showing = match[1];
      break;
    }
  }

  if (showing === null) return unknown;

  if (showing === "true") {
    return {
      state: "locked",
      message: "The phone's screen is locked. Unlock it by hand, then run this again.",
    };
  }

  return { state: "unlocked", message: "The phone is unlocked." };
}

function main() {
  // If adb itself is missing or the phone won't answer, treat that the same
  // as an unreadable dump: say so and let the run continue. A check that
  // can't reach the phone should not become a new reason to block on it.
  const result = spawnSync("adb", ["shell", "dumpsys", "window"], {
    encoding: "utf8",
    maxBuffer: 32 * 1024 * 1024,
  });
  const verdict = keyguardVerdict(result.stdout ?? "");

  if (verdict.state === "locked") {
    console.error(verdict.message);
    process.exit(1);
  }

  if (verdict.state === "unknown") {
    console.warn(verdict.message);
    return;
  }

  console.log(verdict.message);
}

const invokedDirectly = process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url);
if (invokedDirectly) main();
