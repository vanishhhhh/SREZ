import { verifyFixtureAccount } from './verification-fixture';
import { uploadInUI } from './mvp-fixtures';
import { expect, test } from '@playwright/test';
import type { APIRequestContext, Page } from '@playwright/test';
import { fixturePNG, uploadFixture } from './media-fixture';

test.beforeEach(async ({ baseURL }) => {
  test.skip(
    process.env.SREZ_BUYER_E2E_LIVE !== '1',
    'Explicit isolated buyer test environment required.',
  );
  expect(new URL(baseURL!).origin).toBe('http://localhost:5175');
});
const password = 'SREZ integration local password 123';
async function account(api: APIRequestContext, origin: string) {
  const username =
    'buyer_' + Date.now().toString(36) + '_' + Math.random().toString(36).slice(2, 6);
  const registered = await api.post('/api/auth/register', {
    headers: { Origin: origin },
    data: { username, email: username + '@example.com', display_name: 'Автор SREZ', password },
  });
  expect(registered.status()).toBe(201);
  await verifyFixtureAccount(api, username + '@example.com');
  const logged = await api.post('/api/auth/login', {
    headers: { Origin: origin },
    data: { identifier: username, password },
  });
  expect(logged.status()).toBe(200);
  return { username, token: (await logged.json()).access_token as string };
}
async function seed(api: APIRequestContext, token: string) {
  const headers = { Authorization: 'Bearer ' + token };
  const write = async (path: string, data: unknown, method = 'POST') => {
    const r = await api.fetch('/api' + path, { method, headers, data });
    expect(r.ok(), method + ' ' + path + ' ' + r.status()).toBe(true);
    return r.json();
  };
  const brand = await write('/brands', {
    name: 'Городская форма',
    slug: 'city-' + Date.now().toString(36) + Math.random().toString(36).slice(2, 5),
  });
  await write(
    '/brands/' + brand.id,
    { description: 'Независимый бренд. Вещи для города.' },
    'PATCH',
  );
  await uploadFixture(api, token, { purpose: 'brand_logo', brand_id: brand.id });
  await write('/brands/' + brand.id + '/publish', {});
  const p = await write('/products', {
    brand_id: brand.id,
    name: 'Худи Точка',
    description: 'Свободная форма, плотный хлопок.',
    price_kopecks: 355050,
    category: 'hoodie',
    materials: ['Хлопок 100%'],
  });
  const c = await write(`/products/${p.id}/colors`, { color: 'Графит' });
  const v = await write(`/products/${p.id}/colors/${c.id}/variants`, {
    size: 'M',
    stock: 8,
    sku: 'LOCAL-' + p.id,
  });
  await write(`/products/${p.id}/colors/${c.id}/variants`, { size: 'L', stock: 0 });
  await uploadFixture(api, token, { purpose: 'product_image', product_id: p.id, color_id: c.id });
  await write(`/products/${p.id}/publish`, {});
  return { brand, product: p, color: c, variant: v, headers };
}
async function login(page: Page, username: string) {
  await page.goto('/account');
  await expect(page).toHaveURL(/\/login$/);
  await page.getByLabel('Email или имя пользователя', { exact: true }).fill(username);
  await page.getByLabel('Пароль', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await expect(page).toHaveURL(/\/account$/);
}
async function stock(api: APIRequestContext, id: number) {
  const r = await api.get('/api/products/' + id);
  expect(r.status()).toBe(200);
  return (await r.json()).colors[0].variants.find((v: { size: string }) => v.size === 'M')
    .stock as number;
}

test('buyer: real login → favorite → cart quantity/remove → unpaid reserve → cancel → restored stock/history', async ({
  page,
  playwright,
  baseURL,
}) => {
  const origin = new URL(baseURL!).origin;
  const api = await playwright.request.newContext({ baseURL });
  try {
    const user = await account(api, origin);
    const data = await seed(api, user.token);
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await login(page, user.username);
    await page.goto('/products/' + data.product.id);
    await page.getByRole('button', { name: 'Добавить в избранное', exact: true }).click();
    await expect(
      page.getByRole('button', { name: 'Удалить из избранного', exact: true }),
    ).toHaveAttribute('aria-pressed', 'true');
    await expect(
      page.getByRole('button', { name: 'Добавить в корзину', exact: false }),
    ).toBeDisabled();
    await page
      .getByRole('group', { name: 'Размер', exact: true })
      .getByRole('button', { name: 'M', exact: true })
      .click();
    await page.getByRole('button', { name: 'Добавить в корзину', exact: false }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Добавлено в корзину' })).toBeVisible();
    expect(await stock(api, data.product.id)).toBe(8);
    await page.getByRole('link', { name: /^Избранное:/ }).click();
    await expect(page.locator('.product-tile h3')).toHaveText(['Худи Точка']);
    await page.getByRole('link', { name: /^Корзина:/ }).click();
    await expect(page.locator('.cart-line')).toHaveCount(1);
    const quantity = page.locator('.quantity-form input');
    await quantity.fill('2');
    const updateResponse = page.waitForResponse(
      (response) =>
        response.request().method() === 'PATCH' &&
        /^\/api\/cart\/items\/\d+$/.test(new URL(response.url()).pathname),
    );
    await page.getByRole('button', { name: 'Обновить', exact: true }).click();
    const updated = await updateResponse;
    expect(updated.status()).toBe(200);
    expect((await updated.json()).items[0].quantity).toBe(2);
    await expect(page.locator('.cart-total strong')).toContainText('7 101');
    await page.getByRole('button', { name: 'Удалить позицию', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Пока ни одной вещи' })).toBeVisible();
    await page.goto('/products/' + data.product.id);
    await page
      .getByRole('group', { name: 'Размер', exact: true })
      .getByRole('button', { name: 'M', exact: true })
      .click();
    await page.getByLabel('Количество товара').fill('2');
    await page.getByRole('button', { name: 'Добавить в корзину', exact: false }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Добавлено в корзину' })).toBeVisible();
    await page.goto('/cart');
    await expect(page.getByText(/Деньги не списываются/)).toBeVisible();
    let ordersPosted = 0;
    page.on('request', (r) => {
      if (new URL(r.url()).pathname === '/api/orders' && r.method() === 'POST') {
        ordersPosted++;
        expect(r.postDataJSON()).toEqual({});
      }
    });
    await page
      .getByRole('button', { name: /Зарезервировать на 15 минут/ })
      .click({ clickCount: 2 });
    await expect(page).toHaveURL(/\/orders\/\d+$/);
    expect(ordersPosted).toBe(1);
    await expect(page.locator('.order-status')).toBeVisible();
    expect(await stock(api, data.product.id)).toBe(6);
    const id = new URL(page.url()).pathname.split('/').at(-1)!;
    const actual = await (await api.get('/api/orders/' + id, { headers: data.headers })).json();
    expect(actual.status).toBe('unpaid');
    expect(Date.parse(actual.cancel_before) - Date.parse(actual.created_at)).toBe(15 * 60 * 1000);
    await expect(page.getByText(/Деньги не списаны/)).toBeVisible();
    await page.getByRole('button', { name: 'Отменить резерв', exact: true }).click();
    await page
      .getByRole('dialog')
      .getByRole('button', { name: 'Отменить резерв', exact: true })
      .click();
    await expect(page.locator('.order-status')).toHaveText('Резерв отменён');
    expect(await stock(api, data.product.id)).toBe(8);
    const again = await api.post('/api/orders/' + id + '/cancel', {
      headers: data.headers,
      data: {},
    });
    expect(again.status()).toBe(200);
    expect(await stock(api, data.product.id)).toBe(8);
    await page.getByRole('link', { name: 'Все заказы', exact: true }).click();
    await expect(page.locator('.order-card')).toHaveCount(1);
    await expect(page.locator('.order-card')).toContainText('Резерв отменён');
    await page.reload();
    await expect(page.locator('.order-card')).toHaveCount(1);
    await page.setViewportSize({ width: 390, height: 844 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(
      true,
    );
    await page.screenshot({ path: 'test-results/buyer-orders-mobile.jpg', fullPage: true });
    expect(
      await page.evaluate(
        () => Object.keys(localStorage).length + Object.keys(sessionStorage).length,
      ),
    ).toBe(0);
    expect(errors).toEqual([]);
  } finally {
    await api.dispose();
  }
});

test('media/PVM: file uploads for avatar, brand logo/banner and product images preserve auth and stable image IDs', async ({
  page,
  playwright,
  baseURL,
}) => {
  const origin = new URL(baseURL!).origin;
  const api = await playwright.request.newContext({ baseURL });
  try {
    const user = await account(api, origin);
    const data = await seed(api, user.token);
    await login(page, user.username);
    await page.getByLabel('Имя', { exact: true }).fill('Автор интеграции');
    let profileBody: unknown;
    page.on('request', (r) => {
      if (r.method() === 'PATCH' && new URL(r.url()).pathname === '/api/profile')
        profileBody = r.postDataJSON();
    });
    await page.getByRole('button', { name: 'Сохранить профиль', exact: true }).click();
    await expect(page.locator('.account-summary h2')).toHaveText('Автор интеграции');
    expect(profileBody).toEqual({ display_name: 'Автор интеграции' });
    const file = { name: 'srez.png', mimeType: 'image/png', buffer: fixturePNG() };
    await page.getByLabel('Загрузить аватар').setInputFiles({
      ...file,
      buffer: file.buffer.subarray(0, 14),
    });
    await page.getByRole('button', { name: 'Загрузить фотографию', exact: true }).click();
    await expect(
      page.getByText('Не удалось загрузить аватар. Файл повреждён.', { exact: false }),
    ).toBeVisible();
    await uploadInUI(page, 'Загрузить аватар', 75, file);
    await expect(page.getByRole('button', { name: 'Удалить аватар', exact: true })).toBeVisible();
    await expect(page.locator('.account-summary img')).toHaveAttribute('src', /^blob:/);
    const profile = await (await api.get('/api/profile', { headers: data.headers })).json();
    expect(profile.avatar_url).toMatch(/^\/media\/assets\/\d+\/content$/);
    expect((await api.get(profile.avatar_url)).status()).toBe(404);
    await page.goto('/studio/brands/' + data.brand.id + '?tab=settings');
    await uploadInUI(page, 'Загрузить логотип', 75, file);
    await expect(page.locator('.toast')).toContainText('Фотография сохранена');
    await uploadInUI(page, 'Загрузить баннер', 75, file);
    await expect(page.getByRole('button', { name: 'Удалить баннер', exact: true })).toBeVisible();
    const readManagedBrand = async () => {
      const response = await api.get('/api/brands/' + data.brand.id + '/manage', {
        headers: data.headers,
      });
      expect(response.ok()).toBe(true);
      return response.json();
    };
    const originalBrand = await readManagedBrand();
    await uploadInUI(page, 'Загрузить баннер', 110, { ...file, buffer: fixturePNG(110) });
    const updatedBrand = await readManagedBrand();
    expect(updatedBrand.banner_url).toMatch(/^\/media\/assets\/\d+\/content$/);
    expect(updatedBrand.banner_url).not.toBe(originalBrand.banner_url);
    expect(updatedBrand.logo_url).toBe(originalBrand.logo_url);
    await page.getByLabel(/^Название бренда/).fill('Городская форма новая');
    let brandBody: Record<string, unknown> | undefined;
    page.on('request', (r) => {
      if (r.method() === 'PATCH' && new URL(r.url()).pathname === '/api/brands/' + data.brand.id)
        brandBody = r.postDataJSON();
    });
    await page.getByRole('button', { name: 'Сохранить настройки', exact: true }).click();
    await expect(page.locator('.toast')).toContainText('Настройки бренда сохранены');
    expect(brandBody).not.toHaveProperty('logo_url');
    expect(brandBody).not.toHaveProperty('banner_url');
    await page.goto(`/studio/brands/${data.brand.id}/products/${data.product.id}`);
    await expect(page.locator('.editor-image')).toHaveCount(1);
    await uploadInUI(page, 'Добавить фотографию', 75, file);
    await expect(page.locator('.editor-image')).toHaveCount(2);
    const first = await (
      await api.get(`/api/products/${data.product.id}/colors/${data.color.id}/images`, {
        headers: data.headers,
      })
    ).json();
    await page.getByRole('button', { name: 'Заменить фото 1', exact: true }).click();
    const replacementDone = page.waitForResponse(
      (r) => r.request().method() === 'PUT' && r.url().includes('/media/uploads/'),
    );
    await uploadInUI(page, 'Заменить фото 1', 110, { ...file, buffer: fixturePNG(110) });
    expect((await replacementDone).status()).toBe(200);
    await expect(page.locator('.toast')).toContainText('Фотография сохранена');
    const replaced = await (
      await api.get(`/api/products/${data.product.id}/colors/${data.color.id}/images`, {
        headers: data.headers,
      })
    ).json();
    expect(replaced[0].id).toBe(first[0].id);
    expect(replaced[0].media_id).not.toBe(first[0].media_id);
    await page.getByRole('button', { name: 'Переместить фото 2 раньше', exact: true }).click();
    await expect(page.locator('.toast')).toContainText('Порядок фотографий изменён');
    await page.goto('/products/' + data.product.id);
    await expect(page.locator('.product-gallery > .media')).toHaveAttribute(
      'data-media-src',
      /^\/media\/assets\/\d+\/content$/,
    );
    const content = await api.get(replaced[0].url);
    expect(content.status()).toBe(200);
    expect(content.headers()['content-type']).toMatch(/^image\/(png|jpeg)/);
    expect(content.headers()['x-content-type-options']).toBe('nosniff');
  } finally {
    await api.dispose();
  }
});

test('buyer API: concurrent last-stock reservations, ownership, invalid quantity and price recheck', async ({
  playwright,
  baseURL,
  page,
}) => {
  const origin = new URL(baseURL!).origin;
  const a = await playwright.request.newContext({ baseURL }),
    b = await playwright.request.newContext({ baseURL });
  try {
    const first = await account(a, origin),
      second = await account(b, origin);
    const data = await seed(a, first.token);
    const secondHeaders = { Authorization: 'Bearer ' + second.token };
    await a.patch(
      `/api/products/${data.product.id}/colors/${data.color.id}/variants/${data.variant.id}`,
      { headers: data.headers, data: { stock: 1 } },
    );
    expect(
      (
        await a.post('/api/cart/items', {
          headers: data.headers,
          data: { product_variant_id: data.variant.id, quantity: 0 },
        })
      ).status(),
    ).toBe(400);
    await a.post('/api/cart/items', {
      headers: data.headers,
      data: { product_variant_id: data.variant.id, quantity: 1 },
    });
    await b.post('/api/cart/items', {
      headers: secondHeaders,
      data: { product_variant_id: data.variant.id, quantity: 1 },
    });
    const results = await Promise.all([
      a.post('/api/orders', { headers: data.headers, data: {} }),
      b.post('/api/orders', { headers: secondHeaders, data: {} }),
    ]);
    expect(results.map((r) => r.status()).sort()).toEqual([201, 409]);
    expect(await stock(a, data.product.id)).toBe(0);
    const winner = results[0].status() === 201 ? 0 : 1;
    const order = await results[winner].json();
    const winnerAPI = winner === 0 ? a : b,
      winnerHeaders = winner === 0 ? data.headers : secondHeaders,
      loserAPI = winner === 0 ? b : a,
      loserHeaders = winner === 0 ? secondHeaders : data.headers;
    expect(
      (await loserAPI.get('/api/orders/' + order.id, { headers: loserHeaders })).status(),
    ).toBe(404);
    expect(
      (
        await loserAPI.post('/api/orders/' + order.id + '/cancel', {
          headers: loserHeaders,
          data: {},
        })
      ).status(),
    ).toBe(404);
    await winnerAPI.post('/api/orders/' + order.id + '/cancel', {
      headers: winnerHeaders,
      data: {},
    });
    expect(await stock(a, data.product.id)).toBe(1);
    const remainingCart = await (await a.get('/api/cart', { headers: data.headers })).json();
    for (const item of remainingCart.items)
      await a.delete('/api/cart/items/' + item.id, { headers: data.headers });
    await login(page, first.username);
    await page.goto('/products/' + data.product.id);
    await page
      .getByRole('group', { name: 'Размер', exact: true })
      .getByRole('button', { name: 'M', exact: true })
      .click();
    await page.getByRole('button', { name: 'Добавить в корзину', exact: false }).click();
    await expect(page.locator('.toast')).toContainText(/Добавлено|Недостаточно/);
    await page.goto('/cart');
    await expect(page.locator('.cart-line')).toHaveCount(1);
    const patch = await a.patch('/api/products/' + data.product.id, {
      headers: data.headers,
      data: { price_kopecks: 400000 },
    });
    expect(patch.status()).toBe(200);
    let posted = 0;
    page.on('request', (r) => {
      if (r.method() === 'POST' && new URL(r.url()).pathname === '/api/orders') posted++;
    });
    await page.getByRole('button', { name: /Зарезервировать на 15 минут/ }).click();
    await expect(
      page
        .locator('.cart-summary .notice--error')
        .filter({ hasText: 'Корзина или цена изменилась' }),
    ).toBeVisible();
    expect(posted).toBe(0);
    await expect(page.locator('.cart-total strong')).toContainText('4 000');
  } finally {
    await a.dispose();
    await b.dispose();
  }
});

test('ambiguous checkout: committed reserve with lost response is discovered in history without a second POST', async ({
  page,
  playwright,
  baseURL,
}) => {
  const api = await playwright.request.newContext({ baseURL });
  try {
    const user = await account(api, new URL(baseURL!).origin);
    const data = await seed(api, user.token);
    await api.post('/api/cart/items', {
      headers: data.headers,
      data: { product_variant_id: data.variant.id, quantity: 1 },
    });
    await login(page, user.username);
    await page.goto('/cart');
    await expect(page.locator('.cart-line')).toHaveCount(1);
    let attempts = 0;
    await page.route('**/api/orders', async (route) => {
      if (route.request().method() !== 'POST') return route.continue();
      attempts++;
      const response = await route.fetch();
      expect(response.status()).toBe(201);
      await route.abort();
    });
    await page.getByRole('button', { name: /Зарезервировать на 15 минут/ }).click();
    await expect(
      page.locator('#main .notice--error').filter({ hasText: 'Резерв мог быть создан' }),
    ).toBeVisible();
    expect(attempts).toBe(1);
    expect(await stock(api, data.product.id)).toBe(7);
    await page.getByRole('link', { name: 'Проверьте историю заказов', exact: true }).click();
    await expect(page.locator('.order-card')).toHaveCount(1);
    await expect(page.locator('.order-status')).toHaveText('Неоплаченный резерв');
    expect(attempts).toBe(1);
    const orders = await (await api.get('/api/orders', { headers: data.headers })).json();
    await api.post('/api/orders/' + orders[0].id + '/cancel', { headers: data.headers, data: {} });
  } finally {
    await api.dispose();
  }
});

test('reservation expiry: server releases only a newly created isolated test order and UI refreshes its status', async ({
  page,
  playwright,
  baseURL,
}) => {
  test.skip(
    process.env.SREZ_EXPIRY_DB !== 'srez-mvp-qa-db-1',
    'Requires the explicitly named isolated database.',
  );
  expect(new URL(baseURL!).origin).toBe('http://localhost:5175');
  const { execFileSync } = await import('node:child_process');
  const project = execFileSync(
    'docker',
    [
      'inspect',
      'srez-mvp-qa-db-1',
      '--format',
      '{{index .Config.Labels "com.docker.compose.project"}}',
    ],
    { encoding: 'utf8' },
  ).trim();
  expect(project).toBe('srez-mvp-qa');
  const api = await playwright.request.newContext({ baseURL });
  try {
    const user = await account(api, new URL(baseURL!).origin);
    const data = await seed(api, user.token);
    const profile = await (await api.get('/api/profile', { headers: data.headers })).json();
    await api.post('/api/cart/items', {
      headers: data.headers,
      data: { product_variant_id: data.variant.id, quantity: 2 },
    });
    const created = await api.post('/api/orders', { headers: data.headers, data: {} });
    expect(created.status()).toBe(201);
    const order = await created.json();
    expect(Number.isSafeInteger(order.id) && Number.isSafeInteger(profile.id)).toBe(true);
    expect(await stock(api, data.product.id)).toBe(6);
    await login(page, user.username);
    await page.goto('/orders/' + order.id);
    await expect(page.locator('.order-status')).toHaveText('Неоплаченный резерв');
    // Test-only clock fixture on this exact newly created order. No status/stock is
    // set by the frontend: the existing Go expiry worker performs the transition.
    const sql = `UPDATE orders SET created_at=now()-interval '16 minutes',cancel_before=now()-interval '1 minute' WHERE id=${order.id} AND user_id=${profile.id} AND status='unpaid' RETURNING id`;
    const updated = execFileSync(
      'docker',
      [
        'exec',
        'srez-mvp-qa-db-1',
        'psql',
        '-U',
        'srez',
        '-d',
        'srez',
        '-v',
        'ON_ERROR_STOP=1',
        '-Atc',
        sql,
      ],
      { encoding: 'utf8' },
    );
    expect(updated).toContain(String(order.id));
    await expect(page.locator('.order-status')).toHaveText('Срок резерва истёк', {
      timeout: 25000,
    });
    expect(await stock(api, data.product.id)).toBe(8);
    const closed = await api.post('/api/orders/' + order.id + '/cancel', {
      headers: data.headers,
      data: {},
    });
    expect(closed.status()).toBe(409);
    expect(await stock(api, data.product.id)).toBe(8);
    await expect(page.getByRole('button', { name: 'Отменить резерв', exact: true })).toHaveCount(0);
  } finally {
    await api.dispose();
  }
});

test('favorites synchronize across product cards and two tabs without transferring tokens', async ({
  page,
  context,
  playwright,
  baseURL,
}) => {
  const api = await playwright.request.newContext({ baseURL });
  let other: Page | undefined;
  try {
    const user = await account(api, new URL(baseURL!).origin);
    const data = await seed(api, user.token);
    await login(page, user.username);
    await page.goto('/products/' + data.product.id);
    other = await context.newPage();
    await other.goto('/favorites');
    await expect(other.getByRole('heading', { name: 'Здесь будет ваш выбор' })).toBeVisible();
    await page.getByRole('button', { name: 'Добавить в избранное', exact: true }).click();
    await expect(other.locator('.product-tile h3')).toHaveText(['Худи Точка']);
    await other.getByRole('button', { name: 'Удалить из избранного', exact: true }).click();
    await expect(
      page.getByRole('button', { name: 'Добавить в избранное', exact: true }),
    ).toHaveAttribute('aria-pressed', 'false');
    await expect(other.getByRole('heading', { name: 'Здесь будет ваш выбор' })).toBeVisible();
  } finally {
    await other?.close();
    await api.dispose();
  }
});

test('hidden product remains removable in favorites/cart and buyer pages stay responsive', async ({
  page,
  playwright,
  baseURL,
}) => {
  const api = await playwright.request.newContext({ baseURL });
  try {
    const user = await account(api, new URL(baseURL!).origin);
    const data = await seed(api, user.token);
    await api.post('/api/favorites/' + data.product.id, { headers: data.headers });
    await api.post('/api/cart/items', {
      headers: data.headers,
      data: { product_variant_id: data.variant.id, quantity: 1 },
    });
    await login(page, user.username);
    for (const width of [390, 768, 1280]) {
      await page.setViewportSize({ width, height: 850 });
      for (const path of ['/favorites', '/cart', '/orders']) {
        await page.goto(path);
        await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
        expect(
          await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1),
        ).toBe(true);
      }
    }
    await api.post(`/api/products/${data.product.id}/hide`, { headers: data.headers, data: {} });
    await page.goto('/favorites');
    await expect(page.locator('.unavailable-favorite')).toHaveCount(1);
    await page.getByRole('button', { name: 'Удалить из избранного', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Здесь будет ваш выбор' })).toBeVisible();
    await page.goto('/cart');
    await expect(
      page.getByRole('heading', { name: 'Товар недоступен', exact: true }),
    ).toBeVisible();
    await expect(page.getByRole('button', { name: /Зарезервировать на 15 минут/ })).toBeDisabled();
    await expect(page.locator('.cart-total strong')).toContainText('0');
    await page.getByRole('button', { name: 'Удалить позицию', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Пока ни одной вещи' })).toBeVisible();
  } finally {
    await api.dispose();
  }
});
