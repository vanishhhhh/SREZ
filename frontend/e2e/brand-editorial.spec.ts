import { expect, test } from '@playwright/test';
import { account, login, noOverflow, requireMVP, seedCatalog } from './mvp-fixtures';
import { fixturePNG } from './media-fixture';

test('owner assembles repeated editorial blocks with managed images and publishes the saved revision', async ({
  page,
  browser,
  playwright,
  baseURL,
}, testInfo) => {
  requireMVP(baseURL);
  test.setTimeout(60_000);
  const api = await playwright.request.newContext({ baseURL });
  const visitor = await browser.newPage({ baseURL, viewport: { width: 1440, height: 900 } });
  try {
    const owner = await account(api, 'media');
    const { brand } = await seedCatalog(api, owner, 1);
    expect(
      (
        await api.patch('/api/brands/' + brand.id, {
          headers: owner.headers,
          data: {
            description:
              'Редакционная серия объединяет лёгкие ткани, многослойный крой и спокойные природные оттенки. '
                .repeat(8)
                .trim(),
          },
        })
      ).status(),
    ).toBe(200);
    const publicPath = '/brands/' + brand.slug;
    const studioPath = '/studio/brands/' + brand.id + '?tab=page';
    const manage = '/api/brands/' + brand.id + '/page/manage';
    await login(page, owner);
    await page.setViewportSize({ width: 1440, height: 920 });
    await page.goto(studioPath);
    await expect(page.getByRole('heading', { name: 'Оформление страницы' })).toBeVisible();
    await expect(
      page.getByRole('link', { name: 'Корзина', exact: true }).locator('span'),
    ).toHaveCount(0);
    await expect(
      page.getByRole('link', { name: 'Избранное', exact: true }).locator('span'),
    ).toHaveCount(0);
    const settings = page.locator('.brand-editorial-settings');
    async function add(type: string) {
      await page.getByLabel('Тип нового блока').selectOption(type);
      await page.getByRole('button', { name: 'Добавить блок', exact: true }).click();
      await expect(settings).toBeVisible();
    }
    async function upload(index: number, tone: number) {
      const photo = settings.getByRole('group', { name: `Фото ${index}`, exact: true });
      await photo.getByLabel(`Загрузить фото ${index}`).setInputFiles({
        name: `editorial-${tone}.png`,
        mimeType: 'image/png',
        buffer: fixturePNG(tone),
      });
      await expect(photo.getByAltText('Предпросмотр выбранной фотографии')).toBeVisible();
      await photo.getByRole('button', { name: 'Загрузить фотографию' }).click();
      const library = photo.getByLabel(`Фото ${index} из библиотеки`);
      await expect(library).toHaveValue(/[1-9]\d*/);
      return Number(await library.inputValue());
    }
    await add('image');
    const cover = await upload(1, 42);
    await settings.getByLabel('Композиция').selectOption('portrait');
    await settings.getByLabel('По горизонтали').press('Home');
    await settings.getByLabel('По горизонтали').press('ArrowRight');
    await settings.getByLabel('Альтернативное описание').fill('Редакционный портрет');
    await settings.getByText('Отдельное кадрирование для телефона').click();
    await settings
      .getByRole('group', { name: 'Кадрирование телефона' })
      .getByLabel('По горизонтали')
      .press('End');
    await add('horizontal-gallery');
    const photoA = await upload(1, 72);
    const photoB = await upload(2, 101);
    await settings.getByLabel('Заголовок').fill('Съёмка');
    await settings.getByRole('button', { name: 'Переместить фото 2 назад' }).click();
    await expect(settings.getByLabel('Фото 1 из библиотеки')).toHaveValue(String(photoB));
    for (const [id, heading] of [
      [photoA, 'Ткань и форма'],
      [photoB, 'Линии и объём'],
    ] as const) {
      await add('photo-text');
      await settings.getByLabel('Фото 1 из библиотеки').selectOption(String(id));
      await settings.getByLabel('Заголовок').fill(heading);
      await settings.getByLabel('Текст', { exact: true }).fill('Материал и детали коллекции.');
      await settings.getByLabel('Выравнивание').selectOption('center');
    }
    await add('editorial-grid');
    await settings.getByRole('radio', { name: 'Асимметрия' }).check();
    for (const [index, id] of [cover, photoA, photoB].entries()) {
      await settings.getByLabel(`Фото ${index + 1} из библиотеки`).selectOption(String(id));
      await settings
        .getByRole('group', { name: `Фото ${index + 1}`, exact: true })
        .getByLabel('Пропорции')
        .selectOption(index === 0 ? 'portrait' : 'landscape');
    }
    await add('vertical-gallery');
    await settings.getByLabel('Фото 1 из библиотеки').selectOption(String(photoA));
    await settings.getByLabel('Фото 2 из библиотеки').selectOption(String(photoB));
    for (const title of ['Крой', 'Материал', 'Фактура']) {
      await add('text');
      await settings.getByLabel('Заголовок').fill(title);
    }
    const story = page.locator('[data-editor-block="story"]');
    await story.getByRole('button', { name: 'Настроить' }).click();
    await story.getByRole('radio', { name: 'Узкое изображение справа' }).check();
    await story.getByLabel('По высоте').selectOption('bottom');
    await story.getByLabel('Ширина текста').selectOption('narrow');
    await story.getByLabel('Отступы секции').selectOption('compact');
    await story.getByLabel('Фото 1 из библиотеки').selectOption(String(photoA));
    await story.getByRole('button', { name: /Переместить блок .*О бренде.*ниже/ }).click();
    await story.getByRole('button', { name: /Открыть блок .*О бренде.*в предпросмотре/ }).click();
    const storyPreview = page
      .frameLocator('iframe[title="Предпросмотр витрины"]')
      .locator('[data-brand-block="story"]');
    await expect(storyPreview).toHaveAttribute('data-preview-selected', 'true');
    await expect(storyPreview).toHaveAttribute('data-vertical', 'bottom');
    await expect(page.locator('.brand-builder__blocks > li')).toHaveCount(13);
    await expect(
      page
        .frameLocator('iframe[title="Предпросмотр витрины"]')
        .locator('[data-brand-block="photo-text"]'),
    ).toHaveCount(2);
    await page.getByRole('textbox', { name: 'Фон', exact: true }).fill('#ebe1d7');
    await page.getByRole('button', { name: 'Сохранить черновик' }).click();
    await expect(page.getByText('Черновик сохранён', { exact: true })).toBeVisible();
    const saved = await (await api.get(manage, { headers: owner.headers })).json();
    expect(saved.draft.schema_version).toBe(3);
    expect(saved.draft.appearance.background_color).toBe('#ebe1d7');
    expect(saved.draft.blocks[2].type).toBe('story');
    expect(saved.draft.blocks[2].composition).toEqual({
      vertical: 'bottom',
      text_width: 'narrow',
      spacing: 'compact',
    });
    const image = saved.draft.blocks.find((b: { type: string }) => b.type === 'image').content
      .images[0];
    expect(image.crop.x).toBe(0.01);
    expect(image.mobile_crop.x).toBe(1);
    expect((await visitor.request.get(`/api/media/assets/${cover}/content`)).status()).toBe(404);
    await visitor.goto(publicPath);
    await expect(visitor.locator('[data-brand-block="image"]')).toHaveCount(0);
    await page.reload();
    await expect(
      page
        .frameLocator('iframe[title="Предпросмотр витрины"]')
        .locator('[data-brand-block="photo-text"]'),
    ).toHaveCount(2);
    await page.getByRole('button', { name: 'Опубликовать оформление' }).click();
    await expect(page.getByText('Оформление опубликовано.', { exact: true })).toBeVisible();
    expect((await visitor.request.get(`/api/media/assets/${cover}/content`)).ok()).toBe(true);
    const gallery = visitor.getByRole('region', { name: 'Съёмка' });
    const delivery429: string[] = [];
    visitor.on('response', (response) => {
      if (response.status() === 429 && response.url().includes('/media/assets/'))
        delivery429.push(response.url());
    });
    await visitor.reload();
    const publicStory = visitor.locator('[data-brand-block="story"]');
    await expect(publicStory).toHaveAttribute('data-layout', 'editorial-reversed');
    await expect(publicStory).toHaveAttribute('data-text-width', 'narrow');
    expect((await publicStory.locator('p').innerText()).length).toBeGreaterThan(600);
    await expect(publicStory).not.toHaveAttribute('data-preview-selected', 'true');
    const editorial = visitor.locator('[data-brand-block="image"]');
    await editorial.scrollIntoViewIfNeeded();
    await expect(editorial.getByAltText('Редакционный портрет')).toBeVisible();
    for (const width of [1920, 1440, 1024, 834, 390, 360]) {
      await visitor.setViewportSize({ width, height: 920 });
      await expect(visitor.locator('[data-brand-block="photo-text"]')).toHaveCount(2);
      const grid = visitor.locator('[data-brand-block="editorial-grid"]');
      await expect(grid).toHaveAttribute('data-layout', 'asymmetric');
      expect(
        await grid
          .locator('.editorial-photo')
          .evaluateAll((photos) =>
            photos.every((photo) => photo.getBoundingClientRect().right <= innerWidth),
          ),
      ).toBe(true);
      await noOverflow(visitor);
      if (width === 390) {
        await gallery.scrollIntoViewIfNeeded();
        await expect(gallery.locator('img')).toHaveCount(2);
        const next = visitor.getByRole('button', { name: 'Следующие фотографии' });
        await next.focus();
        await visitor.keyboard.press('Enter');
        await expect.poll(() => gallery.evaluate((node) => node.scrollLeft)).toBeGreaterThan(0);
        await visitor.screenshot({ path: testInfo.outputPath('published-editorial-mobile.png') });
      }
    }
    expect(delivery429).toHaveLength(0);
    const single = page
      .locator('.brand-builder__blocks > li')
      .filter({ has: page.getByText('Фотография', { exact: true }) });
    await single.getByRole('button', { name: /^Удалить блок/ }).click();
    await page.getByRole('button', { name: 'Сохранить черновик' }).click();
    await expect(page.getByText('Есть черновик с изменениями.')).toBeVisible();
    await visitor.reload();
    await expect(visitor.locator('[data-brand-block="image"]')).toHaveCount(1);
    await page.reload();
    await expect(
      page
        .frameLocator('iframe[title="Предпросмотр витрины"]')
        .locator('[data-brand-block="image"]'),
    ).toHaveCount(0);
    for (const width of [1920, 1440, 1024, 834, 390, 360]) {
      await page.setViewportSize({ width, height: 920 });
      await noOverflow(page);
    }
    await page.getByRole('button', { name: 'Открыть предпросмотр' }).click();
    await page.getByRole('button', { name: 'Телефон', exact: true }).click();
    await expect(
      page
        .frameLocator('.brand-builder__frame--mobile iframe')
        .locator('[data-brand-block="horizontal-gallery"]'),
    ).toBeVisible();
    await noOverflow(page);
  } finally {
    await visitor.close();
    await api.dispose();
  }
});

