import { useState } from 'react';
import { Link, useParams, useNavigate, useLocation } from 'react-router-dom';
import { api } from '../../lib/api';
import { useLoad } from '../../lib/hooks';
import { categoryName } from '../../lib/types';
import type { ProductDetail } from '../../lib/types';
import { money } from '../../lib/format';
import { Button, Icon, LoadError, Media, Notice, useAction } from '../../components/ui';
import { useAuth } from '../../auth/Auth';
import { FavoriteButton, useBuyer } from '../buyer/Buyer';
import { buyerStore } from '../buyer/store';
import { WishlistButton } from '../wishlists/Picker';
import { DetailSkeleton, Missing } from './shared';

export function ProductPage() {
  const { productId = '' } = useParams();
  const details = useLoad((signal) => api.product(productId, signal), [productId]);
  if (details.loading) return <DetailSkeleton />;
  if (details.errorStatus === 404) return <Missing />;
  if (details.error || !details.data)
    return (
      <div className="container page">
        <LoadError message={details.error || 'Товар недоступен.'} retry={details.reload} />
        <Link className="text-link" to="/catalog">
          В каталог <Icon name="arrow" />
        </Link>
      </div>
    );
  return <ProductContent key={details.data.id} product={details.data} />;
}
function ProductContent({ product: p }: { product: ProductDetail }) {
  const auth = useAuth(),
    buyer = useBuyer(),
    action = useAction(),
    navigate = useNavigate(),
    location = useLocation();
  const [quantity, setQuantity] = useState('1');
  // A failed brand directory must never hide a successfully loaded product.
  const brands = useLoad((signal) => api.brands(signal), []);
  const brand = brands.data?.find((b) => b.id === p.brand_id);
  const [selectedColor, setSelectedColor] = useState<number | null>(null);
  const [size, setSize] = useState<number | null>(null);
  const [photo, setPhoto] = useState(0);
  const color =
    p.colors.find((c) => c.id === selectedColor) ||
    p.colors.find((c) => c.variants.some((v) => v.stock > 0)) ||
    p.colors[0];
  const variant = color?.variants.find((v) => v.id === size && v.stock > 0);
  const available = color?.variants.some((v) => v.stock > 0);
  const imageCount = color?.images.length || 0;
  const photoIndex = Math.min(photo, Math.max(imageCount - 1, 0));
  const movePhoto = (delta: number) => setPhoto((photoIndex + delta + imageCount) % imageCount);
  return (
    <div className="container product-page">
      <nav className="breadcrumbs" aria-label="Путь к товару">
        <Link to="/catalog">Каталог</Link>
        <span>/</span>
        {brand && (
          <>
            <Link to={'/brands/' + brand.slug}>{brand.name}</Link>
            <span>/</span>
          </>
        )}
        <span aria-current="page">{p.name}</span>
      </nav>
      <div className="product-detail">
        <section
          className="product-gallery"
          role="group"
          aria-label="Галерея товара"
          tabIndex={imageCount > 1 ? 0 : undefined}
          onKeyDown={(e) => {
            if (e.target !== e.currentTarget || imageCount < 2) return;
            if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
              e.preventDefault();
              movePhoto(e.key === 'ArrowLeft' ? -1 : 1);
            }
          }}
        >
          <Media
            src={color?.images[photoIndex]}
            alt={`${p.name}, ${color?.color || 'фото'}, ракурс ${photoIndex + 1}`}
            eager
          />
          <div className="gallery-caption">
            <span>{color?.color || 'Фотографии ещё не добавлены'}</span>
            <div className="gallery-controls">
              {imageCount > 1 && (
                <button aria-label="Предыдущее фото" onClick={() => movePhoto(-1)}>
                  <Icon name="back" size={16} />
                </button>
              )}
              <span aria-live="polite">
                {imageCount ? `${photoIndex + 1} / ${imageCount}` : '—'}
              </span>
              {imageCount > 1 && (
                <button aria-label="Следующее фото" onClick={() => movePhoto(1)}>
                  <Icon name="arrow" size={16} />
                </button>
              )}
            </div>
          </div>
          {imageCount > 1 && (
            <div className="gallery-thumbs">
              {color!.images.map((src, i) => (
                <button
                  key={i}
                  className={i === photoIndex ? 'is-selected' : ''}
                  aria-label={'Фото ' + (i + 1)}
                  aria-pressed={i === photoIndex}
                  onClick={() => setPhoto(i)}
                >
                  <Media src={src} alt={'Ракурс ' + (i + 1)} />
                </button>
              ))}
            </div>
          )}
        </section>
        <div className="product-info">
          <p className="eyebrow">
            {brand ? (
              <Link to={'/brands/' + brand.slug}>
                {brand.name} <Icon name="diagonal" size={13} />
              </Link>
            ) : (
              categoryName(p.category)
            )}
          </p>
          <div className="product-title-row">
            <h1>{p.name}</h1>
            <FavoriteButton productId={p.id} />
          </div>
          <p className="product-price">{money(p.price_kopecks)}</p>
          <fieldset className="selection-block">
            <legend>Цвет</legend>
            <div className="selection-label">
              <strong>{color?.color || 'Нет цветов'}</strong>
            </div>
            <div className="color-options">
              {p.colors.map((c) => (
                <button
                  key={c.id}
                  className={c.id === color?.id ? 'is-selected' : ''}
                  aria-pressed={c.id === color?.id}
                  onClick={() => {
                    setSelectedColor(c.id);
                    setSize(null);
                    setPhoto(0);
                  }}
                >
                  {c.color}
                  {!c.variants.some((v) => v.stock > 0) && (
                    <span className="sr-only"> — нет в наличии</span>
                  )}
                </button>
              ))}
            </div>
          </fieldset>
          <fieldset className="selection-block">
            <legend>Размер</legend>
            <div className="selection-label">
              <strong>
                {variant?.size || (available ? 'Выберите размер' : 'Размеры недоступны')}
              </strong>
            </div>
            <div className="size-options">
              {color?.variants.map((v) => (
                <button
                  key={v.id}
                  disabled={v.stock === 0}
                  aria-pressed={variant?.id === v.id}
                  className={variant?.id === v.id ? 'is-selected' : ''}
                  onClick={() => setSize(v.id)}
                  title={v.stock > 0 ? `В наличии: ${v.stock}` : 'Нет в наличии'}
                >
                  {v.size}
                </button>
              ))}
            </div>
            <p className="availability" aria-live="polite">
              <span
                className={'availability__dot' + (available ? '' : ' availability__dot--empty')}
              />
              {variant
                ? `В наличии: ${variant.stock} шт.`
                : available
                  ? 'Есть в наличии — выберите размер'
                  : color
                    ? 'Этот цвет закончился'
                    : 'Товар пока недоступен'}
            </p>
          </fieldset>
          <div className="product-purchase">
            <label className="field">
              <span className="field__label">Количество</span>
              <input
                aria-label="Количество товара"
                type="number"
                min="1"
                max={Math.min(variant?.stock || 100, 100)}
                value={quantity}
                onChange={(e) => setQuantity(e.target.value)}
                disabled={!variant || buyer.busy}
              />
            </label>
            <Button
              className="full-width"
              loading={action.busy}
              disabled={!variant || buyer.busy}
              onClick={() => {
                if (auth.status !== 'authenticated') {
                  navigate('/login', { state: { from: location.pathname + location.search } });
                  return;
                }
                void action.run(async () => {
                  const count = Number(quantity);
                  if (
                    !/^\d+$/.test(quantity) ||
                    !Number.isSafeInteger(count) ||
                    count < 1 ||
                    count > 100 ||
                    !variant ||
                    count > variant.stock
                  )
                    throw new Error('Проверьте количество и выбранный размер.');
                  await buyerStore.addCart(variant.id, count);
                }, 'Добавлено в корзину');
              }}
            >
              Добавить в корзину <Icon name="bag" />
            </Button>
            <Notice error>{action.error}</Notice>
            <p className="field__hint">Добавление в корзину не резервирует товар.</p>
          </div>
          <WishlistButton productId={p.id} variantId={variant?.id ?? null} name={p.name} />
          <details open className="product-accordion">
            <summary>
              О вещи <Icon name="plus" size={16} />
            </summary>
            <p>{p.description || 'Бренд пока не добавил описание.'}</p>
          </details>
          <details open className="product-accordion">
            <summary>
              Состав и детали <Icon name="plus" size={16} />
            </summary>
            <div className="material-tags">
              {p.materials.map((m, i) => (
                <span key={i}>{m}</span>
              ))}
            </div>
            <p className="muted">{categoryName(p.category)}</p>
          </details>
          {brands.error ? (
            <div className="product-brand">
              <Notice error>Информация о бренде временно недоступна.</Notice>
              <Button variant="outline" onClick={brands.reload}>
                Загрузить бренд
              </Button>
            </div>
          ) : (
            brand && (
              <aside className="product-brand">
                <div className="product-brand__heading">
                  <Media src={brand.logo_url} alt={'Логотип ' + brand.name} />
                  <div>
                    <h2>{brand.name}</h2>
                  </div>
                </div>
                <p>{brand.description}</p>
                <Link className="text-link" to={'/brands/' + brand.slug}>
                  История и коллекция <Icon name="diagonal" />
                </Link>
              </aside>
            )
          )}
        </div>
      </div>
    </div>
  );
}
