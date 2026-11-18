import { useState } from 'react';
import { useFormDraft } from '../lib/hooks';
import type { FormEvent } from 'react';
import { Link, Navigate, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/Auth';
import { session } from '../lib/client';
import { ApiError } from '../lib/http';
import { passwordValid, validReturnPath } from '../lib/format';
import { InputField, PasswordField } from '../components/forms';
import { UploadField } from '../features/media/UploadField';
import { deleteAvatar } from '../features/media/api';
import { Avatar, Button, Icon, Notice, PageTitle, useAction } from '../components/ui';

export function AuthPage({ register = false }: { register?: boolean }) {
  const auth = useAuth();
  const location = useLocation();
  const navigate = useNavigate();
  const action = useAction();
  const [needsVerification, setNeedsVerification] = useState('');
  const [form, setForm] = useState({
    identifier: '',
    email: '',
    username: '',
    display_name: '',
    password: '',
  });
  const from = validReturnPath((location.state as { from?: unknown } | null)?.from);
  const creating = register;
  const checking = auth.status === 'checking';
  const formError = action.error || auth.error;
  if (auth.status === 'authenticated') return <Navigate to={from} replace />;
  const field = (name: keyof typeof form, value: string) =>
    setForm((previous) => ({ ...previous, [name]: value }));
  async function submit(event: FormEvent) {
    event.preventDefault();
    await action.run(async () => {
      const identifier = form.identifier;
      if (creating) {
        if (!passwordValid(form.password))
          throw new Error('Пароль: минимум 12 символов и максимум 1024 байта.');
        if (!form.display_name.trim()) throw new Error('Укажите имя.');
        const created = await session.register({
          email: form.email.trim(),
          username: form.username.trim(),
          display_name: form.display_name.trim(),
          password: form.password,
        });
        setForm((previous) => ({ ...previous, password: '' }));
        navigate('/verify-email/pending', {
          replace: true,
          state: {
            identifier: form.email.trim(),
            delivery: created.email_delivery_status,
            retryAfter: created.resend_after_seconds,
            from,
          },
        });
        return;
      }
      try {
        await session.login(identifier, form.password);
      } catch (error) {
        if (error instanceof ApiError && error.code === 'email verification required') {
          setNeedsVerification(identifier);
          setForm((previous) => ({ ...previous, password: '' }));
        }
        throw error;
      }

      setForm((previous) => ({ ...previous, password: '' }));
      navigate(from, { replace: true });
    }, undefined);
  }
  return (
    <div className="auth-page">
      <div className="auth-story">
        <Link to="/" className="wordmark">
          SREZ<span>®</span>
        </Link>
      </div>
      <div className="auth-panel">
        <div className="auth-form">
          <h1>{creating ? 'Регистрация' : 'Вход'}</h1>
          <p className="muted">
            {creating
              ? 'Один аккаунт — для выбора вещей и создания своего бренда.'
              : 'Войдите по email или имени пользователя.'}
          </p>
          {needsVerification && (
            <Notice>
              <Link
                to="/verify-email/pending"
                state={{ identifier: needsVerification, delivery: 'unknown', from }}
              >
                Запросить письмо подтверждения
              </Link>
            </Notice>
          )}
          {auth.status === 'unavailable' && (
            <div className="auth-recovery">
              <Notice error>{auth.error}</Notice>
              <Button
                variant="outline"
                onClick={() => void session.restore()}
                disabled={action.busy}
              >
                Восстановить сессию
              </Button>
              <p className="field__hint">
                Можно повторить проверку существующей сессии или войти заново ниже.
              </p>
            </div>
          )}
          <form onSubmit={submit} aria-label={creating ? 'Регистрация' : 'Вход'}>
            <fieldset disabled={action.busy || checking} className="form-stack">
              {creating ? (
                <>
                  <InputField
                    label="Как к вам обращаться"
                    autoComplete="name"
                    required
                    value={form.display_name}
                    onChange={(e) => field('display_name', e.target.value)}
                    placeholder="Ваше имя"
                  />
                  <InputField
                    label="Имя пользователя"
                    hint="3–30 латинских букв, цифр или _"
                    autoComplete="username"
                    required
                    minLength={3}
                    maxLength={30}
                    pattern="[A-Za-z0-9_]{3,30}"
                    value={form.username}
                    onChange={(e) => field('username', e.target.value)}
                    placeholder="your_name"
                  />
                  <InputField
                    label="Email"
                    type="email"
                    autoComplete="email"
                    required
                    maxLength={254}
                    value={form.email}
                    onChange={(e) => field('email', e.target.value)}
                    placeholder="you@example.com"
                  />
                </>
              ) : (
                <InputField
                  label="Email или имя пользователя"
                  autoComplete="username"
                  required
                  value={form.identifier}
                  onChange={(e) => field('identifier', e.target.value)}
                  placeholder="you@example.com"
                />
              )}
              <PasswordField
                hint={creating ? 'Не менее 12 символов. Пробелы в пароле сохраняются.' : undefined}
                autoComplete={creating ? 'new-password' : 'current-password'}
                required
                value={form.password}
                onChange={(e) => field('password', e.target.value)}
                placeholder="Введите пароль"
              />
              <Notice error>
                {auth.status === 'unavailable' && formError === auth.error ? null : formError}
              </Notice>
              {!creating && (
                <Link className="text-link" to="/forgot-password">
                  Забыли пароль?
                </Link>
              )}
              {checking && (
                <p className="auth-checking" role="status">
                  <span className="spinner" />
                  Проверяем сессию…
                </p>
              )}
              <Button
                type="submit"
                loading={action.busy}
                disabled={checking}
                className="full-width"
              >
                {creating ? 'Создать аккаунт' : 'Войти'}
                <Icon name="arrow" />
              </Button>
            </fieldset>
          </form>
          <p className="auth-switch">
            {register ? 'Уже в SREZ?' : 'Пока нет аккаунта?'}{' '}
            <Link state={{ from }} to={register ? '/login' : '/register'}>
              {register ? 'Войти' : 'Присоединиться'}
            </Link>
          </p>
          <Link to="/" className="text-link muted">
            <Icon name="back" size={16} />
            Продолжить без входа
          </Link>
        </div>
      </div>
    </div>
  );
}

export function AccountPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const action = useAction();
  const [name, setName] = useFormDraft(user?.display_name || '', String(user?.id));
  const [saved, setSaved] = useState(false);
  if (!user) return null;
  const dirty = name.trim() !== user.display_name;
  function save(event: FormEvent) {
    event.preventDefault();
    setSaved(false);
    void action.run(async () => {
      if (!name.trim()) throw new Error('Укажите имя.');
      const updated = await session.updateProfile({ display_name: name.trim() });
      setName(updated.display_name);
      setSaved(true);
    }, 'Профиль сохранён');
  }
  return (
    <div className="container page">
      <PageTitle eyebrow="ЛИЧНОЕ ПРОСТРАНСТВО" title="Мой профиль" />
      <div className="account-layout">
        <aside className="account-summary">
          <Avatar name={user.display_name} url={user.avatar_url} large />
          <h2>{user.display_name}</h2>
          <p className="muted">@{user.username}</p>
          <Link className="button button--outline full-width" to="/studio">
            Студия бренда <Icon name="diagonal" size={16} />
          </Link>
          <Button
            variant="ghost"
            onClick={() =>
              void action.run(async () => {
                await session.logout();
                navigate('/');
              })
            }
            loading={action.busy}
          >
            <Icon name="logout" size={16} />
            Выйти из аккаунта
          </Button>
        </aside>
        <section className="panel">
          <div className="panel-heading">
            <h2>Личные данные</h2>
            <span className="mono">01 / PROFILE</span>
          </div>
          <form onSubmit={save} className="form-stack" aria-label="Изменение профиля">
            <fieldset disabled={action.busy} className="form-stack">
              <InputField
                label="Имя"
                autoComplete="name"
                required
                value={name}
                onChange={(e) => {
                  setName(e.target.value);
                  setSaved(false);
                }}
              />
              <div className="form-grid">
                <InputField label="Имя пользователя" value={user.username} readOnly />
                <InputField label="Email" value={user.email} readOnly />
              </div>
              <Notice error>{action.error}</Notice>
              {saved && !dirty && (
                <p className="form-success" role="status">
                  Изменения профиля сохранены.
                </p>
              )}
              <Button type="submit" loading={action.busy} disabled={!dirty}>
                Сохранить профиль <Icon name="arrow" />
              </Button>
            </fieldset>
          </form>
          <div className="email-status" role="status">
            <strong>{user.email_verified_at ? 'Email подтверждён' : 'Email не подтверждён'}</strong>
            <p className="field__hint">
              {user.email_verified_at
                ? 'Подтверждён ' + new Date(user.email_verified_at).toLocaleDateString('ru-RU')
                : user.email_verification_required
                  ? 'Для входа требуется подтверждение адреса.'
                  : 'Для этого аккаунта вход пока доступен без подтверждения.'}
            </p>
            {!user.email_verified_at && (
              <Link
                className="text-link"
                to="/verify-email/pending"
                state={{ identifier: user.email, delivery: 'unknown' }}
              >
                Подтвердить почту
              </Link>
            )}
          </div>
          <UploadField
            label="Загрузить аватар"
            target={{ purpose: 'user_avatar' }}
            onComplete={() => session.reloadProfile()}
          />
          {user.avatar_url && (
            <Button
              variant="outline"
              disabled={action.busy}
              onClick={() =>
                void action.run(async () => {
                  await deleteAvatar();
                  await session.reloadProfile();
                }, 'Аватар удалён')
              }
            >
              Удалить аватар
            </Button>
          )}
        </section>
      </div>
    </div>
  );
}
