import { buyerAPI, cartFingerprint } from './api';
import type { Cart, Favorite } from './types';
import { errorText } from '../../lib/format';
import { ApiError } from '../../lib/http';
type State = {
  owner: number;
  favorites: Favorite[] | null;
  cart: Cart | null;
  favoritesLoading: boolean;
  cartLoading: boolean;
  favoritesError: string;
  cartError: string;
  busy: boolean;
  checkoutUncertain: boolean;
};
const empty = (owner = 0): State => ({
  owner,
  favorites: null,
  cart: null,
  favoritesLoading: false,
  cartLoading: false,
  favoritesError: '',
  cartError: '',
  busy: false,
  checkoutUncertain: false,
});
export class BuyerStore {
  onChanged = () => {};
  private state = empty();
  private listeners = new Set<() => void>();
  private generation = 0;
  private revision = 0;
  private requests = new Set<AbortController>();
  private favoritesFlight: Promise<void> | null = null;
  private cartFlight: Promise<void> | null = null;
  snapshot = () => this.state;
  subscribe = (fn: () => void) => {
    this.listeners.add(fn);
    return () => {
      this.listeners.delete(fn);
    };
  };
  private emit(patch: Partial<State>) {
    this.state = { ...this.state, ...patch };
    for (const fn of this.listeners) fn();
  }
  activate(owner: number) {
    if (this.state.owner === owner) return;
    this.generation++;
    this.revision++;
    for (const c of this.requests) c.abort();
    this.requests.clear();
    this.favoritesFlight = null;
    this.cartFlight = null;
    this.state = empty(owner);
    this.emit({});
    if (owner) void this.refresh();
  }
  private current(generation: number) {
    if (this.generation !== generation || !this.state.owner)
      throw new ApiError(401, 'session changed');
  }
  private load(kind: 'favorites' | 'cart', force = false): Promise<void> {
    if (!this.state.owner) return Promise.resolve();
    const active = kind === 'favorites' ? this.favoritesFlight : this.cartFlight;
    if (active && !force) return active;
    const gen = this.generation,
      rev = this.revision,
      c = new AbortController();
    this.requests.add(c);
    this.emit(
      kind === 'favorites'
        ? { favoritesLoading: true, favoritesError: '' }
        : { cartLoading: true, cartError: '' },
    );
    let flight!: Promise<void>;
    flight = (async () => {
      try {
        const result = await (kind === 'favorites'
          ? buyerAPI.favorites(c.signal)
          : buyerAPI.cart(c.signal));
        if (gen === this.generation && rev === this.revision && !c.signal.aborted)
          this.emit(
            kind === 'favorites' ? { favorites: result as Favorite[] } : { cart: result as Cart },
          );
      } catch (e) {
        if (gen === this.generation && rev === this.revision && !c.signal.aborted)
          this.emit(
            kind === 'favorites' ? { favoritesError: errorText(e) } : { cartError: errorText(e) },
          );
      } finally {
        this.requests.delete(c);
        if (gen === this.generation) {
          if (kind === 'favorites' && this.favoritesFlight === flight) {
            this.favoritesFlight = null;
            this.emit({ favoritesLoading: false });
          }
          if (kind === 'cart' && this.cartFlight === flight) {
            this.cartFlight = null;
            this.emit({ cartLoading: false });
          }
        }
      }
    })();
    if (kind === 'favorites') this.favoritesFlight = flight;
    else this.cartFlight = flight;
    return flight;
  }
  refreshFavorites = () => this.load('favorites', true);
  refreshCart = () => this.load('cart', true);
  refresh = () =>
    Promise.all([this.load('favorites', true), this.load('cart', true)]).then(() => {});
  acknowledgeCheckout = async () => {
    await this.refreshCart();
    if (!this.state.cartError) this.emit({ checkoutUncertain: false });
  };
  private async change<T>(operation: () => Promise<T>): Promise<T> {
    if (this.state.busy) throw new Error('Дождитесь завершения предыдущего действия.');
    const gen = this.generation;
    this.current(gen);
    this.revision++;
    this.emit({ busy: true });
    try {
      const value = await operation();
      this.current(gen);
      this.onChanged();
      return value;
    } finally {
      if (gen === this.generation) {
        this.revision++;
        await this.refresh();
        if (gen === this.generation) this.emit({ busy: false });
      }
    }
  }
  toggleFavorite = (id: number) =>
    this.change(async () => {
      if (!this.state.favorites || this.state.favoritesError)
        throw new Error('Сначала обновите избранное.');
      const add = !this.state.favorites.some((f) => f.product_id === id);
      await buyerAPI.favorite(id, add);
      return add;
    });
  removeFavorite = (id: number) => this.change(() => buyerAPI.favorite(id, false));
  addCart = (id: number, quantity: number) =>
    this.change(async () => {
      const value = await buyerAPI.addCart(id, quantity);
      return value;
    });
  setQuantity = (id: number, quantity: number) =>
    this.change(() => buyerAPI.setQuantity(id, quantity));
  removeCart = (id: number) => this.change(() => buyerAPI.removeCart(id));
  checkout = () =>
    this.change(async () => {
      const gen = this.generation;
      if (this.state.checkoutUncertain)
        throw new Error(
          'Сначала проверьте историю заказов: результат предыдущего запроса неизвестен.',
        );
      const before = this.state.cart;
      if (!before || this.state.cartError) throw new Error('Сначала обновите корзину.');
      const fresh = await buyerAPI.cart();
      this.current(gen);
      this.emit({ cart: fresh });
      if (cartFingerprint(before) !== cartFingerprint(fresh))
        throw new Error(
          'Корзина или цена изменилась. Проверьте обновлённые позиции и сумму перед резервированием.',
        );
      if (!fresh.items.length || fresh.items.some((i) => i.availability !== 'available'))
        throw new Error('Для резерва нужны доступные позиции корзины.');
      try {
        return await buyerAPI.checkout();
      } catch (e) {
        if (gen === this.generation && e instanceof ApiError && (e.status === 0 || e.status >= 500))
          this.emit({ checkoutUncertain: true });
        throw e;
      }
    });
  cancel = (id: number) => this.change(() => buyerAPI.cancel(id));
}
export const buyerStore = new BuyerStore();
