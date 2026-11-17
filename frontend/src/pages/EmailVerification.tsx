import { useEffect, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { session } from '../lib/client';
import { ApiError } from '../lib/http';
import { validReturnPath } from '../lib/format';
import { verificationLinkToken, forgetVerificationLink } from '../lib/verification-link';
import { InputField } from '../components/forms';
import { Button, Icon, Notice, useAction } from '../components/ui';
import { AuthFrame as Frame } from '../components/AuthFrame';
const mask = (value: string) => {
  const index = value.indexOf('@');
  return index > 0 ? value.slice(0, Math.min(2, index)) + '…' + value.slice(index) : '@' + value;
};
export function VerificationPendingPage() {
  const { state } = useLocation();
  const initial = state && typeof state === 'object' ? (state as Record<string, unknown>) : {};
  const [identifier, setIdentifier] = useState(
    typeof initial.identifier === 'string' ? initial.identifier : '',
  );
  const [deadline, setDeadline] = useState(
    () =>
      Date.now() +
      (typeof initial.retryAfter === 'number'
        ? Math.min(3600, Math.max(0, initial.retryAfter))
        : 0) *
        1000,
  );
  const [now, setNow] = useState(() => Date.now());
  const [requested, setRequested] = useState(false);
  const action = useAction();
  const remaining = Math.max(0, Math.ceil((deadline - now) / 1000));
  const from = validReturnPath(initial.from);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  async function resend() {
    await action.run(async () => {
      try {
        await session.request('/auth/resend-verification', {
          method: 'POST',
          body: { identifier: identifier.trim() },
        });
        setRequested(true);
        setDeadline(Date.now() + 60000);
        setNow(Date.now());
      } catch (error) {
        if (error instanceof ApiError && error.status === 429) {
          setDeadline(Date.now() + 60000);
          setNow(Date.now());
        }
        throw error;
      }
    });
  }
  return (
    <Frame>
      <div className="verification-symbol">
        <Icon name="arrow" size={32} />
      </div>
      <h1>{initial.delivery === 'sent' ? 'Проверьте почту.' : 'Подтвердите email.'}</h1>
      {initial.delivery === 'sent' ? (
        <p className="muted">
          Мы отправили письмо для подтверждения на <strong>{mask(identifier)}</strong>. Ссылка
          действует 30 минут.
        </p>
      ) : (
        <p className="muted">Подтвердите адрес из письма, чтобы войти в свой аккаунт.</p>
      )}
      {initial.delivery === 'unavailable' && (
        <Notice error>
          Аккаунт создан, но письмо отправить не удалось. Проверьте Mailpit и повторите отправку
          ниже. Регистрироваться заново не нужно.
        </Notice>
      )}
      <form
        className="form-stack"
        aria-label="Повторная отправка письма"
        onSubmit={(event) => {
          event.preventDefault();
          void resend();
        }}
      >
        <InputField
          label="Email или имя пользователя"
          autoComplete="username"
          required
          maxLength={254}
          value={identifier}
          onChange={(event) => setIdentifier(event.target.value)}
        />
        <Notice error>{action.error}</Notice>
        {requested && (
          <Notice>
            Если аккаунт существует и почта ещё не подтверждена, мы попытаемся отправить новую
            ссылку. Проверьте входящие; если письма нет, повторите позже. Прежняя ссылка после
            перевыпуска не действует.
          </Notice>
        )}
        <Button type="submit" loading={action.busy} disabled={remaining > 0 || !identifier.trim()}>
          {remaining > 0 ? `Повторить через ${remaining} с` : 'Отправить новую ссылку'}
        </Button>
      </form>
      <p className="field__hint">
        Не чаще одного письма в минуту и не более пяти в час. В локальной версии письма находятся в
        Mailpit — настоящая почта не используется.
      </p>
      <Link className="text-link" to="/login" state={{ from }}>
        Перейти ко входу <Icon name="arrow" size={16} />
      </Link>
    </Frame>
  );
}
export function VerifyEmailPage() {
  const [token, setToken] = useState(verificationLinkToken);
  const [success, setSuccess] = useState(false);
  const action = useAction();
  useEffect(() => () => forgetVerificationLink(), []);
  async function confirm() {
    await action.run(async () => {
      await session.request('/auth/verify-email', { method: 'POST', body: { token } });
      setToken('');
      forgetVerificationLink();
      setSuccess(true);
      if (session.snapshot().status === 'authenticated') await session.reloadProfile();
    });
  }
  return (
    <Frame>
      <h1>{success ? 'Почта подтверждена.' : 'Один шаг до своего круга.'}</h1>
      {success ? (
        <>
          <p className="muted">Адрес подтверждён. Теперь можно войти в SREZ с вашим паролем.</p>
          <Link className="button button--solid" to="/login" state={{ from: '/account' }}>
            Войти в SREZ <Icon name="arrow" />
          </Link>
        </>
      ) : (
        <>
          <p className="muted">
            Нажмите кнопку, чтобы подтвердить email. Открытие страницы само по себе не активирует
            аккаунт.
          </p>
          {!token ? (
            <Notice error>
              Ссылка отсутствует или недействительна. Откройте последнее письмо либо запросите новую
              ссылку.
            </Notice>
          ) : (
            <Button onClick={() => void confirm()} loading={action.busy}>
              Подтвердить почту <Icon name="arrow" />
            </Button>
          )}
          <Notice error>{action.error}</Notice>
          <p>
            <Link className="text-link" to="/verify-email/pending">
              Запросить новую ссылку
            </Link>
          </p>
        </>
      )}
    </Frame>
  );
}
