import { expect, test } from '@playwright/test';
import type { APIRequestContext } from '@playwright/test';
import { spawnSync } from 'node:child_process';
import { verificationMessage } from './verification-fixture';
import { requireMVP, suffix } from './mvp-fixtures';

test.beforeEach(({ baseURL }) => requireMVP(baseURL));
const origin = 'http://localhost:5175';
const password = 'SREZ mail verification password 123';
function isolatedSQL(sql: string) {
  const label = spawnSync(
    'docker',
    [
      'inspect',
      '--format',
      '{{index .Config.Labels "com.docker.compose.project"}}',
      'srez-mvp-qa-db-1',
    ],
    { encoding: 'utf8' },
  );
  expect(label.stdout.trim()).toBe('srez-mvp-qa');
  const r = spawnSync(
    'docker',
    [
      'exec',
      'srez-mvp-qa-db-1',
      'psql',
      '-U',
      'srez',
      '-d',
      'srez',
      '-At',
      '-v',
      'ON_ERROR_STOP=1',
      '-c',
      sql,
    ],
    { encoding: 'utf8' },
  );
  if (r.status !== 0) throw Error('Isolated email fixture SQL failed');
  return r.stdout.trim();
}
async function register(api: APIRequestContext) {
  const username = 'email_' + suffix(),
    email = username + '@example.test';
  const r = await api.post('/api/auth/register', {
    headers: { Origin: origin },
    data: { email, username, display_name: 'Email QA', password },
  });
  expect(r.status()).toBe(201);
  const body = await r.json();
  expect(Number.isSafeInteger(body.id)).toBe(true);
  return { username, email, id: body.id as number, body };
}

