import { PreviewViewport } from './PreviewViewport';
import { HeaderWordmark } from '../../components/HeaderWordmark';
import { CompositionControls } from './CompositionSettings';
import { useEffect, useId, useRef, useState } from 'react';
import {
  Button,
  Field,
  LoadError,
  Loading,
  Notice,
  useAction,
  useToast,
} from '../../components/ui';
import { useConfirmDiscard, useUnsavedChanges } from '../../components/UnsavedChanges';
import { api } from '../../lib/api';
import { ApiError } from '../../lib/client';
import { useLoad } from '../../lib/hooks';
import {
  brandPageBlocks,
  brandPalette,
  editorialBlocks,
  imageLimits,
  pageFingerprint,
  readBrandPageConfig,
  validBrandAppearance,
} from '../../lib/brand-page';
import type { BrandPageBlock, BrandPageEditorState, EditorialBlock } from '../../lib/brand-page';
import type { Brand } from '../../lib/types';
import { EditorialSettings, HeroSize } from './EditorialSettings';
import { BrandPageRenderer, useBrandPageProducts } from './BrandPageRenderer';

const layouts: Record<string, string> = {
  'full-width': 'На всю ширину',
  split: 'Фото и текст рядом',
  text: 'Текстовая секция',
  grid: 'Сетка товаров',
  inline: 'Строка контактов',
} as const;
const paletteFields = [
  ['background_color', 'Фон'],
  ['text_color', 'Текст'],
  ['accent_color', 'Акцент'],
] as const;

export function BrandPageEditor({ brand }: { brand: Brand }) {
  const feed = useLoad((signal) => api.brandPageEditor(brand.id, signal), [brand.id]);
  if (feed.loading) return <Loading />;
  if (!feed.data) return <LoadError message={feed.error} retry={feed.reload} />;
  return <BrandPageForm key={brand.id} brand={brand} initial={feed.data} />;
}

