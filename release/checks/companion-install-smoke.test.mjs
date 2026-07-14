import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const read = (name) => readFileSync(new URL(name, import.meta.url), "utf8");

for (const [name, sourcePrefix, installedPrefix] of [
	["companion-smoke.sh", '"$companion"', '"$installed_companion"'],
	["companion-smoke.ps1", "& $Companion", "& $InstalledCompanion"],
]) {
	const script = read(name);
	for (const command of ["version", "setup", "install", "status", "doctor", "--replace", "rollback", "uninstall"]) {
		assert.match(script, new RegExp(command.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")), `${name} must exercise ${command}`);
	}
	assert.match(script, new RegExp(sourcePrefix.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")), `${name} must invoke the supplied local companion for setup`);
	assert.match(script, new RegExp(installedPrefix.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")), `${name} must invoke the installed companion for lifecycle commands`);
  assert.doesNotMatch(script, /curl|Invoke-WebRequest|https?:\/\//i, `${name} must not download a self-update`);
}

console.log("companion install smoke contract: 18 assertions passed");
