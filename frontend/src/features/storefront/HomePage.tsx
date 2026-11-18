import { Link } from 'react-router-dom';
import { api } from '../../lib/api';
import { useLoad } from '../../lib/hooks';
import { categories } from '../../lib/types';
import { Empty, LoadError, Media } from '../../components/ui';
import { ProductCollection, BrandSkeletons } from './shared';
import fashionCover from './editorial/fashion-cover.webp';
import fashionCoverMobile from './editorial/fashion-cover-mobile.webp';
import './home.css';

export function HomePage() {
  const brands = useLoad((signal) => api.brands(signal), []);
  const latest = useLoad(
    (signal) => api.products(new URLSearchParams({ sort: 'newest', limit: '4' }), signal),
    [],
  );
  return (
    <div className="home">
      <h1 className="sr-only">SREZ</h1>
      <section className="home-cover container" aria-label="Витрина SREZ">
        <picture>
          <source media="(max-width: 620px)" srcSet={fashionCoverMobile} width={768} height={764} />
          <img
            src={fashionCover}
            alt="Модель в графитовом пальто на бетонной террасе"
            width={1536}
            height={1024}
            fetchPriority="high"
          />
        </picture>
        <nav className="home-cover__links" aria-label="Разделы витрины">
          <Link to="/catalog">Каталог</Link>
          <Link to="/brands">Бренды</Link>
        </nav>
      </section>
      <section className="section container" id="latest">
        <div className="section-heading">
          <h2>Новые поступления</h2>
          <Link className="text-link" to="/catalog">
            Весь каталог
          </Link>
        </div>
        <nav className="category-links" aria-label="Категории каталога">
          {categories.map(([key, label]) => (
            <Link className="category-pill" key={key} to={'/catalog?category=' + key}>
              {label}
            </Link>
          ))}
        </nav>
        <ProductCollection {...latest} empty={<Empty title="Пока нет опубликованных товаров" />} />
      </section>
      <section className="section container home-brands">
        <div className="section-heading">
          <h2>Бренды</h2>
          <Link className="text-link" to="/brands">
            Все бренды
          </Link>
        </div>
        {brands.loading ? (
          <BrandSkeletons />
        ) : brands.error ? (
          <LoadError message={brands.error} retry={brands.reload} />
        ) : brands.data?.length ? (
          <div className="brand-tiles">
            {brands.data.slice(0, 4).map((brand) => {
              const cover = brand.banner_url || brand.logo_url;
              return (
                <Link key={brand.id} className="brand-tile" to={'/brands/' + brand.slug}>
                  {cover && (
                    <Media
                      src={cover}
                      alt=""
                      className={
                        brand.banner_url
                          ? 'brand-tile__cover'
                          : 'brand-tile__cover brand-tile__cover--logo'
                      }
                    />
                  )}
                  <span className="brand-tile__name">{brand.name}</span>
                </Link>
              );
            })}
          </div>
        ) : (
          <Empty title="Пока нет опубликованных брендов" />
        )}
      </section>
    </div>
  );
}
