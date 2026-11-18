import {
  cloneElement,
  createContext,
  isValidElement,
  useCallback,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
} from 'react';
import type { ButtonHTMLAttributes, ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { errorText, initials, money, statusName } from '../lib/format';
import type { ProductCard } from '../lib/types';
import { useImage } from '../features/media/useImage';
import { FavoriteButton } from '../features/buyer/Buyer';
import { WishlistButton } from '../features/wishlists/Picker';

const paths: Record<string, ReactNode> = {
  bookmark: <path d="M6 3h12v18l-6-4-6 4V3Z" />,
  copy: (
    <>
      <rect x="8" y="8" width="12" height="13" rx="1" />
      <path d="M16 8V3H3v13h5" />
    </>
  ),
  heart: (
    <path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1.1-1.1a5.5 5.5 0 0 0-7.8 7.8L12 21l8.8-8.6a5.5 5.5 0 0 0 0-7.8Z" />
  ),
  bag: (
    <>
      <path d="M5 7h14l1 14H4L5 7Z" />
      <path d="M9 7V5a3 3 0 0 1 6 0v2" />
    </>
  ),
  arrow: (
    <>
      <path d="M4 12h15M13 5l7 7-7 7" />
    </>
  ),
  diagonal: (
    <>
      <path d="M5 19 19 5M5 5h14v14" />
    </>
  ),
  plus: <path d="M12 4v16M4 12h16" />,
  close: <path d="m5 5 14 14M5 19 19 5" />,
  menu: <path d="M3 7h18M3 12h18M3 17h18" />,
  user: (
    <>
      <circle cx="12" cy="8" r="4" />
      <path d="M4 21v-2a8 8 0 0 1 16 0v2" />
    </>
  ),
  trash: (
    <>
      <path d="M3 6h18M9 6V3h6v3M6 6l1 15h10l1-15M10 10v7M14 10v7" />
    </>
  ),
  check: <path d="m4 12 5 5L20 6" />,
  back: <path d="M20 12H4m7-7-7 7 7 7" />,
  eye: (
    <>
      <path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12Z" />
      <circle cx="12" cy="12" r="3" />
    </>
  ),
  up: <path d="m6 14 6-6 6 6" />,
  down: <path d="m6 10 6 6 6-6" />,
  logout: (
    <>
      <path d="M9 4H4v16h5M10 12h11m-5-5 5 5-5 5" />
    </>
  ),
  image: (
    <>
      <rect x="3" y="3" width="18" height="18" rx="1" />
      <circle cx="8" cy="8" r="1.5" />
      <path d="m3 17 6-6 4 4 3-3 5 5" />
    </>
  ),
};
export function Icon({ name = 'arrow', size = 20 }: { name?: string; size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {paths[name] || paths.arrow}
    </svg>
  );
}
export function Button({
  children,
  loading,
  variant = 'solid',
  className = '',
  type = 'button',
  disabled,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  loading?: boolean;
  variant?: 'solid' | 'outline' | 'ghost' | 'danger';
}) {
  return (
    <button
      {...props}
      type={type}
      disabled={disabled || loading}
      className={`button button--${variant} ${className}`}
      aria-busy={loading || undefined}
    >
      {loading && <span className="spinner" />}
      {children}
    </button>
  );
}
export function Field({
  label,
  hint,
  children,
  className = '',
}: {
  label: string;
  hint?: string;
  children: ReactNode;
  className?: string;
}) {
  const id = useId();
  const element = isValidElement<{ id?: string; 'aria-describedby'?: string }>(children)
    ? children
    : null;
  const native =
    element &&
    typeof element.type === 'string' &&
    ['input', 'select', 'textarea'].includes(element.type);
  const controlID = element?.props.id || id;
  const hintID = id + '-hint';
  return (
    <div className={`field ${className}`}>
      {native && element ? (
        <>
          <label className="field__label" htmlFor={controlID}>
            {label}
          </label>
          {cloneElement(element, {
            id: controlID,
            'aria-describedby':
              [element.props['aria-describedby'], hint ? hintID : ''].filter(Boolean).join(' ') ||
              undefined,
          })}
        </>
      ) : (
        <label className="field__compound">
          <span className="field__label">{label}</span>
          {children}
        </label>
      )}
      {hint && (
        <span id={hintID} className="field__hint">
          {hint}
        </span>
      )}
    </div>
  );
}
export function Notice({ children, error = false }: { children?: ReactNode; error?: boolean }) {
  return children ? (
    <div className={`notice ${error ? 'notice--error' : ''}`} role={error ? 'alert' : 'status'}>
      {children}
    </div>
  ) : null;
}
export function Loading({ text = 'Загружаем…' }: { text?: string }) {
  return (
    <div className="page-loading" role="status">
      <span className="spinner" />
      {text}
    </div>
  );
}
export function LoadError({ message, retry }: { message: string; retry: () => void }) {
  return (
    <div className="load-error">
      <p>{message}</p>
      <Button variant="outline" onClick={retry}>
        Попробовать снова
      </Button>
    </div>
  );
}
export function Empty({
  title,
  children,
  action,
}: {
  title: string;
  children?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="empty-state">
      <span className="empty-state__mark">[ — ]</span>
      <h2>{title}</h2>
      {children && <p>{children}</p>}
      {action}
    </div>
  );
}
export function PageTitle({
  eyebrow,
  title,
  children,
  action,
}: {
  eyebrow?: string;
  title: string;
  children?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="page-heading">
      <div>
        {eyebrow && <p className="eyebrow">{eyebrow}</p>}
        <h1>{title}</h1>
        {children && <p className="page-heading__description">{children}</p>}
      </div>
      {action}
    </div>
  );
}
export function Status({ value }: { value: string }) {
  return (
    <span className={`status status--${value}`}>
      <i />
      {statusName(value)}
    </span>
  );
}
export function Media({
  src,
  alt,
  className = '',
  eager = false,
  authenticated = false,
}: {
  src?: string;
  alt: string;
  className?: string;
  eager?: boolean;
  authenticated?: boolean;
}) {
  const loaded = useImage(src, authenticated);
  const resolved = loaded.src;
  if (loaded.loading)
    return (
      <div className={`media ${className}`} data-media-src={src}>
        <span className="media__loading skeleton" aria-label="Загружаем фото" />
      </div>
    );
  return (
    <MediaFrame
      original={src}
      key={resolved || src || ''}
      src={resolved}
      supplied={!!src}
      alt={alt}
      className={className}
      eager={eager}
    />
  );
}
function MediaFrame({
  src,
  original,
  supplied,
  alt,
  className,
  eager,
}: {
  src?: string;
  original?: string;
  supplied: boolean;
  alt: string;
  className: string;
  eager: boolean;
}) {
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading');
  return (
    <div className={`media ${className}`} data-media-src={original}>
      {src && state !== 'error' ? (
        <>
          {state === 'loading' && <span className="media__loading skeleton" aria-hidden="true" />}
          <img
            src={src}
            alt={alt}
            loading={eager ? 'eager' : 'lazy'}
            onLoad={() => setState('ready')}
            onError={() => setState('error')}
            referrerPolicy="no-referrer"
          />
        </>
      ) : (
        <div
          className="media__fallback"
          role="img"
          aria-label={alt + (supplied ? ' — фото недоступно' : ' — без фотографии')}
        >
          <Icon name="image" size={32} />
          <span>{supplied ? 'Фото недоступно' : 'Без фотографии'}</span>
        </div>
      )}
    </div>
  );
}
export function ProductTile({ product }: { product: ProductCard }) {
  return (
    <article className="product-tile">
      <Link className="product-tile__link" to={`/products/${product.id}`}>
        <div className="product-tile__visual">
          <Media src={product.preview_image_url} alt={product.name} />
          {!product.is_available && <span className="product-tile__sold">Нет в наличии</span>}
        </div>
        <div className="product-tile__meta">
          <span>{product.brand_name}</span>
          <strong>{money(product.price_kopecks)}</strong>
        </div>
        <h3>{product.name}</h3>
        <div className="product-tile__sizes">
          <span>
            {product.available_sizes.length
              ? product.available_sizes.slice(0, 6).join(' / ')
              : 'Размеры недоступны'}
          </span>
          {product.available_colors.length > 0 && (
            <span>{product.available_colors.join(' · ')}</span>
          )}
        </div>
      </Link>
      <FavoriteButton productId={product.id} className="product-tile__favorite" />
      <WishlistButton productId={product.id} name={product.name} compact />
    </article>
  );
}
type ToastFunction = (message: string, error?: boolean) => void;
const ToastContext = createContext<ToastFunction>(() => {});
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<{ message: string; error: boolean } | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );
  const notify: ToastFunction = (message, error = false) => {
    if (timer.current) clearTimeout(timer.current);
    setToast({ message, error });
    timer.current = setTimeout(() => setToast(null), 6500);
  };
  return (
    <ToastContext.Provider value={notify}>
      {children}
      {toast && (
        <div
          className={`toast ${toast.error ? 'toast--error' : ''}`}
          role={toast.error ? 'alert' : 'status'}
        >
          <Icon name={toast.error ? 'close' : 'check'} />
          <span>{toast.message}</span>
          <button aria-label="Закрыть уведомление" onClick={() => setToast(null)}>
            <Icon name="close" size={16} />
          </button>
        </div>
      )}
    </ToastContext.Provider>
  );
}
export const useToast = () => useContext(ToastContext);
export function useAction() {
  const lock = useRef(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const clearError = useCallback(() => setError(''), []);
  const toast = useToast();
  const run = async (fn: () => Promise<void>, success?: string) => {
    if (lock.current) return false;
    lock.current = true;
    setBusy(true);
    setError('');
    try {
      await fn();
      if (success) toast(success);
      return true;
    } catch (err) {
      const text = errorText(err);
      setError(text);
      toast(text, true);
      return false;
    } finally {
      lock.current = false;
      setBusy(false);
    }
  };
  return { busy, error, run, clearError };
}
export function Confirm({
  open,
  title,
  children,
  confirmLabel = 'Удалить',
  busy,
  onCancel,
  onConfirm,
}: {
  open: boolean;
  title: string;
  children: ReactNode;
  confirmLabel?: string;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const id = useId();
  useEffect(() => {
    if (open && !ref.current?.open) ref.current?.showModal();
    if (!open && ref.current?.open) ref.current.close();
  }, [open]);
  return (
    <dialog
      ref={ref}
      className="confirm-dialog"
      aria-labelledby={id}
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) onCancel();
      }}
    >
      <h2 id={id}>{title}</h2>
      <p>{children}</p>
      <div className="button-row">
        <Button variant="outline" onClick={onCancel} disabled={busy}>
          Отмена
        </Button>
        <Button variant="danger" onClick={onConfirm} loading={busy}>
          {confirmLabel}
        </Button>
      </div>
    </dialog>
  );
}

export function Avatar({
  name,
  url,
  large = false,
}: {
  name: string;
  url: string;
  large?: boolean;
}) {
  const loaded = useImage(url, true);
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [loaded.src]);
  return (
    <span className={large ? 'profile-monogram' : 'avatar'}>
      {loaded.src && !failed ? (
        <img src={loaded.src} alt="" referrerPolicy="no-referrer" onError={() => setFailed(true)} />
      ) : (
        initials(name)
      )}
    </span>
  );
}
