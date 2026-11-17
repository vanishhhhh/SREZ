import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router-dom';
import { session } from '../../lib/client';
import { readSharedWishlist, readWishlist, wishlistAPI } from './api';
import { WishlistCard } from './Pages';

const stamp = '2026-11-17T01:00:00Z';
const card = {
  id: 1,
  brand_id: 2,
  name: 'Худи',
  description: 'Описание',
  price_kopecks: 200001,
  category: 'hoodie',
  materials: ['cotton'],
  status: 'published',
  created_at: stamp,
  updated_at: stamp,
  brand_name: 'Brand',
  brand_slug: 'brand',
  preview_image_url: '/media/assets/7/content',
  is_available: true,
  available_colors: ['Black'],
  available_sizes: ['M'],
  colors: [
    {
      id: 10,
      color: 'Black',
      preview_image_url: '/media/assets/7/content',
      is_available: true,
      available_sizes: ['M'],
    },
  ],
};
const item = {
  id: 3,
  product_id: 1,
  product_variant_id: 8,
  position: 0,
  product: card,
  variant: { id: 8, color: 'Black', size: 'M', stock: 2 },
};
const list = {
  id: 4,
  title: 'Моя подборка',
  description: 'Осень',
  visibility: 'private' as const,
  item_count: 1,
  created_at: stamp,
  updated_at: stamp,
  share_path: null,
  items: [item],
};
afterEach(() => vi.restoreAllMocks());
describe('wishlist contracts', () => {
  it('accepts an owner tombstone but rejects a public hidden-product leak', () => {
    const hidden = { ...item, product: null, variant: null };
    expect(readWishlist({ ...list, items: [hidden] }).items[0].product).toBeNull();
    expect(() => readSharedWishlist({ ...list, items: [hidden] })).toThrow();
  });
  it('rejects unsafe or mismatched card, variant and money data', () => {
    for (const invalid of [
      { ...item, product_id: 999 },
      { ...item, variant: { ...item.variant, id: 999 } },
      { ...item, variant: { ...item.variant, stock: -1 } },
      { ...item, product: { ...card, price_kopecks: Number.MAX_SAFE_INTEGER + 1 } },
    ])
      expect(() => readWishlist({ ...list, items: [invalid] })).toThrow();
  });
  it('rejects duplicate products, broken order and mismatched counts', () => {
    expect(() =>
      readWishlist({ ...list, item_count: 2, items: [item, { ...item, id: 5, position: 1 }] }),
    ).toThrow();
    expect(() => readWishlist({ ...list, items: [{ ...item, position: 2 }] })).toThrow();
    expect(() => readWishlist({ ...list, item_count: 2 })).toThrow();
  });
  it('only accepts relative capability URLs belonging to the wishlist route', () => {
    const path = '/w/' + 'a'.repeat(43);
    expect(readWishlist({ ...list, visibility: 'public', share_path: path }).share_path).toBe(path);
    for (const share_path of [
      'https://example.com/w/' + 'a'.repeat(43),
      '//other.example/w/x',
      '/w/short',
      'javascript:alert(1)',
    ])
      expect(() => readWishlist({ ...list, visibility: 'public', share_path })).toThrow();
    expect(() => readWishlist({ ...list, share_path: path })).toThrow();
  });
  it('public reads never attach bearer auth and preserve the DTO', async () => {
    const request = vi
      .spyOn(session, 'request')
      .mockResolvedValue({ title: list.title, description: '', updated_at: stamp, items: [item] });
    await wishlistAPI.shared('a'.repeat(43));
    expect(request).toHaveBeenCalledWith('/wishlists/shared/' + 'a'.repeat(43), {
      signal: undefined,
    });
  });
  it('item writes contain IDs only and support clearing a variant', async () => {
    const request = vi.spyOn(session, 'request').mockResolvedValue(list);
    await wishlistAPI.add(4, 1, 8);
    expect(request).toHaveBeenLastCalledWith('/wishlists/4/items', {
      method: 'POST',
      auth: true,
      body: { product_id: 1, product_variant_id: 8 },
    });
    await wishlistAPI.variant(4, 3, null);
    expect(request).toHaveBeenLastCalledWith('/wishlists/4/items/3', {
      method: 'PATCH',
      auth: true,
      body: { product_variant_id: null },
    });
  });
  it('sharing and revocation use the documented empty/bodyless contracts', async () => {
    const request = vi
      .spyOn(session, 'request')
      .mockResolvedValue({ ...list, visibility: 'public', share_path: '/w/' + 'a'.repeat(43) });
    await wishlistAPI.share(4);
    expect(request).toHaveBeenLastCalledWith('/wishlists/4/share', {
      method: 'POST',
      auth: true,
      body: {},
    });
    request.mockResolvedValue(list);
    await wishlistAPI.unshare(4);
    expect(request).toHaveBeenLastCalledWith('/wishlists/4/share', {
      method: 'DELETE',
      auth: true,
      body: undefined,
    });
  });
});
describe('collection card component', () => {
  it('escapes user content and keeps the link on the local owner route', () => {
    const html = renderToStaticMarkup(
      <MemoryRouter>
        <WishlistCard
          list={{
            ...list,
            title: '<script>alert(1)</script>',
            description: '<img src=x onerror=alert(1)>',
          }}
          index={0}
        />
      </MemoryRouter>,
    );
    expect(html).toContain('&lt;script&gt;');
    expect(html).not.toContain('<script>');
    expect(html).not.toContain('<img');
    expect(html).toContain('href="/wishlists/4"');
    expect(html).toContain('Личная');
  });
});
