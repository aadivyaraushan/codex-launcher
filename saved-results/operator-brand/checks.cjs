const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const {execFileSync} = require('node:child_process');
const {chromium} = require('playwright');
const root = path.resolve(__dirname, '../..');
const baseline = execFileSync('git', ['show', '88637bb:landing/index.html'], {cwd:root, encoding:'utf8'});
const source = fs.readFileSync(path.join(root, 'landing/index.html'), 'utf8');
const failures = [];
function check(name, run) { try { run(); console.log(`PASS ${name}`); } catch(e) { failures.push(name); console.log(`FAIL ${name}: ${e.message}`); } }
const scripts = s => [...s.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/g)].map(m=>m[1]);
check('interactive scripts unchanged', ()=>assert.deepEqual(scripts(source), scripts(baseline)));
check('legacy typefaces removed', ()=>assert(!/Instrument Sans|JetBrains Mono|fonts.googleapis/.test(source)));
const server = http.createServer((req,res)=>{
 const pathname = new URL(req.url,'http://localhost').pathname;
 const file = path.join(root, pathname==='/' ? 'landing/index.html' : pathname);
 if(!file.startsWith(root + '/') || !fs.existsSync(file)) {res.writeHead(404);res.end();return;}
 res.setHeader('Content-Type', file.endsWith('.svg')?'image/svg+xml':file.endsWith('.html')?'text/html':'application/octet-stream');
 res.end(fs.readFileSync(file));
});
(async()=>{
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const base = process.env.CHECK_URL || `http://127.0.0.1:${server.address().port}`;
 const browser = await chromium.launch({headless:true,channel:'chrome'});
 try {
  for(const width of [1440,390,320]) {
   const page=await browser.newPage({viewport:{width,height:900},reducedMotion:'reduce'});
   const errors=[]; page.on('pageerror',e=>errors.push(e.message));
   await page.goto(base,{waitUntil:'networkidle'}); await page.evaluate(()=>document.fonts.ready);
   const actual=await page.evaluate(()=>{
    const css=getComputedStyle(document.body), h=getComputedStyle(document.querySelector('h1'));
    const brand=[...document.querySelectorAll('.nav__mark img,.footer__mark img')];
    return {bg:css.backgroundColor,color:css.color,font:css.fontFamily,weight:h.fontWeight,buttonWeight:getComputedStyle(document.querySelector('.btn')).fontWeight,accent:getComputedStyle(document.documentElement).getPropertyValue('--signal').trim(),rust:getComputedStyle(document.documentElement).getPropertyValue('--rust').trim(),loaded:document.fonts.check('400 16px Switzer')&&document.fonts.check('500 16px Switzer')&&document.fonts.check('700 16px Switzer'),logos:brand.length===2&&brand.every(x=>x.complete&&x.naturalWidth>0),overflow:document.documentElement.scrollWidth>innerWidth};
   });
   check(`${width}px brand palette`,()=>assert.deepEqual([actual.bg,actual.color,actual.accent,actual.rust],['rgb(11, 11, 11)','rgb(232, 230, 226)','#FF5934','#9E3924']));
   check(`${width}px Switzer and weights`,()=>{assert(actual.font.startsWith('Switzer'));assert(actual.loaded);assert.equal(actual.weight,'700');assert.equal(actual.buttonWeight,'500');});
   check(`${width}px supplied logos loaded`,()=>assert(actual.logos));
   check(`${width}px no horizontal overflow`,()=>assert(!actual.overflow));
   check(`${width}px no script errors`,()=>assert.deepEqual(errors,[]));
   const normalize=html=>{const doc=new DOMParser().parseFromString(html,'text/html');doc.querySelectorAll('script,style').forEach(x=>x.remove());doc.querySelectorAll('.nav__mark,.footer__mark').forEach(x=>x.textContent='Operator');return doc.body.textContent.replace(/\s+/g,' ').trim();};
   const textSame=await page.evaluate(({baseline,source,normalizer})=>{const normalize=eval('('+normalizer+')');return normalize(source)===normalize(baseline);},{baseline,source,normalizer:normalize.toString()});
   check(`${width}px original wording preserved`,()=>assert(textSame));
   let calls=0;
   await page.route('**/api/waitlist',route=>{calls++;return route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({ok:true})});});
   await page.locator('.nav__link').click();
   await page.locator('#waitlist-form button').click();
   check(`${width}px invalid email rejected`,()=>assert.equal(calls,0));
   await page.locator('input[type=email]').fill('brand-check@example.com');
   await page.locator('#waitlist-form button').click();
   await page.waitForFunction(()=>document.querySelector('#waitlist-form').hidden);
   check(`${width}px mocked waitlist success`,()=>assert.equal(calls,1));
   await page.goto(base,{waitUntil:'networkidle'});await page.evaluate(()=>document.fonts.ready);
   await page.screenshot({path:path.join(__dirname,'screenshots',`desktop-${width}.png`),fullPage:false});
   await page.close();
  }
 } finally {await browser.close();server.close();}
 console.log(`RESULT ${failures.length} failed checks`);process.exitCode=failures.length?1:0;
})().catch(e=>{console.error(e);server.close();process.exitCode=1;});
