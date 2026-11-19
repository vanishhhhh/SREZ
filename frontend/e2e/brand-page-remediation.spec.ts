import { expect, test } from '@playwright/test';
import type { Locator, Page } from '@playwright/test';
import { account, login, requireMVP, seedCatalog } from './mvp-fixtures';
import { fixturePNG, uploadFixture } from './media-fixture';

const submit = { name: 'Сохранить черновик', exact: true };
const publish = { name: 'Опубликовать оформление', exact: true };
const uploads = '**/api/media/uploads/*/content';
const frame = (page: Page) => page.frameLocator('iframe[title="Предпросмотр витрины"]');

function gate() {
  let release!: () => void;
  const waiting = new Promise<void>((resolve) => {
    release = resolve;
  });
  return { waiting, release };
}

async function choosePhoto(story: Locator, tone: number) {
  await story.locator('input[type=file]').setInputFiles({
    name: 'audit-photo.png',
    mimeType: 'image/png',
    buffer: fixturePNG(tone),
  });
  await story.getByRole('button', { name: 'Загрузить фотографию' }).click();
}

test('pending uploads gate writes and late save responses preserve local edits', async ({
  page,
  playwright,
  baseURL,
}) => {
  requireMVP(baseURL);
  const api = await playwright.request.newContext({ baseURL });
  const unblock: (() => void)[] = [];
  try {
    const owner = await account(api, 'upload');
    const { brand } = await seedCatalog(api, owner, 1);
    const { brand: next } = await seedCatalog(api, owner, 1);
    const path = `/api/brands/${brand.id}/page`;
    await login(page, owner);
    await page.goto(`/studio/brands/${brand.id}?tab=page`);
    const story = page.locator('[data-editor-block="story"]');
    await story.getByRole('button', { name: /Настроить блок/ }).click();
    const started = gate(),
      deliver = gate();
    unblock.push(deliver.release);
    await page.route(
      uploads,
      async (route) => {
        const response = await route.fetch();
        started.release();
        await deliver.waiting;
        await route.fulfill({ response }).catch(() => {});
      },
      { times: 1 },
    );

    await choosePhoto(story, 50);
    await started.waiting;
    await expect(page.getByRole('button', submit)).toBeDisabled();
    await expect(
      page.getByText('Дождитесь завершения загрузки фотографий', { exact: false }),
    ).toBeVisible();
    deliver.release();
    const selected = story.getByLabel(/Фото 1 из библиотеки/);
    await expect(selected).toHaveValue(/^[1-9]\d*$/);
    const imageID = await selected.inputValue();

    const saved = gate(),
      deliverSave = gate();
    unblock.push(deliverSave.release);
    await page.route(
      '**' + path + '/draft',
      async (route) => {
        const response = await route.fetch();
        saved.release();
        await deliverSave.waiting;
        await route.fulfill({ response });
      },
      { times: 1 },
    );
    await page.getByRole('button', submit).click();
    await saved.waiting;
    await page.getByRole('textbox', { name: 'Фон', exact: true }).fill('#abcdef');
    deliverSave.release();
    await expect(page.getByText('Черновик сохранён', { exact: true })).toBeVisible();
    await expect(page.getByRole('textbox', { name: 'Фон', exact: true })).toHaveValue('#abcdef');
    await expect(selected).toHaveValue(imageID);
    await expect(
      page.getByText('Есть несохранённые правки.', { exact: false }).first(),
    ).toBeVisible();
    await page.getByRole('button', submit).click();
    await expect(page.getByRole('button', publish)).toBeEnabled();

    const publishBlocked = gate(),
      releaseCancel = gate();
    unblock.push(releaseCancel.release);
    await page.route(
      uploads,
      async (route) => {
        publishBlocked.release();
        await releaseCancel.waiting;
        await route.abort().catch(() => {});
      },
      { times: 1 },
    );
    await choosePhoto(story, 55);
    await publishBlocked.waiting;
    await expect(page.getByRole('button', publish)).toBeDisabled();
    await story.getByRole('button', { name: 'Отменить загрузку' }).click();
    releaseCancel.release();
    await expect(page.getByRole('button', publish)).toBeEnabled();
    await expect(selected).toHaveValue(imageID);

    await page.route(
      uploads,
      (route) =>
        route.fulfill({
          status: 503,
          json: { error: 'media unavailable' },
        }),
      { times: 1 },
    );
    await choosePhoto(story, 60);
    await expect(story.getByText('Сервер недоступен', { exact: false })).toBeVisible();
    await expect(page.getByRole('button', publish)).toBeEnabled();

    await choosePhoto(story, 70);
    await expect(selected).not.toHaveValue(imageID);
    await expect(page.getByRole('button', submit)).toBeEnabled();
    const changed = await selected.inputValue();
    await page.getByRole('button', submit).click();
    await expect(page.getByRole('button', publish)).toBeEnabled();
    const state = await (await api.get(path + '/manage', { headers: owner.headers })).json();
    expect(
      String(
        state.draft.blocks.find((b: { type: string }) => b.type === 'story').content.images[0]
          .media_id,
      ),
    ).toBe(changed);

    const beforeSwitch = gate(),
      afterSwitch = gate();
    unblock.push(afterSwitch.release);
    await page.route(
      uploads,
      async (route) => {
        const response = await route.fetch();
        beforeSwitch.release();
        await afterSwitch.waiting;
        await route.fulfill({ response }).catch(() => {});
      },
      { times: 1 },
    );
    await choosePhoto(story, 80);
    await beforeSwitch.waiting;
    await page.getByLabel('Активный бренд').selectOption(String(next.id));
    await page.getByRole('button', { name: 'Продолжить без сохранения' }).click();
    await expect(page).toHaveURL(new RegExp('/studio/brands/' + next.id + '$'));
    afterSwitch.release();
    await page.getByRole('tab', { name: 'Оформление страницы' }).click();
    await expect(frame(page).locator('[data-brand-block="story"] .editorial-photo')).toHaveCount(0);
  } finally {
    unblock.forEach((release) => release());
    await api.dispose();
  }
});

