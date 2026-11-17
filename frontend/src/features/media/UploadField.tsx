import { useEffect, useId, useRef, useState } from 'react';
import { Button, Notice, useToast } from '../../components/ui';
import { beginUpload, putUpload, validateFile } from './api';
import { useUnsavedChanges } from '../../components/UnsavedChanges';
import type { UploadTarget } from './api';
import { ApiError } from '../../lib/http';
import { errorText } from '../../lib/format';

export function UploadField({
  label,
  target,
  onComplete,
  disabled = false,
}: {
  label: string;
  target: UploadTarget;
  onComplete: () => void | Promise<void>;
  disabled?: boolean;
}) {
  const fieldID = useId();
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [retry, setRetry] = useState(false);
  const pending = useRef<{ file: File; path: string } | null>(null),
    lock = useRef(false),
    active = useRef<AbortController | null>(null);
  const toast = useToast();
  const [selected, setSelected] = useState<File | null>(null),
    [preview, setPreview] = useState('');
  const clean = useUnsavedChanges(
    !!selected,
    label,
    selected ? `${selected.name}:${selected.size}:${selected.lastModified}` : '',
  );
  useEffect(() => {
    if (!selected) {
      setPreview('');
      return;
    }
    const url = URL.createObjectURL(selected);
    setPreview(url);
    return () => URL.revokeObjectURL(url);
  }, [selected]);
  useEffect(() => () => active.current?.abort(), []);
  async function upload(file?: File) {
    if (lock.current || disabled) return;
    lock.current = true;
    setBusy(true);
    setError('');
    setRetry(false);
    const controller = new AbortController();
    active.current = controller;
    try {
      if (file) {
        pending.current = null;
        const path = await beginUpload(file, target, controller.signal);
        pending.current = { file, path };
      }
      const current = pending.current;
      if (!current) throw new Error('Выберите фотографию.');
      await putUpload(current.path, current.file, controller.signal);
      pending.current = null;
      clean();
      setSelected(null);
      await onComplete();
      toast('Фотография сохранена');
    } catch (e) {
      if (!controller.signal.aborted) {
        setError(errorText(e));
        setRetry(!!pending.current && e instanceof ApiError && e.status === 0);
        if (!(e instanceof ApiError) || e.status !== 0) pending.current = null;
      }
    } finally {
      lock.current = false;
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  return (
    <div id={fieldID} className="upload-field">
      <label className="field">
        <span className="field__label">{label}</span>
        <input
          type="file"
          aria-label={label}
          accept="image/jpeg,image/png,image/webp"
          disabled={disabled || busy}
          onChange={(e) => {
            const file = e.currentTarget.files?.[0];
            e.currentTarget.value = '';
            if (file) {
              setError('');
              setRetry(false);
              pending.current = null;
              try {
                validateFile(file);
                setSelected(file);
              } catch (error) {
                setSelected(null);
                setError(errorText(error));
              }
            }
          }}
        />
        <span className="field__hint">
          JPEG, PNG или WebP, до 5 МБ. Фотография появится после проверки.
        </span>
      </label>
      {selected && (
        <div className="upload-preview">
          <img src={preview || undefined} alt="Предпросмотр выбранной фотографии" />
          <div>
            <strong>{selected.name}</strong>
            <small>{Math.ceil(selected.size / 1024)} КБ</small>
            <div className="button-row">
              <Button
                disabled={disabled}
                loading={busy}
                onClick={() => void upload(pending.current ? undefined : selected)}
              >
                {retry ? 'Повторить передачу' : 'Загрузить фотографию'}
              </Button>
              <Button
                variant="ghost"
                disabled={busy}
                onClick={() => {
                  setSelected(null);
                  setRetry(false);
                  pending.current = null;
                }}
              >
                Убрать файл
              </Button>
            </div>
          </div>
        </div>
      )}
      {busy && <p role="status">Загружаем и проверяем фотографию…</p>}
      <Notice error>{error}</Notice>
      {retry && (
        <Button variant="outline" onClick={() => void upload()}>
          Проверить / повторить передачу
        </Button>
      )}
    </div>
  );
}
