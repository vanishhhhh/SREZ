import { useEffect, useState } from 'react';
import type { FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  Button,
  Confirm,
  Empty,
  Icon,
  LoadError,
  Loading,
  Media,
  Notice,
  PageTitle,
  ProductTile,
  useAction,
} from '../../components/ui';
import { useBuyer } from './Buyer';
import { buyerStore } from './store';
import { buyerAPI } from './api';
import type { CartLine, Order } from './types';
import { money } from '../../lib/format';
import { useLoad } from '../../lib/hooks';
import { ApiError } from '../../lib/http';

export function FavoritesPage() {
  const state = useBuyer();
  const action = useAction();
  return (
    <div className="container page">
      <PageTitle eyebrow="ЛИЧНЫЙ ВЫБОР" title="Избранное">
        Вещи, к которым хочется вернуться.
      </PageTitle>
      {state.favoritesError ? (
        <LoadError message={state.favoritesError} retry={buyerStore.refreshFavorites} />
      ) : state.favorites === null ? (
        <Loading text="Загружаем избранное…" />
      ) : state.favorites.length ? (
        <div className="product-grid">
          {state.favorites.map((f) =>
            f.product ? (
              <ProductTile key={f.product_id} product={f.product} />
            ) : (
              <article className="unavailable-favorite" key={f.product_id}>
                <Media alt="Недоступный товар" />
                <h2>Товар №{f.product_id}</h2>
                <p>Бренд снял вещь с публикации. Сохранённую запись можно удалить.</p>
                <Button
                  variant="outline"
                  disabled={state.busy}
                  onClick={() =>
                    void action.run(
                      () => buyerStore.removeFavorite(f.product_id),
                      'Удалено из избранного',
                    )
                  }
                >
                  Удалить из избранного
                </Button>
              </article>
            ),
          )}
        </div>
      ) : (
        <Empty
          title="Здесь будет ваш выбор"
          action={
            <Link className="button button--solid" to="/catalog">
              Найти своё <Icon name="arrow" />
            </Link>
          }
        >
          Сохраняйте вещи кнопкой с сердцем в каталоге или на странице товара.
        </Empty>
      )}
    </div>
  );
}
function Quantity({ line }: { line: CartLine }) {
  const [value, setValue] = useState(String(line.quantity));
  const state = useBuyer();
  const action = useAction();
  useEffect(() => setValue(String(line.quantity)), [line.quantity]);
  function submit(event: FormEvent) {
    event.preventDefault();
    void action.run(async () => {
      if (!/^\d+$/.test(value) || Number(value) < 1 || Number(value) > 100)
        throw new Error('Количество должно быть целым числом от 1 до 100.');
      await buyerStore.setQuantity(line.id, Number(value));
    }, 'Количество обновлено');
  }
  return (
    <form className="quantity-form" onSubmit={submit}>
      <label>
        <span className="sr-only">Количество {line.product?.name || 'позиции ' + line.id}</span>
        <input
          aria-label={'Количество ' + line.id}
          inputMode="numeric"
          type="number"
          min="1"
          max="100"
          required
          value={value}
          disabled={state.busy || line.availability === 'unavailable'}
          onChange={(e) => setValue(e.target.value)}
        />
      </label>
      <Button
        type="submit"
        variant="outline"
        disabled={
          state.busy || line.availability === 'unavailable' || Number(value) === line.quantity
        }
      >
        Обновить
      </Button>
      <Notice error>{action.error}</Notice>
    </form>
  );
}
export function CartPage() {
  const state = useBuyer();
  const action = useAction();
  const navigate = useNavigate();
  const cart = state.cart;
  const invalid = cart?.items.some((i) => i.availability !== 'available');
  return (
    <div className="container page">
      <PageTitle eyebrow="СОБРАНО ВАМИ" title="Корзина">
        Добавление в корзину не резервирует остатки.
      </PageTitle>
      {state.checkoutUncertain && (
        <Notice error>
          Результат запроса неизвестен. Резерв мог быть создан.{' '}
          <Link className="text-link" to="/orders">
            Проверьте историю заказов
          </Link>{' '}
          перед повторным оформлением.
        </Notice>
      )}
      {state.cartError && <LoadError message={state.cartError} retry={buyerStore.refreshCart} />}
      {cart === null ? (
        !state.cartError && <Loading text="Обновляем корзину…" />
      ) : !cart.items.length ? (
        <Empty
          title="Пока ни одной вещи"
          action={
            <Link className="button button--solid" to="/catalog">
              Перейти в каталог <Icon name="arrow" />
            </Link>
          }
        >
          Выберите цвет и доступный размер на странице товара.
        </Empty>
      ) : (
        <div className="cart-layout">
          <section className="cart-lines" aria-label="Позиции корзины">
            {cart.items.map((line) => (
              <article className="cart-line" key={line.id}>
                <Media
                  src={line.product?.preview_image_url}
                  alt={line.product?.name || 'Товар недоступен'}
                />
                <div className="cart-line__info">
                  {line.product ? (
                    <>
                      <Link to={'/products/' + line.product.product_id}>
                        <h2>{line.product.name}</h2>
                      </Link>
                      <p>
                        {line.product.color} / {line.product.size}
                      </p>
                      <p>{money(line.product.unit_price_kopecks)} за штуку</p>
                    </>
                  ) : (
                    <>
                      <h2>Товар недоступен</h2>
                      <p>Вариант удалён или вещь снята с публикации.</p>
                    </>
                  )}
                  {line.availability === 'insufficient_stock' && (
                    <Notice error>
                      Недостаточно свободных единиц. Уменьшите количество или удалите позицию.
                    </Notice>
                  )}
                  <Quantity line={line} />
                  <button
                    className="text-button"
                    disabled={state.busy}
                    onClick={() =>
                      void action.run(() => buyerStore.removeCart(line.id), 'Позиция удалена')
                    }
                  >
                    Удалить позицию
                  </button>
                </div>
                <strong>
                  {line.availability === 'available'
                    ? money(line.subtotal_kopecks)
                    : 'Не входит в итог'}
                </strong>
              </article>
            ))}
          </section>
          <aside className="cart-summary">
            <p className="eyebrow">ВАШ ВЫБОР</p>
            <div className="cart-total">
              <span>Итого</span>
              <strong>{money(cart.total_kopecks)}</strong>
            </div>
            <p>Сумма рассчитана сервером. Перед оформлением мы обновим цены и доступность.</p>
            <Notice>
              Это резерв на 15 минут. Деньги не списываются — настоящая оплата пока не подключена.
            </Notice>
            {invalid && (
              <Notice error>
                Резерв недоступен, пока в корзине есть проблемные позиции. Итог включает только
                доступные вещи.
              </Notice>
            )}
            <Notice error>{action.error}</Notice>
            {state.checkoutUncertain ? (
              <Notice error>
                Результат запроса неизвестен. Проверьте историю, прежде чем создавать другой резерв.
              </Notice>
            ) : (
              <Button
                className="full-width"
                loading={state.busy}
                disabled={!!invalid || !!state.cartError || state.cartLoading}
                onClick={() =>
                  void action.run(async () => {
                    const order = await buyerStore.checkout();
                    navigate('/orders/' + order.id);
                  }, 'Неоплаченный резерв создан')
                }
              >
                Зарезервировать на 15 минут <Icon name="arrow" />
              </Button>
            )}
            <Link className="text-link" to="/orders">
              Проверить историю заказов <Icon name="diagonal" />
            </Link>
          </aside>
        </div>
      )}
    </div>
  );
}
export const orderStatus = (status: Order['status']) =>
  ({ unpaid: 'Неоплаченный резерв', cancelled: 'Резерв отменён', expired: 'Срок резерва истёк' })[
    status
  ];
