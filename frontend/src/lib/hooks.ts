import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { errorText } from './format';
import { ApiError } from './http';

// Sync persisted JSON form values only when the server changes them. Initial
// mounting never clears fast input; a later response never overwrites a draft.
export function useFormDraft<T>(persisted: T, key: string) {
  const signature = JSON.stringify(persisted);
  const previous = useRef({ key, signature });
  const [draft, setDraft] = useState(persisted);
  useEffect(() => {
    const before = previous.current;
    if (before.key === key && before.signature === signature) return;
    previous.current = { key, signature };
    setDraft((current) =>
      before.key !== key || JSON.stringify(current) === before.signature
        ? (JSON.parse(signature) as T)
        : current,
    );
  }, [key, signature]);
  return [draft, setDraft] as const;
}

export function useLoad<T>(loader: (signal: AbortSignal) => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState('');
  const [errorStatus, setErrorStatus] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const identity = JSON.stringify(deps);
  const [loadedFor, setLoadedFor] = useState(identity);
  const current = useRef({ identity, loader });
  useLayoutEffect(() => {
    current.current = { identity, loader };
  });
  const active = useRef<AbortController | null>(null);

  // Mutations can await the fresh collection before re-enabling controls.
  const reload = useCallback(async () => {
    if (current.current.identity !== identity) return;
    active.current?.abort();
    const controller = new AbortController();
    active.current = controller;
    setLoading(true);
    setError('');
    setErrorStatus(null);
    try {
      const value = await current.current.loader(controller.signal);
      if (!controller.signal.aborted) {
        setData(value);
        setLoadedFor(identity);
      }
    } catch (err) {
      if (!controller.signal.aborted) {
        setError(errorText(err));
        setErrorStatus(err instanceof ApiError ? err.status : null);
        setData(null);
        setLoadedFor(identity);
      }
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [identity]);

  useEffect(() => {
    void reload();
    return () => active.current?.abort();
  }, [reload]);
  return {
    data: loadedFor === identity ? data : null,
    error: loadedFor === identity ? error : '',
    errorStatus: loadedFor === identity ? errorStatus : null,
    loading: loading || loadedFor !== identity,
    reload,
    setData,
  };
}
