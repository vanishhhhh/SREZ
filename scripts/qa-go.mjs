import { readFileSync, openSync, closeSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const mode = process.argv[2] || 'wishlist';
if (!['wishlist', 'full', 'vet', 'email', 'persistence-write', 'persistence-verify'].includes(mode)) throw Error('Unknown QA mode');
const values = Object.fromEntries(readFileSync(resolve(root, '.env'), 'utf8').split(/\r?\n/).flatMap(line => {
  const found = line.match(/^([A-Z0-9_]+)=(.*)$/);
  return found ? [[found[1], found[2].trim().replace(/^(['"])(.*)\1$/, '$2')]] : [];
}));
const env = { ...process.env,
  TEST_DATABASE_URL: `postgres://srez:${encodeURIComponent(values.POSTGRES_PASSWORD)}@db:5432/srez?sslmode=disable`,
  TEST_S3_ENDPOINT: 'http://seaweedfs:8333',
  TEST_MAILPIT_SMTP:'mailpit:1025',TEST_MAILPIT_API:'http://mailpit:8025',
  SREZ_S3_ACCESS_KEY: values.SREZ_S3_ACCESS_KEY,
  SREZ_S3_SECRET_KEY: values.SREZ_S3_SECRET_KEY,
};
if (mode.startsWith('persistence-')) {
  const schema = process.argv[3];
  if (!/^media_restart_[a-z0-9_]+$/.test(schema || '')) throw Error('Unique test schema required');
  env.TEST_MEDIA_PERSISTENCE_SCHEMA = schema;
  env.TEST_S3_WRITE_RESTART = mode === 'persistence-write' ? 'true' : 'false';
  env.TEST_S3_VERIFY_RESTART = mode === 'persistence-verify' ? 'true' : 'false';
}
const file = resolve(root, `mvp-go-${mode}.log`), output = openSync(file, 'w');
const args = mode === 'vet' ? ['vet', './...'] : ['test', './...', '-json', '-p', '1', '-count=1', '-timeout=180s', ...(mode === 'wishlist' ? ['-run', 'Wishlist'] : mode === 'email' ? ['-run','Email|Mail|Verification'] : mode.startsWith('persistence-') ? ['-run', 'TestSeaweedPersistenceAfterRestart'] : [])];
let result;
try {
  result = spawnSync('docker', ['run', '--name', `srez-mvp-${mode}-${Date.now()}`, '--label', 'srez.test-only=true', '--network', 'srez-mvp-tests_default', ...['TEST_DATABASE_URL', 'TEST_MAILPIT_SMTP','TEST_MAILPIT_API', 'TEST_S3_ENDPOINT', 'SREZ_S3_ACCESS_KEY', 'SREZ_S3_SECRET_KEY', 'TEST_MEDIA_PERSISTENCE_SCHEMA', 'TEST_S3_WRITE_RESTART', 'TEST_S3_VERIFY_RESTART'].flatMap(key => ['-e', key]), 'srez-mvp-go-check:local', 'go', ...args], { cwd: root, env, stdio: ['ignore', output, output] });
} finally { closeSync(output); }
const rows = readFileSync(file, 'utf8').split(/\r?\n/).flatMap(line => { try { return [JSON.parse(line)]; } catch { return []; } });
const passed=rows.filter(r=>r.Action==='pass'&&r.Test).length;
const skipped=rows.filter(r=>r.Action==='skip'&&r.Test).map(r=>r.Test);
const incomplete=mode==='full'&&(passed===0||skipped.some(name=>name!=='TestSeaweedPersistenceAfterRestart'));
const exitCode=(result.status??1)||(incomplete?1:0);
console.log(JSON.stringify({mode,exitCode,passed,skipped,failed:rows.filter(r=>r.Action==='fail').map(r=>r.Test||r.Package),incomplete,log:file}));
process.exitCode=exitCode;
