const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const {
  validateWaitlistSubmission,
  assertSheetWebhookSuccess
} = require('./validate.js');

describe('validateWaitlistSubmission', () => {
  const now = 1_000_000;

  it('accepts a normal email after enough time with empty honeypot', () => {
    const result = validateWaitlistSubmission(
      { email: 'you@example.com', website: '', startedAt: now - 3000 },
      { now }
    );
    assert.equal(result.ok, true);
    assert.equal(result.email, 'you@example.com');
    assert.equal(result.silentDrop, false);
  });

  it('rejects missing or invalid email', () => {
    const missing = validateWaitlistSubmission(
      { email: '', website: '', startedAt: now - 3000 },
      { now }
    );
    assert.equal(missing.ok, false);
    assert.equal(missing.status, 400);

    const bad = validateWaitlistSubmission(
      { email: 'not-an-email', website: '', startedAt: now - 3000 },
      { now }
    );
    assert.equal(bad.ok, false);
    assert.equal(bad.status, 400);
  });

  it('silently drops honeypot fills (bot bait)', () => {
    const result = validateWaitlistSubmission(
      { email: 'bot@example.com', website: 'http://spam.example', startedAt: now - 3000 },
      { now }
    );
    assert.equal(result.ok, true);
    assert.equal(result.silentDrop, true);
  });

  it('silently drops submissions that arrive too fast', () => {
    const result = validateWaitlistSubmission(
      { email: 'fast@example.com', website: '', startedAt: now - 200 },
      { now, minMs: 1500 }
    );
    assert.equal(result.ok, true);
    assert.equal(result.silentDrop, true);
  });

  it('trims and lowercases email on accept', () => {
    const result = validateWaitlistSubmission(
      { email: '  You@Example.COM ', website: '', startedAt: now - 3000 },
      { now }
    );
    assert.equal(result.ok, true);
    assert.equal(result.email, 'you@example.com');
  });
});

describe('assertSheetWebhookSuccess', () => {
  it('fails when HTTP is not ok', () => {
    assert.throws(
      () => assertSheetWebhookSuccess(false, '{"ok":true}'),
      /sheet_webhook_http/
    );
  });

  it('fails when body says ok:false even if HTTP 200', () => {
    assert.throws(
      () => assertSheetWebhookSuccess(true, '{"ok":false,"error":"unauthorized"}'),
      /sheet_webhook_body/
    );
  });

  it('passes when HTTP ok and body ok:true', () => {
    assert.equal(assertSheetWebhookSuccess(true, '{"ok":true}'), true);
  });
});
