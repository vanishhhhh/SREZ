import { expect, test } from '@playwright/test';
import { spawnSync } from 'node:child_process';
import { account, requireMVP, qaOrigin, mvpPassword } from './mvp-fixtures';
import { resetMessage, verificationMessage } from './verification-fixture';

test.beforeEach(({ baseURL }) => requireMVP(baseURL));
function fixtureSQL(sql: string) {
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
  const result = spawnSync(
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
  if (result.status !== 0) throw Error('Isolated recovery fixture SQL failed');
  return result.stdout.trim();
}

test('recovery UI: real Mailpit link, explicit reset, old access/refresh revoked, new login and reload', async ({
  page,
  context,
  request,
}) => {
  const user = await account(request, 'reset');
  // Keep a separate pre-reset refresh session and JWT for revocation assertions.
  const previous = await request.post('/api/auth/login', {
    headers: { Origin: qaOrigin },
    data: { identifier: user.username, password: user.password },
  });
  expect(previous.status()).toBe(200);
  const oldAuth = await previous.json();
  const oldCookies = await request.storageState();
  const errors: string[] = [];
  page.on('pageerror', () => errors.push('pageerror'));
  await page.goto('/login');
  await page.getByRole('link', { name: 'Забыли пароль?', exact: true }).click();
  await page.getByLabel('Email', { exact: true }).fill(user.username + '@example.test');
  await page.getByRole('button', { name: 'Отправить ссылку', exact: true }).click();
  await expect(page.getByText(/Если адрес подходит для восстановления/)).toBeVisible();
  await expect(page.getByRole('button', { name: /Повторить через/ })).toBeDisabled();
  const sent = await resetMessage(request, user.username + '@example.test');
  expect(sent.message.Subject).toBe('Сброс пароля — SREZ');
  const mailbox = await context.newPage();
  await mailbox.goto('http://127.0.0.1:18025');
  await mailbox
    .getByRole('textbox', { name: 'Search mailbox', exact: true })
    .fill('to:' + user.username + '@example.test');
  await mailbox.getByRole('textbox', { name: 'Search mailbox', exact: true }).press('Enter');
  await mailbox.getByText('Сброс пароля — SREZ', { exact: true }).first().click();
  const href = await mailbox
    .frameLocator('iframe')
    .getByRole('link', { name: 'Восстановить пароль', exact: true })
    .getAttribute('href');
  if (!href) throw Error('Recovery button has no href');
  await page.goto(href);
  await expect(page).toHaveURL(qaOrigin + '/reset-password');
  // GET never spends the token, even while existing sessions are still valid.
  expect((await request.get('/api/profile', { headers: user.headers })).status()).toBe(200);
  expect(
    await page.evaluate(
      () => Object.keys(localStorage).length + Object.keys(sessionStorage).length,
    ),
  ).toBe(0);
  expect(await page.evaluate(() => JSON.stringify(history.state || {}).includes('token='))).toBe(
    false,
  );
  const html = await request.get('/reset-password');
  expect(html.headers()['cache-control']).toBe('no-store');
  expect(html.headers()['referrer-policy']).toBe('no-referrer');
  const next = 'SREZ reset new password 456';
  await page.getByLabel('Новый пароль', { exact: true }).fill(next);
  await page
    .getByLabel('Повторите новый пароль', { exact: true })
    .fill('a different valid password');
  await page.getByRole('button', { name: 'Сохранить новый пароль', exact: true }).click();
  await expect(
    page
      .getByRole('form', { name: 'Сброс пароля' })
      .getByText('Пароли не совпадают.', { exact: true }),
  ).toBeVisible();
  await page.getByLabel('Повторите новый пароль', { exact: true }).fill(next);
  await page.getByRole('button', { name: 'Сохранить новый пароль', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Пароль обновлён.' })).toBeVisible();
  expect((await request.get('/api/profile', { headers: user.headers })).status()).toBe(401);
  const refreshCookie = oldCookies.cookies.find((c) => c.name === 'srez_refresh_local');
  expect(!!refreshCookie).toBe(true);
  const refresh = await request.post('/api/auth/refresh', {
    headers: {
      Origin: qaOrigin,
      'X-CSRF-Token': oldAuth.csrf_token,
      Cookie: `srez_refresh_local=${refreshCookie!.value}`,
    },
    data: {},
  });
  expect(refresh.status()).toBe(401);
  expect(
    (
      await request.post('/api/auth/login', {
        headers: { Origin: qaOrigin },
        data: { identifier: user.username, password: mvpPassword },
      })
    ).status(),
  ).toBe(401);
  expect(
    (
      await request.post('/api/auth/reset-password', {
        headers: { Origin: qaOrigin },
        data: { token: sent.token, password: next },
      })
    ).status(),
  ).toBe(400);
  await page.getByRole('link', { name: 'Войти с новым паролем', exact: true }).click();
  await page.getByLabel('Email или имя пользователя', { exact: true }).fill(user.username);
  await page.getByLabel('Пароль', { exact: true }).fill(next);
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await expect(page.locator('.account-link')).toContainText(user.username);
  await page.goto('/account');
  await page.reload();
  await expect(page.getByText('Email подтверждён', { exact: true })).toBeVisible();
  expect(errors).toHaveLength(0);
  await mailbox.close();
});

test('recovery security: uniform responses, rotation, expiry, purpose separation and concurrent single use', async ({
  request,
  page,
}) => {
  const user = await account(request, 'rotat'),
    email = user.username + '@example.test';
  const send = (address: string) =>
    request.post('/api/auth/forgot-password', {
      headers: { Origin: qaOrigin },
      data: { email: address },
    });
  const first = await send(email),
    unknown = await send('absent-' + Date.now() + '@example.test');
  expect(first.status()).toBe(202);
  expect(await first.json()).toEqual(await unknown.json());
  const old = await resetMessage(request, email);
  const verification = await verificationMessage(request, email);
  const reset = (token: string) =>
    request.post('/api/auth/reset-password', {
      headers: { Origin: qaOrigin },
      data: { token, password: 'a valid new password 456' },
    });
  expect((await reset(verification.token)).status()).toBe(400);
  expect(
    (
      await request.post('/api/auth/verify-email', {
        headers: { Origin: qaOrigin },
        data: { token: old.token },
      })
    ).status(),
  ).toBe(400);
  const uid = fixtureSQL(`SELECT id FROM users WHERE username='${user.username}'`);
  expect(/^\d+$/.test(uid)).toBe(true);
  const age = () =>
    fixtureSQL(
      `UPDATE password_reset_tokens SET created_at=created_at-interval '61 seconds',expires_at=expires_at-interval '61 seconds' WHERE user_id=${uid} AND used_at IS NULL AND revoked_at IS NULL`,
    );
  age();
  expect((await send(email)).status()).toBe(202);
  const next = await resetMessage(request, email, old.id);
  expect((await reset(old.token)).status()).toBe(400);
  fixtureSQL(
    `UPDATE password_reset_tokens SET created_at=created_at-interval '31 minutes',expires_at=expires_at-interval '31 minutes' WHERE user_id=${uid} AND used_at IS NULL AND revoked_at IS NULL`,
  );
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(next.link);
  await page.getByLabel('Новый пароль', { exact: true }).fill('a valid new password 456');
  await page.getByLabel('Повторите новый пароль', { exact: true }).fill('a valid new password 456');
  await page.getByRole('button', { name: 'Сохранить новый пароль', exact: true }).click();
  await expect(
    page
      .getByRole('form', { name: 'Сброс пароля' })
      .getByText(/Ссылка недействительна или истекла/),
  ).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  expect((await send(email)).status()).toBe(202);
  const current = await resetMessage(request, email, [old.id, next.id]);
  const replies = await Promise.all([reset(current.token), reset(current.token)]);
  expect(replies.map((r) => r.status()).sort()).toEqual([200, 400]);
});
