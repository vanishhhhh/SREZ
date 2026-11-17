import { describe, expect, it } from 'vitest';
import { mediaURL } from './media';
import { applyFilters, countThings, filtersFrom } from './catalog-query';
import { readProduct, readProductPage } from './public-data';

describe('storefront media contract', () => {
  it('uses managed Go paths unchanged and retains legacy HTTPS', () => {
    expect(mediaURL('/media/assets/5b12-abc/content')).toBe('/media/assets/5b12-abc/content');
    expect(mediaURL('https://images.example.com/a.jpg?v=2')).toBe(
      'https://images.example.com/a.jpg?v=2',
    );
  });
  it.each([
    '//storage.test/a',
    'http://127.0.0.1:8333/private/a',
    'javascript:alert(1)',
    '/media/assets/../content',
    '/media/assets/a/content?redirect=x',
    'https://name:password@example.com/a',
    'https://bucket.s3.example.com/private?X-Amz-Signature=secret',
    ' https://example.com/a',
    '/other/file.jpg',
    undefined,
  ])('rejects unsupported media %s', (value) => {
    expect(mediaURL(value)).toBeUndefined();
  });
});
describe('reproducible catalog URLs', () => {
  it('converts rubles to integer kopecks, retains sort/limit and resets page', () => {
    const current = new URLSearchParams('sort=price_asc&limit=1&page=5&ignored=x');
    const next = applyFilters(
      { ...filtersFrom(current), min: '20,01', max: '500', color: ' Чёрный ', size: 'M' },
      current,
    );
    expect(Object.fromEntries(next)).toEqual({
      min_price: '2001',
      max_price: '50000',
      color: 'Чёрный',
      size: 'M',
      sort: 'price_asc',
      limit: '1',
      page: '1',
    });
  });
  it('restores price fields and handles invalid or unsafe URL prices without NaN', () => {
    expect(filtersFrom(new URLSearchParams('min_price=200001')).min).toBe('2000.01');
    for (const p of ['NaN', '-1', '1.5', '9007199254740992'])
      expect(filtersFrom(new URLSearchParams({ min_price: p })).min).toBe('');
  });
  it('rejects reversed prices before sending a request', () => {
    expect(() =>
      applyFilters(
        { ...filtersFrom(new URLSearchParams()), min: '50', max: '10' },
        new URLSearchParams(),
      ),
    ).toThrow('Минимальная');
  });
  it.each([
    [1, '1 вещь'],
    [2, '2 вещи'],
    [11, '11 вещей'],
    [21, '21 вещь'],
    [0, '0 вещей'],
  ])('counts %s', (n, text) => expect(countThings(Number(n))).toBe(text));
});
describe('public API response boundaries', () => {
  const headers = () =>
    new Headers({ 'X-Total-Count': '0', 'X-Page': '3', 'X-Limit': '12', 'X-Total-Pages': '0' });
  it('retains requested out-of-range pages and empty arrays', () =>
    expect(readProductPage([], headers())).toEqual({
      items: [],
      total: 0,
      page: 3,
      limit: 12,
      pages: 0,
    }));
  it('does not invent pagination when headers are missing or inconsistent', () => {
    expect(() => readProductPage([], new Headers())).toThrow();
    const h = headers();
    h.set('X-Total-Pages', '2');
    expect(() => readProductPage([], h)).toThrow();
  });
  it('rejects null collections and negative stock rather than crashing the view', () => {
    expect(() => readProductPage(null, headers())).toThrow();
    expect(() =>
      readProduct({
        id: 1,
        brand_id: 1,
        price_kopecks: 2000,
        name: 'A',
        category: 'hoodie',
        description: '',
        materials: [],
        colors: [{ id: 1, color: 'A', images: [], variants: [{ id: 1, size: 'M', stock: -1 }] }],
      }),
    ).toThrow();
  });
});
