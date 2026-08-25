'use strict';

const {
  validateWaitlistSubmission,
  assertSheetWebhookSuccess
} = require('../lib/waitlist/validate.js');

/** Recent IP hits for light rate limiting within a warm instance. */
const recentByIp = new Map();
const RATE_WINDOW_MS = 60_000;
const RATE_MAX = 8;

function clientIp(request) {
  const forwarded = request.headers['x-forwarded-for'];
  if (typeof forwarded === 'string' && forwarded.length) {
    return forwarded.split(',')[0].trim();
  }
  return request.socket && request.socket.remoteAddress
    ? String(request.socket.remoteAddress)
    : 'unknown';
}

function rateLimited(ip) {
  const now = Date.now();
  const hits = (recentByIp.get(ip) || []).filter((t) => now - t < RATE_WINDOW_MS);
  if (hits.length >= RATE_MAX) {
    recentByIp.set(ip, hits);
    return true;
  }
  hits.push(now);
  recentByIp.set(ip, hits);
  return false;
}

async function appendToSheet(payload) {
  const webhook = process.env.WAITLIST_SHEETS_WEBHOOK_URL;
  const token = process.env.WAITLIST_SHEETS_TOKEN;
  if (!webhook || !token) {
    const err = new Error('WAITLIST_SHEETS_WEBHOOK_URL or WAITLIST_SHEETS_TOKEN is not set');
    err.code = 'misconfigured';
    throw err;
  }

  const body = {
    email: payload.email,
    ip: payload.ip,
    userAgent: payload.userAgent,
    token
  };

  console.log('[waitlist] forwarding signup', {
    emailLen: payload.email.length,
    ip: payload.ip
  });

  const response = await fetch(webhook, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    redirect: 'follow'
  });

  const text = await response.text().catch(() => '');
  try {
    assertSheetWebhookSuccess(response.ok, text);
  } catch (err) {
    console.error('[waitlist] sheet webhook failed', {
      status: response.status,
      bodyPreview: text.slice(0, 200),
      code: err && err.code
    });
    throw err;
  }
}

module.exports = async function handler(request, response) {
  response.setHeader('Cache-Control', 'no-store');

  if (request.method === 'OPTIONS') {
    response.status(204).end();
    return;
  }

  if (request.method !== 'POST') {
    response.status(405).json({ ok: false, error: 'method_not_allowed' });
    return;
  }

  const ip = clientIp(request);
  if (rateLimited(ip)) {
    console.log('[waitlist] rate limited', { ip });
    response.status(429).json({ ok: false, error: 'rate_limited' });
    return;
  }

  const body = request.body && typeof request.body === 'object' ? request.body : {};
  const verdict = validateWaitlistSubmission(body, { now: Date.now() });

  if (!verdict.ok) {
    console.log('[waitlist] reject', { status: verdict.status, error: verdict.error, ip });
    response.status(verdict.status || 400).json({ ok: false, error: verdict.error || 'bad_request' });
    return;
  }

  if (verdict.silentDrop) {
    console.log('[waitlist] silent drop (spam checks)', { ip });
    response.status(200).json({ ok: true });
    return;
  }

  try {
    await appendToSheet({
      email: verdict.email,
      ip,
      userAgent: String(request.headers['user-agent'] || '').slice(0, 300)
    });
    console.log('[waitlist] stored', { email: verdict.email, ip });
    response.status(200).json({ ok: true });
  } catch (err) {
    console.error('[waitlist] store failed', {
      code: err && err.code,
      message: err && err.message,
      detail: err && err.detail,
      ip
    });
    if (err && err.code === 'misconfigured') {
      response.status(500).json({ ok: false, error: 'misconfigured' });
      return;
    }
    response.status(502).json({
      ok: false,
      error: 'store_failed',
      code: err && err.code ? String(err.code) : 'unknown',
      detail: err && err.detail ? String(err.detail).slice(0, 80) : undefined
    });
  }
};
