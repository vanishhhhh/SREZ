// Explicit local catalog maintenance. No SQL writes, deletes, remote hosts, or
// automatic execution at app startup. All domain/media mutations use Go HTTP.
import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { randomBytes, createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const flags = new Set(process.argv.slice(2));
if ([...flags].some(f => !['--apply', '--qa', '--retire-fixtures'].includes(f))) throw Error('Unknown flag');
const apply = flags.has('--apply'), qa = flags.has('--qa');
if (flags.has('--retire-fixtures') && (!apply || qa)) throw Error('Fixture retirement requires explicit integration --apply');
const origin = qa ? 'http://localhost:5175' : 'http://localhost:5174';
const project = qa ? 'srez-mvp-qa' : 'srez-integration';
const db = project + '-db-1';
const label = spawnSync('docker', ['inspect', '--format', '{{index .Config.Labels "com.docker.compose.project"}}', db], { encoding: 'utf8' });
if (label.status !== 0 || label.stdout.trim() !== project) throw Error('Wrong local database project; stopped');
const manifest = JSON.parse(readFileSync(resolve(root, 'demo/catalog/catalog.json'), 'utf8').replace(/^\uFEFF/, ''));
const sha = data => createHash('sha256').update(data).digest('hex');
const assets = manifest.products.map(p => {
  const data = readFileSync(resolve(root, 'demo/catalog', p.image));
  if (!data.subarray(0, 8).equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10])) || data.length > 5 * 1024 * 1024) throw Error('Invalid product PNG: ' + p.key);
  return { key: p.key, bytes: data.length, sha256: sha(data) };
});
if (new Set(assets.map(a => a.sha256)).size !== manifest.products.length) throw Error('Duplicate product image');
if (manifest.brands.length !== 5 || manifest.products.length !== 21 || new Set(manifest.products.map(p => p.key)).size !== 21) throw Error('Invalid manifest');
for (const category of ['tshirt', 'hoodie', 'pants', 'outerwear', 'footwear', 'accessories', 'other']) {
  if (manifest.products.filter(p => p.category === category).length !== 3) throw Error('Expected 3 items per category');
}
for (const b of manifest.brands) if (!existsSync(resolve(root, `demo/catalog/logos/${b.key}.png`))) throw Error('Brand logo missing: ' + b.key);

function sqlRead(query) {
  const r = spawnSync('docker', ['exec', '-e', 'PGOPTIONS=-c default_transaction_read_only=on', db, 'psql', '-U', 'srez', '-d', 'srez', '-At', '-c', query], { encoding: 'utf8', maxBuffer: 8 * 1024 * 1024 });
  if (r.status !== 0) throw Error('Local read-only query failed');
  return JSON.parse(r.stdout.trim() || '[]');
}
const fixtures = sqlRead(`SELECT COALESCE(json_agg(x),'[]'::json) FROM (
 SELECT b.id,b.name,b.slug,b.status,u.username FROM brands b
 JOIN brand_memberships m ON m.brand_id=b.id AND m.role='owner' AND m.status='active'
 JOIN users u ON u.id=m.user_id WHERE
 (b.slug ~ '^city-[a-z0-9]+$' AND b.name IN ('Городская форма','Городская форма новая') AND u.username ~ '^buyer_[a-z0-9]+_[a-z0-9]+$') OR
 (b.slug ~ '^(north|quiet)-[a-z0-9]+$' AND b.name IN ('Северная форма','Тихая линия') AND u.username ~ '^public_[a-z0-9]+$')
 ORDER BY b.id) x`);
if (!apply) {
  console.log(JSON.stringify({ mode: 'read-only preview', origin, brands: manifest.brands.map(b => b.name), categories: 7, products: 21, uniqueImages: assets.length, identifiedTestBrands: fixtures.length }));
  process.exit(0);
}

const privateDir = resolve(root, '.secrets');
mkdirSync(privateDir, { recursive: true });
const suffix = qa ? 'qa' : 'integration';
const credentialsFile = resolve(privateDir, `showcase-${suffix}-owner.json`);
const stateFile = resolve(privateDir, `showcase-${suffix}-state.json`);
const state = existsSync(stateFile) ? JSON.parse(readFileSync(stateFile, 'utf8')) : { version: 1, project, brands: {}, products: {}, retiredFixtures: [] };
if (state.version !== 1 || state.project !== project) throw Error('Unexpected saved state');
const save = () => writeFileSync(stateFile, JSON.stringify(state, null, 2), { mode: 0o600 });
save();
let token = '';
async function request(method, path, body, bearer = token) {
  const headers = { Origin: origin };
  if (bearer) headers.Authorization = 'Bearer ' + bearer;
  const bytes = Buffer.isBuffer(body);
  if (body !== undefined) headers['Content-Type'] = bytes ? 'image/png' : 'application/json';
  const response = await fetch(origin + '/api' + path, { method, headers, body: body === undefined ? undefined : bytes ? body : JSON.stringify(body), signal: AbortSignal.timeout(60000) });
  if(path==='/auth/login'&&response.status===403)throw Error('Demo account needs email verification. Open the latest Mailpit message, confirm the address in SREZ, then rerun the same script. Existing account and state retained.');
  if (!response.ok) throw Error(`Local API rejected ${method} ${path}: HTTP ${response.status}. State retained; no automatic retry.`);
  return response.status === 204 ? null : response.json();
}
let credentials;
if (existsSync(credentialsFile)) credentials = JSON.parse(readFileSync(credentialsFile, 'utf8'));
else {
  credentials = { username: 'srez_showcase_owner_v1', email: 'showcase-owner@example.test', password: randomBytes(32).toString('base64url'), display_name: 'Студия SREZ' };
  writeFileSync(credentialsFile, JSON.stringify(credentials, null, 2), { mode: 0o600, flag: 'wx' });
  await request('POST', '/auth/register', credentials, '');
}
token = (await request('POST', '/auth/login', { identifier: credentials.username, password: credentials.password }, '')).access_token;
if (typeof token !== 'string') throw Error('Login did not return access token');

