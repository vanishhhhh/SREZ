import { expect, test } from '@playwright/test';
import { uploadFixture } from './media-fixture';
import {
  account,
  login,
  noOverflow,
  observeScriptErrors,
  registerInUI,
  requireMVP,
  seedCatalog,
  suffix,
  uploadInUI,
} from './mvp-fixtures';
test.beforeEach(({ baseURL }) => requireMVP(baseURL));

test('seller: owner creates brand and product entirely in UI, publishes, invites editor and revokes access', async ({
  page,
  browser,
  playwright,
  baseURL,
}) => {
  const api = await playwright.request.newContext({ baseURL });
  const editorContext = await browser.newContext();
  try {
    const editor = await account(api, 'edit');
    const errors = observeScriptErrors(page);
    const tag = suffix(),
      name = 'MVP Heavy Hoodie ' + tag;
    await page.goto('/register');
    await registerInUI(page, api, 'Владелец бренда QA');
    await expect(page).toHaveURL(/\/account$/);
    await page.getByRole('link', { name: 'Студия бренда', exact: true }).click();
    await expect(page).toHaveURL(/\/studio$/);
    await page.goto('/studio/brands/new');
    await page.getByLabel('Название бренда', { exact: true }).fill('MVP Studio ' + tag);
    await page.getByLabel(/^Адрес бренда/).fill('mvp-studio-' + tag);
    await page.getByRole('button', { name: /Создать бренд/ }).click();
    await expect(page).toHaveURL(/\/studio\/brands\/\d+$/);
    const brand = Number(new URL(page.url()).pathname.split('/').at(-1));
    await expect(
      page.getByRole('dialog', { name: 'Остались несохранённые изменения' }),
    ).not.toBeVisible();
    await page.getByRole('tab', { name: 'Настройки бренда', exact: true }).click();
    await page
      .getByLabel('Описание бренда', { exact: true })
      .fill('Авторский streetwear. Плотные ткани, свободные силуэты и собственный голос.');
    await page.getByText('Контакты', { exact: true }).click();
    await page.getByLabel('Email бренда', { exact: true }).fill('atelier@example.test');
    await page.getByRole('button', { name: 'Сохранить настройки', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Сохранить настройки' })).toBeDisabled();
    await uploadInUI(page, 'Загрузить логотип', 55);
    await uploadInUI(page, 'Загрузить баннер', 95);
    await page.getByRole('button', { name: 'Опубликовать бренд', exact: true }).click();
    await expect(
      page.getByRole('button', { name: 'Снять с публикации', exact: true }),
    ).toBeVisible();
    const publishedBrand = await api.get('/api/brands/mvp-studio-' + tag);
    expect(publishedBrand.status()).toBe(200);
    expect((await publishedBrand.json()).contacts.email).toBe('atelier@example.test');
    await page.goto(`/studio/brands/${brand}/products/new`);
    await page.getByLabel('Название товара', { exact: true }).fill(name);
    await page.getByLabel('Категория', { exact: true }).selectOption('hoodie');
    await page.getByLabel(/^Цена, ₽/).fill('4990,50');
    await page
      .getByLabel('Описание товара', { exact: true })
      .fill('Свободный силуэт, плотный хлопок. Создано владельцем через реальный интерфейс.');
    await page.getByLabel(/^Материалы/).fill('хлопок');
    await page.getByRole('button', { name: /Создать черновик/ }).click();
    await expect(page).toHaveURL(/\/products\/\d+$/);
    const product = Number(new URL(page.url()).pathname.split('/').at(-1));
    await page.getByLabel('Новый цвет', { exact: true }).fill('Black');
    await page.getByRole('button', { name: 'Добавить цвет', exact: true }).click();
    await expect(page.getByLabel('Название цвета', { exact: true })).toHaveValue('Black');
    await page.getByLabel('Новый размер', { exact: true }).fill('M');
    await page.getByLabel('Остаток', { exact: true }).fill('5');
    await page.getByLabel('SKU', { exact: true }).fill('MVP-' + tag);
    await page.getByRole('button', { name: 'Добавить размер', exact: true }).click();
    await expect(page.getByLabel('Размер M', { exact: true })).toHaveValue('M');
    await uploadInUI(page, 'Добавить фотографию', 65);
    await uploadInUI(page, 'Добавить фотографию', 115);
    await expect(page.locator('.editor-image')).toHaveCount(2);
    const newCover = await page
      .locator('.editor-image .media')
      .nth(1)
      .getAttribute('data-media-src');
    expect(!!newCover).toBe(true);
    await page.getByRole('button', { name: 'Переместить фото 2 раньше', exact: true }).click();
    // Positions change asynchronously. Verify the actual new cover before
    // selecting a position-based action, rather than racing the old first tile.
    await expect(page.locator('.editor-image .media').first()).toHaveAttribute(
      'data-media-src',
      newCover!,
    );
    await page.getByRole('button', { name: 'Заменить фото 1', exact: true }).click();
    await uploadInUI(page, 'Заменить фото 1', 85);
    await expect(page.locator('.editor-image')).toHaveCount(2);
    await page.getByRole('button', { name: 'Опубликовать товар', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Скрыть товар', exact: true })).toBeVisible();
    await page.screenshot({ path: 'docs/previews/mvp-seller-desktop.png', fullPage: true });
    for (const width of [768, 1024]) {
      await page.setViewportSize({ width, height: 1024 });
      await noOverflow(page);
      await expect(page.getByLabel('Название товара', { exact: true })).toBeVisible();
    }
    await page.setViewportSize({ width: 390, height: 844 });
    await noOverflow(page);
    await page.screenshot({ path: 'docs/previews/mvp-seller-mobile.png', fullPage: true });
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto('/catalog?brand_id=' + brand);
    await expect(page.getByRole('heading', { name, exact: true })).toBeVisible();
    await page.getByRole('heading', { name, exact: true }).click();
    await expect(
      page.locator('.size-options').getByRole('button', { name: 'M', exact: true }),
    ).toBeEnabled();
    await page.goto('/studio/brands/' + brand + '?tab=team');
    await page.getByLabel('Имя пользователя редактора', { exact: true }).fill(editor.username);
    await page.getByRole('button', { name: 'Пригласить', exact: true }).click();
    await expect(page.locator('.member-row').filter({ hasText: 'Приглашение' })).toHaveCount(1);
    const editorPage = await editorContext.newPage();
    const editorErrors = observeScriptErrors(editorPage);
    await login(editorPage, editor);
    await editorPage.goto('/studio/invitations');
    await editorPage.getByRole('button', { name: 'Принять', exact: true }).click();
    await expect(editorPage.getByRole('heading', { name: 'Пока нет приглашений' })).toBeVisible();
    const editorPath = `/studio/brands/${brand}/products/${product}`;
    await editorPage.goto(editorPath);
    await expect(
      editorPage
        .locator('.dashboard-nav--brand')
        .getByRole('link', { name: 'Команда', exact: true }),
    ).toHaveCount(0);
    await expect(
      editorPage
        .locator('.dashboard-nav--brand')
        .getByRole('link', { name: 'Оформление и настройки', exact: true }),
    ).toHaveCount(0);
    await editorPage.getByLabel('Остаток M', { exact: true }).fill('7');
    await editorPage.getByLabel('SKU M', { exact: true }).fill('EDITOR-' + tag);
    await editorPage.getByRole('button', { name: 'Сохранить размер M', exact: true }).click();
    await expect(
      editorPage.getByRole('button', { name: 'Сохранить размер M', exact: true }),
    ).toBeDisabled();
    expect(
      (
        await api.patch('/api/brands/' + brand, {
          headers: editor.headers,
          data: { description: 'Недопустимое owner-only изменение' },
        })
      ).status(),
    ).toBe(403);
    expect(
      (await api.get('/api/brands/' + brand + '/members', { headers: editor.headers })).status(),
    ).toBe(403);
    await page.getByRole('button', { name: 'Отозвать доступ', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Отозвать доступ?' })
      .getByRole('button', { name: 'Отозвать доступ', exact: true })
      .click();
    await expect(page.getByRole('button', { name: 'Отозвать доступ', exact: true })).toHaveCount(0);
    const denied = await api.patch('/api/products/' + product, {
      headers: editor.headers,
      data: { name: 'Недопустимое изменение' },
    });
    expect([403, 404].includes(denied.status())).toBe(true);
    await editorPage.goto(editorPath);
    await expect(editorPage.locator('#main')).toContainText(/нет доступа|недостаточно прав/i);
    await expect(
      editorPage.getByRole('button', { name: 'Сохранить товар', exact: true }),
    ).toHaveCount(0);
    const actual = await (await api.get('/api/products/' + product)).json();
    expect(actual.name).toBe(name);
    expect(actual.price_kopecks).toBe(499050);
    expect(errors).toEqual([]);
    expect(editorErrors).toEqual([]);
  } finally {
    await api.dispose();
    await editorContext.close();
  }
});

test('seller forms and PVM: delayed response preserves a draft, explicit hide/complete/publish workflow and deletions', async ({
  page,
  playwright,
  baseURL,
}) => {
  const api = await playwright.request.newContext({ baseURL });
  try {
    const owner = await account(api, 'draft'),
      data = await seedCatalog(api, owner, 1),
      product = data.products[0],
      editorPath = `/studio/brands/${data.brand.id}/products/${product.id}`;
    await login(page, owner);
    await page.goto(editorPath);
    await page.route('**/api/brands/' + data.brand.id + '/products/manage', async (route) => {
      const response = await route.fetch();
      await new Promise((resolve) => setTimeout(resolve, 900));
      await route.fulfill({ response });
    });
    await page.getByLabel('Название товара', { exact: true }).fill(product.name + ' saved');
    await page.getByRole('button', { name: 'Сохранить товар', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Сохранить товар', exact: true })).toBeEnabled();
    await page.getByLabel('Название товара', { exact: true }).fill(product.name + ' unsaved');
    await page.waitForTimeout(1200);
    await expect(page.getByLabel('Название товара', { exact: true })).toHaveValue(
      product.name + ' unsaved',
    );
    await expect(page.getByRole('button', { name: 'Скрыть товар', exact: true })).toBeDisabled();
    await page
      .locator('.dashboard-sidebar')
      .getByRole('link', { name: 'Мои бренды', exact: true })
      .click();
    const warning = page.getByRole('dialog', { name: 'Остались несохранённые изменения' });
    await expect(warning).toBeVisible();
    await warning.getByRole('button', { name: 'Отмена', exact: true }).click();
    await expect(page.getByLabel('Название товара', { exact: true })).toHaveValue(
      product.name + ' unsaved',
    );
    await page
      .locator('.dashboard-sidebar')
      .getByRole('link', { name: 'Мои бренды', exact: true })
      .click();
    await warning.getByRole('button', { name: 'Продолжить без сохранения' }).click();
    await page.unroute('**/api/brands/' + data.brand.id + '/products/manage');
    await page.goto(editorPath);
    await expect(page.getByLabel('Название товара', { exact: true })).toHaveValue(
      product.name + ' saved',
    );
    await expect(page.getByLabel('Новый цвет', { exact: true })).toHaveCount(0);
    await page.getByRole('button', { name: 'Скрыть товар', exact: true }).click();
    await page.getByLabel('Новый цвет', { exact: true }).fill('Blue');
    await page.getByRole('button', { name: 'Добавить цвет', exact: true }).click();
    await expect(page.getByLabel('Название цвета', { exact: true })).toHaveValue('Blue');
    await page.getByRole('button', { name: 'Опубликовать товар', exact: true }).click();
    await expect(page.locator('.notice--error')).toContainText(/Не хватает данных/);
    expect((await api.get('/api/products/' + product.id)).status()).toBe(404);
    await page.getByLabel('Новый размер', { exact: true }).fill('L');
    await page.getByLabel('Остаток', { exact: true }).fill('3');
    await page.getByRole('button', { name: 'Добавить размер', exact: true }).click();
    await uploadInUI(page, 'Добавить фотографию', 100);
    await page.getByRole('button', { name: 'Опубликовать товар', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Скрыть товар', exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Удалить размер L', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Удалить размер L?' })
      .getByRole('button', { name: 'Удалить', exact: true })
      .click();
    await expect(page.getByLabel('Размер L', { exact: true })).toBeVisible();
    expect((await api.get('/api/products/' + product.id)).status()).toBe(200);
    await page
      .getByRole('dialog', { name: 'Удалить размер L?' })
      .getByRole('button', { name: 'Отмена', exact: true })
      .click();
    await page.getByRole('button', { name: 'Скрыть товар', exact: true }).click();
    await page.getByRole('button', { name: 'Удалить размер L', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Удалить размер L?' })
      .getByRole('button', { name: 'Удалить', exact: true })
      .click();
    await expect(page.getByLabel('Размер L', { exact: true })).toHaveCount(0);
    await page.getByRole('button', { name: 'Удалить цвет Blue', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Удалить цвет «Blue»?' })
      .getByRole('button', { name: 'Удалить', exact: true })
      .click();
    await expect(page.getByLabel('Название цвета', { exact: true })).toHaveValue('Black');
    await page.getByRole('button', { name: 'Опубликовать товар', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Скрыть товар', exact: true })).toBeVisible();
  } finally {
    await api.dispose();
  }
});

test('seller security: foreign brands/products/media, private files and anonymous object storage', async ({
  playwright,
  baseURL,
}) => {
  const api = await playwright.request.newContext({ baseURL });
  try {
    const owner = await account(api, 'idora'),
      foreign = await account(api, 'idorb'),
      a = await seedCatalog(api, owner, 1),
      b = await seedCatalog(api, foreign, 1);
    for (const [path, data] of [
      ['/api/brands/' + b.brand.id, { description: 'foreign' }],
      ['/api/products/' + b.products[0].id, { name: 'foreign' }],
    ] as const) {
      const denied = await api.patch(path, { headers: owner.headers, data });
      expect([403, 404].includes(denied.status())).toBe(true);
    }
    const product = a.products[0];
    const attach = await api.post(`/api/products/${product.id}/colors/${product.color.id}/images`, {
      headers: owner.headers,
      data: { media_id: b.products[0].photo.media_id },
    });
    expect([400, 403, 404].includes(attach.status())).toBe(true);
    const upload = await api.post('/api/media/uploads', {
      headers: owner.headers,
      data: {
        purpose: 'product_image',
        product_id: b.products[0].id,
        color_id: b.products[0].color.id,
        content_type: 'image/png',
        size_bytes: 100,
      },
    });
    expect([403, 404].includes(upload.status())).toBe(true);
    expect(
      (
        await api.post('/api/media/uploads', {
          headers: owner.headers,
          data: { purpose: 'user_avatar', user_id: 1, content_type: 'image/png', size_bytes: 100 },
        })
      ).status(),
    ).toBe(400);
    const avatar = await uploadFixture(api, owner.token, { purpose: 'user_avatar' }, 88);
    const path = '/media/assets/' + avatar.media_id + '/content';
    expect((await api.get(path)).status()).toBe(404);
    expect((await api.get(path, { headers: foreign.headers })).status()).toBe(404);
    expect((await api.get(path, { headers: owner.headers })).status()).toBe(200);
    const anonymous = await api.get('http://127.0.0.1:18334/srez-media?list-type=2');
    expect(anonymous.status()).toBe(403);
    const original = await (await api.get('/api/products/' + b.products[0].id)).json();
    expect(original.name).toBe(b.products[0].name);
  } finally {
    await api.dispose();
  }
});
