import { useEffect, useRef, useState } from 'react';
import { Link, NavLink, Outlet, useLocation, useMatches } from 'react-router-dom';
import { useAuth } from '../auth/Auth';
import { Avatar, Icon } from './ui';
import { PageBoundary } from './PageBoundary';
import { useBuyer } from '../features/buyer/Buyer';

export function Layout() {
  const auth = useAuth();
  const buyer = useBuyer();
  const owned = auth.status === 'authenticated' && buyer.owner === auth.user?.id;
  const location = useLocation();
  const storefront = useMatches().some((match) => match.handle === 'storefront');
  const [menu, setMenu] = useState(false);
  const [year] = useState(() => new Date().getFullYear());
  const menuButton = useRef<HTMLButtonElement>(null);
  const navigation = useRef<HTMLElement>(null);
  const signInLabel =
    auth.status === 'checking'
      ? 'Проверяем вход'
      : auth.status === 'unavailable'
        ? 'Проверить вход'
        : 'Войти';
  useEffect(() => {
    if (!menu) return;
    navigation.current?.querySelector('a')?.focus();
    function closeOnEscape(event: KeyboardEvent) {
      if (event.key !== 'Escape') return;
      setMenu(false);
      menuButton.current?.focus();
    }
    document.addEventListener('keydown', closeOnEscape);
    return () => document.removeEventListener('keydown', closeOnEscape);
  }, [menu]);
  useEffect(() => {
    setMenu(false);
    window.scrollTo({ top: 0 });
  }, [location.pathname]);
  return (
    <div className="site-layout" id="site-top" tabIndex={-1}>
      <a className="skip-link" href="#main">
        К содержимому
      </a>
      <header className="header">
        <Link to="/" className="wordmark" aria-label="SREZ — главная">
          SREZ<span>®</span>
        </Link>
        <nav
          ref={navigation}
          id="main-navigation"
          className={menu ? 'main-nav main-nav--open' : 'main-nav'}
          aria-label="Основная навигация"
        >
          <NavLink to="/catalog">Каталог</NavLink>
          <NavLink to="/brands">Бренды</NavLink>
          <NavLink to="/wishlists">Вишлисты</NavLink>
          <NavLink to="/orders">Мои заказы</NavLink>
          <NavLink to="/studio">Студия бренда</NavLink>
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
            <Link to="/account" className="account-link" aria-label="Мой профиль">
              <Avatar name={auth.user.display_name} url={auth.user.avatar_url} />
              <span>{auth.user.username}</span>
            </Link>
          ) : (
            <Link
              to={auth.status === 'unavailable' ? '/unavailable' : '/login'}
              className="account-link"
              aria-label={signInLabel}
            >
              <Icon name="user" size={18} />
              <span>{signInLabel}</span>
            </Link>
          )}
          <button
            ref={menuButton}
            className="menu-toggle"
            aria-label={menu ? 'Закрыть меню' : 'Открыть меню'}
            aria-controls="main-navigation"
            aria-expanded={menu}
            onClick={() => setMenu((open) => !open)}
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
      <footer className={storefront ? 'footer footer--storefront' : 'footer'}>
        <div className="container footer__content">
          <small>© {year} SREZ</small>
          {storefront && <a href="#site-top">Наверх</a>}
        </div>
      </footer>
    </div>
  );
}
