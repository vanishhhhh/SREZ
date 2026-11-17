export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
  ) {
    super(friendlyError(status, code));
    this.name = 'ApiError';
  }
}

function friendlyError(status: number, code: string): string {
  const messages: Record<string, string> = {
    'email verification required':
      'Подтвердите email перед входом. Откройте письмо или запросите новую ссылку.',
    'invalid or expired verification link':
      'Ссылка недействительна, уже использована или срок её действия истёк. Запросите новую.',
    'email verification rate limited':
      'Достигнут лимит писем. Подождите и повторите попытку позже.',
    'email verification unavailable':
      'Подтверждение почты временно недоступно. Проверьте запуск сервера.',
    'insufficient stock': 'Недостаточно свободных единиц. Обновите размер и количество.',
    'cart contains unavailable items':
      'В корзине есть недоступные позиции. Обновите или удалите их.',
    'cart is empty': 'Корзина пуста. Выберите вещи в каталоге.',
    'invalid shopping data': 'Проверьте вариант и целое количество от 1 до 100.',
    'cancellation window closed; contact support':
      'Срок отмены истёк. Обновите статус резерва; при необходимости обратитесь в поддержку.',
    'order cannot be cancelled; contact support':
      'Этот заказ больше нельзя отменить. Обновите его статус.',
    'inventory update conflict; contact support':
      'Возник конфликт остатка. Повторите чтение заказа или обратитесь в поддержку.',
    'money exceeds supported limit': 'Сумма превышает допустимый предел. Измените состав корзины.',
    'image size limit exceeded': 'Фотография превышает допустимый размер.',
    'media processing busy':
      'Сервер обрабатывает другую фотографию. Повторите загрузку через несколько секунд.',
    'invalid credentials': 'Неверный логин или пароль.',
    'account unavailable': 'Этот email или имя пользователя уже заняты.',
    'invalid account data': 'Проверьте email, имя пользователя и остальные поля.',
    'request origin denied':
      'Адрес сайта не совпадает с настройкой API. Для локального запуска используйте compose.frontend.yaml.',
    'request source denied': 'Откройте приложение по настроенному локальному адресу SREZ.',
    'invalid or expired reset link':
      'Ссылка недействительна или истекла. Запросите новую ссылку восстановления.',
    'password recovery rate limited': 'Слишком много запросов восстановления. Повторите позже.',
    'password recovery unavailable': 'Восстановление доступа временно недоступно.',
    'authentication unavailable': 'Проверка сессии временно недоступна. Повторите позже.',
    'invalid csrf': 'Не удалось подтвердить сессию. Повторите восстановление или войдите заново.',
    'refresh already rotated':
      'Сессию обновляет другая вкладка. Повторите восстановление или войдите заново.',
    'session changed': 'Сессия изменилась. Войдите снова.',
    'session expired': 'Сессия завершилась. Войдите снова.',
    'request timeout': 'Сервер не ответил вовремя. Попробуйте ещё раз.',
    'invalid response': 'Сервер вернул неожиданный ответ. Попробуйте ещё раз.',
    'resource is not ready for publication':
      'Не хватает данных для публикации. Нужны описание, цена, материалы, фото и размер у каждого цвета; для бренда — описание и логотип.',
    'state conflict':
      'Конфликт данных. Проверьте дубликаты, текущий статус и ограничение в 10 фотографий.',
    'invalid resource data':
      'Проверьте поля: допустимые значения, HTTPS-ссылки, размеры и остатки.',
    'resource not found': 'Объект не найден или у вас больше нет доступа к нему.',
    'invalid wishlist data': 'Проверьте название, описание, вариант товара и порядок элементов.',
    'wishlist limit reached': 'Можно создать до 20 вишлистов и добавить до 200 вещей в каждый.',
    'product already in wishlist':
      'Эта вещь уже есть в выбранном вишлисте. Вариант можно изменить в самом списке.',
    'wishlist changed; reload before reordering':
      'Список изменился. Мы обновили его — повторите изменение порядка.',
    'wishlists unavailable': 'Вишлисты временно недоступны. Проверьте запуск актуального Go API.',
    'operation forbidden': 'Для этого действия недостаточно прав или бренд заблокирован.',
  };
  if (Object.hasOwn(messages, code)) return messages[code];
  if (status === 0)
    return 'Не удалось связаться с сервером. Проверьте соединение и попробуйте ещё раз.';
  if (status === 401) return 'Сессия завершилась. Войдите снова.';
  if (status === 403) return 'Запрос запрещён. Проверьте права и локальный Origin.';
  if (status === 404) return 'Страница или объект не найдены.';
  if (status === 409) return 'Данные изменились. Обновите страницу и повторите действие.';
  if (status === 413) return 'Слишком большой запрос. Сократите текст или ссылки.';
  if (status === 429) return 'Слишком много попыток. Подождите минуту.';
  if (status >= 500) return 'Сервер временно недоступен. Повторите попытку позже.';
  return 'Не удалось выполнить запрос. Проверьте введённые данные.';
}

export type RequestOptions = {
  method?: string;
  body?: unknown;
  auth?: boolean;
  signal?: AbortSignal;
  headers?: Record<string, string>;
  responseType?: 'json' | 'blob';
};

// Only application-relative API paths enter this transport. Tokens are never
// placed in URLs, persistent storage, logs or the observable React state.
export async function jsonRequest<T>(
  path: string,
  options: RequestOptions = {},
): Promise<{ data: T; headers: Headers }> {
  if (
    !path.startsWith('/') ||
    path.startsWith('//') ||
    Array.from(path).some((char) => char === '\\' || char.charCodeAt(0) < 32)
  ) {
    throw new Error('API path must be application-relative');
  }
  const controller = new AbortController();
  const abort = () => controller.abort();
  let timedOut = false;
  options.signal?.addEventListener('abort', abort, { once: true });
  if (options.signal?.aborted) controller.abort();
  const timeout = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, 20000);
  try {
    const response = await fetch('/api' + path, {
      method: options.method || 'GET',
      credentials: 'include',
      cache: 'no-store',
      signal: controller.signal,
      headers: {
        Accept: 'application/json',
        ...(options.body !== undefined
          ? {
              'Content-Type': options.body instanceof Blob ? options.body.type : 'application/json',
            }
          : {}),
        ...options.headers,
      },
      body:
        options.body === undefined
          ? undefined
          : options.body instanceof Blob
            ? options.body
            : JSON.stringify(options.body),
    });
    let data: unknown = null;
    if (response.ok && options.responseType === 'blob') {
      data = await response.blob();
    } else if (response.status !== 204) {
      try {
        data = await response.json();
      } catch {
        if (response.ok) throw new ApiError(502, 'invalid response');
      }
    }
    if (!response.ok) {
      const code =
        data && typeof data === 'object' && 'error' in data && typeof data.error === 'string'
          ? data.error
          : 'request failed';
      throw new ApiError(response.status, code);
    }
    return { data: data as T, headers: response.headers };
  } catch (error) {
    if (options.signal?.aborted)
      throw options.signal.reason || new DOMException('Aborted', 'AbortError');
    if (error instanceof ApiError) throw error;
    throw new ApiError(0, timedOut ? 'request timeout' : 'network unavailable');
  } finally {
    clearTimeout(timeout);
    options.signal?.removeEventListener('abort', abort);
  }
}