test('empty brand preview avoids empty columns and story can be omitted from V3', async ({
  page,
  playwright,
  baseURL,
}) => {
  requireMVP(baseURL);
  const api = await playwright.request.newContext({ baseURL });
  try {
    const owner = await account(api, 'empty');
    const create = await api.post('/api/brands', {
      headers: owner.headers,
      data: {
        name: 'Длиннаяисториянезависимогоэкспериментальногобренда',
        slug: 'empty-composition-' + Date.now().toString(36),
      },
    });
    expect(create.status()).toBe(201);
    const brand = await create.json();
    await login(page, owner);
    await page.setViewportSize({ width: 360, height: 900 });
    await page.goto('/studio/brands/' + brand.id + '?tab=page');
    await page.getByRole('button', { name: 'Открыть предпросмотр' }).click();
    const preview = page.frameLocator('iframe[title="Предпросмотр витрины"]');
    await expect(preview.getByText('Добавьте баннер в настройках бренда.')).toBeVisible();
    await expect(preview.getByText('Добавьте описание в настройках бренда.')).toBeVisible();
    await expect(preview.locator('.composition-pair__visual')).toHaveCount(0);
    await noOverflow(page);
    await page.getByRole('button', { name: 'К редактированию' }).click();
    await page
      .locator('[data-editor-block="story"]')
      .getByRole('button', { name: /^Удалить блок/ })
      .click();
    await page.getByRole('button', { name: 'Сохранить черновик' }).click();
    await expect(page.getByText('Черновик сохранён', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Добавить историю бренда' })).toBeVisible();
    await page.getByRole('button', { name: 'Добавить блок', exact: true }).click();
    await page.getByRole('button', { name: 'Открыть предпросмотр' }).click();
    await expect(preview.getByText('Выберите изображения для блока.')).toBeVisible();
  } finally {
    await api.dispose();
  }
});
