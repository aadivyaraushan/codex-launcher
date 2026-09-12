import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import {stageRuntime} from '../../package/stage.mjs';

test('packages the real entry contract and patches only the copied public package', t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'operator-package-'));
  t.after(() => fs.rmSync(root, {recursive: true, force: true}));
  const source = path.join(root, 'source');
  const output = path.join(root, 'output');
  fs.mkdirSync(path.join(source, 'dist'), {recursive: true});
  fs.mkdirSync(path.join(source, 'node_modules'));
  const searchModule = 'node_modules/@openclaw/ai/dist/openai-chatgpt-responses-BAJ4gq3i.mjs';
  fs.mkdirSync(path.dirname(path.join(source, searchModule)), {recursive: true});
  const searchSource = fs.readFileSync(new URL(`../../../../build/native-node/runtime/openclaw/${searchModule}`, import.meta.url), 'utf8');
  fs.writeFileSync(path.join(source, searchModule), searchSource);
  fs.writeFileSync(path.join(source, 'package.json'), '{"name":"openclaw","version":"2026.9.1"}');
  fs.writeFileSync(path.join(source, 'dist/server-start-example.js'), 'export async function startGatewayServerCore() {}');
  const original = fs.readFileSync(new URL('../../../../build/native-node/runtime/openclaw/dist/openclaw-state-db-Bh3Bq87y.js', import.meta.url), 'utf8');
  fs.writeFileSync(path.join(source, 'dist/openclaw-state-db-Bh3Bq87y.js'), original);
  const lifecycle = fs.readFileSync(new URL('../../../../build/native-node/runtime/openclaw/dist/run-CrJnbDWP.js', import.meta.url), 'utf8');
  fs.writeFileSync(path.join(source, 'dist/run-CrJnbDWP.js'), lifecycle);
  for (const name of ['main-session-restart-recovery--2blnuu5.js', 'transcript-events-BMaG3A_w.js', 'chat-D3QlhTHk.js', 'chat-send-handler-DBjXcn_1.js']) {
    fs.copyFileSync(new URL(`../../../../build/native-node/runtime/openclaw/dist/${name}`, import.meta.url), path.join(source, 'dist', name));
  }
  const templateSource = new URL('../../../../build/native-node/runtime/openclaw/docs/reference/templates/', import.meta.url);
  fs.cpSync(templateSource, path.join(source, 'docs/reference/templates'), {recursive: true});
  fs.writeFileSync(path.join(source, 'private-account.json'), 'must-not-package');
  stageRuntime(source, output);
  assert.match(fs.readFileSync(path.join(output, 'openclaw', searchModule), 'utf8'), /\[native-search\] provider search completed/);
  assert.equal(fs.readFileSync(path.join(source, searchModule), 'utf8'), searchSource);
  assert.ok(fs.existsSync(path.join(output, 'entry.mjs')));
  assert.ok(fs.existsSync(path.join(output, 'host/start.mjs')));
  assert.ok(!fs.existsSync(path.join(output, 'openclaw/private-account.json')));
  assert.match(fs.readFileSync(path.join(output, 'openclaw/dist/openclaw-state-db-Bh3Bq87y.js'), 'utf8'), /Operator native ownership admission/);
  assert.equal(fs.readFileSync(path.join(source, 'dist/openclaw-state-db-Bh3Bq87y.js'), 'utf8'), original);
  assert.equal(JSON.parse(fs.readFileSync(path.join(output, 'manifest.json'))).gatewayModule, 'dist/server-start-example.js');
  assert.equal(JSON.parse(fs.readFileSync(path.join(output, 'manifest.json'))).lifecycleModule, 'dist/run-CrJnbDWP.js');
  assert.match(fs.readFileSync(path.join(output, 'openclaw/dist/run-CrJnbDWP.js'), 'utf8'), /export \{ runGatewayCommand, runGatewayLoop \};/);
  assert.ok(fs.existsSync(path.join(output, 'openclaw/dist/native-recovery-source-run.mjs')));
  assert.match(fs.readFileSync(path.join(output, 'openclaw/dist/main-session-restart-recovery--2blnuu5.js'), 'utf8'), /registerNativeRecoverySourceRun\(recoveryRunId, sourceRunId\)/);
  assert.match(fs.readFileSync(path.join(output, 'openclaw/dist/transcript-events-BMaG3A_w.js'), 'utf8'), /attachNativeRecoverySourceRunId\(messageWithRunId, normalizedRunId\)/);
  assert.equal(fs.readFileSync(path.join(output, 'openclaw/dist/chat-D3QlhTHk.js'), 'utf8').split('...operatorRecovery ? { operatorRecovery } : {}').length - 1, 2);
  assert.match(fs.readFileSync(path.join(output, 'openclaw/dist/chat-send-handler-DBjXcn_1.js'), 'utf8'), /request\.clientInfo\?\.id === "openclaw-ios"/);
  assert.ok(JSON.parse(fs.readFileSync(path.join(output, 'manifest.json'))).patches.includes('native-recovery-source-run'));
  assert.ok(JSON.parse(fs.readFileSync(path.join(output, 'manifest.json'))).patches.includes('native-ios-restart-safe-admission'));
  assert.equal(fs.readFileSync(path.join(source, 'dist/run-CrJnbDWP.js'), 'utf8'), lifecycle);
  for (const name of fs.readdirSync(templateSource)) {
    assert.deepEqual(fs.readFileSync(path.join(output, 'openclaw/docs/reference/templates', name)),
      fs.readFileSync(new URL(name, templateSource)), `bundled workspace template ${name}`);
  }
  assert.throws(() => stageRuntime(source, output), /exists/i);
});
