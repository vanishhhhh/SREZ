import { spawnSync } from 'node:child_process';
import { randomBytes, randomInt } from 'node:crypto';
import { mkdirSync, mkdtempSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const origin = 'https://srez.remz.biz';
const project = 'srez-volta-qa-' + randomBytes(6).toString('hex');
const parent = join(root, 'tmp');
mkdirSync(parent, { recursive: true });
const scratch = mkdtempSync(join(parent, project + '-'));
const envFile = join(scratch, 'qa.env');
const override = join(scratch, 'qa.json');
const secret = (encoding = 'hex') => randomBytes(32).toString(encoding);
const elasticPassword = secret();
const subnet = '10.238.' + randomInt(1, 255);
const data = ['postgres', 'seaweedfs', 'elasticsearch'];
const dirs = Object.fromEntries(data.map((name) => [name, join(scratch, name)]));
const cert = join(scratch, 'mail.crt');
const key = join(scratch, 'mail.key');
const composeArgs = [
  'compose',
  '--project-directory',
  root,
  '-p',
  project,
  '--env-file',
  envFile,
  '-f',
  join(root, 'compose.prod.yaml'),
  '-f',
  override,
];
// Shell variables take precedence over --env-file in Compose. Discard any
// inherited application configuration so the test uses temporary data only.
const environment = Object.fromEntries(
  Object.entries(process.env).filter(
    ([k]) => !/^(?:SREZ_|COMPOSE_|POSTGRES_PASSWORD$|ELASTIC_PASSWORD$)/i.test(k),
  ),
);
function run(program, args, mode = 'pipe') {
  const result = spawnSync(program, args, {
    cwd: root,
    env: environment,
    encoding: 'utf8',
    stdio: mode,
  });
  if (result.status !== 0) {
    if (result.stderr) process.stderr.write(result.stderr);
    throw Error(program + ' ' + args[0] + ' failed');
  }
  return result.stdout?.trim();
}
const compose = (...args) => run('docker', [...composeArgs, ...args]);
function check(condition, description) {
  if (!condition) throw Error(description);
}
async function freePort() {
  const server = createServer();
  await new Promise((ok, fail) => {
    server.once('error', fail);
    server.listen(0, '127.0.0.1', ok);
  });
  const { port } = server.address();
  await new Promise((ok, fail) => server.close((error) => (error ? fail(error) : ok())));
  return port;
}
let cleanup = false;
try {
  for (const path of Object.values(dirs)) mkdirSync(path);
  run('openssl', [
    'req',
    '-x509',
    '-newkey',
    'rsa:2048',
    '-nodes',
    '-days',
    '1',
    '-subj',
    '/CN=mailpit',
    '-addext',
    'subjectAltName=DNS:mailpit',
    '-keyout',
    key,
    '-out',
    cert,
  ]);
  const port = await freePort();
  const values = {
    POSTGRES_PASSWORD: secret(),
    ELASTIC_PASSWORD: elasticPassword,
    SREZ_ELASTICSEARCH_USERNAME: 'elastic',
    SREZ_ELASTICSEARCH_PASSWORD: elasticPassword,
    SREZ_JWT_KEY: secret('base64'),
    SREZ_CSRF_KEY: secret('base64'),
    SREZ_WISHLIST_KEY: secret('base64'),
    SREZ_S3_ACCESS_KEY: secret(),
    SREZ_S3_SECRET_KEY: secret(),
    SREZ_SMTP_HOST: 'mailpit',
    SREZ_SMTP_PORT: '1025',
    SREZ_SMTP_TLS: 'starttls',
    SREZ_MAIL_FROM: 'no-reply@example.test',
    SREZ_FRONTEND_PORT: port,
    SREZ_EDGE_SUBNET: subnet + '.0/24',
    SREZ_FRONTEND_PROXY_IP: subnet + '.10',
    SREZ_POSTGRES_DATA_DIR: dirs.postgres,
    SREZ_SEAWEEDFS_DATA_DIR: dirs.seaweedfs,
    SREZ_ELASTICSEARCH_DATA_DIR: dirs.elasticsearch,
    SREZ_FRONTEND_CPUS: 1,
    SREZ_FRONTEND_MEMORY: '128m',
    SREZ_APP_CPUS: 2,
    SREZ_APP_MEMORY: '512m',
    SREZ_POSTGRES_CPUS: 1,
    SREZ_POSTGRES_MEMORY: '256m',
    SREZ_SEAWEEDFS_CPUS: 2,
    SREZ_SEAWEEDFS_MEMORY: '512m',
    SREZ_ES_CPUS: 2,
    SREZ_ES_MEMORY: '1g',
  };
  writeFileSync(
    envFile,
    Object.entries(values)
      .map(([k, v]) => k + "='" + String(v).replaceAll('\\', '/') + "'")
      .join('\n') + '\n',
    { mode: 0o600 },
  );
  const bind = (source, target) => ({
    type: 'bind',
    source,
    target,
    read_only: true,
    bind: { create_host_path: false },
  });
  const ephemeral = (target) => [{ type: 'tmpfs', target, tmpfs: { mode: 0o777 } }];
  writeFileSync(
    override,
    JSON.stringify({
      services: {
        db: { volumes: ephemeral('/var/lib/postgresql/data') },
        seaweedfs: { volumes: ephemeral('/data') },
        elasticsearch: { volumes: ephemeral('/usr/share/elasticsearch/data') },
        app: {
          environment: { SSL_CERT_FILE: '/mail-ca.crt' },
          volumes: [bind(cert, '/mail-ca.crt')],
        },
        mailpit: {
          image:
            'axllent/mailpit:v1.34.0@sha256:23a2ef85bc50853ab3093645d1d64679afd8887e89da5e48a88940c172591a4e',
          environment: {
            MP_SMTP_TLS_CERT: '/mail.crt',
            MP_SMTP_TLS_KEY: '/mail.key',
            MP_SMTP_REQUIRE_STARTTLS: 'true',
          },
          volumes: [bind(cert, '/mail.crt'), bind(key, '/mail.key')],
          ports: ['127.0.0.1::8025'],
          networks: ['edge'],
        },
      },
    }),
    { mode: 0o600 },
  );
  const production = JSON.parse(
    run('docker', composeArgs.slice(0, -2).concat('config', '--format', 'json')),
  );
  check(
    production.name === project && Object.keys(production.services).length === 5,
    'Unexpected production services',
  );
  for (const [name, service] of Object.entries(production.services)) {
    check(
      name === 'frontend'
        ? service.ports?.length === 1 && service.ports[0].host_ip === '127.0.0.1'
        : !service.ports?.length,
      'Unexpected published port on ' + name,
    );
    check(
      service.healthcheck &&
        service.mem_limit &&
        service.cpus &&
        service.restart === 'unless-stopped',
      'Runtime constraints missing for ' + name,
    );
  }
  check(
    production.networks.data.internal &&
      production.services.app.environment.SREZ_PUBLIC_ORIGIN === origin &&
      production.services.app.environment.SREZ_AUTH_LOCAL_HTTP === 'false',
    'Unsafe production config',
  );
  console.log('Production Compose topology validated');
  cleanup = true;
  run(
    'docker',
    [...composeArgs, 'up', '-d', '--build', '--wait', '--wait-timeout', '210'],
    'inherit',
  );
  const base = 'http://127.0.0.1:' + port;
  const mailbox = 'http://' + compose('port', 'mailpit', '8025');
  async function request(path, status, { body, headers, method } = {}) {
    const response = await fetch(base + path, {
      method: method ?? (body === undefined ? 'GET' : 'POST'),
      headers: {
        Origin: origin,
        'X-Real-IP': '192.0.2.10',
        ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
        ...headers,
      },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(30000),
    });
    check(
      response.status === status,
      path + ' returned ' + response.status + ', expected ' + status,
    );
    return response;
  }
  for (const path of ['/account?qa=1', '/verify-email', '/reset-password', '/w/public-spa-route']) {
    const response = await request(path, 200);
    check((await response.text()).includes('<div id="root">'), 'SPA fallback missing');
  }
  await request('/api/health', 200);
  const catalog = await request('/api/products?page=2&limit=1', 200);
  check(
    catalog.headers.get('x-page') === '2' && Array.isArray(await catalog.json()),
    'Catalog query was not forwarded',
  );
  const aliases = JSON.parse(
    compose(
      'exec',
      '-T',
      'elasticsearch',
      'sh',
      '-c',
      'curl -fsS -u "elastic:$ELASTIC_PASSWORD" http://127.0.0.1:9200/_cat/aliases?format=json',
    ),
  );
  check(
    aliases.some(({ alias }) => alias.startsWith('srez-catalog-')),
    'Catalog index alias was not built',
  );
  const migrationCount = readdirSync(join(root, 'migrations')).filter((name) =>
    name.endsWith('.sql'),
  ).length;
  const applied = compose(
    'exec',
    '-T',
    'db',
    'psql',
    '-U',
    'srez',
    '-d',
    'srez',
    '-Atc',
    'SELECT count(*) FROM schema_migrations',
  );
  check(Number(applied) === migrationCount, 'Migration journal is incomplete');
  await request('/media/assets/123456/content', 404);
  await request('/api/profile', 401);

  const email = 'qa@example.test',
    password = 'testonly';
  const login = { identifier: email, password };
  const registration = { email, username: 'volta_qa', display_name: 'Volta QA', password };
  async function mailToken(path) {
    const pattern = new RegExp(
      origin.replaceAll('.', '\\.') + '/' + path + '#token=([A-Za-z0-9_-]{43})',
    );
    for (let attempt = 0; attempt < 30; attempt++) {
      const search = await fetch(mailbox + '/api/v1/search?query=to%3Aqa%40example.test');
      if (search.ok) {
        for (const entry of (await search.json()).messages ?? []) {
          const response = await fetch(mailbox + '/api/v1/message/' + encodeURIComponent(entry.ID));
          const token = response.ok && (await response.json()).Text.match(pattern);
          if (token) return token[1];
        }
      }
      await delay(500);
    }
    throw Error('No HTTPS ' + path + ' link arrived via STARTTLS');
  }
  await request('/api/auth/register', 403, {
    body: registration,
    headers: { Origin: 'http://localhost' },
  });
  const registered = await request('/api/auth/register', 201, { body: registration });
  check(
    (await registered.json()).email_delivery_status === 'sent',
    'SMTP did not accept confirmation',
  );
  await request('/api/auth/login', 403, { body: login });
  await request('/api/auth/verify-email', 200, {
    body: { token: await mailToken('verify-email') },
  });
  const loggedIn = await request('/api/auth/login', 200, { body: login });
  const cookie = loggedIn.headers.get('set-cookie') ?? '';
  check(
    /^__Host-srez_refresh=[^;]+;/.test(cookie) &&
      /; Path=\/(?:;|$)/.test(cookie) &&
      /; Secure(?:;|$)/.test(cookie) &&
      /; HttpOnly(?:;|$)/.test(cookie) &&
      /; SameSite=Strict(?:;|$)/.test(cookie) &&
      !/; Domain=/i.test(cookie),
    'Unsafe refresh cookie',
  );
  const bearer = { Authorization: 'Bearer ' + (await loggedIn.json()).access_token };
  await request('/api/profile', 200, { headers: bearer });
  await request('/api/wishlists', 200, { headers: bearer });
  const session = { Cookie: cookie.split(';')[0] };
  await request('/api/auth/refresh', 403, { body: {}, headers: session });
  const csrf = async (headers) =>
    (await (await request('/api/auth/csrf', 200, { headers })).json()).csrf_token;
  const rotated = await request('/api/auth/refresh', 200, {
    body: {},
    headers: { ...session, 'X-CSRF-Token': await csrf(session) },
  });
  const replacement = { Cookie: rotated.headers.get('set-cookie')?.split(';')[0] };
  check(
    replacement.Cookie && replacement.Cookie !== session.Cookie,
    'Refresh cookie was not rotated',
  );
  await request('/api/auth/logout', 204, {
    body: {},
    headers: { ...replacement, 'X-CSRF-Token': await csrf(replacement) },
  });
  await request('/api/profile', 401, {
    headers: { Authorization: 'Bearer ' + (await rotated.json()).access_token },
  });
  await request('/api/auth/forgot-password', 202, { body: { email } });
  await request('/api/auth/reset-password', 200, {
    body: { token: await mailToken('reset-password'), password: 'newpass1' },
  });
  await request('/api/auth/login', 401, { body: login });
  const relogin = await request('/api/auth/login', 200, {
    body: { ...login, password: 'newpass1' },
  });
  check(relogin.headers.has('set-cookie'), 'Reset did not permit a new login');
  compose('stop', 'mailpit');
  const unavailable = await request('/api/auth/register', 201, {
    body: { ...registration, email: 'unavailable@example.test', username: 'volta_unavailable' },
  });
  check(
    (await unavailable.json()).email_delivery_status === 'unavailable',
    'SMTP outage was not reported',
  );
  console.log(
    'Production topology, catalog, proxy, auth, STARTTLS mail and SMTP outage checks passed',
  );
} finally {
  if (cleanup) compose('down');
  if (dirname(scratch) === parent) rmSync(scratch, { recursive: true, force: true });
}
