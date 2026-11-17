import { verificationMessage } from './verification-fixture';
import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';

const fixtureProfile = {
  id: 1,
  username: 'auth_reader',
  display_name: 'Участник SREZ',
  avatar_url: '',
  email: 'reader@example.com',
  email_verified_at: null,
  email_verification_required: false,
};
const authFixture = { access_token: 'test-only-memory-access', csrf_token: 'test-only-csrf' };
async function fixtureSession(
  page: Page,
  options: { offline?: () => boolean; conflict?: () => boolean } = {},
) {
  await page.route('**/api/auth/csrf', (route) =>
    options.offline?.()
      ? route.abort()
      : route.fulfill({ json: { csrf_token: authFixture.csrf_token } }),
  );
  await page.route('**/api/auth/refresh', (route) =>
    options.conflict?.()
      ? route.fulfill({ status: 409, json: { error: 'refresh already rotated' } })
      : route.fulfill({ json: authFixture }),
  );
  await page.route('**/api/profile', (route) => route.fulfill({ json: fixtureProfile }));
}
async function anonymous(page: Page) {
  await page.route('**/api/auth/csrf', (route) =>
    route.fulfill({ status: 401, json: { error: 'invalid credentials' } }),
  );
}

test.describe('auth UI and controlled failure responses', () => {
  test.beforeEach(async ({ page }) => {
    await page.route('**/api/favorites', (r) => r.fulfill({ json: [] }));
    await page.route('**/api/cart', (r) => r.fulfill({ json: { items: [], total_kopecks: 0 } }));
  });
  test('private route waits for recovery after network loss and preserves its URL', async ({
    page,
  }) => {
    let offline = true;
    await fixtureSession(page, { offline: () => offline });
    await page.goto('/account');
    await expect(
      page.getByRole('heading', { name: 'Сервер временно недоступен.', exact: true }),
    ).toBeVisible();
    await expect(page).toHaveURL(/\/account$/);
    await expect(page.getByRole('banner')).toBeVisible();
    await expect(page.getByRole('contentinfo')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Мой профиль', exact: true })).toHaveCount(0);
    offline = false;
    await page.getByRole('button', { name: 'Повторить подключение', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Мой профиль', exact: true })).toBeVisible();
  });
  test('two refresh conflicts stop automatically, explicit recovery can continue', async ({
    page,
  }) => {
    let conflicts = true;
    let refreshes = 0;
    await fixtureSession(page);
    await page.route('**/api/auth/refresh', (route) => {
      refreshes++;
      return route.fulfill(
        conflicts
          ? { status: 409, json: { error: 'refresh already rotated' } }
          : { json: authFixture },
      );
    });
    await page.goto('/account');
    await expect(
      page.getByRole('heading', { name: 'Не удалось подтвердить сессию.', exact: true }),
    ).toBeVisible();
    expect(refreshes).toBe(2);
    conflicts = false;
    await page.getByRole('button', { name: 'Повторить подключение', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Мой профиль', exact: true })).toBeVisible();
    expect(refreshes).toBe(3);
  });
  test('401 without cookie redirects to login and preserves the private destination', async ({
    page,
  }) => {
    await anonymous(page);
    await page.route('**/api/auth/login', (route) => route.fulfill({ json: authFixture }));
    await page.route('**/api/profile', (route) => route.fulfill({ json: fixtureProfile }));
    await page.goto('/account');
    await expect(page).toHaveURL(/\/login$/);
    await page.getByLabel('Email или имя пользователя', { exact: true }).fill('auth_reader');
    await page.getByLabel('Пароль', { exact: true }).fill('test password');
    await page.getByRole('button', { name: 'Войти', exact: true }).click();
    await expect(page).toHaveURL(/\/account$/);
  });
  test('registration retains the pending account when SMTP fails and never auto logs in', async ({
    page,
  }) => {
    let registrations = 0,
      logins = 0;
    await anonymous(page);
    await page.route('**/api/auth/register', (route) => {
      registrations++;
      return route.fulfill({
        status: 201,
        json: {
          ...fixtureProfile,
          email_verification_required: true,
          email_delivery_status: 'unavailable',
          resend_after_seconds: 60,
        },
      });
    });
    await page.route('**/api/auth/login', (route) => {
      logins++;
      return route.fulfill({ status: 503, json: { error: 'try later' } });
    });
    await page.goto('/register');
    await page.getByLabel('Как к вам обращаться', { exact: true }).fill('Участник SREZ');
    await page.getByLabel('Имя пользователя', { exact: true }).fill('auth_reader');
    await page.getByLabel('Email', { exact: true }).fill('reader@example.com');
    await page.getByLabel('Пароль', { exact: true }).fill('test password 123');
    await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click();
    await expect(page).toHaveURL(/\/verify-email\/pending$/);
    await expect(page.getByText(/Аккаунт создан, но письмо отправить не удалось/)).toBeVisible();
    await expect(page.getByRole('button', { name: /Повторить через/ })).toBeDisabled();
    expect(registrations).toBe(1);
    expect(logins).toBe(0);
  });
  test('403 and 409 profile errors retain the session and unsaved form', async ({ page }) => {
    let refreshes = 0;
    let patchStatus = 403;
    await fixtureSession(page);
    await page.route('**/api/auth/refresh', (route) => {
      refreshes++;
      return route.fulfill({ json: authFixture });
    });
    await page.route('**/api/profile', (route) =>
      route.request().method() === 'PATCH'
        ? route.fulfill({
            status: patchStatus,
            json: { error: patchStatus === 403 ? 'operation forbidden' : 'profile changed' },
          })
        : route.fulfill({ json: fixtureProfile }),
    );
    await page.goto('/account');
    await page.getByLabel('Имя', { exact: true }).fill('Новое имя');
    await page.getByRole('button', { name: 'Сохранить профиль', exact: true }).click();
    await expect(page.getByRole('alert').first()).toContainText('недостаточно прав');
    await expect(page.getByLabel('Имя', { exact: true })).toHaveValue('Новое имя');
    patchStatus = 409;
    await page.getByRole('button', { name: 'Сохранить профиль', exact: true }).click();
    await expect(page.getByRole('alert').first()).toContainText('Данные изменились');
    await expect(page).toHaveURL(/\/account$/);
    expect(refreshes).toBe(1);
  });
  test('expired refresh after a private 401 returns to login without looping', async ({ page }) => {
    let refreshes = 0;
    await fixtureSession(page);
    await page.route('**/api/auth/refresh', (route) => {
      refreshes++;
      return route.fulfill(
        refreshes === 1
          ? { json: authFixture }
          : { status: 401, json: { error: 'invalid credentials' } },
      );
    });
    await page.route('**/api/profile', (route) =>
      route.request().method() === 'PATCH'
        ? route.fulfill({ status: 401, json: { error: 'invalid access token' } })
        : route.fulfill({ json: fixtureProfile }),
    );
    await page.goto('/account');
    await page.getByLabel('Имя', { exact: true }).fill('Новое имя');
    await page.getByRole('button', { name: 'Сохранить профиль', exact: true }).click();
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByRole('alert').first()).toContainText('Сессия завершилась');
    expect(refreshes).toBe(2);
  });
  test('failed logout does not pretend that the server revoked the session', async ({ page }) => {
    let fails = true;
    await fixtureSession(page);
    await page.route('**/api/auth/logout', (route) =>
      fails ? route.abort() : route.fulfill({ status: 204 }),
    );
    await page.goto('/account');
    await page.getByRole('button', { name: 'Выйти из аккаунта', exact: true }).click();
    await expect(page.getByRole('alert').first()).toContainText('Не удалось связаться');
    await expect(page).toHaveURL(/\/account$/);
    fails = false;
    await page.getByRole('button', { name: 'Выйти из аккаунта', exact: true }).click();
    await expect(page).toHaveURL(/\/$/);
  });
  test('404 retains layout, and the server-unavailable page is addressable', async ({ page }) => {
    await anonymous(page);
    await page.goto('/does-not-exist');
    await expect(
      page.getByRole('heading', { name: 'Здесь пока ничего нет.', exact: true }),
    ).toBeVisible();
    await expect(page.getByRole('banner')).toBeVisible();
    await expect(page.getByRole('contentinfo')).toBeVisible();
    await page.goto('/unavailable');
    await expect(
      page.getByRole('heading', { name: 'Сервер временно недоступен.', exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole('button', { name: 'Повторить подключение', exact: true }),
    ).toBeEnabled();
    await page.screenshot({ path: 'test-results/auth-unavailable.png', fullPage: true });
  });
});

test('real Go: registration, username/email login, profile, reload, concurrent tabs and logout', async ({
  page,
  context,
  request,
  baseURL,
}) => {
  test.skip(
    process.env.SREZ_AUTH_E2E_LIVE !== '1',
    'Set SREZ_AUTH_E2E_LIVE=1 only with the separate auth test API.',
  );
  expect((await request.get('/api/health')).status()).toBe(200);
  const suffix = Date.now().toString(36);
  const username = 'auth_' + suffix;
  const email = username + '@example.com';
  const password = 'local auth testing 123';
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));
  await page.goto('/account');
  await expect(page).toHaveURL(/\/login$/);
  expect(
    await page
      .locator('.auth-story h2')
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true);
  await page.screenshot({ path: 'test-results/auth-login.png', fullPage: true });
  await page.getByRole('link', { name: 'Присоединиться', exact: true }).click();
  await page.screenshot({ path: 'test-results/auth-register.png', fullPage: true });
  await page.getByLabel('Как к вам обращаться', { exact: true }).fill('Участник SREZ');
  await page.getByLabel('Имя пользователя', { exact: true }).fill(username);
  await page.getByLabel('Email', { exact: true }).fill(email);
  await page.getByLabel('Пароль', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click();
  await expect(page).toHaveURL(/\/verify-email\/pending$/);
  const confirmation = await verificationMessage(context.request, email);
  await page.goto(confirmation.link);
  await expect(page).toHaveURL(/\/verify-email$/);
  await page.getByRole('button', { name: 'Подтвердить почту', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Почта подтверждена.' })).toBeVisible();
  await page.getByRole('link', { name: 'Войти в SREZ', exact: true }).click();
  await page.getByLabel('Email или имя пользователя', { exact: true }).fill(username);
  await page.getByLabel('Пароль', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await expect(page).toHaveURL(/\/account$/);
  await page.getByLabel('Имя', { exact: true }).fill('Автор SREZ Auth');
  await page.getByRole('button', { name: 'Сохранить профиль', exact: true }).click();
  await expect(page.locator('.account-summary h2')).toHaveText('Автор SREZ Auth');
  await expect(page.getByLabel('Email', { exact: true })).toHaveAttribute('readonly', '');
  const before = (await context.cookies()).find((cookie) => cookie.name === 'srez_refresh_local');
  expect(!!before && before.httpOnly && before.sameSite === 'Strict').toBe(true);
  await page.reload();
  await expect(page.locator('.account-summary h2')).toHaveText('Автор SREZ Auth');
  const after = (await context.cookies()).find((cookie) => cookie.name === 'srez_refresh_local');
  expect(!!before && !!after && before.value !== after.value).toBe(true);
  expect(!!before && !!after && Math.abs(before.expires - after.expires) < 2).toBe(true);
  expect(
    await page.evaluate(
      () => Object.keys(localStorage).length + Object.keys(sessionStorage).length,
    ),
  ).toBe(0);
  expect(await page.evaluate(() => document.cookie.includes('srez_refresh'))).toBe(false);
  const second = await context.newPage();
  try {
    await Promise.all([page.reload(), second.goto('/account')]);
    await expect(page.locator('.account-summary h2')).toHaveText('Автор SREZ Auth');
    await expect(second.locator('.account-summary h2')).toHaveText('Автор SREZ Auth');
    expect(
      (
        await context.request.post('/api/auth/login', {
          headers: { Origin: 'http://evil.example' },
          data: { identifier: username, password },
        })
      ).status(),
    ).toBe(403);
    expect(
      (
        await context.request.post('/api/auth/refresh', {
          headers: { Origin: new URL(baseURL!).origin },
          data: {},
        })
      ).status(),
    ).toBe(403);
    expect(
      (
        await context.request.get('/api/profile', { headers: { Authorization: 'Bearer expired' } })
      ).status(),
    ).toBe(401);
    await page.screenshot({ path: 'test-results/auth-profile-desktop.png', fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await page.screenshot({ path: 'test-results/auth-profile-mobile.png', fullPage: true });
    await page.getByRole('button', { name: 'Выйти из аккаунта', exact: true }).click();
    await expect(second).toHaveURL(/\/login$/);
  } finally {
    await second.close();
  }
  await page.goto('/account');
  await page.getByLabel('Email или имя пользователя', { exact: true }).fill(email);
  await page.getByLabel('Пароль', { exact: true }).fill('wrong password');
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await expect(page.getByRole('alert').first()).toContainText('Неверный логин или пароль');
  await page.getByLabel('Пароль', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await expect(page).toHaveURL(/\/account$/);
  await page.getByRole('button', { name: 'Выйти из аккаунта', exact: true }).click();
  await expect(page).toHaveURL(new URL('/', baseURL!).href);
  await page.goto('/account');
  await page.getByLabel('Email или имя пользователя', { exact: true }).fill(username);
  await page.getByLabel('Пароль', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await expect(page.locator('.account-summary h2')).toHaveText('Автор SREZ Auth');
  await page.getByRole('button', { name: 'Выйти из аккаунта', exact: true }).click();
  await expect(page).toHaveURL(new URL('/', baseURL!).href);
  await page.goto('/account');
  await expect(page).toHaveURL(/\/login$/);
  await page.reload();
  await expect(page.getByRole('heading', { name: 'С возвращением.', exact: true })).toBeVisible();
  expect(pageErrors).toEqual([]);
  expect((await request.get('/brands')).headers()['content-type']).toContain('text/html');
  expect((await request.get('/api/brands')).headers()['content-type']).toContain(
    'application/json',
  );
  const media = await request.get('/media/auth-proxy-check');
  expect(media.status()).toBe(404);
  expect(media.headers()['content-type']).not.toContain('text/html');
});
