import { Link, useLocation } from 'react-router-dom';
import { Icon } from '../../components/ui';
import { validReturnPath } from '../../lib/format';
import studioCover from '../storefront/editorial/fashion-cover.webp';
import studioCoverMobile from '../storefront/editorial/fashion-cover-mobile.webp';
import './guest-section.css';

const sections = {
  wishlists: {
    label: 'Вишлисты',
    title: 'Сохраняй то, что нравится',
    description:
      'Собирай любимые вещи в одном месте. Создавай собственные подборки, сохраняй интересные товары и делись находками с друзьями.',
    register: 'Создать аккаунт',
    login: 'Уже есть аккаунт? Войти',
    benefits: [
      [
        'heart',
        'Сохраняй понравившееся',
        'Добавляй товары в свои подборки, чтобы не потерять интересные вещи.',
      ],
      [
        'bookmark',
        'Создавай подборки',
        'Организуй сохранённые товары по стилю, настроению или будущим покупкам.',
      ],
      [
        'copy',
        'Делись с друзьями',
        'Создавай ссылки на свои вишлисты и отправляй их другим пользователям.',
      ],
    ],
  },
  orders: {
    label: 'Мои заказы',
    title: 'Все твои заказы — в одном месте',
    description:
      'Следи за своими покупками, просматривай историю заказов и возвращайся к понравившимся вещам.',
    register: 'Создать аккаунт',
    login: 'Войти',
    benefits: [
      ['bag', 'История покупок', 'Просматривай оформленные заказы и товары в каждом из них.'],
      ['check', 'Статусы заказов', 'Проверяй статус заказа и срок действия резерва.'],
      [
        'bookmark',
        'Информация о покупке',
        'Смотри состав заказа, стоимость и выбранные размеры и цвета.',
      ],
    ],
  },
  studio: {
    label: 'Студия бренда',
    title: 'Твой бренд. Твоя студия.',
    description:
      'Создавай и развивай свой бренд на SREZ. Управляй каталогом, оформляй карточки товаров и представляй свои коллекции покупателям.',
    register: 'Начать работу со SREZ',
    login: 'Войти в аккаунт',
    benefits: [
      ['bag', 'Управляй каталогом', 'Создавай и редактируй карточки товаров своего бренда.'],
      [
        'image',
        'Настраивай товары',
        'Управляй вариантами товаров: размерами, цветами, фотографиями и остатками.',
      ],
      [
        'diagonal',
        'Развивай свой бренд',
        'Представляй коллекции и знакомь покупателей со своей продукцией.',
      ],
    ],
  },
};

export function GuestSection({ section }: { section: keyof typeof sections }) {
  const { pathname, search, hash } = useLocation();
  const state = { from: validReturnPath(pathname + search + hash) };
  const content = sections[section];
  return (
    <div className={`guest-section guest-section--${section}`}>
      <section className="guest-section__hero" aria-labelledby="guest-section-title">
        <div className="container guest-section__inner">
          <div className="guest-section__copy">
            <p className="guest-section__label">{content.label}</p>
            <h1 id="guest-section-title">{content.title}</h1>
            <p className="guest-section__description">{content.description}</p>
            <div className="guest-section__actions">
              <Link className="button button--solid" to="/register" state={state}>
                {content.register} <Icon name="diagonal" />
              </Link>
              <Link className="text-link" to="/login" state={state}>
                {content.login}
              </Link>
            </div>
          </div>
          {section === 'studio' && (
            <picture className="guest-section__image">
              <source media="(max-width: 720px)" srcSet={studioCoverMobile} />
              <img src={studioCover} alt="" />
            </picture>
          )}
        </div>
      </section>
      <ul className="container guest-section__benefits">
        {content.benefits.map(([icon, title, description]) => (
          <li key={title}>
            <Icon name={icon} size={28} />
            <h2>{title}</h2>
            <p>{description}</p>
          </li>
        ))}
      </ul>
    </div>
  );
}
