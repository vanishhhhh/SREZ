import { Link, useParams, useSearchParams } from 'react-router-dom';
import { api } from '../../lib/api';
import { useLoad } from '../../lib/hooks';
import { Icon, Empty, LoadError, Media, PageTitle } from '../../components/ui';
import { BrandSkeletons, Missing } from './shared';
import { BrandPageRenderer, useBrandPageProducts } from '../brand-page/BrandPageRenderer';
import { defaultBrandPage } from '../../lib/brand-page';
import type { Brand } from '../../lib/types';

export function BrandsPage() {
  const brands = useLoad((signal) => api.brands(signal), []);
  return (
    <div className="container page">
      <PageTitle title="Бренды" />
      {brands.loading ? (
        <BrandSkeletons />
      ) : brands.error ? (
        <LoadError message={brands.error} retry={brands.reload} />
      ) : brands.data?.length ? (
        <div className="brand-directory">
          {brands.data.map((brand, i) => (
            <Link key={brand.id} to={'/brands/' + brand.slug} className="brand-directory__row">
              <span className="mono">{String(i + 1).padStart(2, '0')}</span>
              <div className="brand-directory__name">
                <Media src={brand.logo_url} alt={'Логотип ' + brand.name} />
                <h2>{brand.name}</h2>
              </div>
              <p>{brand.description}</p>
              <Icon name="diagonal" size={30} />
            </Link>
          ))}
        </div>
      ) : (
        <Empty title="Пока нет опубликованных брендов" />
      )}
    </div>
  );
}
export function BrandPage() {
  const { slug = '' } = useParams();
  const brand = useLoad((signal) => api.brand(slug, signal), [slug]);
  if (brand.loading)
    return (
      <div className="container page">
        <BrandSkeletons />
      </div>
    );
  if (brand.errorStatus === 404) return <Missing brand />;
  if (brand.error || !brand.data)
    return (
      <div className="container page">
        <LoadError message={brand.error || 'Бренд недоступен.'} retry={brand.reload} />
      </div>
    );
  return <BrandContent key={brand.data.id} brand={brand.data} />;
}
function BrandContent({ brand }: { brand: Brand }) {
  const [params, setParams] = useSearchParams();
  const products = useBrandPageProducts(brand.id, params.get('page') || '1');
  return (
    <BrandPageRenderer
      brand={brand}
      config={brand.page ?? defaultBrandPage()}
      products={products}
      onPage={(page) => {
        setParams({ page: String(page) });
        document.getElementById('brand-products')?.scrollIntoView({ block: 'start' });
      }}
    />
  );
}