test('save and publish reconcile committed snapshots after response loss', async ({
  page,
  playwright,
  baseURL,
}) => {
  requireMVP(baseURL);
  const api = await playwright.request.newContext({ baseURL });
  try {
    const owner = await account(api, 'recover');
    const { brand } = await seedCatalog(api, owner, 1);
    const endpoint = `/api/brands/${brand.id}/page`;
    await login(page, owner);
    await page.goto(`/studio/brands/${brand.id}?tab=page`);
    await page.getByRole('textbox', { name: 'Фон', exact: true }).fill('#def');
    for (const [operation, button, status] of [
      ['draft', submit, 'Черновик сохранён'],
      ['publish', publish, 'Оформление опубликовано'],
    ] as const) {
      await page.route(
        '**' + endpoint + '/' + operation,
        async (route) => {
          const response = await route.fetch();
          expect(response.status()).toBe(200);
          await route.abort('failed');
        },
        { times: 1 },
      );
      await page.getByRole('button', button).click();
      await expect(page.getByText(status, { exact: true })).toBeVisible();
      await expect(page.getByRole('button', { name: 'Загрузить актуальный черновик' })).toHaveCount(
        0,
      );
    }
    const state = await (await api.get(endpoint + '/manage', { headers: owner.headers })).json();
    expect(state.published_revision).toBe(state.revision);
    expect(state.published.appearance.background_color).toBe('#def');
    const pub = await (await api.get('/api/brands/' + brand.slug)).json();
    expect(pub.page.appearance.background_color).toBe('#def');
    await api.patch('/api/brands/' + brand.id, {
      headers: owner.headers,
      data: { description: 'Живое описание после публикации' },
    });
    await page.reload();
    await expect(frame(page).getByText('Живое описание после публикации')).toBeVisible();
    expect((await (await api.get('/api/brands/' + brand.slug)).json()).description).toBe(
      'Живое описание после публикации',
    );
    expect(
      (await (await api.get(endpoint + '/manage', { headers: owner.headers })).json())
        .published_revision,
    ).toBe(state.published_revision);
  } finally {
    await api.dispose();
  }
});