export const orderDate = (value: string) =>
  new Intl.DateTimeFormat('ru-RU', { dateStyle: 'medium', timeStyle: 'short' }).format(
    new Date(value),
  );
function useOrderClock(active: boolean, reload: () => Promise<void>) {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    let inFlight = false;
    const refresh = () => {
      if (document.visibilityState === 'visible' && !inFlight) {
        inFlight = true;
        void reload().finally(() => {
          inFlight = false;
        });
      }
    };
    window.addEventListener('focus', refresh);
    window.addEventListener('srez-shopping-updated', refresh);
    document.addEventListener('visibilitychange', refresh);
    const clock = active ? setInterval(() => setNow(Date.now()), 1000) : undefined;
    const poll = active ? setInterval(refresh, 5000) : undefined;
    return () => {
      clearInterval(clock);
      clearInterval(poll);
      window.removeEventListener('focus', refresh);
      window.removeEventListener('srez-shopping-updated', refresh);
      document.removeEventListener('visibilitychange', refresh);
    };
  }, [active, reload]);
  return now;
}
function Reservation({ order, now }: { order: Order; now: number }) {
  if (order.status !== 'unpaid')
    return (
      <p>
        {order.status === 'cancelled' && order.cancelled_at
          ? 'Отменён ' + orderDate(order.cancelled_at)
          : order.expired_at
            ? 'Освобождён ' + orderDate(order.expired_at)
            : orderStatus(order.status)}
      </p>
    );
  const seconds = Math.max(0, Math.ceil((Date.parse(order.cancel_before) - now) / 1000));
  return (
    <div className="reservation">
      <strong>
        {seconds
          ? `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
          : 'Проверяем освобождение резерва…'}
      </strong>
      <p>
        Резерв до {orderDate(order.cancel_before)}.{' '}
        {seconds
          ? 'После срока сервер вернёт свободный остаток.'
          : 'Ожидаем подтверждение сервера; статус обновляется автоматически.'}
      </p>
    </div>
  );
}
export function OrdersPage() {
  const buyer = useBuyer();
  const orders = useLoad((signal) => buyerAPI.orders(signal), []);
  const now = useOrderClock(!!orders.data?.some((o) => o.status === 'unpaid'), orders.reload);
  return (
    <div className="container page">
      <PageTitle eyebrow="ИСТОРИЯ ВАШЕГО ВЫБОРА" title="Мои заказы">
        Здесь хранятся неоплаченные резервы и их история.
      </PageTitle>
      {buyer.checkoutUncertain && (
        <Notice>
          Результат последнего оформления был неизвестен. Проверьте новые заказы перед повторным
          резервированием.{' '}
          {orders.data && (
            <Button variant="outline" onClick={buyerStore.acknowledgeCheckout}>
              Историю проверил — обновить корзину
            </Button>
          )}
        </Notice>
      )}
      {orders.loading && !orders.data ? (
        <Loading />
      ) : orders.error ? (
        <LoadError message={orders.error} retry={orders.reload} />
      ) : orders.data?.length ? (
        <div className="order-list">
          {orders.data.map((order) => (
            <article className="order-card" key={order.id}>
              <div>
                <span className={'order-status order-status--' + order.status}>
                  {orderStatus(order.status)}
                </span>
                <h2>
                  <Link to={'/orders/' + order.id}>
                    Заказ №{order.id} <Icon name="diagonal" />
                  </Link>
                </h2>
                <p>
                  {orderDate(order.created_at)} · {order.items.length} поз.
                </p>
                <p>{order.items.map((i) => i.product_name).join(' · ')}</p>
              </div>
              <div>
                <strong>{money(order.total_kopecks)}</strong>
                <Reservation order={order} now={now} />
                <Link className="text-link" to={'/orders/' + order.id}>
                  Подробнее <Icon name="arrow" />
                </Link>
              </div>
            </article>
          ))}
        </div>
      ) : (
        <Empty
          title="История ещё впереди"
          action={
            <Link className="button button--solid" to="/catalog">
              Выбрать вещи <Icon name="arrow" />
            </Link>
          }
        >
          Ваши резервы появятся здесь после оформления корзины.
        </Empty>
      )}
    </div>
  );
}
export function OrderPage() {
  const { orderId = '' } = useParams();
  const order = useLoad((signal) => buyerAPI.order(orderId, signal), [orderId]);
  const action = useAction();
  const [confirm, setConfirm] = useState(false);
  const now = useOrderClock(order.data?.status === 'unpaid', order.reload);
  useEffect(() => setConfirm(false), [orderId]);
  const value = order.data;
  if (order.loading && !value) return <Loading text="Загружаем заказ…" />;
  if (order.errorStatus === 404)
    return (
      <div className="container page">
        <Empty
          title="Заказ не найден"
          action={
            <Link className="button button--outline" to="/orders">
              Мои заказы
            </Link>
          }
        >
          Заказ отсутствует или принадлежит другому аккаунту.
        </Empty>
      </div>
    );
  if (order.error || !value)
    return (
      <div className="container page">
        <LoadError message={order.error || 'Заказ недоступен.'} retry={order.reload} />
      </div>
    );
  const canCancel = value.status === 'unpaid' && now < Date.parse(value.cancel_before);
  return (
    <div className="container page">
      <Link className="text-link" to="/orders">
        <Icon name="back" />
        Все заказы
      </Link>
      <PageTitle eyebrow={orderStatus(value.status)} title={'Заказ №' + value.id}>
        {orderDate(value.created_at)}
      </PageTitle>
      <div className="cart-layout">
        <section>
          <div className="order-lines">
            {value.items.map((line) => (
              <article key={line.product_variant_id}>
                <div>
                  <h2>{line.product_name}</h2>
                  <p>
                    {line.color} / {line.size} · {line.quantity} шт.
                  </p>
                  <p>{money(line.unit_price_kopecks)} за штуку</p>
                </div>
                <strong>{money(line.subtotal_kopecks)}</strong>
              </article>
            ))}
          </div>
          <p className="muted">
            В заказе сохранён снимок названий, вариантов и цен на момент оформления.
          </p>
        </section>
        <aside className="cart-summary">
          <span className={'order-status order-status--' + value.status}>
            {orderStatus(value.status)}
          </span>
          <div className="cart-total">
            <span>Итого</span>
            <strong>{money(value.total_kopecks)}</strong>
          </div>
          <Notice>
            Деньги не списаны. Это неоплаченный резерв, настоящая оплата пока не подключена.
          </Notice>
          <Reservation order={value} now={now} />
          <Notice error>{action.error}</Notice>
          {canCancel && (
            <Button
              variant="outline"
              className="full-width"
              disabled={action.busy}
              onClick={() => setConfirm(true)}
            >
              Отменить резерв
            </Button>
          )}
          <Button variant="ghost" disabled={action.busy} onClick={order.reload}>
            Обновить статус
          </Button>
        </aside>
      </div>
      <Confirm
        open={confirm}
        title="Отменить неоплаченный резерв?"
        confirmLabel="Отменить резерв"
        busy={action.busy}
        onCancel={() => setConfirm(false)}
        onConfirm={() =>
          void action.run(async () => {
            try {
              await buyerStore.cancel(value.id);
            } catch (error) {
              if (error instanceof ApiError && error.status === 409) {
                setConfirm(false);
                await order.reload();
              }
              throw error;
            }
            setConfirm(false);
            await order.reload();
          }, 'Резерв отменён')
        }
      >
        Сервер освободит зарезервированные единицы товара. Отмену нельзя отменить — при
        необходимости создайте новый резерв из корзины.
      </Confirm>
    </div>
  );
}
