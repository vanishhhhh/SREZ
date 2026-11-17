import { createContext, Fragment, useContext, useEffect, useSyncExternalStore } from 'react';
import type { ReactNode } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { session } from '../lib/client';
import { ServerUnavailable } from '../components/ServerUnavailable';

const AuthContext = createContext<ReturnType<typeof session.snapshot> | null>(null);
export function AuthProvider({ children }: { children: ReactNode }) {
  const state = useSyncExternalStore(session.subscribe, session.snapshot, session.snapshot);
  useEffect(() => {
    void session.bootstrap();
  }, []);
  return <AuthContext.Provider value={state}>{children}</AuthContext.Provider>;
}
export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error('Auth provider missing');
  return value;
}
export function RequireAuth({ children }: { children: ReactNode }) {
  const auth = useAuth();
  const location = useLocation();
  if (auth.status === 'checking')
    return (
      <div className="page-loading" role="status">
        <span className="spinner" />
        Восстанавливаем сессию…
      </div>
    );
  if (auth.status === 'unavailable')
    return (
      <ServerUnavailable
        error={auth.error}
        status={auth.errorStatus}
        onRetry={() => void session.restore()}
      />
    );
  if (auth.status !== 'authenticated')
    return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />;
  return <Fragment key={auth.user?.id}>{children}</Fragment>;
}