function BrandPageForm({ brand, initial }: { brand: Brand; initial: BrandPageEditorState }) {
  const previewID = useId();
  const [highlight, setHighlight] = useState<string>();
  const [jump, setJump] = useState<{ id: string }>();
  const previewTrigger = useRef<HTMLButtonElement | null>(null);
  const [activeBlock, setActiveBlock] = useState<string | null>(null);
  const [newType, setNewType] = useState<EditorialBlock>('image');
  const [localMedia, setLocalMedia] = useState<number[]>([]);
  const uploadCounter = useRef(0);
  const [uploads, setUploads] = useState(0);
  const pendingWrite = useRef<AbortController | null>(null);
  useEffect(() => () => pendingWrite.current?.abort(), []);
  const [saved, setSaved] = useState(initial);
  const [config, setConfig] = useState(initial.draft);
  const [device, setDevice] = useState<'desktop' | 'mobile'>(() =>
    matchMedia('(max-width: 760px)').matches ? 'mobile' : 'desktop',
  );
  const [showPreview, setShowPreview] = useState(false);
  const [page, setPage] = useState(1);
  const [conflict, setConflict] = useState(false);
  function findInPreview(id: string, trigger: HTMLButtonElement) {
    previewTrigger.current = trigger;
    setHighlight(id);
    setJump({ id });
    if (matchMedia('(max-width: 760px)').matches) setShowPreview(true);
  }
  function togglePreview(trigger: HTMLButtonElement) {
    if (showPreview) {
      setShowPreview(false);
      setJump(undefined);
      requestAnimationFrame(() => {
        const target = previewTrigger.current;
        (target?.isConnected ? target : trigger).focus();
      });
      return;
    }
    previewTrigger.current = trigger;
    setShowPreview(true);
    setJump({ id: highlight || config.blocks[0].id });
  }
  const products = useBrandPageProducts(brand.id, String(page));
  const action = useAction();
  const toast = useToast();
  const fingerprint = pageFingerprint(config);
  const dirty = fingerprint !== pageFingerprint(saved.draft);
  useUnsavedChanges(dirty, 'оформление страницы', fingerprint);

  function startUpload() {
    if (pendingWrite.current) return null;
    setUploads(++uploadCounter.current);
    return () => setUploads(--uploadCounter.current);
  }
  const confirmDiscard = useConfirmDiscard();
  const paletteValid = validBrandAppearance(config.appearance);
  const validConfig = !!readBrandPageConfig(config);
  const library = [...new Set([...localMedia, ...saved.media_ids])];
  const isPublished =
    saved.published_revision !== null && saved.published_revision === saved.revision;

  function updateBlocks(blocks: BrandPageBlock[]) {
    setConfig((current) => ({ ...current, blocks }));
  }
  function changeBlock(block: BrandPageBlock, minVersion?: 2 | 3) {
    setConfig((current) => ({
      ...current,
      schema_version:
        minVersion && minVersion > current.schema_version ? minVersion : current.schema_version,
      blocks: current.blocks.map((item) => (item.id === block.id ? block : item)),
    }));
  }
  function addMedia(blockID: string, mediaID: number, index: number, previous?: number) {
    setLocalMedia((prev) => [mediaID, ...prev]);
    setConfig((current) => ({
      ...current,
      schema_version: current.blocks.some((block) => block.id === blockID && block.type === 'story')
        ? 3
        : current.schema_version,
      blocks: current.blocks.map((block) => {
        if (block.id !== blockID) return block;
        const images = [...(block.content.images ?? [])];
        if (
          index >= imageLimits(block)[1] ||
          index > images.length ||
          images[index]?.media_id !== previous
        )
          return block;
        images[index] = { ...images[index], media_id: mediaID };
        return { ...block, content: { ...block.content, images } };
      }),
    }));
  }
  function addBlock(type: EditorialBlock | 'story' = newType) {
    const id = `b-${crypto.randomUUID()}`;
    const products = config.blocks.findIndex((block) => block.type === 'products');
    const blocks = [...config.blocks];
    blocks.splice(products, 0, {
      id,
      type,
      layout: type === 'story' ? 'split' : brandPageBlocks[type].layouts[0],
      content: { source: brandPageBlocks[type].source },
    });
    setConfig((current) => ({
      ...current,
      schema_version: type === 'story' || current.schema_version === 3 ? 3 : 2,
      blocks,
    }));
    setActiveBlock(id);
    setHighlight(id);
  }
  function move(index: number, step: -1 | 1, trigger: HTMLButtonElement) {
    const blocks = [...config.blocks];
    const destination = index + step;
    if (destination < 0 || destination >= blocks.length) return;
    [blocks[index], blocks[destination]] = [blocks[destination], blocks[index]];
    const siblings = trigger.parentElement;
    updateBlocks(blocks);
    requestAnimationFrame(() => {
      if (!trigger.isConnected) return;
      (trigger.disabled
        ? siblings?.querySelector<HTMLButtonElement>('[data-move]:not(:disabled)')
        : trigger
      )?.focus();
    });
  }
  function save(publish: boolean) {
    if (uploadCounter.current || pendingWrite.current || conflict) return;
    const controller = new AbortController();
    pendingWrite.current = controller;
    const signal = controller.signal;
    const sent = config;
    const expected = publish ? saved.draft : sent;
    const revision = saved.revision;
    void action.run(async () => {
      try {
        let state: BrandPageEditorState;
        try {
          state = publish
            ? await api.publishBrandPage(brand.id, revision, signal)
            : await api.saveBrandPage(brand.id, revision, sent, signal);
        } catch (error) {
          if (signal.aborted) return;
          if (!(error instanceof ApiError) || ![0, 409, 502, 503, 504, 500].includes(error.status))
            throw error;
          state = await api.brandPageEditor(brand.id, signal);
          if (signal.aborted) return;
          const snapshot = publish ? state.published : state.draft;
          const committed = publish ? (state.published_revision ?? 0) : state.revision;
          if (
            !snapshot ||
            committed <= revision ||
            pageFingerprint(snapshot) !== pageFingerprint(expected)
          ) {
            if (state.revision === revision) throw error;
            setConflict(true);
            throw new Error(
              'Оформление изменилось в другой сессии. Ваши правки пока сохранены в этой вкладке.',
            );
          }
        }
        if (signal.aborted) return;
        setSaved(state);
        setConfig((current) => (current === sent ? state.draft : current));
        setConflict(false);
        toast(publish ? 'Оформление опубликовано' : 'Черновик сохранён');
      } catch (error) {
        if (!signal.aborted) throw error;
      } finally {
        if (pendingWrite.current === controller) pendingWrite.current = null;
      }
    });
  }
  function reloadDraft() {
    confirmDiscard(() => {
      if (uploadCounter.current || pendingWrite.current) return;
      const controller = new AbortController();
      pendingWrite.current = controller;
      const sent = config;
      void action.run(async () => {
        try {
          const state = await api.brandPageEditor(brand.id, controller.signal);
          if (controller.signal.aborted) return;
          setSaved(state);
          setConfig((current) => (current === sent ? state.draft : current));
          setConflict(false);
        } catch (error) {
          if (!controller.signal.aborted) throw error;
        } finally {
          if (pendingWrite.current === controller) pendingWrite.current = null;
        }
      });
    });
  }
  return (
    <section className="brand-builder">
      <div className="panel-heading">
        <h2>Оформление страницы</h2>
      </div>
      <p className="muted">
        {saved.published
          ? isPublished
            ? 'Оформление опубликовано.'
            : 'Есть черновик с изменениями.'
          : 'Оформление ещё не опубликовано.'}
        {dirty && ' Есть несохранённые правки.'}
      </p>
      {brand.status === 'draft' && (
        <Notice>Страница станет доступна посетителям после публикации бренда.</Notice>
      )}
      <Notice error>{action.error}</Notice>
      {uploads > 0 && (
        <p role="status">
          Дождитесь завершения загрузки фотографий, чтобы сохранить или опубликовать оформление.
        </p>
      )}
      <p className="muted">
        Сохранение хранит ваш черновик. Публикация обновляет оформление и фото в блоках. Название,
        описание, контакты, логотип, баннер и товары меняются на витрине сразу после изменения в
        настройках бренда или каталоге.
      </p>
      {conflict && (
        <Button
          variant="outline"
          onClick={reloadDraft}
          loading={action.busy}
          disabled={uploads > 0}
        >
          Загрузить актуальный черновик
        </Button>
      )}
      <Button
        className="brand-builder__switch"
        variant="outline"
        aria-controls={previewID}
        aria-expanded={showPreview}
        onClick={(event) => togglePreview(event.currentTarget)}
      >
        {showPreview ? 'К редактированию' : 'Открыть предпросмотр'}
      </Button>
      <div
        className={`brand-builder__columns${showPreview ? ' brand-builder__columns--preview' : ''}`}
      >
        <fieldset className="panel brand-builder__controls" aria-label="Настройки блоков и палитры">
          <div className="panel-heading">
            <h3>Блоки</h3>
          </div>
          <ol className="brand-builder__blocks">
            {config.blocks.map((block, index) => {
              const definition = brandPageBlocks[block.type];
              const blockLabel = `блок ${index + 1}, ${definition.label}${block.content.title ? ', ' + block.content.title : ''}`;
              return (
                <li key={block.id} data-editor-block={block.type} data-editor-id={block.id}>
                  <div className="brand-builder__block-heading">
                    <strong>{definition.label}</strong>
                    {(block.type === 'story' || block.content.source === 'editorial') && (
                      <Button
                        variant="outline"
                        aria-label={`${activeBlock === block.id ? 'Свернуть' : 'Настроить'} ${blockLabel}`}
                        aria-expanded={activeBlock === block.id}
                        onClick={() => {
                          setActiveBlock(activeBlock === block.id ? null : block.id);
                          setHighlight(block.id);
                        }}
                      >
                        {activeBlock === block.id ? 'Свернуть' : 'Настроить'}
                      </Button>
                    )}
                  </div>
                  <div className="button-row brand-builder__order">
                    <Button
                      variant="outline"
                      data-move="up"
                      aria-label={`Переместить ${blockLabel} выше`}
                      disabled={index === 0}
                      onClick={(event) => move(index, -1, event.currentTarget)}
                    >
                      Выше
                    </Button>
                    <Button
                      variant="outline"
                      data-move="down"
                      aria-label={`Переместить ${blockLabel} ниже`}
                      disabled={index === config.blocks.length - 1}
                      onClick={(event) => move(index, 1, event.currentTarget)}
                    >
                      Ниже
                    </Button>
                    <Button
                      variant="ghost"
                      onClick={(event) => findInPreview(block.id, event.currentTarget)}
                      aria-label={`Открыть ${blockLabel} в предпросмотре`}
                    >
                      В предпросмотре
                    </Button>
                    {(block.type === 'story' ||
                      block.type === 'contacts' ||
                      block.content.source === 'editorial') && (
                      <Button
                        variant="outline"
                        aria-label={`Удалить ${blockLabel}`}
                        onClick={() =>
                          setConfig((current) => ({
                            ...current,
                            schema_version: block.type === 'story' ? 3 : current.schema_version,
                            blocks: current.blocks.filter((item) => item.id !== block.id),
                          }))
                        }
                      >
                        {block.type === 'contacts' ? 'Удалить контакты' : 'Удалить блок'}
                      </Button>
                    )}
                  </div>
                  {block.type === 'hero' && (
                    <Field label={`Композиция блока «${definition.label}»`}>
                      <select
                        value={block.layout}
                        onChange={(e) => {
                          const layout = e.target.value;
                          const next = { ...block, layout };
                          updateBlocks(
                            config.blocks.map((item) => (item.id === block.id ? next : item)),
                          );
                        }}
                      >
                        {definition.layouts.map((layout) => (
                          <option key={layout} value={layout}>
                            {layouts[layout]}
                          </option>
                        ))}
                      </select>
                    </Field>
                  )}
                  {block.type === 'hero' && (
                    <HeroSize
                      block={block}
                      change={(size) =>
                        changeBlock(
                          { ...block, content: { ...block.content, size } },
                          size ? 2 : undefined,
                        )
                      }
                    />
                  )}
                  {block.type !== 'story' && block.content.source !== 'editorial' && (
                    <CompositionControls block={block} change={(next) => changeBlock(next, 3)} />
                  )}
                  {(block.type === 'story' || block.content.source === 'editorial') &&
                    activeBlock === block.id && (
                      <EditorialSettings
                        block={block}
                        brandID={brand.id}
                        library={library}
                        blockLabel={blockLabel}
                        onUploadStart={startUpload}
                        saving={action.busy}
                        change={changeBlock}
                        onUpload={(id, index, previous) => addMedia(block.id, id, index, previous)}
                      />
                    )}
                </li>
              );
            })}
          </ol>
          <div className="brand-builder__add">
            <Field label="Тип нового блока">
              <select
                value={newType}
                onChange={(e) => setNewType(e.target.value as EditorialBlock)}
              >
                {editorialBlocks.map((type) => (
                  <option key={type} value={type}>
                    {brandPageBlocks[type].label}
                  </option>
                ))}
              </select>
            </Field>
            <Button
              variant="outline"
              disabled={config.blocks.length >= 20}
              onClick={() => addBlock()}
            >
              Добавить блок
            </Button>
          </div>
          {config.blocks.length < 20 && !config.blocks.some((block) => block.type === 'story') && (
            <Button variant="outline" onClick={() => addBlock('story')}>
              Добавить историю бренда
            </Button>
          )}
          {config.blocks.length < 20 &&
            !config.blocks.some((block) => block.type === 'contacts') && (
              <Button
                variant="outline"
                onClick={() =>
                  updateBlocks([
                    ...config.blocks,
                    {
                      id: `contacts-${crypto.randomUUID().slice(0, 12)}`,
                      type: 'contacts',
                      layout: 'inline',
                      content: { source: 'brand_contacts' },
                    },
                  ])
                }
              >
                Добавить контакты
              </Button>
            )}
          <div className="brand-builder__palette">
            <h3>Палитра</h3>
            {paletteFields.map(([field, label]) => (
              <Field key={field} label={label} hint="HEX или пустое поле">
                <input
                  value={config.appearance[field]}
                  maxLength={9}
                  onChange={(e) =>
                    setConfig((current) => ({
                      ...current,
                      appearance: { ...current.appearance, [field]: e.target.value },
                    }))
                  }
                />
              </Field>
            ))}
            {!paletteValid && <Notice error>Введите HEX-цвет, например #f6f5f1.</Notice>}
            {paletteValid && brandPalette(config.appearance).adjusted && (
              <Notice>Для читаемости используется контрастный цвет текста или акцента.</Notice>
            )}
          </div>
          {!validConfig && paletteValid && (
            <Notice>Заполните обязательные фотографии и текст в новых блоках.</Notice>
          )}
          <div className="button-row">
            <Button
              onClick={() => save(false)}
              disabled={
                uploads > 0 ||
                action.busy ||
                !validConfig ||
                conflict ||
                (!dirty && saved.revision > 0)
              }
            >
              Сохранить черновик
            </Button>
            <Button
              variant="outline"
              onClick={() => save(true)}
              disabled={
                uploads > 0 || action.busy || dirty || !saved.revision || isPublished || conflict
              }
            >
              Опубликовать оформление
            </Button>
          </div>
        </fieldset>
        <section className="brand-builder__preview" id={previewID}>
          <div className="panel-heading">
            <h3>Предпросмотр</h3>
            <div className="button-row" role="group" aria-label="Размер предпросмотра">
              <Button
                variant="outline"
                aria-pressed={device === 'desktop'}
                onClick={() => setDevice('desktop')}
              >
                Компьютер
              </Button>
              <Button
                variant="outline"
                aria-pressed={device === 'mobile'}
                onClick={() => setDevice('mobile')}
              >
                Телефон
              </Button>
            </div>
          </div>
          <PreviewViewport device={device} focus={jump}>
            <header className="header" aria-label="Шапка витрины">
              <HeaderWordmark brand={brand} authenticated />
            </header>
            <main>
              <BrandPageRenderer
                brand={brand}
                config={config}
                products={products}
                onPage={setPage}
                preview={device}
                selectedID={highlight}
              />
            </main>
          </PreviewViewport>
        </section>
      </div>
    </section>
  );
}
