import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { defaultBrandPage } from '../src/lib/brand-page';
import type { Brand, ProductDetail } from '../src/lib/types';

// Controlled browser fixtures: these tests never write to a backend or upload media.
function brand(id: number, name: string, logo = id): Brand {
  return {
    id,
    name,
    slug: `brand-${id}`,
    description: 'Независимая одежда. Простая форма, внимание к деталям.',
    logo_url: logo ? `/media/assets/${logo}/content` : '',
    banner_url: '/media/assets/90/content',
    contacts: { email: '', telegram_url: '', website_url: '' },
    appearance: defaultBrandPage().appearance,
    page: defaultBrandPage(),
    status: 'active',
    created_at: '2026-11-24T00:00:00Z',
    updated_at: '2026-11-24T00:00:00Z',
  };
}
const brands = [
  brand(1, 'КОНТУР'),
  brand(2, 'ПОЛЮС'),
  brand(3, 'СМЕНА'),
  brand(4, 'Повреждённый логотип'),
  brand(5, 'Бренд с длинным названием без логотипа', 0),
  brand(6, 'Недоступный логотип'),
];
const product: ProductDetail = {
  id: 42,
  brand_id: 1,
  name: 'Куртка «Ритм»',
  description: 'Свободный силуэт на каждый день.',
  category: 'hoodie',
  materials: ['Хлопок'],
  price_kopecks: 850000,
  status: 'published',
  created_at: '2026-11-24T00:00:00Z',
  updated_at: '2026-11-24T00:00:00Z',
  colors: [
    {
      id: 1,
      color: 'Графит',
      images: ['/media/assets/92/content'],
      variants: [
        { id: 1, size: 'S', stock: 1, sku: 'rhythm-s' },
        { id: 2, size: 'M', stock: 0, sku: 'rhythm-m' },
      ],
    },
  ],
};

