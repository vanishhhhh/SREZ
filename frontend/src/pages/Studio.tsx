import { UploadField } from '../features/media/UploadField';
import { deleteBrandImage } from '../features/media/api';
import { useUnsavedChanges } from '../components/UnsavedChanges';
import { useState } from 'react';
import type { FormEvent } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { api } from '../lib/api';
import { useFormDraft, useLoad } from '../lib/hooks';
import { brandSlugPattern, loadBrandContext, slugFromName } from '../lib/studio';
import type { Brand, BrandInput, Membership } from '../lib/types';
import { categoryName } from '../lib/types';
import { httpsURL, money, statusName } from '../lib/format';
import {
  Button,
  Media,
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

export function StudioPage({ invitationsOnly = false }: { invitationsOnly?: boolean }) {
  const feed = useLoad(async (signal) => {
    const [brands, invitations] = await Promise.all([
      api.myBrands(signal),
      api.invitations(signal),
    ]);
    return { brands, invitations };
  }, []);
  const action = useAction();
  return (
    <div className="container page">
      <PageTitle
        eyebrow="ТВОЁ ИМЯ. ТВОИ ПРАВИЛА."
        title={invitationsOnly ? 'Приглашения в команду' : 'Студия бренда'}
        action={
          <Link className="button button--solid" to="/studio/brands/new">
            <Icon name="plus" size={18} />
            Создать бренд
          </Link>
        }
      >
        Место, где идеи становятся вещами.
      </PageTitle>
      {feed.loading ? (
        <Loading />
      ) : feed.error ? (
        <LoadError message={feed.error} retry={feed.reload} />
      ) : (
        <>
          {!!feed.data?.invitations.length && (
            <section className="panel invitations">
              <div className="panel-heading">
                <h2>Вас приглашают в команду</h2>
                <span>{feed.data.invitations.length}</span>
              </div>
              {feed.data.invitations.map(({ brand }) => (
                <div key={brand.id} className="invitation-row">
                  <div>
                    <strong>{brand.name}</strong>
                    <p className="muted">Приглашение на роль редактора</p>
                  </div>
                  <div className="button-row">
                    <Button
                      variant="outline"
                      disabled={action.busy}
                      onClick={() =>
                        void action.run(async () => {
                          await api.invitation(brand.id, false);
                          feed.reload();
                        }, 'Приглашение отклонено')
                      }
                    >
                      Отклонить
                    </Button>
                    <Button
                      disabled={action.busy}
                      onClick={() =>
                        void action.run(async () => {
                          await api.invitation(brand.id, true);
                          feed.reload();
                        }, 'Вы в команде')
                      }
                    >
                      Принять <Icon name="check" size={16} />
                    </Button>
                  </div>
                </div>
              ))}
            </section>
          )}
          {invitationsOnly ? (
            !feed.data?.invitations.length && (
              <Empty title="Пока нет приглашений">
                Когда владелец бренда пригласит тебя редактором, приглашение появится здесь.
              </Empty>
            )
          ) : feed.data?.brands.length ? (
            <div className="studio-brand-grid">
              {feed.data.brands.map(({ brand, role }) => (
                <Link key={brand.id} to={`/studio/brands/${brand.id}`} className="studio-brand">
                  <div>
                    <span className="eyebrow">{role === 'owner' ? 'ВЛАДЕЛЕЦ' : 'РЕДАКТОР'}</span>
                    <Status value={brand.status} />
                  </div>
                  <h2>{brand.name}</h2>
                  <p>
                    {brand.description || 'У каждой истории есть начало. Добавьте описание бренда.'}
                  </p>
                  <div className="studio-brand__bottom">
                    <span>/{brand.slug || 'без адреса'}</span>
                    <span>
                      Открыть студию <Icon name="diagonal" size={18} />
                    </span>
                  </div>
                </Link>
              ))}
            </div>
          ) : (
            <Empty
              title="Здесь начинается твой бренд"
              action={
                <Link className="button button--solid" to="/studio/brands/new">
                  Создать первый бренд <Icon name="diagonal" />
                </Link>
              }
            >
              Придумай имя, добавь вещи и расскажи свою историю.
            </Empty>
          )}
        </>
      )}
    </div>
  );
}
export function NewBrandPage() {
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [customSlug, setCustomSlug] = useState(false);
  const action = useAction();
  const navigate = useNavigate();
  const clean = useUnsavedChanges(!!(name || slug), 'новый бренд', JSON.stringify([name, slug]));
  function submit(event: FormEvent) {
    event.preventDefault();
    void action.run(async () => {
      const brand = await api.createBrand(name.trim(), slug);
      clean();
      window.dispatchEvent(new Event('srez:brands-changed'));
      navigate(`/studio/brands/${brand.id}`);
    }, 'Бренд создан');
  }
  return (
    <div className="container page page--narrow">
      <Link to="/studio" className="text-link">
        <Icon name="back" size={16} />
        Студия
      </Link>
      <PageTitle eyebrow="НАЧАЛО НОВОЙ ИСТОРИИ" title="Твой бренд." />
      <section className="panel">
        <p className="form-intro">
          Начни с имени. Описание, логотип и первую коллекцию можно добавить следующим шагом.
        </p>
        <form onSubmit={submit} className="form-stack">
          <Field label="Название бренда">
            <input
              required
              minLength={2}
              maxLength={100}
              value={name}
              placeholder="Имя со своим характером"
              onChange={(e) => {
                setName(e.target.value);
                if (!customSlug) setSlug(slugFromName(e.target.value));
              }}
            />
          </Field>
          <Field label="Адрес бренда" hint="3–60 строчных латинских букв, цифр или дефисов.">
            <div className="input-prefix">
              <span>srez /</span>
              <input
                required
                pattern={brandSlugPattern}
                minLength={3}
                maxLength={60}
                value={slug}
                placeholder="your-brand"
                onChange={(e) => {
                  setSlug(e.target.value);
                  setCustomSlug(true);
                }}
              />
            </div>
          </Field>
          <Notice error>{action.error}</Notice>
          <Button type="submit" loading={action.busy}>
            Создать бренд <Icon name="arrow" />
          </Button>
        </form>
      </section>
    </div>
  );
}

function editableBrand(brand: Brand): BrandInput {
  const { name, slug, description, contacts, appearance } = brand;
  return { name, slug, description, contacts, appearance };
}
function BrandSettings({ brand, reload }: { brand: Brand; reload: () => void }) {
  const [form, setForm] = useFormDraft(editableBrand(brand), String(brand.id));
  const action = useAction();
  const field = (key: keyof Pick<BrandInput, 'name' | 'slug' | 'description'>, value: string) =>
    setForm((previous) => ({ ...previous, [key]: value }));
  const dirty = ['name', 'slug', 'description', 'contacts', 'appearance'].some(
    (key) =>
      JSON.stringify(form[key as keyof BrandInput]) !==
      JSON.stringify(brand[key as keyof BrandInput]),
  );
  const clean = useUnsavedChanges(dirty, 'настройки бренда', JSON.stringify(form));
  const blocked = brand.status === 'suspended';
  function submit(event: FormEvent) {
    event.preventDefault();
    void action.run(async () => {
      if (!httpsURL(form.contacts.telegram_url, true) || !httpsURL(form.contacts.website_url, true))
        throw new Error('Ссылки должны начинаться с https://.');
      const { name, slug, description, contacts, appearance } = form;
      const saved = await api.patchBrand(brand.id, {
        name: name.trim(),
        slug,
        description: description.trim(),
        contacts,
        appearance,
      });
      setForm(editableBrand(saved));
      clean();
      window.dispatchEvent(new Event('srez:brands-changed'));
      reload();
    }, 'Настройки бренда сохранены');
  }
  return (
    <section className="panel">
      <div className="panel-heading">
        <h2>Настройки бренда</h2>
        <span className="mono">OWNER</span>
      </div>
      <form onSubmit={submit} className="form-stack">
        <fieldset disabled={action.busy || blocked}>
          <div className="form-grid">
            <Field label="Название бренда">
              <input
                required
                minLength={2}
                maxLength={100}
                value={form.name}
                onChange={(e) => field('name', e.target.value)}
              />
            </Field>
            <Field label="Адрес бренда">
              <input
                pattern={brandSlugPattern}
                required
                value={form.slug}
                onChange={(e) => field('slug', e.target.value)}
              />
            </Field>
          </div>
          <Field label="Описание бренда">
            <textarea
              rows={4}
              maxLength={5000}
              value={form.description}
              placeholder="Расскажите, во что верит ваш бренд"
              onChange={(e) => field('description', e.target.value)}
            />
          </Field>
          <details className="form-details">
            <summary>
              Контакты и оформление <Icon name="plus" size={16} />
            </summary>
            <div className="form-stack">
              <Field label="Email бренда">
                <input
                  type="email"
                  value={form.contacts.email}
                  onChange={(e) =>
                    setForm({ ...form, contacts: { ...form.contacts, email: e.target.value } })
                  }
                />
              </Field>
              <div className="form-grid">
                <Field label="Сайт бренда">
                  <input
                    type="url"
                    value={form.contacts.website_url}
                    placeholder="https://…"
                    onChange={(e) =>
                      setForm({
                        ...form,
                        contacts: { ...form.contacts, website_url: e.target.value },
                      })
                    }
                  />
                </Field>
                <Field label="Telegram">
                  <input
                    type="url"
                    value={form.contacts.telegram_url}
                    placeholder="https://t.me/…"
                    onChange={(e) =>
                      setForm({
                        ...form,
                        contacts: { ...form.contacts, telegram_url: e.target.value },
                      })
                    }
                  />
                </Field>
              </div>
              <div className="form-grid form-grid--three">
                {(
                  [
                    ['accent_color', 'Акцент'],
                    ['background_color', 'Фон'],
                    ['text_color', 'Текст'],
                  ] as const
                ).map(([key, label]) => (
                  <Field label={label} key={key} hint="HEX или пустое поле">
                    <input
                      placeholder="#111111"
                      pattern="#([0-9A-Fa-f]{3}|[0-9A-Fa-f]{4}|[0-9A-Fa-f]{6}|[0-9A-Fa-f]{8})"
                      value={form.appearance[key]}
                      onChange={(e) =>
                        setForm({
                          ...form,
                          appearance: { ...form.appearance, [key]: e.target.value },
                        })
                      }
                    />
                  </Field>
                ))}
              </div>
            </div>
          </details>
        </fieldset>
        <Notice error>{action.error}</Notice>
        <div className="button-row">
          <Button type="submit" disabled={blocked || !dirty} loading={action.busy}>
            Сохранить настройки
          </Button>
          <Button
            variant="outline"
            disabled={blocked || dirty || action.busy}
            onClick={() =>
              void action.run(
                async () => {
                  await api.brandAction(
                    brand.id,
                    brand.status === 'active' ? 'unpublish' : 'publish',
                  );
                  reload();
                },
                brand.status === 'active' ? 'Бренд снят с публикации' : 'Бренд опубликован',
              )
            }
          >
            {brand.status === 'active' ? 'Снять с публикации' : 'Опубликовать бренд'}
          </Button>
        </div>
        <p className="field__hint">
          {dirty
            ? 'Сохраните изменения перед публикацией.'
            : 'Для публикации нужны название, адрес, описание и загруженный логотип.'}
        </p>
      </form>
      {dirty && <p className="field__hint">Сохраните настройки перед изменением изображений.</p>}
      <div className="form-grid brand-media-fields">
        {(['brand_logo', 'brand_banner'] as const).map((purpose) => (
          <div key={purpose}>
            <Media
              authenticated
              src={purpose === 'brand_logo' ? brand.logo_url : brand.banner_url}
              alt={purpose === 'brand_logo' ? 'Логотип бренда' : 'Баннер бренда'}
            />
            <UploadField
              label={purpose === 'brand_logo' ? 'Загрузить логотип' : 'Загрузить баннер'}
              target={{ purpose, brand_id: brand.id }}
              disabled={blocked || action.busy || dirty}
              onComplete={reload}
            />
            {(purpose === 'brand_logo' ? brand.logo_url : brand.banner_url) && (
              <Button
                variant="outline"
                disabled={blocked || action.busy || dirty}
                onClick={() =>
                  void action.run(async () => {
                    await deleteBrandImage(brand.id, purpose);
                    reload();
                  }, 'Изображение удалено')
                }
              >
                Удалить {purpose === 'brand_logo' ? 'логотип' : 'баннер'}
              </Button>
            )}
          </div>
        ))}
      </div>
    </section>
  );
}
function Members({ brandID }: { brandID: number }) {
  const list = useLoad(() => api.members(brandID), [brandID]);
  const [username, setUsername] = useState('');
  const clean = useUnsavedChanges(!!username, 'приглашение редактора', username);
  const [target, setTarget] = useState<Membership | null>(null);
  const action = useAction();
  function invite(event: FormEvent) {
    event.preventDefault();
    void action.run(async () => {
      await api.invite(brandID, username.trim());
      clean();
      setUsername('');
      list.reload();
    }, 'Приглашение отправлено');
  }
  return (
    <section className="panel">
      <div className="panel-heading">
        <h2>Команда</h2>
        <span className="mono">OWNER / EDITOR</span>
      </div>
      <p className="muted">
        Пригласите зарегистрированного пользователя. Он сможет управлять товарами после принятия
        приглашения в своей студии.
      </p>
      <form onSubmit={invite} className="invite-form">
        <Field label="Имя пользователя редактора">
          <input
            required
            minLength={3}
            maxLength={30}
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="username"
          />
        </Field>
        <Button type="submit" loading={action.busy}>
          Пригласить <Icon name="plus" size={16} />
        </Button>
      </form>
      {list.loading ? (
        <Loading />
      ) : list.error ? (
        <LoadError message={list.error} retry={list.reload} />
      ) : (
        <div className="member-list">
          {list.data?.map((member) => (
            <div className="member-row" key={member.user_id}>
              <div>
                <strong>
                  {member.role === 'owner' ? 'Владелец' : 'Редактор'} #{member.user_id}
                </strong>
                <span className="muted">
                  {member.status === 'active' ? 'Доступ активен' : statusName(member.status)}
                </span>
              </div>
              {member.role === 'editor' && ['active', 'invited'].includes(member.status) && (
                <Button variant="ghost" disabled={action.busy} onClick={() => setTarget(member)}>
                  Отозвать доступ
                </Button>
              )}
            </div>
          ))}
        </div>
      )}
      <Confirm
        open={!!target}
        title="Отозвать доступ?"
        confirmLabel="Отозвать доступ"
        busy={action.busy}
        onCancel={() => setTarget(null)}
        onConfirm={() =>
          void action.run(async () => {
            if (target) await api.revoke(brandID, target.user_id);
            setTarget(null);
            list.reload();
          }, 'Доступ отозван')
        }
      >
        Редактор больше не сможет управлять товарами этого бренда.
      </Confirm>
    </section>
  );
}

export function BrandStudioPage() {
  const { brandId = '' } = useParams();
  const [tabParams, setTabParams] = useSearchParams();
  const requestedTab = tabParams.get('tab');
  const ownerTab =
    requestedTab === 'settings' || requestedTab === 'team' ? requestedTab : 'products';
  const setTab = (value: 'products' | 'settings' | 'team') =>
    setTabParams(value === 'products' ? {} : { tab: value }, { replace: true });
  const feed = useLoad(
    async (signal) => {
      const context = await loadBrandContext(brandId, signal);
      const products =
        context.brand.status === 'suspended' ? [] : await api.manageProducts(brandId, signal);
      return { ...context, products };
    },
    [brandId],
  );
  if (feed.loading && !feed.data) return <Loading />;
  if (feed.error || !feed.data)
    return (
      <div className="container page">
        <LoadError message={feed.error || 'Бренд недоступен.'} retry={feed.reload} />
        <Link className="text-link" to="/studio">
          Все мои бренды <Icon name="arrow" />
        </Link>
      </div>
    );
  const { brand, role, products } = feed.data;
  const tab = role === 'owner' ? ownerTab : 'products';
  const blocked = brand.status === 'suspended';
  return (
    <div className="container page">
      <Link className="text-link" to="/studio">
        <Icon name="back" size={16} />
        Мои бренды
      </Link>
      <PageTitle
        eyebrow={role === 'owner' ? 'СТУДИЯ / ВЛАДЕЛЕЦ' : 'СТУДИЯ / РЕДАКТОР'}
        title={brand.name}
        action={
          !blocked && (
            <Link className="button button--solid" to={`/studio/brands/${brand.id}/products/new`}>
              <Icon name="plus" size={18} />
              Добавить товар
            </Link>
          )
        }
      >
        <Status value={brand.status} />
      </PageTitle>
      {blocked && (
        <Notice error>Бренд заблокирован. Изменение настроек и товаров недоступно.</Notice>
      )}
      {brand.status === 'draft' && (
        <Notice>
          Бренд пока не виден в каталоге.{' '}
          {role === 'owner'
            ? 'Заполните настройки и опубликуйте его, когда будете готовы.'
            : 'Попросите владельца опубликовать бренд перед публикацией товаров.'}
        </Notice>
      )}
      <div className="tabs" role="tablist" aria-label="Разделы студии">
        <button role="tab" aria-selected={tab === 'products'} onClick={() => setTab('products')}>
          Товары <span>{products.length}</span>
        </button>
        {role === 'owner' && (
          <>
            <button
              role="tab"
              aria-selected={tab === 'settings'}
              onClick={() => setTab('settings')}
            >
              Настройки бренда
            </button>
            <button role="tab" aria-selected={tab === 'team'} onClick={() => setTab('team')}>
              Команда
            </button>
          </>
        )}
        {brand.status === 'active' && (
          <Link to={'/brands/' + brand.slug} className="tabs__public">
            Публичная страница <Icon name="diagonal" size={14} />
          </Link>
        )}
      </div>
      {tab === 'products' && (
        <section>
          {products.length ? (
            <div className="product-table">
              <div className="product-table__head">
                <span>Товар</span>
                <span>Цена</span>
                <span>Статус</span>
                <span />
              </div>
              {products.map((product) => (
                <Link
                  className="product-table__row"
                  to={`/studio/brands/${brand.id}/products/${product.id}`}
                  key={product.id}
                >
                  <div>
                    <strong>{product.name}</strong>
                    <span>{categoryName(product.category)}</span>
                  </div>
                  <span>{money(product.price_kopecks)}</span>
                  <Status value={product.status} />
                  <Icon name="diagonal" size={18} />
                </Link>
              ))}
            </div>
          ) : (
            <Empty
              title="Первая вещь — начало коллекции"
              action={
                !blocked && (
                  <Link
                    className="button button--outline"
                    to={`/studio/brands/${brand.id}/products/new`}
                  >
                    Создать товар <Icon name="plus" size={16} />
                  </Link>
                )
              }
            >
              Добавьте товар, фотографии и размеры. Всё можно подготовить в черновике.
            </Empty>
          )}
        </section>
      )}
      {tab === 'settings' && role === 'owner' && (
        <BrandSettings brand={brand} reload={feed.reload} />
      )}
      {tab === 'team' && role === 'owner' && <Members brandID={brand.id} />}
    </div>
  );
}
