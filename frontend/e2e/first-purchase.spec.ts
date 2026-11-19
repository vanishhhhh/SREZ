import { expect, test } from '@playwright/test';
import { account, noOverflow, registerInUI, requireMVP, seedCatalog } from './mvp-fixtures';
import { uploadFixture } from './media-fixture';

test('first buyer: guest catalog to verified account, wishlist and unpaid reservation', async ({
  page,
  request,
  baseURL,
}) => {
  requireMVP(baseURL);
  const owner = await account(request, 'retail');
  const { brand, products } = await seedCatalog(request, owner);
  const [product] = products;
  await uploadFixture(
    request,
    owner.token,
    {
      purpose: 'product_image',
      product_id: product.id,
      color_id: product.color.id,
    },
    120,
  );

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');
  await page.getByRole('button', { name: 'Открыть меню' }).click();
  await page
    .getByRole('navigation', { name: 'Основная навигация' })
    .getByRole('link', { name: 'Каталог' })
    .click();
  await page.getByRole('button', { name: /^Фильтры/ }).click();
  await expect(page.getByLabel('Категория', { exact: true })).toBeFocused();
  await page.getByLabel('Категория', { exact: true }).selectOption('hoodie');
  await page.getByLabel('Бренд', { exact: true }).selectOption(String(brand.id));
  await page.getByRole('button', { name: 'Показать вещи' }).click();
  await page.getByLabel('Сортировка').selectOption('price_asc');
  await expect(page.locator('.product-tile h3')).toHaveText(products.map((item) => item.name));
  await page.getByRole('heading', { name: product.name, exact: true }).click();
  await expect(page.getByRole('heading', { name: product.name, level: 1 })).toBeVisible();
  await page.getByRole('button', { name: 'Следующее фото', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Фото 2', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await page
    .getByRole('group', { name: 'Размер' })
    .getByRole('button', { name: 'M', exact: true })
    .click();

  const favorite = page.getByRole('button', { name: 'Добавить в избранное', exact: true });
  await favorite.click();
  await expect(page).toHaveURL(/\/login$/);
  await page.getByRole('link', { name: 'Присоединиться', exact: true }).click();
  const buyer = await registerInUI(page, request, 'Покупатель QA', true);
  await expect(page).toHaveURL(new RegExp('/products/' + product.id + '$'));
  await favorite.click();
  await expect(page.getByRole('button', { name: 'Удалить из избранного' })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await page
    .getByRole('group', { name: 'Размер' })
    .getByRole('button', { name: 'M', exact: true })
    .click();

  await page.getByRole('button', { name: /в вишлист/ }).click();
  const picker = page.getByRole('dialog', { name: 'Сохранить в подборку' });
  await picker.getByLabel('Название нового вишлиста').fill('Городская подборка QA');
  await picker.getByRole('button', { name: 'Сохранить', exact: true }).click();
  await expect(picker).toBeHidden();
  await page.getByRole('button', { name: /Добавить в корзину/ }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Добавлено в корзину' })).toBeVisible();
  await page.getByRole('link', { name: /^Корзина:/ }).click();
  await page.locator('.quantity-form input').fill('2');
  await page.getByRole('button', { name: 'Обновить', exact: true }).click();
  await expect(page.locator('.cart-total strong')).toContainText('9 981');
  await expect(page.getByText(/Деньги не списываются/)).toBeVisible();
  await page.getByRole('button', { name: /Зарезервировать на 15 минут/ }).click();
  await expect(page).toHaveURL(/\/orders\/\d+$/);
  await expect(page.getByText(/Деньги не списаны/)).toBeVisible();
  await noOverflow(page);
  const orderID = new URL(page.url()).pathname.split('/').at(-1);
  const order = await request.get('/api/orders/' + orderID, { headers: buyer.headers });
  expect(order.status()).toBe(200);
  expect(await order.json()).toMatchObject({ status: 'unpaid', items: [{ quantity: 2 }] });
  await page.reload();
  await expect(page.locator('.order-status')).toBeVisible();
});
