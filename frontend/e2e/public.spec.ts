import { verifyFixtureAccount } from './verification-fixture';
import { uploadFixture } from './media-fixture';
import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import type { Brand, ProductDetail } from '../src/lib/types';

const live = process.env.SREZ_PUBLIC_E2E_LIVE === '1';
let brand: Brand, otherBrand: Brand, hoodie: ProductDetail;
async function anonymous(page: Page) {
  await page.route('**/api/auth/csrf', (route) =>
    route.fulfill({ status: 401, json: { error: 'invalid credentials' } }),
  );
}
async function noOverflow(page: Page) {
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1),
  ).toBe(true);
}

test.describe('public storefront against real Go and isolated PostgreSQL', () => {
  test.skip(!live, 'Set SREZ_PUBLIC_E2E_LIVE=1 only with a separate test database.');
  test.beforeAll(async ({ playwright, baseURL }) => {
    const origin = new URL(baseURL!).origin;
    const api = await playwright.request.newContext({
      baseURL,
      extraHTTPHeaders: { Origin: origin },
    });
    try {
      const existing = (await (await api.get('/api/brands')).json()) as Brand[];
      const north = existing.find((b) => b.slug.startsWith('north-'));
      const quiet = existing.find((b) => b.slug.startsWith('quiet-'));
      if (north && quiet) {
        brand = north;
        otherBrand = quiet;
        const cards = await (
          await api.get('/api/products?brand_id=' + brand.id + '&category=hoodie')
        ).json();
        hoodie = await (await api.get('/api/products/' + cards[0].id)).json();
        return;
      }
      const suffix = Date.now().toString(36);
      const username = 'public_' + suffix;
      const password = 'SREZ public integration password 123';
      const registered = await api.post('/api/auth/register', {
        data: {
          username,
          email: username + '@example.com',
          display_name: 'Public API tests',
          password,
        },
      });
      expect(registered.status()).toBe(201);
      await verifyFixtureAccount(api, username + '@example.com');
      const loggedIn = await api.post('/api/auth/login', {
        data: { identifier: username, password },
      });
      expect(loggedIn.ok()).toBe(true);
      const token = (await loggedIn.json()).access_token as string;
      const write = async (path: string, data: unknown, method = 'POST') => {
        const result = await api.fetch('/api' + path, {
          method,
          data,
          headers: { Authorization: 'Bearer ' + token },
        });
        expect(result.ok(), `fixture ${method} ${path}: ${result.status()}`).toBe(true);
        return result.status() === 204 ? null : result.json();
      };
      async function newBrand(name: string, slug: string): Promise<Brand> {
        const b = await write('/brands', { name, slug });
        await write(
          '/brands/' + b.id,
          {
            description: 'Независимый взгляд. Свободная форма и внимание к деталям.',
            contacts: {
              email: 'hello@example.com',
              website_url: 'https://example.com',
              telegram_url: 'https://t.me/example',
            },
          },
          'PATCH',
        );
        await uploadFixture(api, token, { purpose: 'brand_logo', brand_id: b.id });
        await uploadFixture(api, token, { purpose: 'brand_banner', brand_id: b.id }, 85);
        return write('/brands/' + b.id + '/publish', {});
      }
      brand = await newBrand('Северная форма', 'north-' + suffix);
      otherBrand = await newBrand('Тихая линия', 'quiet-' + suffix);
      async function newProduct(
        b: Brand,
        name: string,
        price: number,
        category: string,
        stock: number,
        extraColor = false,
      ) {
        const p = await write('/products', {
          brand_id: b.id,
          name,
          description: 'Плотный хлопок, свободный крой. Сделано для города.',
          price_kopecks: price,
          category,
          materials: ['Хлопок 100%'],
        });
        const c = await write(`/products/${p.id}/colors`, { color: 'Чёрный' });
        await uploadFixture(api, token, {
          purpose: 'product_image',
          product_id: p.id,
          color_id: c.id,
        });
        await write(`/products/${p.id}/colors/${c.id}/variants`, { size: 'M', stock });
        if (extraColor) {
          await uploadFixture(
            api,
            token,
            { purpose: 'product_image', product_id: p.id, color_id: c.id },
            75,
          );
          await write(`/products/${p.id}/colors/${c.id}/variants`, { size: 'L', stock: 0 });
          const white = await write(`/products/${p.id}/colors`, { color: 'Белый' });
          await uploadFixture(
            api,
            token,
            { purpose: 'product_image', product_id: p.id, color_id: white.id },
            175,
          );
          await write(`/products/${p.id}/colors/${white.id}/variants`, { size: 'S', stock: 3 });
          await write(`/products/${p.id}/colors/${white.id}/variants`, { size: 'M', stock: 0 });
        }
        return write(`/products/${p.id}/publish`, {});
      }
      hoodie = await newProduct(brand, 'Худи Контур', 200001, 'hoodie', 7, true);
      await newProduct(brand, 'Футболка Линия', 100000, 'tshirt', 4);
      for (let i = 1; i <= 11; i++)
        await newProduct(brand, 'Аксессуар ' + i, 500000 + i, 'accessories', 1);
      await newProduct(otherBrand, 'Худи Тишина', 900000, 'hoodie', 0);
    } finally {
      await api.dispose();
    }
  });
  test('home, real collections, categories, brands and 16:10 layout', async ({ page, request }) => {
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    page.on('console', (msg) => {
      if (msg.type() === 'error' && !msg.text().includes('status of 401')) errors.push(msg.text());
    });
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto('/');
    await page.screenshot({ path: 'test-results/public-home-16-10.jpg' });
    await expect(page.getByRole('heading', { level: 1, name: 'SREZ' })).toBeVisible();
    await expect(page.locator('.home-cover img')).toHaveAttribute('fetchpriority', 'high');
    await expect
      .poll(() =>
        page.locator('.home-cover img').evaluate((image: HTMLImageElement) => image.naturalWidth),
      )
      .toBeGreaterThan(0);
    const storefront = page.getByRole('navigation', { name: 'Разделы витрины' });
    await expect(storefront.getByRole('link', { name: 'Каталог' })).toHaveAttribute(
      'href',
      '/catalog',
    );
    await expect(storefront.getByRole('link', { name: 'Бренды' })).toHaveAttribute(
      'href',
      '/brands',
    );
    await expect(page.locator('#latest .product-tile')).toHaveCount(4);
    await expect(page.getByRole('main').getByRole('heading', { level: 2 })).toHaveText([
      'Новые поступления',
      'Бренды',
    ]);
    const brandCards = page.getByRole('link').and(page.locator('.brand-tile'));
    await expect(brandCards.first()).toHaveAttribute('href', /^\/brands\//);
    const footer = page.getByRole('contentinfo');
    await expect(footer.getByRole('link')).toHaveCount(1);
    await footer.getByRole('link', { name: 'Наверх' }).click();
    await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(0);
    await page
      .locator('.category-links')
      .getByRole('link', { name: 'Футболки', exact: true })
      .click();
    await expect(page).toHaveURL(/category=tshirt/);
    const categoryResponse = await request.get(
      '/api/products?category=tshirt&sort=newest&limit=12',
    );
    expect(categoryResponse.status()).toBe(200);
    const actualCategory = await categoryResponse.json();
    expect(
      actualCategory.length > 0 &&
        actualCategory.every((p: { category: string }) => p.category === 'tshirt'),
    ).toBe(true);
    await expect(page.locator('.product-tile h3')).toHaveText(
      actualCategory.map((p: { name: string }) => p.name),
    );
    await page
      .getByRole('navigation', { name: 'Основная навигация' })
      .getByRole('link', { name: 'Бренды', exact: true })
      .click();
    await expect(page.locator('.brand-directory__row').first()).toBeVisible();
    await noOverflow(page);
    expect(errors).toEqual([]);
  });
  test('all filters, exact kopecks, sorting, URL history and reset', async ({ page }) => {
    await page.goto('/catalog');
    await expect(page.locator('.product-tile')).toHaveCount(12);
    await page.getByLabel('Категория', { exact: true }).selectOption('hoodie');
    await page.getByLabel('Бренд', { exact: true }).selectOption(String(brand.id));
    await page.getByLabel('Размер', { exact: false }).fill('M');
    await page.getByLabel('Цвет', { exact: false }).fill('Чёрный');
    await page.getByLabel('Цена от').fill('2000,01');
    await page.getByLabel('Цена до').fill('3000');
    await page.getByRole('button', { name: 'Показать вещи' }).click();
    await expect(page.locator('.product-tile h3')).toHaveText(['Худи Контур']);
    expect(new URL(page.url()).searchParams.get('min_price')).toBe('200001');
    await expect(page.locator('.product-tile__meta strong')).toContainText('2 000,01');
    await page.getByLabel('Сортировка').selectOption('price_asc');
    await expect(page).toHaveURL(/sort=price_asc/);
    await page.goBack();
    await expect(page.getByLabel('Сортировка')).toHaveValue('newest');
    await page.getByRole('button', { name: 'Сбросить', exact: true }).click();
    await expect(page.getByLabel('Цена от')).toHaveValue('');
    await page.getByLabel('Цена от').fill('100');
    await page.getByRole('button', { name: 'Сбросить', exact: true }).click();
    await expect(page.getByLabel('Цена от')).toHaveValue('');
  });
  test('pagination reads HTTP headers, sorting is global, invalid and out-of-range URLs recover', async ({
    page,
  }) => {
    const query = `/catalog?brand_id=${brand.id}&limit=1&sort=price_asc`;
    const response = page.waitForResponse(
      (r) => r.url().includes('/api/products?') && r.status() === 200,
    );
    await page.goto(query);
    expect((await response).headers()['x-total-count']).toBe('13');
    await expect(page.locator('.product-tile h3').first()).toHaveText('Футболка Линия');
    await page.getByRole('button', { name: 'Далее', exact: true }).click();
    await expect(page.locator('.product-tile h3')).toHaveText(['Худи Контур']);
    await expect(page).toHaveURL(/page=2/);
    await page.goto(query + '&page=999');
    await expect(page.getByRole('heading', { name: 'На этой странице нет вещей' })).toBeVisible();
    await page.getByRole('button', { name: 'Назад', exact: true }).click();
    await expect(page).toHaveURL(/page=13/);
    await expect(page.locator('.product-tile')).toHaveCount(1);
    await page.goto('/catalog?page=0');
    await expect(page.getByRole('button', { name: 'Сбросить параметры запроса' })).toBeVisible();
    await page.getByRole('button', { name: 'Сбросить параметры запроса' }).click();
    await expect(page.locator('.product-tile')).toHaveCount(12);
  });
  test('gallery and color-specific sizes and stock, brand failure never hides the product', async ({
    page,
  }) => {
    await page.goto('/products/' + hoodie.id);
    await expect(page.getByRole('heading', { name: 'Худи Контур', exact: true })).toBeVisible();
    const sizes = page.getByRole('group', { name: 'Размер', exact: true });
    await expect(sizes.getByRole('button', { name: 'L', exact: true })).toBeDisabled();
    await sizes.getByRole('button', { name: 'M', exact: true }).click();
    await expect(page.locator('.availability')).toHaveText('В наличии: 7 шт.');
    await page.screenshot({ path: 'test-results/public-product-desktop.jpg', fullPage: true });
    await page.getByRole('button', { name: 'Следующее фото', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Фото 2', exact: true })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    await page.getByRole('button', { name: 'Белый', exact: true }).click();
    await expect(sizes.getByRole('button', { name: 'M', exact: true })).toBeDisabled();
    await expect(sizes.getByRole('button', { name: 'S', exact: true })).toBeEnabled();
    await expect(page.locator('.availability')).toContainText('выберите размер');
    await expect(page.locator('.gallery-caption')).toContainText('1 / 1');
    await sizes.getByRole('button', { name: 'S', exact: true }).click();
    await expect(page.locator('.availability')).toHaveText('В наличии: 3 шт.');
    await page.route('**/api/brands', (route) =>
      route.fulfill({ status: 503, json: { error: 'unavailable' } }),
    );
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Худи Контур', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Загрузить бренд' })).toBeVisible();
  });
  test('brand detail has logo, banner, public contacts and paginated real products', async ({
    page,
  }) => {
    await page.goto('/brands/' + brand.slug);
    await expect(page.getByRole('heading', { name: brand.name, exact: true })).toBeVisible();
    await expect(
      page.locator('.header').getByRole('img', { name: brand.name, exact: true }),
    ).toBeVisible();
    await expect(page.locator('[data-brand-block="story"] .brand-story-logo')).toHaveCount(0);
    await expect(page.locator('.brand-hero__banner')).toHaveAttribute(
      'data-media-src',
      /\/media\/assets\/\d+\/content/,
    );
    await expect(page.getByRole('link', { name: 'Сайт бренда', exact: false })).toHaveAttribute(
      'href',
      'https://example.com/',
    );
    await expect(page.locator('.product-tile')).toHaveCount(12);
    await page.getByRole('button', { name: 'Далее', exact: true }).click();
    await expect(page.locator('.product-tile')).toHaveCount(1);
    await page.screenshot({ path: 'test-results/public-brand-desktop.jpg', fullPage: true });
    await page.goto('/catalog?brand_id=' + otherBrand.id);
    await expect(page.locator('.product-tile__sold')).toHaveText('Нет в наличии');
    await page.locator('.product-tile').click();
    await expect(page.locator('.availability')).toHaveText('Этот цвет закончился');
    await expect(
      page
        .getByRole('group', { name: 'Размер', exact: true })
        .getByRole('button', { name: 'M', exact: true }),
    ).toBeDisabled();
    await page.goto('/catalog?brand_id=' + brand.id + '&color=Белый&size=M');
    await expect(page.getByRole('heading', { name: 'Пока ничего не нашлось' })).toBeVisible();
  });
  test('mobile navigation, filters and all public pages fit the viewport', async ({
    page,
    request,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    for (const path of [
      '/',
      '/catalog',
      '/brands',
      '/brands/' + brand.slug,
      '/products/' + hoodie.id,
    ]) {
      await page.goto(path);
      await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
      await expect(page.locator('.skeleton[role="status"]')).toHaveCount(0);
      await noOverflow(page);
    }
    await page.goto('/catalog');
    const filters = page.getByRole('button', { name: /^Фильтры/ });
    const category = page.getByLabel('Категория', { exact: true });
    await filters.focus();
    await filters.press('Enter');
    await expect(category).toBeFocused();
    await page.keyboard.press('Escape');
    await expect(filters).toBeFocused();
    await expect(category).toBeHidden();
    await filters.press('Enter');
    await category.selectOption('tshirt');
    await page.getByRole('button', { name: 'Показать вещи' }).click();
    await expect(filters).toBeFocused();
    await expect(category).toBeHidden();
    const categoryResponse = await request.get(
      '/api/products?category=tshirt&sort=newest&limit=12',
    );
    expect(categoryResponse.status()).toBe(200);
    const actualCategory = await categoryResponse.json();
    expect(
      actualCategory.length > 0 &&
        actualCategory.every((p: { category: string }) => p.category === 'tshirt'),
    ).toBe(true);
    await expect(page.locator('.product-tile h3')).toHaveText(
      actualCategory.map((p: { name: string }) => p.name),
    );
    const menu = page.getByRole('button', { name: 'Открыть меню' });
    const navigation = page.getByRole('navigation', { name: 'Основная навигация' });
    await menu.focus();
    await menu.press('Enter');
    await expect(navigation.getByRole('link', { name: 'Каталог' })).toBeFocused();
    await page.keyboard.press('Escape');
    await expect(menu).toBeFocused();
    await expect(navigation).toBeHidden();
    await menu.click();
    await navigation.getByRole('link', { name: 'Бренды', exact: true }).click();
    await expect(page).toHaveURL(/\/brands$/);
    await page.screenshot({ path: 'test-results/public-mobile.jpg', fullPage: true });
    for (const width of [768, 1920]) {
      await page.setViewportSize({ width, height: Math.round((width * 10) / 16) });
      await page.goto('/products/' + hoodie.id);
      await expect(page.getByRole('heading', { name: 'Худи Контур', exact: true })).toBeVisible();
      await noOverflow(page);
    }
  });
  test('missing resources and React routes give useful navigation', async ({ page }) => {
    for (const [path, heading] of [
      ['/products/9007199254740000', 'Товар не найден'],
      ['/brands/missing-public-brand', 'Бренд не найден'],
      ['/no-such-page', 'Здесь пока ничего нет.'],
    ]) {
      await page.goto(path);
      await expect(page.getByRole('heading', { name: heading, exact: true })).toBeVisible();
      await expect(page.getByRole('banner')).toBeVisible();
      await expect(page.getByRole('contentinfo')).toBeVisible();
    }
  });
});

// Controlled failures test UI resilience; they are never shipped as catalog data.
test.describe('public loading, media and failure states', () => {
  test.beforeEach(async ({ page }) => {
    await anonymous(page);
  });
  test('skeleton, empty, network retry, invalid success payload', async ({ page }) => {
    await page.route('**/api/brands', (route) => route.fulfill({ json: [] }));
    let mode = 'loading';
    let release!: () => void;
    const blocked = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route('**/api/products?**', async (route) => {
      if (mode === 'loading') await blocked;
      if (mode === 'network') return route.abort();
      if (mode === 'invalid') return route.fulfill({ json: { products: [] } });
      return route.fulfill({
        json: [],
        headers: { 'X-Total-Count': '0', 'X-Page': '1', 'X-Limit': '12', 'X-Total-Pages': '0' },
      });
    });
    await page.goto('/catalog');
    await expect(page.getByRole('status', { name: 'Загружаем товары' })).toBeVisible();
    mode = 'empty';
    release();
    await expect(page.getByRole('heading', { name: 'Пока ничего не нашлось' })).toBeVisible();
    mode = 'network';
    await page.reload();
    await expect(
      page.getByRole('button', { name: 'Попробовать снова', exact: true }),
    ).toBeVisible();
    mode = 'empty';
    await page.getByRole('button', { name: 'Попробовать снова', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Пока ничего не нашлось' })).toBeVisible();
    mode = 'invalid';
    await page.reload();
    await expect(
      page.getByRole('button', { name: 'Попробовать снова', exact: true }),
    ).toBeVisible();
  });
  test('managed media uses /media, failed photo has a fallback and changing photo recovers', async ({
    page,
  }) => {
    await page.route('**/api/brands', (route) => route.fulfill({ json: [] }));
    await page.route('**/api/products/42', (route) =>
      route.fulfill({
        json: {
          id: 42,
          brand_id: 1,
          name: 'Проверка Media',
          description: '',
          materials: [],
          category: 'hoodie',
          price_kopecks: 100000,
          colors: [
            {
              id: 1,
              color: 'Чёрный',
              images: ['/media/assets/public-image/content', '/media/assets/broken/content'],
              variants: [{ id: 1, size: 'M', stock: 1 }],
            },
          ],
        },
      }),
    );
    await page.route('**/media/assets/public-image/content', (route) =>
      route.fulfill({
        contentType: 'image/svg+xml',
        body: '<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"><rect width="100" height="100" fill="#333"/></svg>',
      }),
    );
    await page.route('**/media/assets/broken/content', (route) => route.fulfill({ status: 404 }));
    await page.goto('/products/42');
    await expect(page.locator('.product-gallery > .media')).toHaveAttribute(
      'data-media-src',
      '/media/assets/public-image/content',
    );
    await page.getByRole('button', { name: 'Фото 2', exact: true }).click();
    await expect(page.locator('.product-gallery > .media .media__fallback')).toContainText(
      'Фото недоступно',
    );
    await page.getByRole('button', { name: 'Фото 1', exact: true }).click();
    await expect(page.locator('.product-gallery > .media img')).toBeVisible();
  });
});
