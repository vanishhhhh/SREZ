import { Link } from 'react-router-dom';
import { Button, Icon, PageTitle } from './ui';

export function ServerUnavailable({
  error,
  status,
  busy = false,
  onRetry,
}: {
  error?: string | null;
  status?: number | null;
  busy?: boolean;
  onRetry: () => void;
}) {
  const connection = status === 0 || (status != null && status >= 500) || status == null;
  return (
    <section
      className="container page page--narrow system-page"
      aria-label="Восстановление соединения"
    >
      <span className="system-mark" aria-hidden="true">
        [ — ]
      </span>
      <PageTitle
        eyebrow="SREZ / СОЕДИНЕНИЕ"
        title={connection ? 'Сервер временно недоступен.' : 'Не удалось подтвердить сессию.'}
      />
      <p className="muted">
        {error ||
          'Не удалось связаться с сервером. Повторите попытку, когда соединение восстановится.'}
      </p>
      <p className="muted">
        Ваши данные не удалены. Доступ к личным разделам откроется после проверки сессии.
      </p>
      <div className="button-row">
        <Button onClick={onRetry} loading={busy}>
          Повторить подключение <Icon name="arrow" size={16} />
        </Button>
        <Link to="/login" className="button button--outline">
          Войти заново
        </Link>
      </div>
      <Link className="text-link muted" to="/">
        На главную <Icon name="diagonal" size={16} />
      </Link>
    </section>
  );
}