async function fixtures(page: Page, authenticated = false) {
  await page.route('**/api/**', async (route) => {
    const { pathname, searchParams } = new URL(route.request().url());
    let json: unknown;
    if (pathname === '/api/auth/csrf') {
      if (!authenticated)
        return route.fulfill({ status: 401, json: { error: 'invalid credentials' } });
      json = { csrf_token: 'test-only-csrf' };
    } else if (pathname === '/api/auth/refresh') {
      json = { access_token: 'test-only-access', csrf_token: 'test-only-csrf' };
    } else if (pathname === '/api/profile') {
      json = {
        id: 1,
        username: 'branding_owner',
        display_name: 'Владелец',
        avatar_url: '',
        email: 'owner@example.com',
        email_verified_at: '2026-11-24T00:00:00Z',
        email_verification_required: true,
      };
    } else if (pathname === '/api/brands') json = brands;
    else if (pathname === '/api/brands/1/products/manage') json = [product];
    else if (pathname === '/api/products/42/colors')
      json = [{ id: 1, product_id: 42, color: 'Графит', images: product.colors[0].images }];
    else if (pathname === '/api/products/42/colors/1/variants')
      json = product.colors[0].variants.map((variant) => ({ ...variant, product_color_id: 1 }));
    else if (pathname === '/api/products/42/colors/1/images')
      json = [92, 94].map((id, position) => ({
        id,
        media_id: id,
        product_color_id: 1,
        position,
        url: `/media/assets/${id}/content`,
      }));
    else if (pathname === '/api/me/brands') json = [{ brand: brands[0], role: 'owner' }];
    else if (pathname === '/api/brands/1/manage') json = brands[0];
    else if (pathname === '/api/brands/1/page/manage') {
      json = {
        draft: defaultBrandPage(),
        published: defaultBrandPage(),
        revision: 1,
        published_revision: 1,
        media_ids: [91],
      };
    } else if (pathname === '/api/cart') json = { items: [], total_kopecks: 0 };
    else if (['/api/favorites', '/api/me/brand-invitations'].includes(pathname)) json = [];
    else if (pathname === '/api/products/42') json = product;
    else if (pathname === '/api/products') {
      return route.fulfill({
        json: [
          {
            ...product,
            brand_name: brands[0].name,
            brand_slug: brands[0].slug,
            preview_image_url: '/media/assets/92/content',
            is_available: true,
            available_colors: ['Графит'],
            available_sizes: ['S'],
            colors: [],
          },
        ],
        headers: {
          'X-Total-Count': '1',
          'X-Page': '1',
          'X-Limit': searchParams.get('limit') || '12',
          'X-Total-Pages': '1',
        },
      });
    } else {
      const match = brands.find((b) => pathname === '/api/brands/' + b.slug);
      if (!match) return route.fulfill({ status: 404, json: { error: 'resource not found' } });
      json = match;
    }
    return route.fulfill({ json });
  });
  await page.route('**/media/assets/*/content*', (route) => {
    const id = Number(new URL(route.request().url()).pathname.match(/\/assets\/(\d+)\//)?.[1]);
    if (id === 6) return route.fulfill({ status: 404 });
    if (id === 4) return route.fulfill({ contentType: 'image/png', body: 'broken image' });
    const [width, height] = id === 1 ? [600, 100] : id === 3 ? [80, 400] : [300, 300];
    return route.fulfill({
      contentType: 'image/svg+xml',
      body: `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}"><rect width="100%" height="100%" fill="${id >= 90 ? '#bebcaf' : '#171716'}"/><text x="50%" y="55%" dominant-baseline="middle" text-anchor="middle" fill="#f6f5f1" font-size="${id === 1 ? 74 : 32}">${brands.find((b) => b.id === id)?.name || 'SREZ'}</text></svg>`,
    });
  });
}

async function logoFits(page: Page, name: string) {
  const logo = page.locator('.header').getByRole('img', { name, exact: true });
  await expect(logo).toBeVisible();
  await expect(logo).toHaveJSProperty('complete', true);
  const bounds = await logo.evaluate((image: HTMLImageElement) => {
    const rect = image.getBoundingClientRect();
    return {
      width: rect.width,
      height: rect.height,
      ratio: image.naturalWidth / image.naturalHeight,
      mobile: innerWidth <= 620,
    };
  });
  expect(bounds.width).toBeGreaterThan(0);
  expect(bounds.width).toBeLessThanOrEqual(bounds.mobile ? 120 : 160);
  expect(bounds.height).toBeLessThanOrEqual(bounds.mobile ? 36 : 44);
  expect(bounds.width / bounds.height).toBeCloseTo(bounds.ratio, 1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
}

for (const width of [1440, 390]) {
  test(`brand logos, fallbacks and button layouts at ${width}px`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 960 });
    await fixtures(page);
    for (const current of brands) {
      await page.goto('/brands/' + current.slug);
      const link = page.locator('.header').getByRole('link', { name: `${current.name} — главная` });
      await expect(link).toHaveAttribute('href', '/');
      if (current.id <= 3) {
        await logoFits(page, current.name);
        if (current.id === 1)
          await page.screenshot({
            path: testInfo.outputPath(`brand-${width}.png`),
            fullPage: true,
          });
      } else {
        await expect(link).toHaveText(current.name);
        await expect(link.locator('img')).toHaveCount(0);
      }
      await expect(
        page.locator('.brand-page-content').getByRole('img', { name: current.name }),
      ).toHaveCount(0);
      await expect(page.getByRole('heading', { name: 'О бренде', exact: true })).toBeVisible();
    }
    await page.goto('/products/42');
    await expect(page.locator('.header .wordmark')).toHaveText('SREZ®');
    await expect(page.getByRole('button', { name: 'M', exact: true })).toBeDisabled();
    await page.getByRole('button', { name: 'S', exact: true }).click();
    await expect(page.getByRole('button', { name: 'S', exact: true })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    await expect(
      page.locator('.header').getByRole('link', { name: 'Войти', exact: true }),
    ).toBeVisible();
    await page.evaluate(() => scrollTo(0, 0));
    await page.screenshot({ path: testInfo.outputPath(`product-${width}.png`), fullPage: true });
    await page.goto('/catalog?category=hoodie');
    await expect(page.getByRole('heading', { name: 'Каталог', exact: true })).toBeVisible();
    await page.screenshot({ path: testInfo.outputPath(`catalog-${width}.png`), fullPage: true });
    await fixtures(page, true);
    await page.goto('/studio/brands/1/products/42');
    await page.getByRole('button', { name: 'Заменить фото 2', exact: true }).click();
    await expect(
      page.getByLabel('Заменить фото 2', { exact: true }).and(page.locator('input[type=file]')),
    ).toBeVisible();
    await page
      .locator('#colors')
      .screenshot({ path: testInfo.outputPath(`seller-controls-${width}.png`) });
  });
}

test('client navigation clears previous branding before a different brand loads', async ({
  page,
}) => {
  await fixtures(page, true);
  await page.goto('/brands');
  await page.locator('a.brand-directory__row[href="/brands/brand-1"]').click();
  await logoFits(page, brands[0].name);
  await page.getByRole('link', { name: 'Все бренды', exact: true }).click();
  await expect(page.locator('.header .wordmark')).toBeVisible();
  await page.locator('a.brand-directory__row[href="/brands/brand-2"]').click();
  await logoFits(page, brands[1].name);
  await page.evaluate(() => history.go(-2));
  await logoFits(page, brands[0].name);

  let release!: () => void;
  const waiting = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route('**/api/brands/brand-2', async (route) => {
    await waiting;
    await route.fulfill({ json: brands[1] });
  });
  try {
    await page.evaluate(() => history.go(2));
    await expect(page).toHaveURL(/\/brands\/brand-2$/);
    await expect(page.locator('.header .brand-wordmark')).toHaveCount(0);
    await expect(page.locator('.header .wordmark')).toBeVisible();
  } finally {
    release();
  }
  await logoFits(page, brands[1].name);
  await page.locator('a[href="/products/42"]').first().click();
  await expect(page.locator('.header .wordmark')).toBeVisible();
  await page
    .getByRole('navigation', { name: 'Путь к товару' })
    .getByRole('link', { name: 'КОНТУР' })
    .click();
  await logoFits(page, brands[0].name);
  for (const name of ['Каталог', 'Мой профиль', 'Студия бренда']) {
    await page.locator('.header').getByRole('link', { name, exact: true }).click();
    await expect(page.locator('.header .wordmark')).toBeVisible();
    await page.goBack();
    await logoFits(page, brands[0].name);
  }
  await page.locator('.brand-wordmark').click();
  await expect(page).toHaveURL(/\/$/);
  await expect(page.locator('.header .wordmark')).toBeVisible();
});

for (const version of [1, 2, 3] as const) {
  test(`saved V${version} pages keep their content without a standalone logo`, async ({ page }) => {
    await fixtures(page);
    const config = defaultBrandPage();
    config.schema_version = version;
    if (version >= 2)
      config.blocks.splice(2, 0, {
        id: 'editorial',
        type: 'image',
        layout: 'contained',
        content: {
          source: 'editorial',
          images: [{ media_id: 91, alt: 'Редакционная фотография' }],
        },
      });
    if (version === 3) config.blocks[1].content.images = [{ media_id: 93, alt: 'Фото истории' }];
    await page.route('**/api/brands/brand-1', (route) =>
      route.fulfill({ json: { ...brands[0], page: config } }),
    );
    await page.goto('/brands/brand-1');
    await logoFits(page, brands[0].name);
    await expect(page.locator('[data-brand-block="story"]')).toContainText(brands[0].description);
    await expect(page.locator('.brand-page-content .brand-wordmark')).toHaveCount(0);
    if (version >= 2) {
      const photo = page.getByRole('img', { name: 'Редакционная фотография', exact: true });
      await page.locator('[data-block-id="editorial"]').scrollIntoViewIfNeeded();
      await expect(photo).toBeVisible();
    }
    if (version === 3) {
      const photo = page.getByRole('img', { name: 'Фото истории', exact: true });
      await page.locator('[data-brand-block="story"]').scrollIntoViewIfNeeded();
      await expect(photo).toBeVisible();
    }
    const tile = page.locator('a[href="/products/42"]').first();
    await tile.scrollIntoViewIfNeeded();
    await expect(tile).toBeVisible();
  });
}

test('Studio keeps SREZ while preview has one managed brand logo and no logo placement control', async ({
  page,
}, testInfo) => {
  await fixtures(page, true);
  const logoRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/media/assets/1/content') && !!request.headers().authorization,
  );
  await page.goto('/studio/brands/1?tab=page');
  const frame = page.frameLocator('iframe[title="Предпросмотр витрины"]');
  await expect(
    frame.locator('header').getByRole('img', { name: 'КОНТУР', exact: true }),
  ).toBeVisible();
  await logoRequest;
  await expect(page.locator('.header .wordmark')).toBeVisible();
  await expect(page.locator('.header .brand-wordmark')).toHaveCount(0);
  await expect(frame.locator('main').getByRole('img', { name: /КОНТУР/ })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Использовать логотип', exact: true })).toHaveCount(
    0,
  );
  await page.screenshot({ path: testInfo.outputPath('studio-preview.png'), fullPage: true });
  await page.getByRole('button', { name: 'Телефон', exact: true }).click();
  await expect(frame.locator('header .brand-wordmark')).toHaveCSS('width', '120px');
  await frame.locator('.brand-wordmark').click();
  await expect(page).toHaveURL(/\/studio\/brands\/1\?tab=page$/);
  await page.getByRole('tab', { name: 'Настройки бренда', exact: true }).click();
  await expect(page.getByLabel('Загрузить логотип', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Удалить логотип', exact: true })).toBeVisible();
});
