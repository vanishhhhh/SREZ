import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/Auth';
import { ServerUnavailable } from '../components/ServerUnavailable';
import { Icon, PageTitle, useAction } from '../components/ui';
import { session } from '../lib/client';
import { validReturnPath } from '../lib/format';

export function NotFoundPage() {
  return (
    <div className="container page not-found">
      <span className="not-found__number">404</span>
      <PageTitle eyebrow="НЕМНОГО ВНЕ МАРШРУТА" title="Здесь пока ничего нет." />
      <Link className="button button--solid" to="/">
        На главную <Icon name="arrow" />
      </Link>
    </div>
  );
}

export function UnavailablePage() {
  const auth = useAuth();
  const action = useAction();
  const navigate = useNavigate();
  const location = useLocation();
  const from = validReturnPath((location.state as { from?: unknown } | null)?.from);
  const retry = () =>
    void action.run(async () => {
      await session.checkServer();
      await session.restore();
      if (session.snapshot().status !== 'unavailable') navigate(from, { replace: true });
    });
  return (
    <ServerUnavailable
      error={action.error || auth.error}
      status={auth.errorStatus}
      busy={action.busy || auth.status === 'checking'}
      onRetry={retry}
    />
  );
}
