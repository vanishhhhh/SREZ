import { expect, test } from '@playwright/test';
import {
  account,
  login,
  noOverflow,
  observeScriptErrors,
  qaOrigin,
  requireMVP,
  seedCatalog,
  suffix,
} from './mvp-fixtures';
test.beforeEach(({ baseURL }) => requireMVP(baseURL));

test('wishlists: create, add selected variants and catalog products, reorder, share with guest, revoke and rotate', async ({
  page,
  browser,
  playwright,
  baseURL,
}) => {
  const api = await playwright.request.newContext({ baseURL });
  const guest = await browser.newContext();
  const visitor = await guest.newPage();
  try {
    const user = await account(api, 'wish'),
      data = await seedCatalog(api, user);
    const errors = observeScriptErrors(page);
    const guestErrors = observeScriptErrors(visitor);
    const title = 'Вне сезона / ' + suffix();
    await login(page, user);
    await page.goto('/wishlists');
    await page.getByRole('link', { name: /Создать подборку/ }).click();
    await page.getByLabel('Название вишлиста', { exact: true }).fill(title);
    await page.getByLabel(/^Описание/).fill('Моя подборка независимых вещей.');
    await page.getByRole('button', { name: 'Создать вишлист', exact: true }).click();
    await expect(page).toHaveURL(/\/wishlists\/\d+$/);
    const id = Number(new URL(page.url()).pathname.split('/').at(-1));
    await expect(
      page.getByRole('dialog', { name: 'Остались несохранённые изменения' }),
    ).not.toBeVisible();
    await page.goto('/products/' + data.products[0].id);
    await page.locator('.size-options').getByRole('button', { name: 'M', exact: true }).click();
    await page.getByRole('button', { name: /в вишлист/ }).click();
    const picker = page.getByRole('dialog', { name: 'Сохранить в подборку' });
    await picker.getByLabel('Вишлист', { exact: true }).selectOption(String(id));
    await picker.getByRole('button', { name: 'Сохранить', exact: true }).click();
    await expect(picker).not.toBeVisible();
    await page.goto('/catalog?brand_id=' + data.brand.id);
    const second = page
      .locator('.product-tile')
      .filter({ has: page.getByRole('heading', { name: data.products[1].name, exact: true }) });
    await second.getByRole('button', { name: /в вишлист/ }).click();
    await picker.getByRole('button', { name: 'Сохранить', exact: true }).click();
    await expect(picker).not.toBeVisible();
    await page.goto('/wishlists/' + id);
    await expect(page.locator('[data-wishlist-item]')).toHaveCount(2);
    await expect(page.locator('[data-wishlist-item]').first()).toContainText('Black / M');
    await page
      .locator('[data-wishlist-item]')
      .last()
      .getByRole('button', { name: /Переместить .* выше/ })
      .click();
    await expect(
      page.locator('[data-wishlist-item]').first().getByRole('heading', { level: 3 }),
    ).toHaveText(data.products[1].name);
    await page
      .locator('[data-wishlist-item]')
      .first()
      .getByRole('button', { name: 'Выбрать вариант', exact: true })
      .click();
    const variant = page.getByRole('dialog', { name: 'Цвет и размер' });
    await variant.getByLabel('Вариант товара').selectOption(String(data.products[1].variant.id));
    await variant.getByRole('button', { name: 'Сохранить вариант' }).click();
    await expect(variant).not.toBeVisible();
    await page.getByLabel('Название вишлиста', { exact: true }).fill(title + ' / edit');
    await page.getByRole('button', { name: 'Сохранить описание' }).click();
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(title + ' / edit');
    await page.getByRole('button', { name: /Создать публичную ссылку/ }).click();
    const link = page.getByLabel('Публичная ссылка', { exact: true });
    await expect(link).toBeVisible();
    const old = await link.inputValue();
    await page
      .context()
      .grantPermissions(['clipboard-read', 'clipboard-write'], { origin: qaOrigin });
    await page.getByRole('button', { name: /Копировать ссылку/ }).click();
    expect((await page.evaluate(() => navigator.clipboard.readText())) === old).toBe(true);
    await visitor.goto(old).catch(() => {
      throw Error('Guest wishlist navigation failed');
    });
    await expect(visitor.getByRole('heading', { level: 1 })).toHaveText(title + ' / edit');
    await expect(visitor.locator('.product-tile')).toHaveCount(2);
    await expect(visitor.getByRole('button', { name: 'Сохранить описание' })).toHaveCount(0);
    expect(
      (
        await visitor.request.post('/api/wishlists/' + id + '/items', {
          data: { product_id: data.products[0].id },
        })
      ).status(),
    ).toBe(401);
    await expect
      .poll(() =>
        visitor
          .locator('.product-tile img')
          .evaluateAll(
            (nodes) =>
              nodes.length === 2 &&
              nodes.every(
                (node) =>
                  (node as HTMLImageElement).complete &&
                  (node as HTMLImageElement).naturalWidth > 0,
              ),
          ),
      )
      .toBe(true);
    await visitor.screenshot({ path: 'docs/previews/mvp-wishlist-desktop.png', fullPage: true });
    for (const width of [768, 1024]) {
      await visitor.setViewportSize({ width, height: 1024 });
      await noOverflow(visitor);
      await expect(visitor.locator('.product-tile')).toHaveCount(2);
    }
    await visitor.setViewportSize({ width: 390, height: 844 });
    await noOverflow(visitor);
    await visitor.screenshot({ path: 'docs/previews/mvp-wishlist-mobile.png', fullPage: true });
    await page.getByRole('button', { name: 'Закрыть доступ', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Закрыть публичный доступ?' })
      .getByRole('button', { name: 'Закрыть доступ', exact: true })
      .click();
    await expect(link).toHaveCount(0);
    await visitor.reload();
    await expect(visitor.getByRole('heading', { name: 'Эта подборка недоступна' })).toBeVisible();
    await page.getByRole('button', { name: /Создать публичную ссылку/ }).click();
    const before = await link.inputValue();
    expect(before === old).toBe(false);
    await page.getByRole('button', { name: 'Перевыпустить ссылку', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Перевыпустить ссылку?' })
      .getByRole('button', { name: 'Перевыпустить', exact: true })
      .click();
    await expect.poll(async () => (await link.inputValue()) !== before).toBe(true);
    await visitor.goto(before).catch(() => {
      throw Error('Revoked link navigation failed');
    });
    await expect(visitor.getByRole('heading', { name: 'Эта подборка недоступна' })).toBeVisible();
    const fresh = await link.inputValue();
    await visitor.goto(fresh).catch(() => {
      throw Error('Rotated link navigation failed');
    });
    await expect(visitor.locator('.product-tile')).toHaveCount(2);
    await page.reload();
    await expect.poll(() => link.inputValue().then((value) => value === fresh)).toBe(true);
    expect(errors).toEqual([]);
    expect(guestErrors).toEqual([]);
  } finally {
    await api.dispose();
    await guest.close();
  }
});

test('wishlists security: owner scoping, mass assignment, hidden catalog and invalid public capabilities', async ({
  page,
  playwright,
  baseURL,
}) => {
  const api = await playwright.request.newContext({ baseURL });
  try {
    const owner = await account(api, 'safe'),
      other = await account(api, 'other'),
      data = await seedCatalog(api, owner, 1);
    const made = await api.post('/api/wishlists', {
      headers: owner.headers,
      data: { title: 'Проверка приватности' },
    });
    expect(made.status()).toBe(201);
    const list = await made.json();
    const path = '/api/wishlists/' + list.id;
    expect(
      (await api.patch(path, { headers: other.headers, data: { title: 'Чужое' } })).status(),
    ).toBe(404);
    expect((await api.get(path, { headers: other.headers })).status()).toBe(404);
    expect((await api.post(path + '/share', { headers: other.headers, data: {} })).status()).toBe(
      404,
    );
    for (const data of [
      { title: 'Подмена', owner_user_id: 1 },
      { title: 'Подмена', visibility: 'public' },
      { title: 'Подмена', share_token_hash: 'x' },
    ])
      expect((await api.patch(path, { headers: owner.headers, data })).status()).toBe(400);
    expect(
      (
        await api.post(path + '/items', {
          headers: owner.headers,
          data: {
            product_id: data.products[0].id,
            product_variant_id: data.products[0].variant.id,
          },
        })
      ).status(),
    ).toBe(201);
    const shared = await api.post(path + '/share', { headers: owner.headers, data: {} });
    const value = await shared.json();
    const token = value.share_path.split('/').at(-1);
    const publicPath = '/api/wishlists/shared/' + token;
    const payload = await (await api.get(publicPath)).json();
    expect(Object.keys(payload).sort()).toEqual(['description', 'items', 'title', 'updated_at']);
    expect(payload.items.length).toBe(1);
    expect(
      (
        await api.post('/api/products/' + data.products[0].id + '/hide', {
          headers: owner.headers,
          data: {},
        })
      ).status(),
    ).toBe(200);
    expect((await (await api.get(publicPath)).json()).items).toEqual([]);
    await page.goto(value.share_path).catch(() => {
      throw Error('Public navigation failed');
    });
    await expect(page.getByRole('heading', { name: 'Подборка ещё собирается' })).toBeVisible();
    await expect(
      page.getByRole('heading', { name: data.products[0].name, exact: true }),
    ).toHaveCount(0);
    const own = await (await api.get(path, { headers: owner.headers })).json();
    expect(own.items[0].product).toBeNull();
    expect(
      (await api.delete(path + '/items/' + own.items[0].id, { headers: owner.headers })).status(),
    ).toBe(200);
    expect((await api.get('/api/wishlists/shared/not-valid')).status()).toBe(404);
    expect((await api.delete(path, { headers: owner.headers })).status()).toBe(204);
    expect((await api.get(publicPath)).status()).toBe(404);
  } finally {
    await api.dispose();
  }
});

test('collection forms: route guard, beforeunload protection, empty state and mobile layout', async ({
  page,
  playwright,
  baseURL,
}) => {
  const api = await playwright.request.newContext({ baseURL });
  try {
    const user = await account(api, 'draft');
    await login(page, user);
    await page.goto('/wishlists');
    await expect(
      page.getByRole('heading', { name: 'Твой стиль начинается с одной вещи' }),
    ).toBeVisible();
    await page.goto('/wishlists/new');
    await page.getByLabel('Название вишлиста', { exact: true }).fill('Несохранённая идея');
    await page.locator('header').getByRole('link', { name: 'Каталог', exact: true }).click();
    const warning = page.getByRole('dialog', { name: 'Остались несохранённые изменения' });
    await expect(warning).toBeVisible();
    await page.keyboard.press('Tab');
    expect(await warning.evaluate((dialog) => dialog.contains(document.activeElement))).toBe(true);
    await page.keyboard.press('Escape');
    await expect(warning).not.toBeVisible();
    await expect(page.getByLabel('Название вишлиста', { exact: true })).toHaveValue(
      'Несохранённая идея',
    );
    await page.locator('header').getByRole('link', { name: 'Каталог', exact: true }).click();
    await expect(warning).toBeVisible();
    await warning.getByRole('button', { name: 'Отмена', exact: true }).click();
    await expect(page.getByLabel('Название вишлиста', { exact: true })).toHaveValue(
      'Несохранённая идея',
    );
    expect(
      await page.evaluate(() => {
        const event = new Event('beforeunload', { cancelable: true });
        window.dispatchEvent(event);
        return event.defaultPrevented;
      }),
    ).toBe(true);
    await page.locator('header').getByRole('link', { name: 'Каталог', exact: true }).click();
    await warning.getByRole('button', { name: 'Продолжить без сохранения' }).click();
    await expect(page).toHaveURL(/\/catalog$/);
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto('/wishlists');
    await noOverflow(page);
    await page.goto('/wishlists/new');
    await noOverflow(page);
  } finally {
    await api.dispose();
  }
});
