/**
 * Paste into Extensions → Apps Script on your waitlist Google Sheet.
 * Deploy → New deployment → Web app
 *   Execute as: Me
 *   Who has access: Anyone
 * Then put the Web app URL in Vercel as WAITLIST_SHEETS_WEBHOOK_URL.
 * Required: set Script property WAITLIST_SHEETS_TOKEN to match Vercel env.
 *
 * Sheet columns: A timestamp | B email | C ip | D userAgent
 * Example row: 2026-07-22T17:30:00.000Z | you@example.com | 203.0.113.10 | Mozilla/5.0...
 */
function doPost(e) {
  var lock = LockService.getScriptLock();
  var locked = lock.tryLock(10000);
  if (!locked) {
    return ContentService
      .createTextOutput(JSON.stringify({ ok: false, error: 'busy' }))
      .setMimeType(ContentService.MimeType.JSON);
  }
  try {
    var raw = e && e.postData && e.postData.contents ? e.postData.contents : '{}';
    var data = JSON.parse(raw);
    var expected = PropertiesService.getScriptProperties().getProperty('WAITLIST_SHEETS_TOKEN') || '';
    if (!expected || data.token !== expected) {
      return ContentService
        .createTextOutput(JSON.stringify({ ok: false, error: 'unauthorized' }))
        .setMimeType(ContentService.MimeType.JSON);
    }

    var email = String(data.email || '').trim().toLowerCase();
    if (!email || email.indexOf('@') < 1) {
      return ContentService
        .createTextOutput(JSON.stringify({ ok: false, error: 'invalid_email' }))
        .setMimeType(ContentService.MimeType.JSON);
    }

    var ss = SpreadsheetApp.getActiveSpreadsheet();
    var sheet = ss.getSheetByName('signups');
    if (!sheet) {
      sheet = ss.insertSheet('signups');
      sheet.appendRow(['timestamp', 'email', 'ip', 'userAgent']);
    }

    sheet.appendRow([
      new Date().toISOString(),
      email,
      String(data.ip || ''),
      String(data.userAgent || '').slice(0, 300)
    ]);

    return ContentService
      .createTextOutput(JSON.stringify({ ok: true }))
      .setMimeType(ContentService.MimeType.JSON);
  } catch (err) {
    return ContentService
      .createTextOutput(JSON.stringify({ ok: false, error: String(err) }))
      .setMimeType(ContentService.MimeType.JSON);
  } finally {
    lock.releaseLock();
  }
}
