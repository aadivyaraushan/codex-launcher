import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

test('Operator refreshes sign-in after its native gateway becomes ready', () => {
  const source = readFileSync(new URL('../../../../Sources/app/OperatorApp.swift', import.meta.url), 'utf8');
  const readiness = source.indexOf('try await self.embeddedRuntime.waitUntilReady(token: token)');
  assert.ok(readiness >= 0, 'test must locate the real embedded readiness path');
  const confirmed = source.indexOf('self.chat.runtimeBecameReady()', readiness);
  const refresh = source.indexOf('await self.setup.check()', confirmed);
  const catchBoundary = source.indexOf('} catch', confirmed);
  assert.ok(refresh > confirmed && refresh < catchBoundary,
    'refresh setup in the successful readiness path so an early unavailable result cannot hide Connect ChatGPT');
});
