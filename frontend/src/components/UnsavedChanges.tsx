import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from 'react';
import type { ReactNode } from 'react';
import { useBeforeUnload, useBlocker } from 'react-router-dom';
import { Confirm } from './ui';

type Actions = {
  set: (id: string, label: string | null) => void;
  request: (action: () => void) => void;
  clearAll: () => void;
};
const ActionsContext = createContext<Actions | null>(null);
const DirtyContext = createContext(false);

export function UnsavedChangesProvider({ children }: { children: ReactNode }) {
  const entries = useRef(new Map<string, string>());
  const [labels, setLabels] = useState<string[]>([]);
  const [pending, setPending] = useState<{ run: () => void } | null>(null);
  const set = useCallback((id: string, label: string | null) => {
    if (entries.current.get(id) === (label ?? undefined)) return;
    if (label) entries.current.set(id, label);
    else entries.current.delete(id);
    setLabels([...entries.current.values()]);
  }, []);
  const request = useCallback((run: () => void) => {
    if (entries.current.size) setPending({ run });
    else run();
  }, []);
  const clearAll = useCallback(() => {
    entries.current.clear();
    setLabels([]);
  }, []);
  const blocker = useBlocker(
    useCallback(
      ({ currentLocation, nextLocation }) =>
        entries.current.size > 0 &&
        (currentLocation.pathname !== nextLocation.pathname ||
          currentLocation.search !== nextLocation.search),
      [],
    ),
  );
  useBeforeUnload(
    useCallback((event) => {
      if (entries.current.size) {
        event.preventDefault();
        event.returnValue = '';
      }
    }, []),
  );
  const actions = useMemo(() => ({ set, request, clearAll }), [set, request, clearAll]);
  const dirty = labels.length > 0;
  return (
    <ActionsContext.Provider value={actions}>
      <DirtyContext.Provider value={dirty}>
        {children}
        <Confirm
          open={blocker.state === 'blocked' || !!pending}
          title="Остались несохранённые изменения"
          confirmLabel="Продолжить без сохранения"
          busy={false}
          onCancel={() => {
            if (blocker.state === 'blocked') blocker.reset();
            setPending(null);
          }}
          onConfirm={() => {
            if (blocker.state === 'blocked') blocker.proceed();
            const action = pending;
            setPending(null);
            action?.run();
          }}
        >
          {`Изменения в форме могут потеряться: ${[...new Set(labels)].join(', ')}. Можно остаться и сохранить их.`}
        </Confirm>
      </DirtyContext.Provider>
    </ActionsContext.Provider>
  );
}
export function useUnsavedChanges(dirty: boolean, label: string, fingerprint: string) {
  const actions = useContext(ActionsContext);
  if (!actions) throw Error('Unsaved changes provider missing');
  const id = useId();
  const accepted = useRef<string | null>(null);
  const set = actions.set;
  useEffect(() => {
    if (!dirty) accepted.current = null;
    set(id, dirty && accepted.current !== fingerprint ? label : null);
    return () => set(id, null);
  }, [dirty, label, fingerprint, id, set]);
  return useCallback(() => {
    accepted.current = fingerprint;
    set(id, null);
  }, [id, set, fingerprint]);
}
export const useHasUnsavedChanges = () => useContext(DirtyContext);
export function useConfirmDiscard() {
  const value = useContext(ActionsContext);
  if (!value) throw Error('Unsaved changes provider missing');
  return value.request;
}
export function useClearUnsavedChanges() {
  const value = useContext(ActionsContext);
  if (!value) throw Error('Unsaved changes provider missing');
  return value.clearAll;
}
