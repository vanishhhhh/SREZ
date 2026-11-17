import { useEffect, useState } from 'react';
import type { FormEvent } from 'react';
import { useSearchParams } from 'react-router-dom';
import { api } from '../../lib/api';
import { useLoad } from '../../lib/hooks';
import { categories, categoryName } from '../../lib/types';
import { errorText, money } from '../../lib/format';
import { applyFilters, filtersFrom, filterNames, countThings } from '../../lib/catalog-query';
import type { CatalogFilters } from '../../lib/catalog-query';
import { Button, Empty, Icon, Field, Notice, PageTitle } from '../../components/ui';
import { ProductCollection, Pagination } from './shared';

export function CatalogPage() {
  const [params, setParams] = useSearchParams();
  const key = params.toString();
  const [filters, setFilters] = useState<CatalogFilters>(() => filtersFrom(params));
  const [filterError, setFilterError] = useState('');
  const [open, setOpen] = useState(false);
  useEffect(() => {
    setFilters(filtersFrom(new URLSearchParams(key)));
    setFilterError('');
  }, [key]);
  const brands = useLoad((signal) => api.brands(signal), []);
  const feed = useLoad(
    (signal) => {
      const query = new URLSearchParams(key);
      if (!query.has('limit')) query.set('limit', '12');
      if (!query.has('sort')) query.set('sort', 'newest');
      return api.products(query, signal);
    },
    [key],
  );
  const field = (name: keyof CatalogFilters, value: string) =>
    setFilters((previous) => ({ ...previous, [name]: value }));
  function apply(event: FormEvent) {
    event.preventDefault();
    setFilterError('');
    try {
      const next = applyFilters(filters, params);
      setParams(next);
      setOpen(false);
    } catch (err) {
      setFilterError(errorText(err));
    }
  }
  function sort(value: string) {
    const next = new URLSearchParams(params);
    next.set('sort', value);
    next.set('page', '1');
    setParams(next);
  }
  function page(value: number) {
    const next = new URLSearchParams(params);
    next.set('page', String(value));
    setParams(next);
    document.getElementById('catalog-results')?.scrollIntoView({ block: 'start' });
  }
  function reset() {
    setFilters(filtersFrom(new URLSearchParams()));
    setFilterError('');
    setParams({});
  }
  const active = filterNames.flatMap((name) =>
    params.has(name) ? [[name, params.get(name)!]] : [],
  );
  return (
    <div className="container page">
      <PageTitle eyebrow="ВЫБИРАЙ ПО СВОИМ ПРАВИЛАМ" title="Каталог">
        Одежда, в которой остаёшься собой.
      </PageTitle>
      <div className="catalog-layout">
        <aside
          id="catalog-filters"
          className={'catalog-filters ' + (open ? 'catalog-filters--open' : '')}
        >
          <div className="filter-heading">
            <h2>Фильтры</h2>
            <button className="text-button" onClick={reset}>
              Сбросить
            </button>
          </div>
          <form onSubmit={apply}>
            <Field label="Категория">
              <select
                aria-label="Категория"
                value={filters.category}
                onChange={(e) => field('category', e.target.value)}
              >
                <option value="">Все категории</option>
                {categories.map(([id, name]) => (
                  <option key={id} value={id}>
                    {name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Бренд">
              <select
                aria-label="Бренд"
                disabled={brands.loading}
                value={filters.brand_id}
                onChange={(e) => field('brand_id', e.target.value)}
              >
                <option value="">Все бренды</option>
                {brands.data?.map((brand) => (
                  <option key={brand.id} value={brand.id}>
                    {brand.name}
                  </option>
                ))}
              </select>
            </Field>
            {brands.error && (
              <div className="filter-fetch-error">
                <Notice error>Список брендов недоступен. Остальные фильтры работают.</Notice>
                <Button variant="outline" onClick={brands.reload}>
                  Повторить загрузку брендов
                </Button>
              </div>
            )}
            <div className="field">
              <span className="field__label">Цена, ₽</span>
              <div className="price-fields">
                <input
                  aria-label="Цена от"
                  placeholder="От"
                  inputMode="decimal"
                  value={filters.min}
                  onChange={(e) => field('min', e.target.value)}
                />
                <span>—</span>
                <input
                  aria-label="Цена до"
                  placeholder="До"
                  inputMode="decimal"
                  value={filters.max}
                  onChange={(e) => field('max', e.target.value)}
                />
              </div>
            </div>
            <Field label="Размер" hint="Например: M, One Size, 44 EU">
              <input
                aria-label="Размер"
                value={filters.size}
                maxLength={20}
                placeholder="Любой"
                onChange={(e) => field('size', e.target.value)}
              />
            </Field>
            <Field label="Цвет" hint="Название цвета, указанное брендом">
              <input
                aria-label="Цвет"
                value={filters.color}
                placeholder="Любой"
                onChange={(e) => field('color', e.target.value)}
              />
            </Field>
            <Notice error>{filterError}</Notice>
            <Button type="submit" className="full-width">
              Показать вещи <Icon name="arrow" size={16} />
            </Button>
          </form>
          <p className="filter-note">Размер и цвет учитывают только варианты в наличии.</p>
        </aside>
        <section className="catalog-results" id="catalog-results">
          <div className="catalog-toolbar">
            <span>{feed.data && !feed.loading ? countThings(feed.data.total) : 'Каталог'}</span>
            <div>
              <button
                className="filter-toggle text-button"
                onClick={() => setOpen(!open)}
                aria-expanded={open}
                aria-controls="catalog-filters"
              >
                Фильтры {active.length ? `(${active.length})` : ''}
              </button>
              <select
                aria-label="Сортировка"
                value={params.get('sort') || 'newest'}
                onChange={(e) => sort(e.target.value)}
              >
                <option value="newest">Сначала новые</option>
                <option value="oldest">Сначала ранние</option>
                <option value="price_asc">Цена по возрастанию</option>
                <option value="price_desc">Цена по убыванию</option>
                <option value="id_asc">По порядку</option>
              </select>
            </div>
          </div>
          {active.length > 0 && (
            <div className="active-filters">
              {active.map(([name, value]) => (
                <button
                  key={name}
                  onClick={() => {
                    const next = new URLSearchParams(params);
                    next.delete(name);
                    next.set('page', '1');
                    setParams(next);
                  }}
                >
                  {name === 'category'
                    ? categoryName(value)
                    : name === 'brand_id'
                      ? brands.data?.find((b) => String(b.id) === value)?.name || 'Бренд'
                      : name.includes('price')
                        ? Number.isSafeInteger(Number(value)) && Number(value) >= 0
                          ? `${name === 'min_price' ? 'От' : 'До'} ${money(Number(value))}`
                          : 'Некорректная цена'
                        : value}
                  <Icon name="close" size={12} />
                  <span className="sr-only">Убрать фильтр</span>
                </button>
              ))}
            </div>
          )}
          <ProductCollection
            {...feed}
            catalog
            empty={
              <Empty
                title={
                  feed.data && feed.data.page > feed.data.pages && feed.data.page > 1
                    ? 'На этой странице нет вещей'
                    : 'Пока ничего не нашлось'
                }
                action={
                  <Button variant="outline" onClick={reset}>
                    Сбросить фильтры
                  </Button>
                }
              >
                Попробуйте другой размер, цвет или диапазон цены.
              </Empty>
            }
          />
          {feed.errorStatus === 400 && (
            <Button variant="outline" onClick={reset}>
              Сбросить параметры запроса
            </Button>
          )}
          {!feed.loading && feed.data && <Pagination data={feed.data} onPage={page} />}
        </section>
      </div>
    </div>
  );
}
