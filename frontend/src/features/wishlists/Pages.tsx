import { useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useAuth } from '../../auth/Auth';
import {
  Button,
  Confirm,
  Empty,
  Field,
  Icon,
  LoadError,
  Loading,
  Media,
  Notice,
  PageTitle,
  ProductTile,
  useAction,
  useToast,
} from '../../components/ui';
import { Modal } from '../../components/Modal';
import { useClearUnsavedChanges, useUnsavedChanges } from '../../components/UnsavedChanges';
import { api } from '../../lib/api';
import { ApiError } from '../../lib/http';
import { useLoad } from '../../lib/hooks';
import { wishlistAPI, wishlistLimits } from './api';
import type { Wishlist, WishlistItem, WishlistSummary } from './api';

export function WishlistCard({ list, index }: { list: WishlistSummary; index: number }) {
  return (
    <Link className="wishlist-card" to={'/wishlists/' + list.id}>
      <div className="wishlist-card__top">
        <span className="mono">ПОДБОРКА / {String(index + 1).padStart(2, '0')}</span>
        <span
          className={'wishlist-visibility ' + (list.visibility === 'public' ? 'is-public' : '')}
        >
          {list.visibility === 'public' ? 'По ссылке' : 'Личная'}
        </span>
      </div>
      <h2>{list.title}</h2>
      <p>{list.description || 'Вещи, к которым хочется вернуться.'}</p>
      <div className="wishlist-card__bottom">
        <span>{list.item_count} вещей</span>
        <span>
          {new Date(list.updated_at).toLocaleDateString('ru-RU')} <Icon name="diagonal" />
        </span>
      </div>
    </Link>
  );
}
export function WishlistsPage() {
  const auth = useAuth();
  const feed = useLoad((signal) => wishlistAPI.list(signal), [auth.user?.id]);
  const full = (feed.data?.length || 0) >= wishlistLimits.lists;
  return (
    <div className="container page">
      <PageTitle
        title="Мои вишлисты"
        action={
          !full && (
            <Link className="button button--solid" to="/wishlists/new">
              Создать подборку <Icon name="plus" size={18} />
            </Link>
          )
        }
      >
        Личные подборки и списки, которыми можно делиться. Избранное остаётся отдельным.
      </PageTitle>
      {full && <Notice>Можно хранить до 20 вишлистов. Отредактируйте существующий список.</Notice>}
      {feed.loading ? (
        <Loading />
      ) : feed.error ? (
        <LoadError message={feed.error} retry={feed.reload} />
      ) : feed.data?.length ? (
        <div className="wishlist-grid">
          {feed.data.map((list, index) => (
            <WishlistCard key={list.id} list={list} index={index} />
          ))}
        </div>
      ) : (
        <Empty title="У вас пока нет вишлистов" />
      )}
    </div>
  );
}
export function NewWishlistPage() {
  const [title, setTitle] = useState(''),
    [description, setDescription] = useState('');
  const action = useAction(),
    navigate = useNavigate();
  const clean = useUnsavedChanges(
    !!(title || description),
    'новый вишлист',
    JSON.stringify([title, description]),
  );
  return (
    <div className="container page page--narrow">
      <Link className="text-link" to="/wishlists">
        <Icon name="back" size={16} />
        Мои вишлисты
      </Link>
      <PageTitle title="Новый вишлист">
        Новый список виден только тебе. Публичную ссылку можно включить позже.
      </PageTitle>
      <form
        className="panel form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          void action.run(async () => {
            const list = await wishlistAPI.create(title, description);
            clean();
            navigate('/wishlists/' + list.id);
          }, 'Вишлист создан');
        }}
      >
        <fieldset disabled={action.busy}>
          <Field label="Название вишлиста">
            <input
              required
              maxLength={100}
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              placeholder="Например, вне сезона"
            />
          </Field>
          <Field label="Описание" hint="До 2000 символов. Можно оставить пустым.">
            <textarea
              rows={5}
              maxLength={2000}
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              placeholder="Идея, настроение или заметки к подборке"
            />
          </Field>
        </fieldset>
        <Notice error>{action.error}</Notice>
        <Button type="submit" loading={action.busy}>
          Создать вишлист <Icon name="arrow" size={18} />
        </Button>
      </form>
    </div>
  );
}
export function WishlistPage() {
  const { id = '' } = useParams();
  const auth = useAuth(),
    navigate = useNavigate();
  const feed = useLoad((signal) => wishlistAPI.get(id, signal), [id, auth.user?.id]);
  const action = useAction();
  const clearUnsaved = useClearUnsavedChanges();
  const [confirm, setConfirm] = useState<'delete' | 'rotate' | 'revoke' | WishlistItem | null>(
      null,
    ),
    [variant, setVariant] = useState<WishlistItem | null>(null);
  const mutate = async (fn: () => Promise<Wishlist>) => {
    try {
      feed.setData(await fn());
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) await feed.reload();
      throw error;
    }
  };
  if (feed.loading && !feed.data) return <Loading />;
  if (feed.error || !feed.data)
    return (
      <div className="container page">
        <LoadError message={feed.error || 'Вишлист недоступен.'} retry={feed.reload} />
        <Link className="text-link" to="/wishlists">
          Все мои вишлисты <Icon name="arrow" />
        </Link>
      </div>
    );
  const list = feed.data;
  const move = (index: number, direction: number) => {
    const ids = list.items.map((item) => item.id);
    [ids[index], ids[index + direction]] = [ids[index + direction], ids[index]];
    void action.run(() => mutate(() => wishlistAPI.reorder(list.id, ids)), 'Порядок сохранён');
  };
  return (
    <div className="container page">
      <Link className="text-link" to="/wishlists">
        <Icon name="back" size={16} />
        Мои вишлисты
      </Link>
      <PageTitle
        eyebrow={
          list.visibility === 'public' ? 'ПОДБОРКА / ДОСТУП ПО ССЫЛКЕ' : 'ПОДБОРКА / ТОЛЬКО ТЕБЕ'
        }
        title={list.title}
        action={
          <Link className="button button--solid" to="/catalog">
            Добавить вещи <Icon name="plus" size={18} />
          </Link>
        }
      >
        {list.description}
      </PageTitle>
      <div className="wishlist-settings-grid">
        <WishlistSettings key={list.id} list={list} change={feed.setData} />
        <section className="panel share-panel">
          <h2>{list.share_path ? 'Подборка открыта по ссылке' : 'Личное пространство'}</h2>
          <p className="muted">
            {list.share_path
              ? 'Ссылка позволяет смотреть опубликованные товары без входа. Редактировать список можешь только ты.'
              : 'Без публичной ссылки список доступен только твоему аккаунту.'}
          </p>
          {list.share_path ? (
            <>
              <ShareLink path={list.share_path} />
              <div className="button-row">
                <Button
                  variant="outline"
                  disabled={action.busy}
                  onClick={() => setConfirm('rotate')}
                >
                  Перевыпустить ссылку
                </Button>
                <Button variant="ghost" disabled={action.busy} onClick={() => setConfirm('revoke')}>
                  Закрыть доступ
                </Button>
              </div>
            </>
          ) : (
            <Button
              disabled={action.busy}
              onClick={() =>
                void action.run(
                  () => mutate(() => wishlistAPI.share(list.id)),
                  'Публичная ссылка создана',
                )
              }
            >
              Создать публичную ссылку <Icon name="diagonal" size={18} />
            </Button>
          )}
          <small className="muted">
            Локальная ссылка открывается только на компьютере, где запущен SREZ.
          </small>
        </section>
      </div>
      <div className="section-heading wishlist-items-heading">
        <div>
          <h2>
            Вещи <span className="muted">/ {list.items.length}</span>
          </h2>
        </div>
        <Button variant="ghost" disabled={action.busy} onClick={() => setConfirm('delete')}>
          Удалить вишлист <Icon name="trash" size={16} />
        </Button>
      </div>
      <Notice error>{action.error}</Notice>
      {list.items.length ? (
        <div className="product-grid wishlist-product-grid">
          {list.items.map((item, index) => (
            <div key={item.id} className="wishlist-item" data-wishlist-item={item.id}>
              {item.product ? (
                <ProductTile product={item.product} />
              ) : (
                <div className="wishlist-unavailable">
                  <Icon name="image" size={32} />
                  <h3>Товар больше недоступен</h3>
                  <p>Он скрыт или бренд снят с публикации.</p>
                </div>
              )}
              <div className="wishlist-item__details">
                <span>
                  {item.variant
                    ? `${item.variant.color} / ${item.variant.size}`
                    : 'Цвет и размер не выбраны'}
                </span>
                {item.variant && (
                  <small className="muted">
                    {item.variant.stock > 0
                      ? `В наличии: ${item.variant.stock}`
                      : 'Этот вариант закончился'}
                  </small>
                )}
                <Button
                  variant="ghost"
                  disabled={action.busy || !item.product}
                  onClick={() => setVariant(item)}
                >
                  Выбрать вариант
                </Button>
              </div>
              <div className="wishlist-item__actions">
                <button
                  type="button"
                  className="icon-button"
                  aria-label={`Переместить ${item.product?.name || 'товар'} выше`}
                  disabled={action.busy || index === 0}
                  onClick={() => move(index, -1)}
                >
                  <Icon name="up" />
                </button>
                <button
                  type="button"
                  className="icon-button"
                  aria-label={`Переместить ${item.product?.name || 'товар'} ниже`}
                  disabled={action.busy || index === list.items.length - 1}
                  onClick={() => move(index, 1)}
                >
                  <Icon name="down" />
                </button>
                <Button variant="ghost" disabled={action.busy} onClick={() => setConfirm(item)}>
                  Убрать <Icon name="close" size={16} />
                </Button>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <Empty
          title="Место для твоих находок"
          action={
            <Link className="button button--solid" to="/catalog">
              Открыть каталог <Icon name="diagonal" />
            </Link>
          }
        >
          Нажми на закладку в каталоге или карточке товара, чтобы добавить вещь.
        </Empty>
      )}
      <Confirm
        open={confirm !== null}
        title={
          confirm === 'delete'
            ? 'Удалить вишлист?'
            : confirm === 'rotate'
              ? 'Перевыпустить ссылку?'
              : confirm === 'revoke'
                ? 'Закрыть публичный доступ?'
                : 'Убрать вещь из вишлиста?'
        }
        confirmLabel={
          confirm === 'rotate'
            ? 'Перевыпустить'
            : confirm === 'revoke'
              ? 'Закрыть доступ'
              : confirm === 'delete'
                ? 'Удалить вишлист'
                : 'Убрать'
        }
        busy={action.busy}
        onCancel={() => setConfirm(null)}
        onConfirm={() =>
          void action.run(
            async () => {
              if (confirm === 'delete') {
                await wishlistAPI.remove(list.id);
                clearUnsaved();
                setConfirm(null);
                navigate('/wishlists');
                return;
              }
              if (confirm === 'rotate') await mutate(() => wishlistAPI.share(list.id));
              else if (confirm === 'revoke') await mutate(() => wishlistAPI.unshare(list.id));
              else if (confirm) await mutate(() => wishlistAPI.removeItem(list.id, confirm.id));
              setConfirm(null);
            },
            confirm === 'rotate'
              ? 'Старая ссылка отозвана'
              : confirm === 'revoke'
                ? 'Доступ закрыт'
                : 'Изменения сохранены',
          )
        }
      >
        {confirm === 'delete'
          ? 'Список и его элементы будут удалены. Товары, избранное и заказы сохранятся.'
          : confirm === 'rotate' || confirm === 'revoke'
            ? 'Прежняя публичная ссылка перестанет открывать эту подборку.'
            : 'Товар останется в каталоге и других подборках.'}
      </Confirm>
      {variant && (
        <VariantPicker
          list={list.id}
          item={variant}
          close={() => setVariant(null)}
          changed={feed.setData}
        />
      )}
    </div>
  );
}
function WishlistSettings({ list, change }: { list: Wishlist; change: (value: Wishlist) => void }) {
  const [title, setTitle] = useState(list.title),
    [description, setDescription] = useState(list.description);
  const action = useAction();
  const dirty = title !== list.title || description !== list.description;
  const clean = useUnsavedChanges(dirty, 'вишлист', JSON.stringify([title, description]));
  return (
    <form
      className="panel form-stack"
      onSubmit={(event) => {
        event.preventDefault();
        void action.run(async () => {
          const updated = await wishlistAPI.update(list.id, title, description);
          setTitle(updated.title);
          setDescription(updated.description);
          clean();
          change(updated);
        }, 'Вишлист обновлён');
      }}
    >
      <h2>Твоя идея</h2>
      <fieldset disabled={action.busy}>
        <Field label="Название вишлиста">
          <input
            required
            maxLength={100}
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
        </Field>
        <Field label="Описание вишлиста">
          <textarea
            rows={3}
            maxLength={2000}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </Field>
      </fieldset>
      <Notice error>{action.error}</Notice>
      <div className="button-row">
        <Button type="submit" disabled={!dirty} loading={action.busy}>
          Сохранить описание
        </Button>
        {dirty && (
          <Button
            variant="ghost"
            disabled={action.busy}
            onClick={() => {
              setTitle(list.title);
              setDescription(list.description);
              clean();
            }}
          >
            Отменить изменения
          </Button>
        )}
      </div>
    </form>
  );
}
function ShareLink({ path }: { path: string }) {
  const ref = useRef<HTMLInputElement>(null),
    toast = useToast();
  const url = new URL(path, window.location.origin).href;
  return (
    <div className="share-link">
      <Field label="Публичная ссылка">
        <input
          ref={ref}
          readOnly
          value={url}
          spellCheck={false}
          onFocus={(e) => e.target.select()}
        />
      </Field>
      <Button
        variant="outline"
        onClick={() =>
          void (async () => {
            try {
              await navigator.clipboard.writeText(url);
              toast('Ссылка скопирована');
            } catch {
              ref.current?.focus();
              ref.current?.select();
              toast('Ссылка выделена — скопируйте её сочетанием Ctrl+C.');
            }
          })()
        }
      >
        Копировать ссылку <Icon name="copy" size={16} />
      </Button>
    </div>
  );
}
function VariantPicker({
  list,
  item,
  close,
  changed,
}: {
  list: number;
  item: WishlistItem;
  close: () => void;
  changed: (value: Wishlist) => void;
}) {
  const feed = useLoad((signal) => api.product(String(item.product_id), signal), [item.product_id]);
  const [selected, setSelected] = useState(String(item.product_variant_id || ''));
  const action = useAction();
  return (
    <Modal title="Цвет и размер" onClose={close} busy={action.busy}>
      {feed.loading ? (
        <Loading />
      ) : feed.error ? (
        <LoadError message={feed.error} retry={feed.reload} />
      ) : (
        <form
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            void action.run(async () => {
              const updated = await wishlistAPI.variant(
                list,
                item.id,
                selected ? Number(selected) : null,
              );
              changed(updated);
              close();
            }, 'Вариант сохранён');
          }}
        >
          <p className="muted">
            Выбор в вишлисте не резервирует остаток. Недоступный размер можно сохранить на будущее.
          </p>
          <Field label="Вариант товара">
            <select
              value={selected}
              disabled={action.busy}
              onChange={(e) => setSelected(e.target.value)}
            >
              <option value="">Любой цвет и размер</option>
              {feed.data?.colors.flatMap((color) =>
                color.variants.map((v) => (
                  <option key={v.id} value={v.id}>
                    {color.color} / {v.size} — {v.stock > 0 ? `${v.stock} шт.` : 'нет в наличии'}
                  </option>
                )),
              )}
            </select>
          </Field>
          <Notice error>{action.error}</Notice>
          <Button type="submit" loading={action.busy}>
            Сохранить вариант
          </Button>
        </form>
      )}
    </Modal>
  );
}
export function SharedWishlistPage() {
  const { token = '' } = useParams();
  const feed = useLoad((signal) => wishlistAPI.shared(token, signal), [token]);
  const reload = feed.reload;
  useEffect(() => {
    const refresh = () => {
      if (document.visibilityState === 'visible') void reload();
    };
    const interval = setInterval(refresh, 30000);
    window.addEventListener('focus', refresh);
    return () => {
      clearInterval(interval);
      window.removeEventListener('focus', refresh);
    };
  }, [reload]);
  if (feed.loading && !feed.data) return <Loading />;
  if (feed.error || !feed.data)
    return (
      <div className="container page">
        {feed.errorStatus === 404 ? (
          <Empty
            title="Эта подборка недоступна"
            action={
              <Link className="button button--solid" to="/catalog">
                Открыть каталог <Icon name="diagonal" />
              </Link>
            }
          >
            Владелец мог закрыть доступ, перевыпустить ссылку или удалить список.
          </Empty>
        ) : (
          <LoadError message={feed.error || 'Не удалось открыть подборку.'} retry={feed.reload} />
        )}
      </div>
    );
  const list = feed.data;
  return (
    <div className="container page shared-wishlist">
      <section className="shared-wishlist__hero">
        <div>
          <h1>{list.title}</h1>
          {list.description && <p className="shared-wishlist__description">{list.description}</p>}
          <span className="mono">{list.items.length} ВЕЩЕЙ / ПУБЛИЧНАЯ ПОДБОРКА</span>
        </div>
        {!!list.items.length && (
          <div className="shared-wishlist__covers" aria-hidden="true">
            {list.items.slice(0, 3).map((item) => (
              <Media key={item.id} src={item.product?.preview_image_url} alt="" />
            ))}
          </div>
        )}
      </section>
      {list.items.length ? (
        <div className="product-grid">
          {list.items.map((item) => (
            <div key={item.id}>
              <ProductTile product={item.product!} />
              {item.variant && (
                <p className="shared-variant">
                  Выбор в подборке: {item.variant.color} / {item.variant.size}
                  {item.variant.stock === 0 ? ' · сейчас закончился' : ''}
                </p>
              )}
            </div>
          ))}
        </div>
      ) : (
        <Empty title="Подборка ещё собирается">
          Опубликованные вещи появятся здесь, когда владелец добавит их в список.
        </Empty>
      )}
      <div className="shared-wishlist__footer">
        <Link className="button button--outline" to="/wishlists">
          Собрать свою подборку <Icon name="diagonal" />
        </Link>
      </div>
    </div>
  );
}