test('preview reproduces published geometry and transfers focus on a phone', async ({
  page,
  browser,
  playwright,
  baseURL,
}, info) => {
  requireMVP(baseURL);
  const api = await playwright.request.newContext({ baseURL });
  const visitor = await browser.newPage({ baseURL, viewport: { width: 1440, height: 900 } });
  try {
    const owner = await account(api, 'preview');
    const { brand } = await seedCatalog(api, owner, 4);
    await uploadFixture(api, owner.token, { purpose: 'brand_banner', brand_id: brand.id });
    const photo = await uploadFixture(api, owner.token, {
      purpose: 'brand_editorial_image',
      brand_id: brand.id,
    });
    const endpoint = `/api/brands/${brand.id}/page`;
    const state = await (await api.get(endpoint + '/manage', { headers: owner.headers })).json();
    const images = Array.from({ length: 3 }, () => ({
      media_id: photo.media_id,
      ratio: 'wide',
      crop: { x: 0.2, y: 0.7 },
      mobile_crop: { x: 0.6, y: 0.9 },
    }));
    const config = {
      ...state.draft,
      schema_version: 3,
      blocks: [
        ...state.draft.blocks.filter(
          (b: { type: string }) => b.type === 'hero' || b.type === 'story',
        ),
        ...[
          ['editorial-grid', 'asymmetric', 3],
          ['photo-text', 'photo-left', 1],
          ['horizontal-gallery', 'carousel', 3],
          ['image', 'contained', 1],
          ['image', 'portrait', 1],
        ].map(([type, layout, n], i) => ({
          id: 'visual-' + i,
          type,
          layout,
          content: { source: 'editorial', images: images.slice(0, Number(n)) },
        })),
        ...Array.from({ length: 5 }, (_, i) => ({
          id: 'text-' + i,
          type: 'text',
          layout: 'text',
          content: { source: 'editorial', title: 'Редакционная заметка' },
        })),
        state.draft.blocks.find((b: { type: string }) => b.type === 'products'),
      ],
    };
    const saved = await api.put(endpoint + '/draft', {
      headers: owner.headers,
      data: { config, revision: state.revision },
    });
    expect(saved.status()).toBe(200);
    expect(
      (
        await api.post(endpoint + '/publish', {
          headers: owner.headers,
          data: { revision: (await saved.json()).revision },
        })
      ).status(),
    ).toBe(200);
    await login(page, owner);
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto(`/studio/brands/${brand.id}?tab=page`);
    const preview = frame(page);
    const blockControls = page.locator('.brand-builder__blocks > li');
    await expect(blockControls).toHaveCount(13);
    const actionNames = await blockControls
      .locator('button[aria-label]')
      .evaluateAll((buttons) => buttons.map((button) => button.getAttribute('aria-label')));
    expect(new Set(actionNames).size).toBe(actionNames.length);
    for (const [device, width] of [
      ['Компьютер', 1440],
      ['Телефон', 390],
    ] as const) {
      await page.getByRole('button', { name: device, exact: true }).click();
      await visitor.setViewportSize({ width, height: width === 390 ? 844 : 900 });
      await visitor.goto('/brands/' + brand.slug);
      for (const id of ['hero', 'story', 'visual-0', 'visual-1', 'visual-2', 'products']) {
        const selector = `[data-block-id="${id}"]`;
        const measure = (locator: Locator) =>
          locator.evaluate((e, block) => {
            const doc = e.ownerDocument;
            const child =
              e.querySelector(
                block === 'products'
                  ? '.product-grid'
                  : block === 'hero'
                    ? 'h1'
                    : '.composition-pair, .brand-editorial__photos',
              ) || e;
            const style = doc.defaultView!.getComputedStyle(child);
            return {
              width: Math.round(e.getBoundingClientRect().width),
              child: Math.round(child.getBoundingClientRect().width),
              columns: style.gridTemplateColumns,
              font: style.fontSize,
            };
          }, id);
        if (id === 'products') {
          await expect(preview.locator(selector + ' .product-tile')).toHaveCount(4);
          await expect(visitor.locator(selector + ' .product-tile')).toHaveCount(4);
        }
        const publishedGeometry = await measure(visitor.locator(selector));
        await expect.poll(() => measure(preview.locator(selector))).toEqual(publishedGeometry);
      }
      const selector = '[data-block-id="visual-1"] .editorial-photo img';
      await preview.locator('[data-block-id="visual-1"] .editorial-photo').scrollIntoViewIfNeeded();
      await visitor.locator('[data-block-id="visual-1"] .editorial-photo').scrollIntoViewIfNeeded();
      await expect(preview.locator(selector)).toBeVisible();
      await expect(visitor.locator(selector)).toBeVisible();
      const crop = (e: Locator) =>
        e.evaluate(
          (image) => image.ownerDocument.defaultView!.getComputedStyle(image).objectPosition,
        );
      expect(await crop(preview.locator(selector))).toBe(await crop(visitor.locator(selector)));
      if (width === 1440) {
        await page.screenshot({ path: info.outputPath('preview-1440.png') });
      }
      await visitor.screenshot({ path: info.outputPath('published-' + width + '.png') });
    }
    await page.setViewportSize({ width: 390, height: 844 });
    const first = page.locator('[data-editor-id="visual-3"]');
    const second = page.locator('[data-editor-id="visual-4"]');
    const trigger = first.getByRole('button', { name: /Открыть блок 6, Фотография/ });
    await expect(second.getByRole('button', { name: /Открыть блок 7, Фотография/ })).toBeVisible();
    await trigger.focus();
    await page.keyboard.press('Enter');
    await expect(preview.locator('[data-block-id="visual-3"]')).toBeFocused();
    await page.screenshot({ path: info.outputPath('preview-390.png') });
    await page.getByRole('button', { name: 'К редактированию' }).click();
    await expect(trigger).toBeFocused();
    const up = first.getByRole('button', { name: /Переместить блок 6, Фотография.*выше/ });
    await up.focus();
    await page.keyboard.press('Enter');
    await expect(
      first.getByRole('button', { name: /Переместить блок 5, Фотография.*выше/ }),
    ).toBeFocused();
  } finally {
    await visitor.close();
    await api.dispose();
  }
});

