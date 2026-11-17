import { Variants } from '../features/product-editor/Variants';
import { Images } from '../features/product-editor/Images';
import {
  useConfirmDiscard,
  useHasUnsavedChanges,
  useUnsavedChanges,
} from '../components/UnsavedChanges';
import { useState } from 'react';
import type { FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { api } from '../lib/api';
import { ApiError } from '../lib/client';
import { useFormDraft, useLoad } from '../lib/hooks';
import { loadBrandContext } from '../lib/studio';
import { categories } from '../lib/types';
import type { ManagedColor, Product, ProductInput } from '../lib/types';
import { parsePrice, rublesInput } from '../lib/format';
import {
  Button,
  Confirm,
  Empty,
  Field,
  Icon,
  LoadError,
  Loading,
  Notice,
  PageTitle,
  Status,
  useAction,
} from '../components/ui';

type ProductFormValues = {
  name: string;
  description: string;
  price: string;
  category: string;
  materials: string;
};
function valuesFrom(product?: Product): ProductFormValues {
  return {
    name: product?.name || '',
    description: product?.description || '',
    price: product ? rublesInput(product.price_kopecks) : '',
    category: product?.category || '',
    materials: product?.materials.join(', ') || '',
  };
}
function productInput(form: ProductFormValues): ProductInput {
  const materials = form.materials
    .split(/[,;\n]/)
    .map((item) => item.trim())
    .filter(Boolean);
  if (materials.length > 10 || new Set(materials).size !== materials.length)
    throw new Error('Укажите до 10 материалов без повторений.');
  return {
    name: form.name.trim(),
    description: form.description.trim(),
    price_kopecks: form.price.trim() ? parsePrice(form.price) : 0,
    category: form.category,
    materials,
  };
}
function ProductForm({
  product,
  onSave,
  create = false,
}: {
  product?: Product;
  onSave: (input: ProductInput, saved: () => void) => Promise<void>;
  create?: boolean;
}) {
  const [form, setForm] = useFormDraft(valuesFrom(product), String(product?.id ?? 'new'));
  const clean = useUnsavedChanges(
    JSON.stringify(form) !== JSON.stringify(valuesFrom(product)),
    'основное о товаре',
    JSON.stringify(form),
  );
  const action = useAction();
  const field = (key: keyof ProductFormValues, value: string) =>
    setForm((previous) => ({ ...previous, [key]: value }));
  function submit(event: FormEvent) {
    event.preventDefault();
    void action.run(
      async () => {
        await onSave(productInput(form), clean);
        clean();
      },
      create ? 'Черновик создан' : 'Данные товара сохранены',
    );
  }
  return (
    <form onSubmit={submit} className="form-stack">
      <fieldset disabled={action.busy}>
        <Field label="Название товара">
          <input
            required
            minLength={2}
            maxLength={120}
            value={form.name}
            placeholder="Например, худи свободного кроя"
            onChange={(e) => field('name', e.target.value)}
          />
        </Field>
        <div className="form-grid">
          <Field label="Категория">
            <select value={form.category} onChange={(e) => field('category', e.target.value)}>
              <option value="">Пока не выбрана</option>
              {categories.map(([id, name]) => (
                <option key={id} value={id}>
                  {name}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Цена, ₽" hint="До двух знаков после запятой.">
            <input
              inputMode="decimal"
              value={form.price}
              placeholder="0"
              onChange={(e) => field('price', e.target.value)}
            />
          </Field>
        </div>
        <Field label="Описание товара">
          <textarea
            rows={5}
            maxLength={5000}
            value={form.description}
            placeholder="Силуэт, идея, детали — расскажите о вещи"
            onChange={(e) => field('description', e.target.value)}
          />
        </Field>
        <Field label="Материалы" hint="Через запятую или с новой строки, до 10 материалов.">
          <input
            value={form.materials}
            placeholder="Хлопок, полиэстер"
            onChange={(e) => field('materials', e.target.value)}
          />
        </Field>
      </fieldset>
      <Notice error>{action.error}</Notice>
      <Button type="submit" loading={action.busy}>
        {create ? 'Создать черновик' : 'Сохранить товар'}
        <Icon name="arrow" size={16} />
      </Button>
    </form>
  );
}

export function NewProductPage() {
  const { brandId = '' } = useParams();
  const navigate = useNavigate();
  const context = useLoad((signal) => loadBrandContext(brandId, signal), [brandId]);
  if (context.loading) return <Loading />;
  if (context.error || !context.data)
    return (
      <div className="container page">
        <LoadError message={context.error || 'Бренд недоступен'} retry={context.reload} />
      </div>
    );
  return (
    <div className="container page page--narrow">
      <Link className="text-link" to={'/studio/brands/' + brandId}>
        <Icon name="back" size={16} />
        {context.data.brand.name}
      </Link>
      <PageTitle eyebrow="НОВАЯ ВЕЩЬ" title="Начнём с идеи." />
      <section className="panel">
        {context.data.brand.status === 'suspended' ? (
          <Notice error>Бренд заблокирован. Создание товаров недоступно.</Notice>
        ) : (
          <>
            <p className="form-intro">
              Создайте черновик, затем добавьте цвета, фотографии и размеры. В каталог попадёт
              только опубликованный товар.
            </p>
            <ProductForm
              create
              onSave={async (input, saved) => {
                const product = await api.createProduct(context.data!.brand.id, input);
                saved();
                navigate(`/studio/brands/${brandId}/products/${product.id}`, { replace: true });
              }}
            />
          </>
        )}
      </section>
    </div>
  );
}

function ColorEditor({
  color,
  productID,
  colorChanged,
}: {
  color: ManagedColor;
  productID: number;
  colorChanged: () => Promise<void>;
}) {
  const [name, setName] = useFormDraft(color.color, String(color.id));
  const clean = useUnsavedChanges(name !== color.color, 'название цвета', name);
  const [confirm, setConfirm] = useState(false);
  const action = useAction();
  const data = useLoad(
    async (signal) => {
      const [images, variants] = await Promise.all([
        api.images(productID, color.id, signal),
        api.variants(productID, color.id, signal),
      ]);
      return { images, variants };
    },
    [productID, color.id],
  );
  const refresh = async () => {
    await Promise.all([data.reload(), colorChanged()]);
  };
  function rename(event: FormEvent) {
    event.preventDefault();
    void action.run(async () => {
      const saved = await api.renameColor(productID, color.id, name);
      setName(saved.color);
      clean();
      await colorChanged();
    }, 'Цвет переименован');
  }
  return (
    <div className="color-editor">
      <form className="color-rename" onSubmit={rename}>
        <Field label="Название цвета">
          <input required value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Button
          type="submit"
          variant="outline"
          loading={action.busy}
          disabled={name === color.color}
        >
          Переименовать
        </Button>
        <Button
          variant="ghost"
          aria-label={'Удалить цвет ' + color.color}
          disabled={action.busy}
          onClick={() => setConfirm(true)}
        >
          <Icon name="trash" size={18} />
        </Button>
      </form>
      {data.loading && !data.data ? (
        <Loading />
      ) : data.error ? (
        <LoadError message={data.error} retry={data.reload} />
      ) : (
        data.data && (
          <>
            <Images
              images={data.data.images}
              productID={productID}
              colorID={color.id}
              changed={refresh}
            />
            <Variants
              variants={data.data.variants}
              productID={productID}
              colorID={color.id}
              changed={refresh}
            />
          </>
        )
      )}
      <Confirm
        open={confirm}
        title={'Удалить цвет «' + color.color + '»?'}
        busy={action.busy}
        onCancel={() => setConfirm(false)}
        onConfirm={() =>
          void action.run(async () => {
            await api.deleteColor(productID, color.id);
            setConfirm(false);
            await colorChanged();
          }, 'Цвет удалён')
        }
      >
        Его фотографии, размеры и остатки также будут удалены. Действие нельзя отменить.
      </Confirm>
    </div>
  );
}
function ColorPanel({ product }: { product: Product }) {
  const colors = useLoad((signal) => api.colors(product.id, signal), [product.id]);
  const [selected, setSelected] = useState<number | null>(null);
  const [name, setName] = useState('');
  const clean = useUnsavedChanges(!!name, 'новый цвет', name);
  const confirmDiscard = useConfirmDiscard();
  const action = useAction();
  const color = colors.data?.find((c) => c.id === selected) || colors.data?.[0];
  function add(event: FormEvent) {
    event.preventDefault();
    void action.run(async () => {
      const c = await api.createColor(product.id, name);
      setName('');
      clean();
      await colors.reload();
      confirmDiscard(() => setSelected(c.id));
    }, 'Цвет добавлен');
  }
  return (
    <section className="panel" id="colors">
      <div className="panel-heading">
        <h2>Цвета и варианты</h2>
        <span className="mono">02 / DETAILS</span>
      </div>
      <p className="muted">У каждого цвета — свои фотографии, размеры и остатки.</p>
      {product.status === 'published' ? (
        <Notice>
          Для добавления нового цвета сначала скройте товар. Готовые цвета и остатки можно
          редактировать сразу. У опубликованного цвета должны оставаться фото и размер. Чтобы
          изменить структуру, нажмите «Скрыть товар», завершите изменения и опубликуйте его снова.
        </Notice>
      ) : (
        <form onSubmit={add} className="color-add">
          <Field label="Новый цвет">
            <input
              required
              value={name}
              placeholder="Например, чёрный"
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Button type="submit" loading={action.busy}>
            <Icon name="plus" size={16} />
            Добавить цвет
          </Button>
        </form>
      )}
      <Notice error>{action.error}</Notice>
      {colors.loading && !colors.data ? (
        <Loading />
      ) : colors.error ? (
        <LoadError message={colors.error} retry={colors.reload} />
      ) : colors.data?.length ? (
        <>
          <div className="color-tabs" role="tablist" aria-label="Цвета товара">
            {colors.data.map((c) => (
              <button
                key={c.id}
                role="tab"
                aria-selected={c.id === color?.id}
                className={c.id === color?.id ? 'is-selected' : ''}
                onClick={() => {
                  if (c.id !== color?.id) confirmDiscard(() => setSelected(c.id));
                }}
              >
                {c.color}
                <span>{c.images.length} фото</span>
              </button>
            ))}
          </div>
          {color && (
            <ColorEditor
              key={color.id}
              color={color}
              productID={product.id}
              colorChanged={colors.reload}
            />
          )}
        </>
      ) : (
        <Empty title="Добавьте первый цвет">
          После этого можно будет добавить фотографии и размеры.
        </Empty>
      )}
    </section>
  );
}

export function ProductEditorPage() {
  const { brandId = '', productId = '' } = useParams();
  const action = useAction();
  const unsaved = useHasUnsavedChanges();
  const feed = useLoad(
    async (signal) => {
      const context = await loadBrandContext(brandId, signal);
      const products = await api.manageProducts(brandId, signal);
      const product = products.find((p) => String(p.id) === productId);
      if (!product) throw new ApiError(404, 'resource not found');
      return { ...context, product };
    },
    [brandId, productId],
  );
  if (feed.loading && !feed.data) return <Loading />;
  if (feed.error || !feed.data)
    return (
      <div className="container page">
        <LoadError message={feed.error || 'Товар недоступен.'} retry={feed.reload} />
        <Link to={'/studio/brands/' + brandId} className="text-link">
          К бренду <Icon name="arrow" />
        </Link>
      </div>
    );
  const { product, brand, role } = feed.data;
  const transition = (name: 'publish' | 'hide' | 'unpublish') =>
    void action.run(
      async () => {
        await api.productAction(product.id, name);
        feed.reload();
      },
      name === 'publish'
        ? 'Товар опубликован'
        : name === 'hide'
          ? 'Товар скрыт'
          : 'Товар снят с публикации',
    );
  return (
    <div className="container page">
      <Link className="text-link" to={'/studio/brands/' + brandId}>
        <Icon name="back" size={16} />
        {brand.name}
      </Link>
      <PageTitle
        eyebrow="СТУДИЯ / РЕДАКТОР ТОВАРА"
        title={product.name}
        action={
          <div className="button-row">
            {product.status === 'published' ? (
              <>
                <Link
                  className="button button--outline"
                  to={'/products/' + product.id}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  Посмотреть <Icon name="diagonal" size={16} />
                </Link>
                <Button
                  variant="outline"
                  disabled={action.busy || unsaved}
                  onClick={() => transition('hide')}
                >
                  Скрыть товар
                </Button>
              </>
            ) : (
              <Button
                loading={action.busy}
                disabled={brand.status !== 'active' || unsaved}
                onClick={() => transition('publish')}
              >
                Опубликовать товар <Icon name="diagonal" size={16} />
              </Button>
            )}
            {product.status === 'hidden' && (
              <Button
                variant="ghost"
                disabled={action.busy || unsaved}
                onClick={() => transition('unpublish')}
              >
                В черновик
              </Button>
            )}
          </div>
        }
      >
        <Status value={product.status} />
      </PageTitle>
      {brand.status !== 'active' && (
        <Notice>
          Перед публикацией товара нужно опубликовать бренд.{' '}
          {role === 'owner' ? (
            <Link to={`/studio/brands/${brand.id}?tab=settings`}>
              Перейти к настройкам бренда →
            </Link>
          ) : (
            'Попросите владельца завершить настройки бренда.'
          )}
        </Notice>
      )}
      <Notice error>{action.error}</Notice>
      {unsaved && <Notice>Сохраните изменения в формах перед изменением публикации товара.</Notice>}
      <div className="editor-layout">
        <aside className="editor-sidebar">
          <a href="#basics">
            <span>01</span>Основное
          </a>
          <a href="#colors">
            <span>02</span>Цвета и варианты
          </a>
          <a href="#photos">
            <span>03</span>Фотографии
          </a>
          <a href="#sizes">
            <span>04</span>Размеры и остатки
          </a>
          <div className="editor-checklist">
            <strong>Перед публикацией</strong>
            <p>Описание, цена и материалы. Хотя бы одно фото и один размер у каждого цвета.</p>
            <p>Товар появится в публичном каталоге только после публикации.</p>
          </div>
        </aside>
        <div className="editor-content">
          <section className="panel" id="basics">
            <div className="panel-heading">
              <h2>Основное</h2>
              <span className="mono">01 / PRODUCT</span>
            </div>
            <ProductForm
              product={product}
              onSave={async (input) => {
                await api.patchProduct(product.id, input);
                feed.reload();
              }}
            />
          </section>
          <ColorPanel product={product} />
        </div>
      </div>
    </div>
  );
}
