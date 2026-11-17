import { useEffect, useState } from 'react';
import type { FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { AuthFrame } from '../components/AuthFrame';
import { InputField, PasswordField } from '../components/forms';
import { Button, Icon, Notice, useAction } from '../components/ui';
import { session } from '../lib/client';
import { passwordValid } from '../lib/format';
import { resetLinkToken, forgetResetLink } from '../lib/verification-link';

export function ForgotPasswordPage() {
  const [email, setEmail] = useState(''),
    [requested, setRequested] = useState(false),
    [deadline, setDeadline] = useState(0),
    [now, setNow] = useState(Date.now);
  const action = useAction();
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  const remaining = Math.max(0, Math.ceil((deadline - now) / 1000));
  async function submit(e: FormEvent) {
    e.preventDefault();
    await action.run(async () => {
      await session.request('/auth/forgot-password', {
        method: 'POST',
        body: { email: email.trim() },
      });
      setRequested(true);
      setDeadline(Date.now() + 60000);
      setNow(Date.now());
    });
  }
  return (
    <AuthFrame>
      <h1>Вернём доступ.</h1>
      <p className="muted">
        Укажите подтверждённый email вашего аккаунта. Мы попробуем отправить ссылку для нового
        пароля.
      </p>
      <form className="form-stack" onSubmit={submit} aria-label="Восстановление пароля">
        <InputField
          label="Email"
          type="email"
          autoComplete="email"
          required
          maxLength={254}
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        <Notice error>{action.error}</Notice>
        {requested && (
          <Notice>
            Если адрес подходит для восстановления, мы попробуем отправить письмо. Проверьте
            Mailpit; при отсутствии письма повторите позже. Ответ не подтверждает наличие аккаунта
            или доставку.
          </Notice>
        )}
        <Button type="submit" loading={action.busy} disabled={remaining > 0}>
          {remaining > 0 ? `Повторить через ${remaining} с` : 'Отправить ссылку'}
          <Icon name="arrow" />
        </Button>
      </form>
      <p className="field__hint">
        Ссылка действует 30 минут. Не чаще одного письма в минуту и не более пяти в час. Если email
        ещё не подтверждён, сначала откройте письмо подтверждения.
      </p>
      <Link className="text-link" to="/login">
        Вернуться ко входу
      </Link>
    </AuthFrame>
  );
}
export function ResetPasswordPage() {
  const [token, setToken] = useState(resetLinkToken),
    [password, setPassword] = useState(''),
    [repeat, setRepeat] = useState(''),
    [success, setSuccess] = useState(false);
  const action = useAction();
  useEffect(() => () => forgetResetLink(), []);
  async function submit(e: FormEvent) {
    e.preventDefault();
    await action.run(async () => {
      if (!passwordValid(password))
        throw Error('Пароль: минимум 12 символов и максимум 1024 байта.');
      if (password !== repeat) throw Error('Пароли не совпадают.');
      await session.resetPassword(token, password);
      setPassword('');
      setRepeat('');
      setToken('');
      forgetResetLink();
      setSuccess(true);
    });
  }
  return (
    <AuthFrame>
      <h1>{success ? 'Пароль обновлён.' : 'Новый пароль.'}</h1>
      {success ? (
        <>
          <p className="muted">
            Все прежние сессии этого аккаунта завершены. Войдите с новым паролем.
          </p>
          <Link className="button button--solid" to="/login">
            Войти с новым паролем
            <Icon name="arrow" />
          </Link>
        </>
      ) : (
        <>
          <p className="muted">
            Придумайте новый пароль. Ссылка используется только после отправки формы.
          </p>
          {!token ? (
            <Notice error>
              Ссылка отсутствует или недействительна. Откройте последнее письмо либо запросите
              новую.
            </Notice>
          ) : (
            <form className="form-stack" onSubmit={submit} aria-label="Сброс пароля">
              <fieldset className="form-stack" disabled={action.busy}>
                <PasswordField
                  label="Новый пароль"
                  autoComplete="new-password"
                  required
                  minLength={12}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  hint="Минимум 12 символов. Пробелы сохраняются."
                />
                <PasswordField
                  label="Повторите новый пароль"
                  autoComplete="new-password"
                  required
                  minLength={12}
                  value={repeat}
                  onChange={(e) => setRepeat(e.target.value)}
                />
                <Notice error>{action.error}</Notice>
                <Button type="submit" loading={action.busy}>
                  Сохранить новый пароль
                  <Icon name="arrow" />
                </Button>
              </fieldset>
            </form>
          )}
          <p>
            <Link className="text-link" to="/forgot-password">
              Запросить новую ссылку
            </Link>
          </p>
        </>
      )}
    </AuthFrame>
  );
}
