import { spawn, spawnSync } from 'node:child_process';
import { createWriteStream, readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, resolve, join, basename } from 'node:path';
import { fileURLToPath } from 'node:url';
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..'), frontend = resolve(root, 'frontend');
const scope = process.argv[2] || 'all';
if (!['all', 'wishlists', 'seller', 'buyer', 'auth', 'public', 'email-verification', 'password-recovery'].includes(scope)) throw Error('Unknown QA browser scope');
const log = resolve(root, `mvp-browser-${scope}.log`), out = createWriteStream(log);
const redact = text => text.replace(/((?:\/api)?\/wishlists\/shared\/|\/w\/)[A-Za-z0-9_-]+/g, '$1[redacted]')
  .replace(/([#?&]token=)[A-Za-z0-9_%+-]+/g, '$1[redacted]')
  .replace(/\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b/g, '[redacted-jwt]')
  .replace(/(?<![A-Za-z0-9_-])[A-Za-z0-9_-]{43}(?![A-Za-z0-9_-])/g, '[redacted-secret]')
  .replace(/((?:authorization|set-cookie|cookie):)[^\r\n]*/gi, '$1 [redacted]');
const env = { ...process.env, CI: '1', SREZ_E2E_BASE_URL: 'http://localhost:5175', SREZ_PUBLIC_E2E_URL: 'http://localhost:5175', SREZ_AUTH_E2E_LIVE: '1', SREZ_PUBLIC_E2E_LIVE: '1', SREZ_BUYER_E2E_LIVE: '1', SREZ_MVP_E2E_LIVE: '1', SREZ_EXPIRY_DB: 'srez-mvp-qa-db-1' };
const cli = resolve(frontend, 'node_modules/@playwright/test/cli.js');
let files;
if (scope === 'all') {
  // Discover every active spec, then give each its own native process. This
  // avoids Windows worker reuse fast-fails without skipping/retrying tests.
  const listing = spawnSync(process.execPath, [cli, 'test', '--list', '--reporter=json'], { cwd: frontend, env, encoding: 'utf8', maxBuffer: 8 * 1024 * 1024 });
  if (listing.status !== 0) throw Error('QA test discovery failed');
  const report = JSON.parse(listing.stdout);
  files = [...new Set(report.suites.map(suite => basename(suite.file)))];
  if (!files.length || files.some(file => !/^[A-Za-z0-9_.-]+\.spec\.ts$/.test(file))) throw Error('Unexpected spec discovery');
} else files = [scope + '.spec.ts'];
const results = [];
for (const file of files) {
  out.write(`\nQA group: ${file}\n`);
  const result = await new Promise(resolveRun => {
    const child = spawn(process.execPath, [cli, 'test', `e2e/${file}`, '--reporter=list', '--output=' + resolve(frontend, 'test-results', scope + '-' + file.replace('.spec.ts', ''))], { cwd: frontend, env, stdio: ['ignore', 'pipe', 'pipe'] });
    let all = ''; const flushes = [];
    for (const stream of [child.stdout, child.stderr]) {
      let pending = ''; stream.setEncoding('utf8');
      stream.on('data', chunk => { pending += chunk; let newline; while ((newline = pending.indexOf('\n')) >= 0) { const safe = redact(pending.slice(0, newline + 1)); out.write(safe); all += safe; pending = pending.slice(newline + 1); } });
      flushes.push(() => { const safe = redact(pending); out.write(safe); all += safe; });
    }
    child.on('error', () => resolveRun({ file, exitCode: 1, passed: 0 }));
    child.on('close', code => {
      flushes.forEach(flush => flush());
      const plain = all.replace(/\u001b\[[0-9;]*m/g, '');
      const passed = [...plain.matchAll(/^\s*(\d+) passed\b/gm)].reduce((sum, match) => sum + Number(match[1]), 0);
      resolveRun({ file, exitCode: code ?? 1, passed });
    });
  });
  results.push(result);
}
const scrub = dir => { try { for (const e of readdirSync(dir, { withFileTypes: true })) { const path = join(dir, e.name); if (e.isDirectory()) scrub(path); else if (/\.(md|json|txt)$/.test(e.name)) { const raw = readFileSync(path, 'utf8'), safe = redact(raw); if (safe !== raw) writeFileSync(path, safe); } } } catch (error) { if (error.code !== 'ENOENT') throw error; } };
scrub(resolve(frontend, 'test-results'));
const summary = { scope, exitCode: results.some(r => r.exitCode !== 0 || r.passed === 0) ? 1 : 0, passed: results.reduce((sum, r) => sum + r.passed, 0), groups: results, log };
writeFileSync(resolve(root, `mvp-browser-${scope}.results.json`), JSON.stringify(summary, null, 2));
await new Promise(resolveEnd => out.end(resolveEnd));
console.log(JSON.stringify(summary)); process.exitCode = summary.exitCode;