test('Mailpit: registration, password gate, real email link, explicit confirmation and login', async ({
  page,
  context,
  request,
}) => {
  const username = 'mailui_' + suffix(),
    email = username + '@example.test';
  await page.goto('/register');
  await page.getByLabel('Как к вам обращаться', { exact: true }).fill('Почта SREZ');
  await page.getByLabel('Имя пользователя', { exact: true }).fill(username);
  await page.getByLabel('Email', { exact: true }).fill(email);
  await page.getByLabel('Пароль', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click();
  await expect(page).toHaveURL(/\/verify-email\/pending$/);
  await expect(page.getByRole('heading', { name: 'Проверьте почту.' })).toBeVisible();
  await expect(page.getByRole('button', { name: /Повторить через/ })).toBeDisabled();
  expect((await context.cookies()).some((c) => c.name.includes('srez_refresh'))).toBe(false);
  const refused = await request.post('/api/auth/login', {
    headers: { Origin: origin },
    data: { identifier: username, password },
  });
  expect(refused.status()).toBe(403);
  expect(refused.headers()['set-cookie']).toBeUndefined();
  const sent = await verificationMessage(request, email);
  expect(sent.message.Subject).toBe('Подтвердите email — SREZ');
  expect(sent.message.HTML).toContain('Подтвердить почту');
  const mailbox = await context.newPage();
  await mailbox.goto('http://127.0.0.1:18025');
  await mailbox.getByRole('textbox', { name: 'Search mailbox', exact: true }).fill('to:' + email);
  await mailbox.getByRole('textbox', { name: 'Search mailbox', exact: true }).press('Enter');
  await mailbox.getByText('Подтвердите email — SREZ', { exact: true }).first().click();
  const button = mailbox
    .frameLocator('iframe')
    .getByRole('link', { name: 'Подтвердить почту', exact: true });
  await expect(button).toBeVisible();
  // The actual link from the rendered email is used, without exposing it in logs.
  const href = await button.getAttribute('href');
  if (!href) throw Error('Mailpit button missing href');
  await page.goto(href);
  await expect(page).toHaveURL(origin + '/verify-email');
  expect(
    (
      await request.post('/api/auth/login', {
        headers: { Origin: origin },
        data: { identifier: username, password },
      })
    ).status(),
  ).toBe(403);
  expect(
    await page.evaluate(
      () => Object.keys(localStorage).length + Object.keys(sessionStorage).length,
    ),
  ).toBe(0);
  expect(
    await page.evaluate(() => history.state && JSON.stringify(history.state).includes('token=')),
  ).toBeFalsy();
  const html = await request.get('/verify-email');
  expect(html.headers()['cache-control']).toBe('no-store');
  expect(html.headers()['referrer-policy']).toBe('no-referrer');
  await page.getByRole('button', { name: 'Подтвердить почту', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Почта подтверждена.' })).toBeVisible();
  await page.screenshot({ path: 'docs/previews/email-confirmed-desktop.png', fullPage: true });
  expect((await context.cookies()).some((c) => c.name.includes('srez_refresh'))).toBe(false);
  await page.getByRole('link', { name: 'Войти в SREZ', exact: true }).click();
  await page.getByLabel('Email или имя пользователя', { exact: true }).fill(username);
  await page.getByLabel('Пароль', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await expect(page).toHaveURL(/\/account$/);
  await expect(page.getByText('Email подтверждён', { exact: true })).toBeVisible();
  expect((await context.cookies()).find((c) => c.name === 'srez_refresh_local_qa')?.httpOnly).toBe(
    true,
  );
  await mailbox.close();
});

test('email links: cooldown, rotation, expiry, generic resend and concurrent one-time use', async ({
  request,
  page,
}) => {
  const user = await register(request),
    old = await verificationMessage(request, user.email);
  const send = (identifier: string) =>
    request.post('/api/auth/resend-verification', {
      headers: { Origin: origin },
      data: { identifier },
    });
  const existing = await send(user.email),
    missing = await send('missing-' + suffix() + '@example.test');
  expect(existing.status()).toBe(202);
  expect(await existing.json()).toEqual(await missing.json());
  expect(
    Number(isolatedSQL(`SELECT count(*) FROM email_verification_tokens WHERE user_id=${user.id}`)),
  ).toBe(1);
  isolatedSQL(
    `UPDATE email_verification_tokens SET created_at=created_at-interval '61 seconds',expires_at=expires_at-interval '61 seconds' WHERE user_id=${user.id}`,
  );
  expect((await send(user.email)).status()).toBe(202);
  const newer = await verificationMessage(request, user.email, old.id);
  const confirm = (token: string) =>
    request.post('/api/auth/verify-email', { headers: { Origin: origin }, data: { token } });
  expect((await confirm(old.token)).status()).toBe(400);
  isolatedSQL(
    `UPDATE email_verification_tokens SET created_at=created_at-interval '31 minutes',expires_at=expires_at-interval '31 minutes' WHERE user_id=${user.id} AND used_at IS NULL AND revoked_at IS NULL`,
  );
  expect((await confirm(newer.token)).status()).toBe(400);
  await page.goto(newer.link);
  await page.getByRole('button', { name: 'Подтвердить почту', exact: true }).click();
  await expect(
    page.getByText(/Ссылка недействительна, уже использована или срок её действия истёк/).first(),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(
    true,
  );
  await page.screenshot({ path: 'docs/previews/email-expired-mobile.png', fullPage: true });
  expect((await send(user.email)).status()).toBe(202);
  // Select the newest message, not either of the two revoked/expired ones.
  let fresh = await verificationMessage(request, user.email, newer.id);
  await expect
    .poll(async () => {
      fresh = await verificationMessage(request, user.email, newer.id);
      return fresh.id !== old.id;
    })
    .toBe(true);
  const results = await Promise.all([confirm(fresh.token), confirm(fresh.token)]);
  expect(results.map((r) => r.status()).sort()).toEqual([200, 400]);
  expect(await (await send(user.email)).json()).toEqual(await missing.json());
  expect(
    Number(isolatedSQL(`SELECT count(*) FROM email_verification_tokens WHERE user_id=${user.id}`)),
  ).toBe(3);
});

test('SMTP outage keeps the account; restarting isolated Mailpit permits resend', async ({
  request,
}) => {
  const name = 'srez-mvp-qa-mailpit-1';
  const label = spawnSync(
    'docker',
    ['inspect', '--format', '{{index .Config.Labels "com.docker.compose.project"}}', name],
    { encoding: 'utf8' },
  );
  expect(label.stdout.trim()).toBe('srez-mvp-qa');
  try {
    expect(spawnSync('docker', ['stop', name], { stdio: 'ignore' }).status).toBe(0);
    const user = await register(request);
    expect(user.body.email_delivery_status).toBe('unavailable');
    expect(
      (
        await request.post('/api/auth/login', {
          headers: { Origin: origin },
          data: { identifier: user.username, password },
        })
      ).status(),
    ).toBe(403);
    expect(spawnSync('docker', ['start', name], { stdio: 'ignore' }).status).toBe(0);
    await expect
      .poll(async () => {
        try {
          return (await request.get('http://127.0.0.1:18025/api/v1/info')).status();
        } catch {
          return 0;
        }
      })
      .toBe(200);
    isolatedSQL(
      `UPDATE email_verification_tokens SET created_at=created_at-interval '61 seconds',expires_at=expires_at-interval '61 seconds' WHERE user_id=${user.id}`,
    );
    expect(
      (
        await request.post('/api/auth/resend-verification', {
          headers: { Origin: origin },
          data: { identifier: user.email },
        })
      ).status(),
    ).toBe(202);
    const sent = await verificationMessage(request, user.email);
    expect(
      (
        await request.post('/api/auth/verify-email', {
          headers: { Origin: origin },
          data: { token: sent.token },
        })
      ).status(),
    ).toBe(200);
    expect(
      (
        await request.post('/api/auth/login', {
          headers: { Origin: origin },
          data: { identifier: user.email, password },
        })
      ).status(),
    ).toBe(200);
  } finally {
    spawnSync('docker', ['start', name], { stdio: 'ignore' });
  }
});
