import { useEffect, useSyncExternalStore } from 'react';
import type { ReactNode } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../../auth/Auth';
import { buyerStore } from './store';
import { errorText } from '../../lib/format';
import { Icon, useToast } from '../../components/ui';
export const useBuyer = () =>
  useSyncExternalStore(buyerStore.subscribe, buyerStore.snapshot, buyerStore.snapshot);
export function BuyerProvider({ children }: { children: ReactNode }) {
  const auth = useAuth();
  useEffect(() => {
    buyerStore.activate(auth.status === 'authenticated' ? auth.user!.id : 0);
  }, [auth.status, auth.user?.id]);
  useEffect(() => {
    const channel = 'BroadcastChannel' in window ? new BroadcastChannel('srez-buyer-events') : null;
    buyerStore.onChanged = () => {
      channel?.postMessage('changed');
      window.dispatchEvent(new Event('srez-shopping-updated'));
    };
    if (channel)
      channel.onmessage = (e) => {
        if (e.data === 'changed') {
          void buyerStore.refresh();
          window.dispatchEvent(new Event('srez-shopping-updated'));
        }
      };
    const focus = () => {
      if (document.visibilityState === 'visible') void buyerStore.refresh();
    };
    window.addEventListener('focus', focus);
    document.addEventListener('visibilitychange', focus);
    return () => {
      channel?.close();
      buyerStore.onChanged = () => {};
      window.removeEventListener('focus', focus);
      document.removeEventListener('visibilitychange', focus);
      buyerStore.activate(0);
    };
  }, []);
  return children;
}
export function FavoriteButton({
  productId,
  className = '',
  disabled = false,
}: {
  productId: number;
  className?: string;
  disabled?: boolean;
}) {
  const auth = useAuth(),
    buyer = useBuyer(),
    navigate = useNavigate(),
    location = useLocation(),
    toast = useToast();
  const current = auth.status === 'authenticated' && buyer.owner === auth.user?.id;
  const selected = current && !!buyer.favorites?.some((f) => f.product_id === productId);
  async function toggle() {
    if (auth.status !== 'authenticated') {
      navigate('/login', { state: { from: location.pathname + location.search } });
      return;
    }
    try {
      if (buyer.favoritesError || !buyer.favorites) {
        await buyerStore.refreshFavorites();
        return;
      }
      const added = await buyerStore.toggleFavorite(productId);
      toast(added ? 'Добавлено в избранное' : 'Удалено из избранного');
    } catch (e) {
      toast(errorText(e), true);
    }
  }
  return (
    <button
      className={`favorite-button ${selected ? 'is-active' : ''} ${className}`}
      type="button"
      aria-label={selected ? 'Удалить из избранного' : 'Добавить в избранное'}
      aria-pressed={selected}
      disabled={
        disabled ||
        auth.status === 'checking' ||
        (current && (buyer.busy || buyer.favoritesLoading))
      }
      onClick={() => void toggle()}
    >
      <Icon name="heart" size={19} />
      <span className="sr-only">{buyer.favoritesError ? 'Повторить загрузку избранного' : ''}</span>
    </button>
  );
}
