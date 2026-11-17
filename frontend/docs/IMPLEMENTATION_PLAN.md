# SREZ: план реализации и критерии готовности

Актуально на **17.11.2026**. Рабочая копия `integration-repository`, локальная ветка main, база `5133b878d6610b041aaf2aa687988221ca30648b`. Изменения не закоммичены и не отправлены. [Отчёт MVP](MVP_REPORT.md), [новый этап email verification](EMAIL_VERIFICATION_REPORT.md), [Windows запуск](../README.md).

Первоначальный аудит сохранён в ARCHIVED_AUDIT_2026-11-16.md, предыдущая синхронизация — INTEGRATION_REPORT.md. Они описывают прежние состояния. Главный источник контрактов — текущий Go-код.

## Состояние

| Область             | Реализовано                                                                                     | Проверка готовности                                                                               |
| ------------------- | ----------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| Инфраструктура/Auth | React/TS/Vite, routing, layout, общий transport/формы/ошибки; Docker frontend, безопасный proxy | Сборка, typecheck/lint, реальный login/reload/logout, ошибки и гонки refresh                      |
| Публичная часть     | Главная, бренды, фильтры/sort/pagination, карточка/галерея/цвет/размер                          | Реальные API, заголовки пагинации, копейки, mobile/16:10, empty/error/loading                     |
| Покупатель          | Избранное, cart variant/quantity/server totals, unpaid reserve/history/cancel/expiry            | Покупка последней единицы, цена/остаток, отмена и возврат stock                                   |
| Media               | POST upload + raw PUT, managed media_id, private Blob/public URL/legacy HTTPS                   | Avatar/logo/banner/PVM, MIME/size, стабильный image ID, private/foreign deny, restart persistence |
| Этап 5: вишлисты    | Go/PostgreSQL/React CRUD, variant, reorder, share/revoke/rotate, guest подборка                 | Owner/guest UI, IDOR/mass assignment, hidden products, конкуренция/лимиты, криптография           |
| Этап 6: продавец    | Общая студия, owner/editor, команды/приглашения, товары/PVM, upload preview, защита форм        | Полный owner → editor → revoke UI; публикация, variant/image операции и права                     |
| Этап 7: QA/Polish   | Изолированные окружения, CSP/fonts/log privacy, исправления гонок форм/времени, документация    | 900 Go cases + vet, 88 frontend unit, 31 browser, отдельный restart test                          |

## Этапы и зависимости

### 1. Инфраструктура и авторизация — завершён

- [x] Общий layout/404/unavailable, типизированный клиент и DTO guards.
- [x] Proxy только /api с rewrite и /media без rewrite; exact Origin, CSRF/JWT сохранены.
- [x] Register/email-or-username login/profile/logout/reload/private routes.
- [x] Access в памяти, HttpOnly refresh; single-flight/Web Locks/409 и смена пользователя.
- [x] Стабильный Docker frontend localhost:5174; Vite остаётся инструментом разработки.

Критерий: реальные auth-сценарии и отсутствие токенов в persistent storage подтверждены. Независимый production AppSec-review остаётся отдельной задачей.

### 2. Публичные страницы — завершён

- [x] Home, catalog, brands; реальный ассортимент.
- [x] Filter/sort/pagination, HTTP headers; цены в копейках.
- [x] Product gallery и варианты по цвету; недоступная комбинация не выбирается.
- [x] Managed и HTTPS media, placeholders, loading/empty/retry, адаптивность.

Зависит от transport и публичного Go API. Критерий: реальные товары и переходы desktop/mobile без JS-ошибок; редкие сетевые состояния проверены управляемыми ответами.

### 3–4. Покупатель — завершён

- [x] Favorites отдельно от wishlists; shared state, counters и guest-login.
- [x] Cart реального variant_id, quantity1–100, remove, availability, server totals.
- [x] Повторная проверка до checkout, price conflict, защита повторного POST.
- [x] POST orders `{}`, явный неоплаченный 15-минутный резерв, cancel_before.
- [x] History/detail/cancel/expiry; возврат stock подтверждён Go API.

Зависит от auth, каталога/PVM и существующего buyer backend. Backend избранного/cart/orders не дублировался. Критерий: полный сценарий и конфликт покупки последней единицы проходят.

### 5. Shareable Wishlists — завершён

- [x] Entities/usecase/storage/handler и миграции 012–013; ограничения20/200.
- [x] CRUD, один товар на список, nullable вариант, атомарный полный reorder.
- [x] 256-bit capability, hash lookup, encrypted owner recovery с отдельным стабильным ключом.
- [x] Share/rotate/revoke, строгий public DTO, фильтрация скрытых товаров/неактивных брендов.
- [x] Страницы /wishlists и /w/:token, picker на товарах, копирование/подтверждения.
- [x] Тесты чужих ID/подмены владельца/конкуренции/лимитов/FK/отзыва.

