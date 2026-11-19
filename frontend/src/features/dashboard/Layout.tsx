import { useEffect } from 'react';
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { api } from '../../lib/api';
import { useLoad } from '../../lib/hooks';
import { Icon } from '../../components/ui';

export function DashboardLayout() {
  const location = useLocation(),
    navigate = useNavigate();
  const feed = useLoad((signal) => api.myBrands(signal), []);
  const reload = feed.reload;
  const active = location.pathname.match(/^\/studio\/brands\/(\d+)(?:\/|$)/)?.[1] || '';
  const brand = feed.data?.find((item) => String(item.brand.id) === active);
  useEffect(() => {
    const refresh = () => void reload();
    window.addEventListener('focus', refresh);
    window.addEventListener('srez:brands-changed', refresh);
    return () => {
      window.removeEventListener('focus', refresh);
      window.removeEventListener('srez:brands-changed', refresh);
    };
  }, [reload]);
  useEffect(() => {
    void reload();
  }, [location.pathname, reload]);
  return (
    <div className="dashboard-shell">
      <aside className="dashboard-sidebar" aria-label="Навигация студии">
        <div className="dashboard-sidebar__brand">
          <span className="eyebrow">SREZ / ДЛЯ БРЕНДОВ</span>
          <h2>
            Твоя студия<span>.</span>
          </h2>
        </div>
        <nav className="dashboard-nav">
          <NavLink end to="/studio">
            <Icon name="menu" size={18} />
            Мои бренды
          </NavLink>
          <NavLink to="/studio/brands/new">
            <Icon name="plus" size={18} />
            Создать бренд
          </NavLink>
          <NavLink to="/studio/invitations">
            <Icon name="user" size={18} />
            Приглашения
          </NavLink>
        </nav>
        {feed.error ? (
          <p className="field__hint">
            Список брендов временно недоступен.{' '}
            <button type="button" className="text-link" onClick={() => void reload()}>
              Повторить
            </button>
          </p>
        ) : (
          <label className="field dashboard-brand-select">
            <span className="field__label">Активный бренд</span>
            <select
              value={active}
              disabled={feed.loading && !feed.data}
              onChange={(event) => {
                if (event.target.value) navigate('/studio/brands/' + event.target.value);
              }}
            >
              <option value="">Выбрать бренд</option>
              {feed.data?.map((item) => (
                <option key={item.brand.id} value={item.brand.id}>
                  {item.brand.name} · {item.role === 'owner' ? 'владелец' : 'редактор'}
                </option>
              ))}
            </select>
          </label>
        )}
        {brand && (
          <>
            <p className="dashboard-role">
              {brand.role === 'owner' ? 'Владелец' : 'Редактор'} /{' '}
              {brand.brand.status === 'active'
                ? 'Опубликован'
                : brand.brand.status === 'suspended'
                  ? 'Заблокирован'
                  : 'Черновик'}
            </p>
          </>
        )}
        <div className="dashboard-sidebar__foot">
          <Link className="text-link" to="/catalog">
            Вернуться к вещам <Icon name="diagonal" size={16} />
          </Link>
        </div>
      </aside>
      <div className="dashboard-main">
        <Outlet />
      </div>
    </div>
  );
}