test('public hero recovers from a delivery limit and transient storage failure at adaptive width', async ({
  page,
  playwright,
  baseURL,
}) => {
  requireMVP(baseURL);
  const api = await playwright.request.newContext({ baseURL });
  try {
    const owner = await account(api, 'hero');
    const { brand } = await seedCatalog(api, owner, 1);
    const banner = await uploadFixture(api, owner.token, {
      purpose: 'brand_banner',
      brand_id: brand.id,
    });
    const path = `/api/media/assets/${banner.media_id}/content`;
    const fallbackAttempts: number[] = [];
    await page.setViewportSize({ width: 390, height: 844 });
    await page.route(
      (url) => url.pathname === path,
      async (route) => {
        fallbackAttempts.push(Date.now());
        if (fallbackAttempts.length <= 3) {
          const busy = fallbackAttempts.length < 3;
          await route.fulfill({
            status: busy ? 429 : 503,
            headers: { 'Retry-After': '1' },
            json: { error: busy ? 'media processing busy' : 'media temporarily unavailable' },
          });
        } else await route.continue();
      },
    );
    await page.goto('/brands/' + brand.slug);
    const hero = page.locator('.brand-hero__banner img');
    await expect(hero).toHaveAttribute('src', /^blob:/);
    await expect
      .poll(() => hero.evaluate((img: HTMLImageElement) => img.naturalWidth))
      .toBeGreaterThan(0);
    expect(fallbackAttempts).toHaveLength(4);
    expect(fallbackAttempts[2] - fallbackAttempts[1]).toBeGreaterThanOrEqual(900);
    expect(fallbackAttempts[3] - fallbackAttempts[2]).toBeGreaterThanOrEqual(900);
    await page.unrouteAll();
    await page.reload();
    await expect(hero).toHaveAttribute('srcset', /width=480 480w/);
    await expect
      .poll(() => hero.evaluate((img: HTMLImageElement) => img.currentSrc))
      .toContain('width=480');
  } finally {
    await api.dispose();
  }
});
