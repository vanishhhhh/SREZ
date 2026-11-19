import { expect, test } from '@playwright/test';
import { account, login, noOverflow, requireMVP, seedCatalog } from './mvp-fixtures';
import { uploadFixture } from './media-fixture';

test('owner edits and previews a brand page, publishes a snapshot, and handles revision conflicts', async ({
  page,
  playwright,
  baseURL,
}, testInfo) => {
  requireMVP(baseURL);
  const api = await playwright.request.newContext({ baseURL });
  try {
    const owner = await account(api, 'page');
    const { brand, products } = await seedCatalog(api, owner, 1);
    await uploadFixture(api, owner.token, { purpose: 'brand_banner', brand_id: brand.id }, 70);
    const publicPath = '/brands/' + brand.slug;
    const studioPath = '/studio/brands/' + brand.id + '?tab=page';
    const endpoint = '/api/brands/' + brand.id + '/page';
    const getPublic = async () => (await api.get('/api' + publicPath)).json();
    await page.goto(publicPath);
    await expect(page.locator('[data-brand-block]')).toHaveCount(3);
    await expect(page.locator('.brand-hero--full-width')).toBeVisible();
    await expect(page.getByRole('heading', { name: products[0].name })).toBeVisible();
    expect(await getPublic()).not.toHaveProperty('page');

    await login(page, owner);
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto(studioPath);
    await expect(page.getByRole('heading', { name: 'Оформление страницы' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Открыть предпросмотр' })).toBeHidden();
    await page.getByRole('combobox', { name: 'Композиция блока «Обложка»' }).selectOption('split');
    const moveProducts = page.getByRole('button', { name: /Переместить блок .*Товары.*выше/ });
    await moveProducts.focus();
    await expect(moveProducts).toBeFocused();
    await page.keyboard.press('Enter');
    await page.getByRole('button', { name: /Удалить блок .*Контакты/ }).click();
    await expect(page.getByRole('button', { name: 'Добавить контакты' })).toBeVisible();
    await page.getByRole('button', { name: 'Добавить контакты' }).click();
    await page.getByRole('button', { name: /Удалить блок .*Контакты/ }).click();
    await page.getByRole('textbox', { name: 'Фон', exact: true }).fill('#151515cf');
    await page.getByRole('textbox', { name: 'Текст', exact: true }).fill('#ffffff');
    const preview = page.frameLocator('iframe[title="Предпросмотр витрины"]');
    await expect(preview.locator('[data-brand-block="hero"]')).toHaveClass(/brand-hero--split/);
    await expect(preview.locator('[data-brand-block="story"][data-layout="split"]')).toBeVisible();
    await expect(preview.locator('[data-brand-block]').nth(1)).toHaveAttribute(
      'data-brand-block',
      'products',
    );
    await page.locator('header').getByRole('link', { name: 'Каталог' }).click();
    await expect(
      page.getByRole('dialog', { name: 'Остались несохранённые изменения' }),
    ).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.getByRole('textbox', { name: 'Фон', exact: true })).toHaveValue('#151515cf');

    await page.getByRole('button', { name: 'Сохранить черновик' }).click();
    await expect(page.getByText('Черновик сохранён', { exact: true })).toBeVisible();
    expect(await getPublic()).not.toHaveProperty('page');
    await page.reload();
    await expect(page.getByRole('textbox', { name: 'Фон', exact: true })).toHaveValue('#151515cf');
    await page.getByRole('button', { name: 'Опубликовать оформление' }).click();
    await expect(page.getByText('Оформление опубликовано.', { exact: true })).toBeVisible();
    expect((await getPublic()).page.appearance.background_color).toBe('#151515cf');

    const loaded = await (await api.get(endpoint + '/manage', { headers: owner.headers })).json();
    await page.getByRole('textbox', { name: 'Фон', exact: true }).fill('#d9d9d9');
    const raced = await api.put(endpoint + '/draft', {
      headers: owner.headers,
      data: {
        revision: loaded.revision,
        config: {
          ...loaded.draft,
          appearance: { ...loaded.draft.appearance, background_color: '#aaa' },
        },
      },
    });
    expect(raced.status()).toBe(200);
    await page.getByRole('button', { name: 'Сохранить черновик' }).click();
    await expect(
      page.getByText('Оформление изменилось в другой сессии.', { exact: false }).first(),
    ).toBeVisible();
    await expect(page.getByRole('textbox', { name: 'Фон', exact: true })).toHaveValue('#d9d9d9');
    expect((await getPublic()).page.appearance.background_color).toBe('#151515cf');
    await page.getByRole('button', { name: 'Загрузить актуальный черновик' }).click();
    await page.getByRole('button', { name: 'Продолжить без сохранения' }).click();
    await expect(page.getByRole('textbox', { name: 'Фон', exact: true })).toHaveValue('#aaa');
    await page.getByRole('textbox', { name: 'Фон', exact: true }).fill('#151515cf');
    await noOverflow(page);
    await page.screenshot({ path: testInfo.outputPath('builder-desktop.png'), fullPage: true });

    for (const width of [834, 390, 360]) {
      await page.setViewportSize({ width, height: 900 });
      await noOverflow(page);
      if (width < 761) {
        const open = page.getByRole('button', { name: 'Открыть предпросмотр' });
        if (await open.isVisible()) await open.click();
        await page.getByRole('button', { name: 'Телефон', exact: true }).click();
        await expect(page.locator('.brand-builder__frame')).toHaveClass(/--mobile/);
        await expect(preview.locator('[data-brand-block="hero"]')).toBeVisible();
        await noOverflow(page);
        if (width === 390)
          await page.screenshot({
            path: testInfo.outputPath('builder-mobile-preview.png'),
            fullPage: true,
          });
        await page.getByRole('button', { name: 'К редактированию' }).click();
        if (width === 390)
          await page.screenshot({
            path: testInfo.outputPath('builder-mobile-editor.png'),
            fullPage: true,
          });
      }
    }
    await page.locator('header').getByRole('link', { name: 'SREZ — главная' }).click();
    await page.getByRole('button', { name: 'Продолжить без сохранения' }).click();
    await expect(page).toHaveURL(baseURL + '/');

    for (const width of [1440, 834, 390, 360]) {
      await page.setViewportSize({ width, height: 900 });
      for (const [path, title, filename] of [
        [publicPath, brand.name, 'brand'],
        ['/wishlists', 'Мои вишлисты', 'wishlists'],
        ['/wishlists/new', 'Новый вишлист', 'new-wishlist'],
        ['/favorites', 'Избранное', 'favorites'],
        ['/orders', 'Мои заказы', 'orders'],
      ]) {
        await page.goto(path);
        await expect(
          page.getByRole('heading', { level: 1, name: title, exact: true }),
        ).toBeVisible();
        await noOverflow(page);
        if (path === publicPath) {
          await expect(page.locator('[data-brand-block="hero"]')).toHaveClass(/brand-hero--split/);
          await expect(page.locator('[data-brand-block]').nth(1)).toHaveAttribute(
            'data-brand-block',
            'products',
          );
          await expect(page.locator('header')).not.toHaveAttribute('style', /--brand-/);
        } else {
          await expect(page.locator('main .page-heading .eyebrow')).toHaveCount(0);
          if (path === '/orders')
            await expect(
              page.getByText(
                'Здесь можно посмотреть заказы и их статусы. Оплата пока не подключена.',
              ),
            ).toBeVisible();
        }
        if (width === 390 || width === 1440) {
          await page.screenshot({
            path: testInfo.outputPath(`${filename}-${width}.png`),
            fullPage: true,
          });
        }
      }
    }
  } finally {
    await api.dispose();
  }
});
