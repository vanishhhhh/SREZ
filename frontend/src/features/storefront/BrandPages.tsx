import { Link, useParams, useSearchParams } from 'react-router-dom';
import { api } from '../../lib/api';
import { useLoad } from '../../lib/hooks';
import { Icon, Empty, LoadError, Media, PageTitle } from '../../components/ui';
import { BrandSkeletons, Missing, Pagination, ProductCollection } from './shared';
import { mediaURL } from '../../lib/media';
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
function BrandContent({ brand: b }: { brand: Brand }) {
  const [params, setParams] = useSearchParams();
  const page = params.get('page') || '1';
  const products = useLoad(
    (signal) =>
      api.products(
        new URLSearchParams({ brand_id: String(b.id), limit: '12', sort: 'newest', page }),
        signal,
      ),
    [b.id, page],
  );
  const website = mediaURL(b.contacts.website_url);
  const telegram = mediaURL(b.contacts.telegram_url);
  const email = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(b.contacts.email) ? b.contacts.email : '';
  return (
    <>
      <section className="brand-hero">
        {b.banner_url && (
          <Media className="brand-hero__banner" src={b.banner_url} alt={'Баннер ' + b.name} eager />
        )}
        <div className="container">
          <Link className="text-link" to="/brands">
            <Icon name="back" size={16} />
            Все бренды
          </Link>
          <h1>{b.name}</h1>
        </div>
      </section>
      <div className="container">
        <section className="brand-story">
          <div className="brand-story__identity">
            <Media src={b.logo_url} alt={'Логотип ' + b.name} />
            <h2>О бренде</h2>
          </div>
          <div>
            <p>{b.description}</p>
            <div className="brand-contacts">
              {website && (
                <a href={website} target="_blank" rel="noopener noreferrer">
                  Сайт бренда <Icon name="diagonal" size={15} />
                </a>
              )}
              {telegram && (
                <a href={telegram} target="_blank" rel="noopener noreferrer">
                  Telegram <Icon name="diagonal" size={15} />
                </a>
              )}
              {email && (
                <a href={'mailto:' + email}>
                  Написать бренду <Icon name="diagonal" size={15} />
                </a>
              )}
            </div>
          </div>
        </section>
        <section className="section" id="brand-products">
          <div className="section-heading">
            <h2>Вещи бренда</h2>
            <Link className="text-link" to={'/catalog?brand_id=' + b.id}>
              Фильтры каталога <Icon name="arrow" />
            </Link>
          </div>
          <ProductCollection {...products} />
          {!products.loading && products.data && (
            <Pagination
              data={products.data}
              onPage={(value) => {
                setParams({ page: String(value) });
                document.getElementById('brand-products')?.scrollIntoView({ block: 'start' });
              }}
            />
          )}
          {products.errorStatus === 400 && (
            <Link className="text-link" to={'/brands/' + b.slug}>
              К первой странице <Icon name="arrow" />
            </Link>
          )}
        </section>
      </div>
    </>
  );
}
