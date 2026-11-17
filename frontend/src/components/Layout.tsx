import { useEffect, useState } from 'react';
import { Link, NavLink, Outlet, useLocation } from 'react-router-dom';
import { useAuth } from '../auth/Auth';
import { Avatar, Icon } from './ui';
import { PageBoundary } from './PageBoundary';
import { categories } from '../lib/types';
import { useBuyer } from '../features/buyer/Buyer';

export function Layout() {
  const auth = useAuth();
  const buyer = useBuyer();
  const owned = auth.status === 'authenticated' && buyer.owner === auth.user?.id;
  const location = useLocation();
  const [menu, setMenu] = useState(false);
  const [year] = useState(() => new Date().getFullYear());
  useEffect(() => {
    setMenu(false);
    window.scrollTo({ top: 0 });
  }, [location.pathname]);
  return (
    <>
      <a className="skip-link" href="#main">
        К содержимому
      </a>
      <div className="announcement">
        <span>НЕЗАВИСИМЫЕ БРЕНДЫ. ЛИЧНЫЙ ВЫБОР.</span>
        <span>SREZ — ВНЕ ОБЩЕГО РЯДА</span>
      </div>
      <header className="header">
        <Link to="/" className="wordmark" aria-label="SREZ — главная">
          SREZ<span>®</span>
        </Link>
        <nav
          className={menu ? 'main-nav main-nav--open' : 'main-nav'}
          aria-label="Основная навигация"
        >
          <NavLink to="/catalog">Каталог</NavLink>
          <NavLink to="/brands">Бренды</NavLink>
          <NavLink to="/wishlists">Вишлисты</NavLink>
          <NavLink to="/orders">Мои заказы</NavLink>
          <NavLink to="/studio">
            Студия бренда <Icon name="diagonal" size={14} />
          </NavLink>
        </nav>
        <div className="header__right">
          <Link
            className="header-counter"
            to="/favorites"
            aria-label={
              'Избранное' + (owned && buyer.favorites ? ': ' + buyer.favorites.length : '')
            }
          >
            <Icon name="heart" />
            {owned && buyer.favorites && <span>{buyer.favorites.length}</span>}
          </Link>
          <Link
            className="header-counter"
            to="/cart"
            aria-label={
              'Корзина' +
              (owned && buyer.cart
                ? ': ' + buyer.cart.items.reduce((n, i) => n + i.quantity, 0)
                : '')
            }
          >
            <Icon name="bag" />
            {owned && buyer.cart && (
              <span>{buyer.cart.items.reduce((n, i) => n + i.quantity, 0)}</span>
            )}
          </Link>
          {auth.status === 'authenticated' && auth.user ? (
            <Link to="/account" className="account-link">
              <Avatar name={auth.user.display_name} url={auth.user.avatar_url} />
              <span>{auth.user.username}</span>
            </Link>
          ) : (
            <Link
              to={auth.status === 'unavailable' ? '/unavailable' : '/login'}
              className="account-link"
            >
              <Icon name="user" size={18} />
              <span>
                {auth.status === 'checking'
                  ? 'Проверяем вход'
                  : auth.status === 'unavailable'
                    ? 'Проверить вход'
                    : 'Войти'}
              </span>
            </Link>
          )}
          <button
            className="menu-toggle"
            aria-label="Открыть меню"
            aria-expanded={menu}
            onClick={() => setMenu(!menu)}
          >
            <Icon name={menu ? 'close' : 'menu'} />
          </button>
        </div>
      </header>
      <main id="main">
        <PageBoundary routeKey={location.pathname + location.search}>
          <Outlet />
        </PageBoundary>
      </main>
      <footer className="footer">
        <div className="footer__top">
          <Link to="/" className="wordmark wordmark--footer">
            SREZ<span>®</span>
          </Link>
          <p>
            Независимый взгляд.
            <br />
            Вещи с характером.
          </p>
          <nav aria-label="Навигация внизу страницы">
            <Link to="/catalog">
              Каталог <Icon name="diagonal" size={14} />
            </Link>
            <Link to="/brands">
              Бренды <Icon name="diagonal" size={14} />
            </Link>
            <Link to="/studio">
              Создать свой бренд <Icon name="diagonal" size={14} />
            </Link>
          </nav>
        </div>
        <div className="footer__categories" aria-label="Категории одежды">
          {categories.map(([id, label]) => (
            <Link key={id} to={'/catalog?category=' + id}>
              {label}
            </Link>
          ))}
        </div>
        <div className="footer__bottom">
          <span>© {year} SREZ</span>
          <span>СОЗДАВАЙ. НОСИ. ВЫБИРАЙ.</span>
          <span>Независимость начинается с выбора.</span>
        </div>
      </footer>
    </>
  );
}
