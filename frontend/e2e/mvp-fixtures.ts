import { verificationMessage, verifyFixtureAccount } from './verification-fixture';
import { expect, test } from '@playwright/test';
import type { APIRequestContext, Page } from '@playwright/test';
type FilePayload = { name: string; mimeType: string; buffer: Buffer };
import { randomBytes } from 'node:crypto';
import { fixturePNG, uploadFixture } from './media-fixture';
import { redactWishlistLinks } from '../scripts/redact-logs';
export const qaOrigin = 'http://localhost:5175';
export const mvpPassword = 'SREZ local MVP test 123';
export function requireMVP(baseURL?: string) {
  test.skip(
    process.env.SREZ_MVP_E2E_LIVE !== '1',
    'Requires the explicitly isolated MVP QA environment.',
  );
  expect(new URL(baseURL!).origin === qaOrigin).toBe(true);
}
export const suffix = () => Date.now().toString(36) + randomBytes(2).toString('hex');
export type Account = {
  username: string;
  password: string;
  token: string;
  headers: Record<string, string>;
};
export async function account(api: APIRequestContext, label = 'test'): Promise<Account> {
  const username = 'mvp_' + label.slice(0, 5) + '_' + suffix();
  const registered = await api.post('/api/auth/register', {
    headers: { Origin: qaOrigin },
    data: {
      username,
      email: username + '@example.test',
      display_name: 'MVP ' + label,
      password: mvpPassword,
    },
  });
  expect(registered.status()).toBe(201);
  await verifyFixtureAccount(api, username + '@example.test');
  const login = await api.post('/api/auth/login', {
    headers: { Origin: qaOrigin },
    data: { identifier: username, password: mvpPassword },
  });
  expect(login.status()).toBe(200);
  const token = (await login.json()).access_token;
  return { username, password: mvpPassword, token, headers: { Authorization: 'Bearer ' + token } };
}
export async function registerInUI(
  page: Page,
  api: APIRequestContext,
  label: string,
  keepReturnPage = false,
) {
  const username = 'journey_' + suffix();
  const email = username + '@example.test';
  await page.getByLabel('Как к вам обращаться', { exact: true }).fill(label);
  await page.getByLabel('Имя пользователя', { exact: true }).fill(username);
  await page.getByLabel('Email', { exact: true }).fill(email);
  await page.getByLabel('Пароль', { exact: true }).fill(mvpPassword);
  await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click();
  await expect(page).toHaveURL(/\/verify-email\/pending$/);
  const confirmation = keepReturnPage ? await page.context().newPage() : page;
  try {
    const { link } = await verificationMessage(api, email);
    await confirmation.goto(link).catch(() => {
      throw new Error('Не удалось открыть QA-ссылку подтверждения почты');
    });
    await confirmation.getByRole('button', { name: 'Подтвердить почту', exact: true }).click();
    await expect(confirmation.getByRole('heading', { name: 'Почта подтверждена.' })).toBeVisible();
  } finally {
    if (keepReturnPage) await confirmation.close();
  }
  if (keepReturnPage) {
    await page.bringToFront();
    await page.getByRole('link', { name: /Перейти ко входу/ }).click();
  } else {
    await page.getByRole('link', { name: 'Войти в SREZ', exact: true }).click();
  }
  await expect(page).toHaveURL(/\/login$/);
  const identifier = page.getByLabel('Email или имя пользователя', { exact: true });
  await identifier.fill(username);
  await page.getByLabel('Пароль', { exact: true }).fill(mvpPassword);
  const session = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === '/api/auth/login' &&
      response.request().method() === 'POST',
  );
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  const response = await session;
  expect(response.status()).toBe(200);
  const token: string = (await response.json()).access_token;
  return { username, password: mvpPassword, token, headers: { Authorization: 'Bearer ' + token } };
}

export async function login(page: Page, user: Account) {
  await page.goto('/login');
  await page.getByLabel('Email или имя пользователя', { exact: true }).fill(user.username);
  await page.getByLabel('Пароль', { exact: true }).fill(user.password);
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await expect(page.locator('.account-link')).toContainText(user.username);
}
export async function seedCatalog(api: APIRequestContext, user: Account, count = 2) {
  const tag = suffix(),
    headers = user.headers;
  const made = await api.post('/api/brands', {
    headers,
    data: { name: 'MVP Atelier ' + tag, slug: 'mvp-' + tag },
  });
  expect(made.status()).toBe(201);
  const brand = await made.json();
  expect(
    (
      await api.patch('/api/brands/' + brand.id, {
        headers,
        data: { description: 'Независимый бренд для изолированной проверки MVP.' },
      })
    ).status(),
  ).toBe(200);
  await uploadFixture(api, user.token, { purpose: 'brand_logo', brand_id: brand.id }, 50);
  expect(
    (await api.post('/api/brands/' + brand.id + '/publish', { headers, data: {} })).status(),
  ).toBe(200);
  const products = [];
  for (let i = 0; i < count; i++) {
    const made = await api.post('/api/products', {
      headers,
      data: {
        brand_id: brand.id,
        name: 'MVP Hoodie ' + tag + ' ' + (i + 1),
        description: 'Плотный хлопок, свободный силуэт. Настоящая API-запись тестового окружения.',
        price_kopecks: 499050 + i * 10000,
        category: 'hoodie',
        materials: ['хлопок'],
      },
    });
    expect(made.status()).toBe(201);
    const product = await made.json();
    const colored = await api.post('/api/products/' + product.id + '/colors', {
      headers,
      data: { color: 'Black' },
    });
    expect(colored.status()).toBe(201);
    const color = await colored.json();
    const sized = await api.post(`/api/products/${product.id}/colors/${color.id}/variants`, {
      headers,
      data: { size: 'M', stock: 5, sku: tag + '-' + i },
    });
    expect(sized.status()).toBe(201);
    const variant = await sized.json();
    const photo = await uploadFixture(
      api,
      user.token,
      { purpose: 'product_image', product_id: product.id, color_id: color.id },
      60 + i * 30,
    );
    expect(
      (await api.post('/api/products/' + product.id + '/publish', { headers, data: {} })).status(),
    ).toBe(200);
    products.push({ ...product, color, variant, photo });
  }
  return { brand, products, headers };
}
export async function uploadInUI(page: Page, label: string, tone = 75, payload?: FilePayload) {
  const input = page.getByLabel(label, { exact: true }).and(page.locator('input[type="file"]'));
  const id = await input
    .locator(
      'xpath=ancestor::div[contains(concat(" ",normalize-space(@class)," ")," upload-field ")][1]',
    )
    .getAttribute('id');
  expect(!!id).toBe(true);
  const widget = page.locator(`[id="${id}"]`);
  await widget
    .locator('input[type="file"]')
    .setInputFiles(
      payload || { name: 'srez-test.png', mimeType: 'image/png', buffer: fixturePNG(tone) },
    );
  await expect(widget.getByAltText('Предпросмотр выбранной фотографии')).toBeVisible();
  await widget.getByRole('button', { name: 'Загрузить фотографию', exact: true }).click();
  await expect(widget.getByAltText('Предпросмотр выбранной фотографии')).toHaveCount(0);
}
export async function noOverflow(page: Page) {
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1),
  ).toBe(true);
}
export function observeScriptErrors(page: Page) {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(redactWishlistLinks(error.message)));
  page.on('console', (message) => {
    if (message.type() === 'error' && !/Failed to load resource.*(?:401|404)/.test(message.text()))
      errors.push(redactWishlistLinks(message.text()));
  });
  return errors;
}
