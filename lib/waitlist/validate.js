'use strict';

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/**
 * Validate a waitlist POST body.
 * Inputs: { email, website?, startedAt? }, options { now?, minMs? }
 * Outputs: { ok, status?, email?, silentDrop?, error? }
 */
function validateWaitlistSubmission(body, options) {
  const opts = options || {};
  const now = typeof opts.now === 'number' ? opts.now : Date.now();
  const minMs = typeof opts.minMs === 'number' ? opts.minMs : 1500;
  const raw = body && typeof body === 'object' ? body : {};

  const honeypot = typeof raw.website === 'string' ? raw.website.trim() : '';
  if (honeypot.length > 0) {
    return { ok: true, silentDrop: true };
  }

  const startedAt = Number(raw.startedAt);
  if (!Number.isFinite(startedAt) || now - startedAt < minMs) {
    return { ok: true, silentDrop: true };
  }

  const email = typeof raw.email === 'string' ? raw.email.trim().toLowerCase() : '';
  if (!email || !EMAIL_RE.test(email)) {
    return { ok: false, status: 400, error: 'invalid_email' };
  }

  return { ok: true, silentDrop: false, email };
}

/**
 * Google Apps Script often returns HTTP 200 with { ok: false }.
 * Treat that as failure so the UI does not show a fake success.
 */
function assertSheetWebhookSuccess(httpOk, responseText) {
  if (!httpOk) {
    const err = new Error('sheet_webhook_http');
    err.code = 'sheet_webhook_http';
    throw err;
  }
  let parsed;
  try {
    parsed = JSON.parse(responseText || '{}');
  } catch (_) {
    const err = new Error('sheet_webhook_json');
    err.code = 'sheet_webhook_json';
    throw err;
  }
  if (!parsed || parsed.ok !== true) {
    const err = new Error('sheet_webhook_body');
    err.code = 'sheet_webhook_body';
    err.detail = parsed && parsed.error;
    throw err;
  }
  return true;
}

module.exports = { validateWaitlistSubmission, assertSheetWebhookSuccess, EMAIL_RE };
