import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { Button, Empty, Icon, LoadError, ProductTile } from '../../components/ui';
import type { ProductPage } from '../../lib/types';

export function ProductSkeletons({
  count = 4,
  catalog = false,
}: {
  count?: number;
  catalog?: boolean;
}) {
  return (
    <div
      role="status"
      aria-label="Загружаем товары"
      aria-busy="true"
      className={`product-grid ${catalog ? 'product-grid--catalog' : ''}`}
    >
      {Array.from({ length: count }, (_, i) => (
        <div className="product-skeleton" aria-hidden="true" key={i}>
          <div className="skeleton product-skeleton__image" />
          <div className="skeleton skeleton--line" />
          <div className="skeleton skeleton--short" />
        </div>
      ))}
      <span className="sr-only">Загружаем товары…</span>
    </div>
  );
}
export function BrandSkeletons() {
  return (
    <div className="brand-tiles" role="status" aria-label="Загружаем бренды" aria-busy="true">
      {[0, 1, 2, 3].map((i) => (
        <div key={i} aria-hidden="true" className="skeleton brand-skeleton" />
      ))}
    </div>
  );
}
export function DetailSkeleton() {
  return (
    <div
      className="container page product-detail"
      role="status"
      aria-label="Загружаем товар"
      aria-busy="true"
    >
      <div className="skeleton detail-skeleton" />
      <div aria-hidden="true">
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="skeleton detail-skeleton__line" />
        ))}
      </div>
    </div>
  );
}
export function Pagination({
  data,
  onPage,
}: {
  data: ProductPage;
  onPage: (page: number) => void;
}) {
  if (!data.total && data.page === 1) return null;
  return (
    <nav className="pagination" aria-label="Страницы каталога">
      <Button
        variant="outline"
        disabled={data.page <= 1}
        onClick={() => onPage(Math.min(data.page - 1, Math.max(data.pages, 1)))}
      >
        <Icon name="back" size={16} />
        Назад
      </Button>
      <span aria-live="polite">
        {data.page} / {Math.max(data.pages, 1)}
      </span>
      <Button
        variant="outline"
        disabled={data.page >= data.pages}
        onClick={() => onPage(data.page + 1)}
      >
        Далее
        <Icon name="arrow" size={16} />
      </Button>
    </nav>
  );
}
export function ProductCollection({
  loading,
  error,
  data,
  reload,
  empty,
  catalog = false,
}: {
  loading: boolean;
  error: string;
  data: ProductPage | null;
  reload: () => Promise<void>;
  empty?: ReactNode;
  catalog?: boolean;
}) {
  if (loading) return <ProductSkeletons count={catalog ? 6 : 4} catalog={catalog} />;
  if (error) return <LoadError message={error} retry={reload} />;
  if (!data?.items.length)
    return (
      empty || (
        <Empty title="Коллекция готовится">Новые вещи появятся после публикации брендом.</Empty>
      )
    );
  return (
    <div className={`product-grid ${catalog ? 'product-grid--catalog' : ''}`}>
      {data.items.map((product) => (
        <ProductTile key={product.id} product={product} />
      ))}
    </div>
  );
}
export function Missing({ brand = false }: { brand?: boolean }) {
  return (
    <div className="container page">
      <Empty
        title={brand ? 'Бренд не найден' : 'Товар не найден'}
        action={
          <Link className="button button--outline" to={brand ? '/brands' : '/catalog'}>
            {brand ? 'Все бренды' : 'В каталог'}
            <Icon name="arrow" />
          </Link>
        }
      >
        {brand
          ? 'Возможно, бренд ещё не опубликован или больше недоступен.'
          : 'Возможно, вещь снята с публикации. Посмотрите другие вещи в каталоге.'}
      </Empty>
    </div>
  );
}
