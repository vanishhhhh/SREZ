import { expect, test } from '@playwright/test';
import type { BrowserContext, Page } from '@playwright/test';

const sections = [
  {
    path: '/wishlists',
    guestTitle: 'Сохраняй то, что нравится',
    memberTitle: 'Мои вишлисты',
    login: 'Уже есть аккаунт? Войти',
    register: 'Создать аккаунт',
  },
  {
    path: '/orders',
    guestTitle: 'Все твои заказы — в одном месте',
    memberTitle: 'Мои заказы',
    login: 'Войти',
    register: 'Создать аккаунт',
  },
  {
    path: '/studio',
    guestTitle: 'Твой бренд. Твоя студия.',
    memberTitle: 'Студия бренда',
    login: 'Войти в аккаунт',
    register: 'Начать работу со SREZ',
  },
];
const credentials = { access_token: 'test-only-access', csrf_token: 'test-only-csrf' };
const listTitle = 'Подборка участника';
const timestamp = '2026-11-25T10:00:00Z';
const member = { id: 1, username: 'member', display_name: 'Участник SREZ', avatar_url: '' };

async function mockAPI(context: BrowserContext, signedIn = false) {
  const requests: string[] = [];
  await context.route('**/api/**', (route) => {
    const path = new URL(route.request().url()).pathname.slice(4);
    requests.push(path);
    switch (path) {
      case '/auth/csrf':
        return signedIn
          ? route.fulfill({ json: { csrf_token: credentials.csrf_token } })
          : route.fulfill({ status: 401, json: { error: 'invalid credentials' } });
      case '/auth/login':
        signedIn = true;
        return route.fulfill({ json: credentials });
      case '/auth/register':
        return route.fulfill({
          status: 201,
          json: {
            ...member,
            email_verification_required: true,
            email_delivery_status: 'sent',
            resend_after_seconds: 60,
          },
        });
      case '/auth/refresh':
        return route.fulfill({ json: credentials });
      case '/auth/logout':
        signedIn = false;
        return route.fulfill({ status: 204 });
      case '/profile':
        return route.fulfill({
          json: {
            ...member,
            email: 'member@example.com',
            email_verified_at: timestamp,
            email_verification_required: false,
          },
        });
      case '/cart':
        return route.fulfill({ json: { items: [], total_kopecks: 0 } });
      case '/favorites':
      case '/orders':
      case '/brands':
      case '/products':
      case '/me/brands':
      case '/me/brand-invitations':
        return route.fulfill({ json: [] });
      case '/wishlists':
        return route.fulfill({
          json: [
            {
              id: 1,
              title: listTitle,
              description: '',
              visibility: 'private',
              item_count: 0,
              created_at: timestamp,
              updated_at: timestamp,
            },
          ],
        });
      default:
        return route.fulfill({ status: 404, json: { error: 'Ресурс недоступен' } });
    }
  });
  return requests;
}

async function login(page: Page) {
  // The verification prompt has the same identifier label during the route transition.
  const form = page.getByRole('form', { name: 'Вход', exact: true });
  await expect(form).toBeVisible();
  await form.getByLabel('Email или имя пользователя', { exact: true }).fill('member');
  await form.getByLabel('Пароль', { exact: true }).fill('test password 123');
  await form.getByRole('button', { name: 'Войти', exact: true }).click();
}

