import { describe, it, expect, vi, beforeEach } from 'vitest';
import { buyerAPI, readCart, readOrder, readFavorites } from './api';
import { BuyerStore } from './store';
import { ApiError } from '../../lib/http';
const cart = {
  items: [
    {
      id: 1,
      product_variant_id: 11,
      quantity: 2,
      product: {
        product_id: 1,
        brand_id: 1,
        name: 'Худи',
        preview_image_url: '',
        color: 'Чёрный',
        size: 'M',
        unit_price_kopecks: 100050,
      },
      availability: 'available' as const,
      subtotal_kopecks: 200100,
    },
  ],
  total_kopecks: 200100,
};
const order = {
  id: 4,
  status: 'unpaid' as const,
  created_at: '2026-11-17T10:00:00Z',
  cancel_before: '2026-11-17T10:15:00Z',
  cancelled_at: null,
  expired_at: null,
  total_kopecks: 200100,
  items: [
    {
      product_variant_id: 11,
      product_id: 1,
      brand_id: 1,
      product_name: 'Худи',
      color: 'Чёрный',
      size: 'M',
      quantity: 2,
      unit_price_kopecks: 100050,
      subtotal_kopecks: 200100,
    },
  ],
};
describe('buyer DTO boundaries', () => {
  it('accepts unavailable favorites and cart lines without exposing a hidden product', () => {
    expect(readFavorites([{ product_id: 4, product: null }])).toEqual([
      { product_id: 4, product: null },
    ]);
    expect(
      readCart({
        items: [
          {
            id: 1,
            product_variant_id: null,
            quantity: 2,
            product: null,
            availability: 'unavailable',
            subtotal_kopecks: 0,
          },
        ],
        total_kopecks: 0,
      }).items[0].product,
    ).toBeNull();
  });
  it('retains integer kopecks and authoritative cancellation deadlines', () => {
    expect(readCart(cart).total_kopecks).toBe(200100);
    expect(readOrder(order).cancel_before).toBe(order.cancel_before);
  });
  it.each([0, 101, 1.5, null])('rejects invalid quantity %s', (quantity) => {
    expect(() => readCart({ ...cart, items: [{ ...cart.items[0], quantity }] })).toThrow();
  });
  it('rejects unsafe money, inconsistent totals, unknown statuses and invalid deadlines', () => {
    expect(() => readCart({ ...cart, total_kopecks: 9007199254740992 })).toThrow();
    expect(() => readCart({ ...cart, total_kopecks: 1 })).toThrow();
    expect(() => readOrder({ ...order, status: 'paid' })).toThrow();
    expect(() => readOrder({ ...order, cancel_before: 'invalid' })).toThrow();
  });
});
describe('buyer shared state and write safety', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(buyerAPI, 'favorites').mockResolvedValue([]);
    vi.spyOn(buyerAPI, 'cart').mockResolvedValue(cart);
  });
  it("does not leak a previous user's delayed collection after identity changes", async () => {
    let release!: (value: []) => void;
    vi.mocked(buyerAPI.favorites).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          release = resolve;
        }),
    );
    const store = new BuyerStore();
    store.activate(1);
    store.activate(2);
    await store.refresh();
    release([]);
    await Promise.resolve();
    expect(store.snapshot().owner).toBe(2);
    expect(store.snapshot().favorites).toEqual([]);
  });
  it('updates the shared favorite collection after a successful write', async () => {
    let saved = false;
    vi.mocked(buyerAPI.favorites).mockImplementation(async () =>
      saved ? [{ product_id: 9, product: null }] : [],
    );
    vi.spyOn(buyerAPI, 'favorite').mockImplementation(async (_id, add) => {
      saved = add;
    });
    const store = new BuyerStore();
    store.activate(1);
    await store.refresh();
    await store.toggleFavorite(9);
    expect(store.snapshot().favorites?.[0].product_id).toBe(9);
    await store.removeFavorite(9);
    expect(store.snapshot().favorites).toEqual([]);
  });
  it('refreshes changed prices and requires a fresh confirmation before POST orders', async () => {
    const post = vi.spyOn(buyerAPI, 'checkout').mockResolvedValue(order);
    const store = new BuyerStore();
    store.activate(1);
    await store.refresh();
    const changed = {
      ...cart,
      items: [
        {
          ...cart.items[0],
          product: { ...cart.items[0].product, unit_price_kopecks: 200000 },
          subtotal_kopecks: 400000,
        },
      ],
      total_kopecks: 400000,
    };
    vi.mocked(buyerAPI.cart).mockResolvedValue(changed);
    await expect(store.checkout()).rejects.toThrow('цена изменилась');
    expect(post).not.toHaveBeenCalled();
    expect(store.snapshot().cart?.total_kopecks).toBe(400000);
  });
  it('blocks a second checkout while the first is in flight', async () => {
    let complete!: (value: typeof order) => void;
    const post = vi.spyOn(buyerAPI, 'checkout').mockImplementation(
      () =>
        new Promise((resolve) => {
          complete = resolve;
        }),
    );
    const store = new BuyerStore();
    store.activate(1);
    await store.refresh();
    const first = store.checkout();
    await expect(store.checkout()).rejects.toThrow('Дождитесь');
    await Promise.resolve();
    complete(order);
    await first;
    expect(post).toHaveBeenCalledTimes(1);
  });
  it('does not automatically retry an ambiguous checkout result', async () => {
    const post = vi
      .spyOn(buyerAPI, 'checkout')
      .mockRejectedValue(new ApiError(0, 'network unavailable'));
    const store = new BuyerStore();
    store.activate(1);
    await store.refresh();
    await expect(store.checkout()).rejects.toBeInstanceOf(ApiError);
    expect(store.snapshot().checkoutUncertain).toBe(true);
    await expect(store.checkout()).rejects.toThrow('историю');
    expect(post).toHaveBeenCalledTimes(1);
  });
  it('clears private counts and uncertainty on logout', async () => {
    const store = new BuyerStore();
    store.activate(1);
    await store.refresh();
    store.activate(0);
    expect(store.snapshot()).toMatchObject({
      owner: 0,
      cart: null,
      favorites: null,
      checkoutUncertain: false,
    });
  });
});
