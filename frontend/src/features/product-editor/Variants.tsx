import { useState } from 'react';
import type { FormEvent } from 'react';
import { useFormDraft } from '../../lib/hooks';
import { useUnsavedChanges } from '../../components/UnsavedChanges';
import { api } from '../../lib/api';
import type { ManagedVariant } from '../../lib/types';
import { parseStock } from '../../lib/format';
import { Button, Confirm, Field, Icon, Notice, useAction } from '../../components/ui';

function VariantRow({
  variant,
  productID,
  colorID,
  changed,
}: {
  variant: ManagedVariant;
  productID: number;
  colorID: number;
  changed: () => Promise<void>;
}) {
  const [form, setForm] = useFormDraft(
    {
      size: variant.size,
      stock: String(variant.stock),
      sku: variant.sku,
    },
    String(variant.id),
  );
  const [confirm, setConfirm] = useState(false);
  const action = useAction();
  const dirty =
    form.size !== variant.size || form.stock !== String(variant.stock) || form.sku !== variant.sku;
  const clean = useUnsavedChanges(dirty, 'размер ' + variant.size, JSON.stringify(form));
  function submit(event: FormEvent) {
    event.preventDefault();
    void action.run(async () => {
      await api.patchVariant(productID, colorID, variant.id, {
        size: form.size,
        stock: parseStock(form.stock),
        sku: form.sku,
      });
      clean();
      await changed();
    }, 'Размер обновлён');
  }
  return (
    <>
      <form onSubmit={submit} className="variant-row">
        <input
          aria-label={'Размер ' + variant.size}
          required
          maxLength={20}
          value={form.size}
          onChange={(e) => setForm({ ...form, size: e.target.value })}
        />
        <input
          aria-label={'Остаток ' + variant.size}
          required
          inputMode="numeric"
          value={form.stock}
          onChange={(e) => setForm({ ...form, stock: e.target.value })}
        />
        <input
          aria-label={'SKU ' + variant.size}
          value={form.sku}
          onChange={(e) => setForm({ ...form, sku: e.target.value })}
          placeholder="Необязательно"
        />
        <div>
          <Button
            type="submit"
            variant="outline"
            loading={action.busy}
            disabled={!dirty}
            aria-label={'Сохранить размер ' + variant.size}
          >
            <Icon name="check" size={16} />
          </Button>
          <Button
            variant="ghost"
            disabled={action.busy}
            onClick={() => setConfirm(true)}
            aria-label={'Удалить размер ' + variant.size}
          >
            <Icon name="trash" size={16} />
          </Button>
        </div>
      </form>
      <Confirm
        open={confirm}
        title={'Удалить размер ' + variant.size + '?'}
        busy={action.busy}
        onCancel={() => setConfirm(false)}
        onConfirm={() =>
          void action.run(async () => {
            await api.deleteVariant(productID, colorID, variant.id);
            setConfirm(false);
            await changed();
          }, 'Размер удалён')
        }
      >
        Вариант и его остаток будут удалены. У опубликованного цвета должен остаться хотя бы один
        размер.
      </Confirm>
    </>
  );
}
export function Variants({
  variants,
  productID,
  colorID,
  changed,
}: {
  variants: ManagedVariant[];
  productID: number;
  colorID: number;
  changed: () => Promise<void>;
}) {
  const [form, setForm] = useState({ size: '', stock: '0', sku: '' });
  const clean = useUnsavedChanges(
    !!(form.size || form.sku || form.stock !== '0'),
    'новый размер',
    JSON.stringify(form),
  );
  const action = useAction();
  function submit(event: FormEvent) {
    event.preventDefault();
    void action.run(async () => {
      await api.createVariant(productID, colorID, {
        size: form.size,
        stock: parseStock(form.stock),
        sku: form.sku,
      });
      clean();
      setForm({ size: '', stock: '0', sku: '' });
      await changed();
    }, 'Размер добавлен');
  }
  return (
    <section className="editor-subsection" id="sizes">
      <div className="panel-heading">
        <h3>Размеры и остатки</h3>
        <span className="mono">{variants.length} ВАРИАНТОВ</span>
      </div>
      <p className="muted">
        Любые обозначения до 20 символов: M, One Size, 44 EU. Нулевой остаток допустим.
      </p>
      {variants.length > 0 && (
        <div className="variants-table">
          <div className="variants-table__heading">
            <span>Размер</span>
            <span>Остаток, шт.</span>
            <span>SKU</span>
            <span />
          </div>
          {variants.map((variant) => (
            <VariantRow
              key={variant.id}
              variant={variant}
              productID={productID}
              colorID={colorID}
              changed={changed}
            />
          ))}
        </div>
      )}
      <form onSubmit={submit} className="variant-add">
        <div className="form-grid form-grid--three">
          <Field label="Новый размер">
            <input
              required
              maxLength={20}
              value={form.size}
              placeholder="M"
              onChange={(e) => setForm({ ...form, size: e.target.value })}
            />
          </Field>
          <Field label="Остаток">
            <input
              required
              inputMode="numeric"
              value={form.stock}
              onChange={(e) => setForm({ ...form, stock: e.target.value })}
            />
          </Field>
          <Field label="SKU">
            <input
              value={form.sku}
              placeholder="Необязательно"
              onChange={(e) => setForm({ ...form, sku: e.target.value })}
            />
          </Field>
        </div>
        <Notice error>{action.error}</Notice>
        <Button type="submit" variant="outline" loading={action.busy}>
          <Icon name="plus" size={16} />
          Добавить размер
        </Button>
      </form>
    </section>
  );
}
