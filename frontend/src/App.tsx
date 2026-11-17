import { useEffect } from 'react';
import type { ReactNode } from 'react';
import {
  createBrowserRouter,
  createRoutesFromElements,
  Outlet,
  Route,
  RouterProvider,
  useLocation,
} from 'react-router-dom';
import { AuthProvider, RequireAuth } from './auth/Auth';
import { Layout } from './components/Layout';
import { ToastProvider } from './components/ui';
import { NotFoundPage, UnavailablePage } from './pages/System';
import { HomePage, CatalogPage, ProductPage, BrandsPage, BrandPage } from './pages/Public';
import { AccountPage, AuthPage } from './pages/Auth';
import { VerificationPendingPage, VerifyEmailPage } from './pages/EmailVerification';
import { ForgotPasswordPage, ResetPasswordPage } from './pages/PasswordRecovery';
import { BrandStudioPage, NewBrandPage, StudioPage } from './pages/Studio';
import { NewProductPage, ProductEditorPage } from './pages/ProductEditor';
import { UnsavedChangesProvider } from './components/UnsavedChanges';
import { WishlistProvider } from './features/wishlists/Picker';
import {
  WishlistsPage,
  NewWishlistPage,
  WishlistPage,
  SharedWishlistPage,
} from './features/wishlists/Pages';
import { DashboardLayout } from './features/dashboard/Layout';
import { BuyerProvider } from './features/buyer/Buyer';
import { FavoritesPage, CartPage, OrdersPage, OrderPage } from './features/buyer/Pages';

function Titles() {
  const { pathname } = useLocation();
  useEffect(() => {
    const title = ['/forgot-password', '/reset-password'].includes(pathname)
      ? 'Восстановление доступа'
      : pathname.startsWith('/verify-email')
        ? 'Подтверждение почты'
        : pathname === '/unavailable'
          ? 'Соединение'
          : pathname === '/'
            ? 'Независимый взгляд'
            : pathname.startsWith('/catalog')
              ? 'Каталог'
              : pathname.startsWith('/wishlists') || pathname.startsWith('/w/')
                ? 'Вишлисты'
                : pathname === '/favorites'
                  ? 'Избранное'
                  : pathname === '/cart'
                    ? 'Корзина'
                    : pathname.startsWith('/orders')
                      ? 'Заказы'
                      : pathname.startsWith('/brands')
                        ? 'Бренды'
                        : pathname.startsWith('/products')
                          ? 'Вещь'
                          : pathname === '/login'
                            ? 'Вход'
                            : pathname === '/register'
                              ? 'Регистрация'
                              : pathname === '/account'
                                ? 'Профиль'
                                : 'Студия';
    document.title = title + ' — SREZ';
  }, [pathname]);
  return null;
}
const protect = (page: ReactNode) => <RequireAuth>{page}</RequireAuth>;
function AppProviders() {
  return (
    <AuthProvider>
      <ToastProvider>
        <BuyerProvider>
          <UnsavedChangesProvider>
            <WishlistProvider>
              <Titles />
              <Outlet />
            </WishlistProvider>
          </UnsavedChangesProvider>
        </BuyerProvider>
      </ToastProvider>
    </AuthProvider>
  );
}
const router = createBrowserRouter(
  createRoutesFromElements(
    <Route element={<AppProviders />}>
      <Route path="verify-email" element={<VerifyEmailPage />} />
      <Route path="verify-email/pending" element={<VerificationPendingPage />} />
      <Route path="forgot-password" element={<ForgotPasswordPage />} />
      <Route path="reset-password" element={<ResetPasswordPage />} />
      <Route element={<Layout />}>
        <Route index element={<HomePage />} />
        <Route path="catalog" element={<CatalogPage />} />
        <Route path="products/:productId" element={<ProductPage />} />
        <Route path="brands" element={<BrandsPage />} />
        <Route path="brands/:slug" element={<BrandPage />} />
        <Route path="login" element={<AuthPage key="login" />} />
        <Route path="register" element={<AuthPage key="register" register />} />
        <Route path="account" element={protect(<AccountPage />)} />
        <Route path="favorites" element={protect(<FavoritesPage />)} />
        <Route path="cart" element={protect(<CartPage />)} />
        <Route path="orders" element={protect(<OrdersPage />)} />
        <Route path="orders/:orderId" element={protect(<OrderPage />)} />
        <Route path="wishlists" element={protect(<WishlistsPage />)} />
        <Route path="wishlists/new" element={protect(<NewWishlistPage />)} />
        <Route path="wishlists/:id" element={protect(<WishlistPage />)} />
        <Route path="w/:token" element={<SharedWishlistPage />} />
        <Route path="studio" element={protect(<DashboardLayout />)}>
          <Route index element={<StudioPage />} />
          <Route path="invitations" element={<StudioPage invitationsOnly />} />
          <Route path="brands/new" element={<NewBrandPage />} />
          <Route path="brands/:brandId" element={<BrandStudioPage />} />
          <Route path="brands/:brandId/products/new" element={<NewProductPage />} />
          <Route path="brands/:brandId/products/:productId" element={<ProductEditorPage />} />
        </Route>
        <Route path="unavailable" element={<UnavailablePage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Route>,
  ),
);
export default function App() {
  return <RouterProvider router={router} />;
}