async function upload(target, file) {
  const data = readFileSync(file);
  const pending = await request('POST', '/media/uploads', { ...target, content_type: 'image/png', size_bytes: data.length });
  if (!/^\/media\/uploads\/[1-9]\d*\/content$/.test(pending.content_url)) throw Error('Unexpected upload path');
  const result = await request('PUT', pending.content_url, data);
  if (result.state !== 'complete' || result.asset?.state !== 'ready') throw Error('Media not ready');
  return result;
}
const mine = await request('GET', '/me/brands');
for (const b of manifest.brands) {
  const slug = 'showcase-' + b.key;
  let brand = mine.find(entry => entry.role === 'owner' && entry.brand.slug === slug)?.brand;
  if (!brand) brand = await request('POST', '/brands', { name: b.name, slug });
  if (state.brands[b.key] && state.brands[b.key] !== brand.id) throw Error('Brand identity mismatch');
  state.brands[b.key] = brand.id; save();
  // Existing curated entries are never rewritten: seller edits remain intact.
  if (brand.status === 'draft') {
    await request('PATCH', '/brands/' + brand.id, { description: b.description, appearance: { accent_color: b.accent, background_color: b.background, text_color: '#171715' } });
    if (!brand.logo_url) await upload({ purpose: 'brand_logo', brand_id: brand.id }, resolve(root, `demo/catalog/logos/${b.key}.png`));
    if (!brand.banner_url) {
      const cover = manifest.products.find(p => p.brand === b.key);
      await upload({ purpose: 'brand_banner', brand_id: brand.id }, resolve(root, 'demo/catalog', cover.image));
    }
    await request('POST', '/brands/' + brand.id + '/publish', {});
  }
  const existing = await request('GET', `/brands/${brand.id}/products/manage`);
  for (const p of manifest.products.filter(p => p.brand === b.key)) {
    let product = existing.find(item => item.id === state.products[p.key]?.id || item.name === p.name);
    if (!product) product = await request('POST', '/products', { brand_id: brand.id, name: p.name, description: p.description, price_kopecks: p.price_kopecks, category: p.category, materials: p.materials });
    if (product.brand_id !== brand.id) throw Error('Product owner mismatch');
    state.products[p.key] = { ...state.products[p.key], id: product.id }; save();
    if (product.status === 'published') continue;
    if (product.status !== 'draft') throw Error('Existing item was hidden by seller; leaving unchanged');
    const path = '/products/' + product.id;
    let colors = await request('GET', path + '/colors');
    let color = colors.find(c => c.color === p.color);
    if (!color) color = await request('POST', path + '/colors', { color: p.color });
    const images = await request('GET', `${path}/colors/${color.id}/images`);
    if (!images.length) await upload({ purpose: 'product_image', product_id: product.id, color_id: color.id }, resolve(root, 'demo/catalog', p.image));
    const variants = await request('GET', `${path}/colors/${color.id}/variants`);
    for (const [i, size] of p.sizes.entries()) {
      if (!variants.some(v => v.size === size)) await request('POST', `${path}/colors/${color.id}/variants`, { size, stock: 6 + i * 3, sku: `SHOWCASE-${p.key.toUpperCase()}-${i + 1}` });
    }
    await request('POST', path + '/publish', {});
    console.log(JSON.stringify({ published: p.key, category: p.category }));
  }
}

// New catalog must be complete before any old fixture is taken off display.
for (const p of manifest.products) {
  const actual = await request('GET', '/products/' + state.products[p.key].id, undefined, '');
  if (actual.status !== 'published' || !actual.colors?.[0]?.images?.[0]?.startsWith('/media/')) throw Error('New catalog incomplete; old display retained');
}
if (flags.has('--retire-fixtures')) {
  for (const fixture of fixtures.filter(f => f.status === 'active')) {
    const password = fixture.username.startsWith('public_') ? 'SREZ public integration password 123' : 'SREZ integration local password 123';
    const auth = await request('POST', '/auth/login', { identifier: fixture.username, password }, '');
    const owned = await request('GET', '/me/brands', undefined, auth.access_token);
    if (!owned.some(e => e.role === 'owner' && e.brand.id === fixture.id && e.brand.slug === fixture.slug)) throw Error('Fixture ownership changed; stopped');
    if (!state.retiredFixtures.some(f => f.id === fixture.id)) { state.retiredFixtures.push(fixture); save(); }
    await request('POST', '/brands/' + fixture.id + '/unpublish', {}, auth.access_token);
  }
}
const report = { project, origin, brands: Object.keys(state.brands).length, products: Object.keys(state.products).length, categories: 7, images: assets, retiredFixtureBrands: state.retiredFixtures.length, deletedRows: 0, usedExistingGoMediaAPI: true };
writeFileSync(resolve(root, `demo/catalog/result-${suffix}.json`), JSON.stringify(report, null, 2));
console.log(JSON.stringify({ project, brands: report.brands, products: report.products, categories: 7, retiredFixtureBrands: report.retiredFixtureBrands, deletedRows: 0, credentialFile: '.secrets/' + `showcase-${suffix}-owner.json` }));