for (const section of sections) {
  test(`guest content and auth return on ${section.path}`, async ({ page, context }) => {
    const requests = await mockAPI(context);
    const destination = section.path + '?source=navigation#section';
    await page.goto(destination);
    await expect(page.getByRole('heading', { name: section.guestTitle })).toBeVisible();
    await expect(page).toHaveURL((url) => url.pathname + url.search + url.hash === destination);
    await expect(page.getByRole('banner')).toBeVisible();
    await expect(page.getByRole('contentinfo')).toBeVisible();
    await expect(page.getByRole('main').getByRole('listitem')).toHaveCount(3);
    for (const width of [360, 768, 1440]) {
      await page.setViewportSize({ width, height: 900 });
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
        width,
      );
    }
    expect(requests).toEqual(['/auth/csrf']);

    await page.getByRole('link', { name: section.register, exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Регистрация', exact: true })).toBeVisible();
    await page.goBack();
    await page.getByRole('main').getByRole('link', { name: section.login, exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Вход', exact: true })).toBeVisible();
    await login(page);
    await expect(page).toHaveURL((url) => url.pathname + url.search + url.hash === destination);
    await expect(
      page.getByRole('heading', { name: section.memberTitle, exact: true }),
    ).toBeVisible();
    await expect(page.getByRole('heading', { name: section.guestTitle })).toHaveCount(0);
  });
}

test('registration keeps the studio destination through the email confirmation prompt', async ({
  page,
  context,
}) => {
  await mockAPI(context);
  await page.goto('/studio');
  await page.getByRole('link', { name: 'Начать работу со SREZ' }).click();
  await page.getByLabel('Как к вам обращаться', { exact: true }).fill('Участник SREZ');
  await page.getByLabel('Имя пользователя', { exact: true }).fill('member');
  await page.getByLabel('Email', { exact: true }).fill('member@example.com');
  await page.getByLabel('Пароль', { exact: true }).fill('test password 123');
  await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Проверьте почту.' })).toBeVisible();
  await page.getByRole('link', { name: 'Перейти ко входу' }).click();
  await login(page);
  await expect(page).toHaveURL(/\/studio$/);
  await expect(page.getByRole('heading', { name: 'Студия бренда', exact: true })).toBeVisible();
});

test('reload waits for session restoration and logout in another tab clears personal content', async ({
  page,
  context,
}) => {
  await mockAPI(context, true);
  let releaseRefresh = () => {};
  const waitForRelease = new Promise<void>((resolve) => {
    releaseRefresh = resolve;
  });
  await page.route(
    '**/api/auth/refresh',
    async (route) => {
      await waitForRelease;
      await route.fulfill({ json: credentials });
    },
    { times: 1 },
  );
  await page.goto('/wishlists');
  await expect(page.getByText('Восстанавливаем сессию…')).toBeVisible();
  await expect(page.getByRole('heading', { name: sections[0].guestTitle })).toHaveCount(0);
  releaseRefresh();
  await expect(page.getByRole('link', { name: new RegExp(listTitle) })).toBeVisible();
  await page.reload();
  await expect(page.getByRole('link', { name: new RegExp(listTitle) })).toBeVisible();

  const account = await context.newPage();
  await account.goto('/account');
  await account.getByRole('button', { name: 'Выйти из аккаунта', exact: true }).click();
  await expect(page.getByRole('heading', { name: sections[0].guestTitle })).toBeVisible();
  await expect(page.getByText(listTitle)).toHaveCount(0);
  await expect(page).toHaveURL(/\/wishlists$/);
});

test('public wishlist is still available to guests', async ({ page, context }) => {
  await mockAPI(context);
  const token = 'a'.repeat(43);
  await page.route('**/api/wishlists/shared/' + token, (route) =>
    route.fulfill({
      json: { title: 'Публичная подборка', description: '', updated_at: timestamp, items: [] },
    }),
  );
  await page.goto('/w/' + token);
  await expect(page.getByRole('heading', { name: 'Публичная подборка' })).toBeVisible();
});

test('studio management still requires login and membership', async ({ page, context }) => {
  await mockAPI(context);
  await page.goto('/studio/brands/42');
  await expect(page).toHaveURL(/\/login$/);
  await login(page);
  await expect(page).toHaveURL(/\/studio\/brands\/42$/);
  await expect(page.getByText('Страница или объект не найдены.')).toBeVisible();
  await expect(page.getByRole('heading', { name: sections[2].guestTitle })).toHaveCount(0);
});
