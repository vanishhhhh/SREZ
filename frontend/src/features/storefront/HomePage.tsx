import { Link } from 'react-router-dom';
import { api } from '../../lib/api';
import { useLoad } from '../../lib/hooks';
import { categories } from '../../lib/types';
import { Empty, Icon, LoadError, Media } from '../../components/ui';
import { ProductCollection, BrandSkeletons } from './shared';

export function HomePage() {
  const brands = useLoad((signal) => api.brands(signal), []);
  const latest = useLoad(
    (signal) => api.products(new URLSearchParams({ sort: 'newest', limit: '4' }), signal),
    [],
  );
  const hoodies = useLoad(
    (signal) =>
      api.products(new URLSearchParams({ category: 'hoodie', sort: 'newest', limit: '4' }), signal),
    [],
  );
  return (
    <>
      <section className="hero">
        <div className="hero__copy">
          <p className="eyebrow">
            <span className="small-cross">+</span> ПЛАТФОРМА НЕЗАВИСИМЫХ БРЕНДОВ
          </p>
          <h1>
            ВНЕ
            <br />
            ОБЩЕГО
            <br />
            <span>РЯДА.</span>
          </h1>
          <div className="hero__bottom">
            <p>
              Вещи, которые говорят за тебя.
              <br />
              Бренды, у которых есть свой голос.
            </p>
            <Link className="button button--solid" to="/catalog">
              Найти своё <Icon name="diagonal" />
            </Link>
          </div>
        </div>
        <div className="hero__visual">
          <img
            src="/editorial.svg"
            alt="Редакционная иллюстрация: чёрное худи на нейтральном фоне"
            fetchPriority="high"
          />
          <span className="hero__label">ФОРМА СВОБОДЫ / SREZ</span>
          <a href="#latest" className="hero__scroll" aria-label="К новым поступлениям">
            <Icon name="down" />
          </a>
        </div>
      </section>
      <div className="manifest-strip" aria-hidden="true">
        <span>НЕЗАВИСИМОСТЬ</span>
        <span>↗</span>
        <span>ХАРАКТЕР</span>
        <span>↗</span>
        <span>СВОЙ ВЗГЛЯД</span>
        <span>↗</span>
        <span>НЕЗАВИСИМОСТЬ</span>
      </div>
      <section className="section container" id="latest">
        <div className="section-heading">
          <div>
            <p className="eyebrow">НОВЫЙ СРЕЗ</p>
            <h2>Вещи с характером</h2>
          </div>
          <Link className="text-link" to="/catalog">
            Весь каталог <Icon name="arrow" />
          </Link>
        </div>
        <div className="category-links">
          <Link to="/catalog" className="category-pill category-pill--dark">
            Все вещи
          </Link>
          {categories.map(([key, label]) => (
            <Link className="category-pill" key={key} to={'/catalog?category=' + key}>
              {label}
            </Link>
          ))}
        </div>
        <ProductCollection
          {...latest}
          empty={
            <Empty title="Первый дроп ещё впереди">
              Здесь появятся опубликованные вещи независимых брендов.
            </Empty>
          }
        />
      </section>
      <section className="section container hoodie-collection">
        <div className="section-heading">
          <div>
            <p className="eyebrow">ОСНОВА ГАРДЕРОБА</p>
            <h2>Худи. Каждый день.</h2>
          </div>
          <Link className="text-link" to="/catalog?category=hoodie">
            Смотреть коллекцию <Icon name="arrow" />
          </Link>
        </div>
        <ProductCollection {...hoodies} />
      </section>
      <section className="brands-feature container">
        <div className="section-heading">
          <div>
            <p className="eyebrow">ЛЮДИ ЗА ВЕЩАМИ</p>
            <h2>Независимые. Настоящие.</h2>
          </div>
          <Link className="text-link" to="/brands">
            Все бренды <Icon name="arrow" />
          </Link>
        </div>
        {brands.loading ? (
          <BrandSkeletons />
        ) : brands.error ? (
          <LoadError message={brands.error} retry={brands.reload} />
        ) : brands.data?.length ? (
          <div className="brand-tiles">
            {brands.data.slice(0, 4).map((brand, i) => (
              <Link key={brand.id} className="brand-tile" to={'/brands/' + brand.slug}>
                <span className="brand-tile__number">[{String(i + 1).padStart(2, '0')}]</span>
                <Media
                  src={brand.logo_url}
                  alt={'Логотип ' + brand.name}
                  className="brand-tile__logo"
                />
                <strong>{brand.name}</strong>
                <div>
                  <span>{brand.slug}</span>
                  <Icon name="diagonal" />
                </div>
              </Link>
            ))}
          </div>
        ) : (
          <p className="muted">Новые имена появятся здесь после публикации брендов.</p>
        )}
      </section>
      <section className="studio-invitation">
        <div>
          <p className="eyebrow">ТВОЁ ИМЯ. ТВОИ ПРАВИЛА.</p>
          <h2>
            СОЗДАЁШЬ ВЕЩИ?
            <br />
            ТЕБЕ В SREZ.
          </h2>
        </div>
        <div>
          <p>
            Собери свой бренд, расскажи его историю
            <br />и покажи первую коллекцию.
          </p>
          <Link to="/studio" className="button button--light">
            Открыть студию <Icon name="diagonal" />
          </Link>
        </div>
      </section>
    </>
  );
}
