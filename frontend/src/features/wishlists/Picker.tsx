import { createContext, useContext, useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../../auth/Auth';
import { Button, Field, Icon, LoadError, Loading, Notice, useAction } from '../../components/ui';
import { Modal } from '../../components/Modal';
import { useLoad } from '../../lib/hooks';
import { wishlistAPI, wishlistLimits } from './api';

type Selection = { productId: number; variantId: number | null; name: string; owner: number };
const PickerContext = createContext<
  ((productId: number, variantId: number | null, name: string) => void) | null
>(null);
export function WishlistProvider({ children }: { children: ReactNode }) {
  const auth = useAuth(),
    navigate = useNavigate(),
    location = useLocation();
  const [selection, setSelection] = useState<Selection | null>(null);
  const owner = auth.status === 'authenticated' ? auth.user?.id : undefined;
  useEffect(() => {
    if (selection && selection.owner !== owner) setSelection(null);
  }, [owner, selection]);
  return (
    <PickerContext.Provider
      value={(productId, variantId, name) => {
        if (!owner) {
          navigate(auth.status === 'unavailable' ? '/unavailable' : '/login', {
            state: { from: location.pathname + location.search },
          });
          return;
        }
        setSelection({ productId, variantId, name, owner });
      }}
    >
      {children}
      {selection && selection.owner === owner && (
        <Picker
          key={selection.owner + ':' + selection.productId}
          selection={selection}
          close={() => setSelection(null)}
        />
      )}
    </PickerContext.Provider>
  );
}
export function WishlistButton({
  productId,
  variantId = null,
  name = '',
  compact = false,
}: {
  productId: number;
  variantId?: number | null;
  name?: string;
  compact?: boolean;
}) {
  const open = useContext(PickerContext),
    auth = useAuth();
  if (!open) throw Error('Wishlist picker provider missing');
  return (
    <button
      type="button"
      className={
        compact ? 'wishlist-save wishlist-save--compact' : 'button button--outline wishlist-save'
      }
      aria-label={name ? `Добавить ${name} в вишлист` : 'Добавить в вишлист'}
      title="Добавить в вишлист"
      disabled={auth.status === 'checking'}
      onClick={() => open(productId, variantId, name)}
    >
      <Icon name="bookmark" size={18} />
      {!compact && <span>В вишлист</span>}
    </button>
  );
}
function Picker({ selection, close }: { selection: Selection; close: () => void }) {
  const feed = useLoad((signal) => wishlistAPI.list(signal), [selection.owner]);
  const [selected, setSelected] = useState('');
  const [creating, setCreating] = useState(false);
  const [title, setTitle] = useState('');
  const action = useAction();
  useEffect(() => {
    if (feed.data?.length && !selected) setSelected(String(feed.data[0].id));
  }, [feed.data, selected]);
  const create = creating || feed.data?.length === 0;
  const submit = () =>
    void action.run(async () => {
      let id = Number(selected);
      if (create) {
        if (!title.trim()) throw Error('Назовите новый вишлист.');
        const made = await wishlistAPI.create(title, '');
        id = made.id;
        setSelected(String(id));
        setCreating(false);
        setTitle('');
        await feed.reload();
      }
      if (!Number.isSafeInteger(id) || id <= 0) throw Error('Выберите вишлист.');
      await wishlistAPI.add(id, selection.productId, selection.variantId);
      close();
    }, 'Товар добавлен в вишлист');
  return (
    <Modal title="Сохранить в подборку" onClose={close} busy={action.busy}>
      <p className="muted">
        {selection.name || 'Выбранная вещь'}
        {selection.variantId
          ? ' — с выбранным цветом и размером.'
          : '. Размер можно выбрать позже.'}
      </p>
      {feed.loading ? (
        <Loading />
      ) : feed.error ? (
        <LoadError message={feed.error} retry={feed.reload} />
      ) : (
        <form
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          {!create && (
            <Field label="Вишлист">
              <select
                value={selected}
                onChange={(event) => setSelected(event.target.value)}
                disabled={action.busy}
              >
                {feed.data?.map((list) => (
                  <option key={list.id} value={list.id}>
                    {list.title} · {list.item_count}/{wishlistLimits.items}
                  </option>
                ))}
              </select>
            </Field>
          )}
          {create && (
            <Field label="Название нового вишлиста">
              <input
                required
                maxLength={100}
                value={title}
                disabled={action.busy}
                placeholder="Например, осенняя коллекция"
                onChange={(event) => setTitle(event.target.value)}
              />
            </Field>
          )}
          <Notice error>{action.error}</Notice>
          <div className="button-row">
            <Button type="submit" loading={action.busy}>
              Сохранить <Icon name="bookmark" size={16} />
            </Button>
            {!!feed.data?.length && (
              <Button
                variant="ghost"
                disabled={action.busy || (!create && feed.data.length >= wishlistLimits.lists)}
                onClick={() => setCreating(!creating)}
              >
                {creating ? 'Выбрать существующий' : 'Создать новый список'}
              </Button>
            )}
          </div>
          <Link
            className="text-link"
            to="/wishlists"
            onClick={(event) => {
              if (action.busy) event.preventDefault();
              else close();
            }}
          >
            Все мои вишлисты <Icon name="arrow" size={16} />
          </Link>
        </form>
      )}
    </Modal>
  );
}