Зависит от auth, ProductCard/Media и действующего каталога. Критерий: отдельная гостевая сессия читает только публичный список; старые ссылки после rotate/revoke дают404; UI обновляет публичность при следующем fetch/focus/30-секундном опросе.

### 6. Seller Dashboard — завершён в пределах текущего API

- [x] Сохранены /studio routes и существующие формы; общий адаптивный sidebar/brand selector.
- [x] Owner: бренд, оформление, logo/banner, publish/hide, приглашение/отзыв editor.
- [x] Editor: доступный ассортимент/PVM; owner-only запреты проверяет Go.
- [x] Товар: price/category/materials, colors, sizes/stock/SKU, photos/replace/cover/reorder/delete.
- [x] Явный hide → изменение структуры → publish без автоматической смены статуса.
- [x] Preview перед upload, неизвестный результат PUT, нормальные ошибки и подтверждения удаления.
- [x] Общая защита несохранённых форм/выбранного файла; поздний GET не затирает draft.

Зависит от auth/RBAC, Media и PVM. Критерий: полный UI owner/editor, отзыв роли и повторный запрет, foreign media/brand/product тесты проходят. Transfer ownership и обработка seller orders не добавлялись вне backend-контрактов.

### 7. Integration, QA & Polish — завершён для локального MVP

- [x] Проверенный файловый backup + PostgreSQL dump, restore в отдельную QA БД.
- [x] Три раздельных новых окружения: working, browser QA, Go tests; старое сохранено.
- [x] Новые миграции сначала QA, затем working; ранее применённый SQL не редактировался.
- [x] Go build/full900/vet, frontend npm ci/typecheck/lint/unit88/build/format.
- [x] Browser31, desktop/mobile/16:10, guest/owner/editor/security/ошибки.
- [x] Сохранение media после restart проверено отдельно (write + verify).
- [x] Русские docs, локальные screenshots, команды запуска/остановки/диагностики.

Критерий: рабочий frontend5174 и API8081 доступны, исходные строки/файлы сохранены; QA не использует пользовательские данные для разрушительных сценариев. См. доказательства и точные оговорки в MVP_REPORT.

## Оставшиеся работы вне локального MVP

1. Оплата/доставка/обработка заказов брендами: сейчас только неоплаченный резерв.
2. Production HTTPS/секреты/monitoring и независимый AppSec-review. Восстановление пароля реализовано отдельным этапом ниже.
3. Idempotency key checkout: при неопределённом POST клиент сначала предлагает историю, не повторяет автоматически.
4. Серверный endpoint фасетов каталога; сейчас size/color поля соответствуют имеющемуся API.
5. Гостевая корзина/слияние при login, передача владения брендом — отдельные продуктовые решения.
6. Публичные ссылки локальные; реальный домен требует конфигурации и отдельного разрешения на публикацию.

Никакой автоматической синхронизации/commit/push/PR. Работать в integration-repository, старые файлы и volumes сохранять.

## Подтверждение email с Mailpit — завершённый этап

- [x] Миграция014, новые required=true и legacy=false без выдуманных дат.
- [x] Одноразовый32-byte token, SHA-256,30минут, atomic verify/revoke/rotate.
- [x] MailSender, SMTP text/HTML, timeout и сохранение аккаунта при отказе.
- [x] Persistent IP/account quotas, одинаковый resend response.
- [x] Страницы pending/verify, memory-only token, очистка URL, явная кнопка.
- [x] Login/refresh/session eligibility, реальные статусы профиля.
- [x] Изолированные PostgreSQL/HTTP/Mailpit/browser tests, документация.
- [x] Реальный smoke в рабочей копии, legacy login и сохранность всех прежних строк БД; Mailpit доступен на 8025.

Проверено: 919 Go cases, vet, 93 frontend unit, 34 browser; build/typecheck/lint/format успешны. Один opt-in Seaweed restart test не запускался в этом этапе. Дальше остаются внешний SMTP и production эксплуатация, перечисленные выше; локальный email flow готов.

Контракты и ограничения: [email-verification.md](../../docs/email-verification.md). Фактические итоговые результаты: [EMAIL_VERIFICATION_REPORT.md](EMAIL_VERIFICATION_REPORT.md). Предыдущие900/88/31 выше описывают завершение MVP до этого этапа.

## Account Recovery, review и подготовка исходников

- [x] Forgot/reset, отдельные hash tokens и миграция015, verified email eligibility.
- [x] Общий MailSender/Mailpit, persistent account/IP quotas и нейтральный ответ.
- [x] Атомарный password reset/revoke и немедленная проверка access session.
- [x] Stale login/refresh races, rollback, purpose separation и реальный browser flow.
- [x] Разделены PVM размеры/фото, переиспользован AuthFrame; дизайн сохранён.
- [x] Единые README/API/recovery/review и CI с реальной test инфраструктурой.

Точные результаты и публикация исходников: [release report](../../docs/release-report.md). Deployment и автоматического merge нет.
