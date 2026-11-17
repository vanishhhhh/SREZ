import { readFileSync, writeFileSync, readdirSync, statSync, mkdirSync, copyFileSync, openSync, closeSync, existsSync } from 'node:fs';
import { resolve, relative, join } from 'node:path';
import { createHash } from 'node:crypto';
import { execFileSync, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const source = resolve(fileURLToPath(new URL('../', import.meta.url)));
const root = resolve(source,'..');
const target = join(root, 'backups', new Date().toISOString().replaceAll(':','-')+'-srez');
const databaseContainer=process.env.SREZ_BACKUP_DB_CONTAINER || 'srez-integration-db-1';
if (existsSync(target)) throw new Error('Backup destination already exists; nothing overwritten.');
const hash = path => createHash('sha256').update(readFileSync(path)).digest('hex');
const ignored = new Set(['node_modules', 'dist', 'test-results', 'playwright-report', '.vite', 'coverage', 'tmp', 'temp']);
function files(dir, base = dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const path = join(dir, entry.name);
    if (entry.isSymbolicLink()) throw new Error('Unexpected link in backup source: ' + relative(base, path));
    if (entry.isDirectory()) return ignored.has(entry.name) ? [] : files(path, base);
    if (/\.log$|\.tsbuildinfo$/.test(entry.name) || entry.name === 'index.lock') return [];
    return [relative(base, path).replaceAll('\\', '/')];
  });
}
const runGit = (...args) => execFileSync('git', ['--no-optional-locks', ...args], { cwd: source, encoding: 'utf8' }).trim();
const before = { head: runGit('rev-parse', 'HEAD'), branch: runGit('branch', '--show-current'), status: runGit('status', '--porcelain=v1', '--untracked-files=all'), refs: runGit('show-ref') };
mkdirSync(join(target, 'repository'), { recursive: true });
const manifest = files(source).map(path => ({ path, bytes: statSync(join(source, path)).size, sha256: hash(join(source, path)) }));
for (const file of manifest) {
  const destination = join(target, 'repository', file.path);
  mkdirSync(resolve(destination, '..'), { recursive: true });
  copyFileSync(join(source, file.path), destination);
  if (hash(destination) !== file.sha256 || hash(join(source, file.path)) !== file.sha256) throw new Error('Backup verification failed: ' + file.path);
}
writeFileSync(join(target, 'files.sha256.json'), JSON.stringify(manifest, null, 2), { flag: 'wx' });
writeFileSync(join(target, 'git-state.json'), JSON.stringify(before, null, 2), { flag: 'wx' });
const dump = join(target, 'postgres.dump');
const fd = openSync(dump, 'wx');
let result;
try { result = spawnSync('docker', ['exec', databaseContainer, 'pg_dump', '-U', 'srez', '-d', 'srez', '--format=custom', '--no-owner', '--no-acl'], { stdio: ['ignore', fd, 'pipe'] }); } finally { closeSync(fd); }
if (result.status !== 0) throw new Error('pg_dump failed; backup is not complete.');
const input = openSync(dump, 'r');
let archive;
// A networkless archive reader, no connection to any database and no mounts.
try { archive = spawnSync('docker', ['run', '--rm', '-i', '--network', 'none', '--label','srez.backup-check=true','postgres:16-alpine', 'pg_restore', '--list'], { stdio: [input, 'pipe', 'pipe'], encoding: 'utf8' }); } finally { closeSync(input); }
if (archive.status !== 0 || !archive.stdout.includes('schema_migrations')) throw new Error('Invalid PostgreSQL archive.');
const verification = { source, target, files: manifest.length, frontendFiles: manifest.filter(f => f.path.startsWith('frontend/')).length, allFileHashesVerified: true, postgres: { bytes: statSync(dump).size, sha256: hash(dump), archiveTOCVerified: true }, sourceHead: before.head, sourceBranch: before.branch };
writeFileSync(join(target, 'VERIFIED.json'), JSON.stringify(verification, null, 2), { flag: 'wx' });
console.log(JSON.stringify(verification, null, 2));
