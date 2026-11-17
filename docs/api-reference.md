# Карта HTTP API SREZ

Актуальный реестр маршрутов локального Go backend. Подробные DTO, примеры и инварианты сгруппированы по доменам в документах ниже; исторический `docs/legacy/SPEC.md` не является текущим контрактом.

## Общие правила

- Префикса `/api` в Go нет: его добавляет браузерный transport, Vite/nginx удаляет при proxy. `/media` проксируется без rewrite.
- `B` — Bearer JWT + активная серверная сессия; `O` — active owner, `E` — active owner/editor **конкретного** бренда, дополнительно к B. Роли проверяются в PostgreSQL, не принимаются от клиента.
- `C` — HttpOnly refresh-cookie, точный Origin, CSRF для POST. Auth POST без C всё равно требуют Origin и JSON/source middleware.
- Ошибка JSON: `{"error":"message"}`. Общие ошибки: 400 malformed/лишние поля/ID, 401 отсутствующие/отозванные credentials, 403 Origin/CSRF/роль, 404 неизвестный/чужой объект, 409 conflict, 413 size, 415 Content-Type, 429 limiter, 500/503 инфраструктура. В таблицах указаны дополнительные доменные ограничения; SQL/секреты не выдаются.
- ID положительные int64; размеры/цвета и stock берутся из конкретного варианта. Деньги — целые **копейки**; даты — RFC3339; nullable значения — JSON null, списки — массивы, не null.
- `—` в запросе означает отсутствие тела. `{}` — обязательное пустое JSON-тело; это разные контракты. PATCH принимает только явно разрешённые поля.
- PublicUser: id/username/display_name/avatar_url, без email/hash. Profile добавляет email/email_verified_at/email_verification_required. Доменная документация содержит полные вложенные DTO; пути к ней приведены в каждом разделе.

## Auth и Profile

[Email verification](email-verification.md), [password recovery](account-recovery.md), [security architecture](architecture.md).

| Метод и URL                    | Доступ                  | Запрос                               | Успех / ограничения                                                                                                            |
| ------------------------------ | ----------------------- | ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------ |
| POST /auth/register            | Origin                  | email,username,display_name,password | 201 PublicUser + email_verification_required, email_delivery_status, resend_after_seconds; 409 account unavailable; без сессии |
| POST /auth/login               | Origin                  | identifier,password                  | 200 access_token,csrf_token + refresh-cookie; 401 неверный пароль; 403 pending только после правильного пароля                 |
| POST /auth/verify-email        | Origin                  | token                                | 200 message; 400 invalid/expired/reused/revoked; без сессии                                                                    |
| POST /auth/resend-verification | Origin                  | identifier                           | 202 generic message,retry; неизвестный/verified/cooldown не различаются; 429 IP                                                |
| POST /auth/forgot-password     | Origin                  | email                                | 202 generic message,retry; только verified адрес может получить письмо; 429 IP                                                 |
| POST /auth/reset-password      | Origin                  | token,password                       | 200 message; 400 токен/пароль; отзыв всех сессий и access JWT, без auto-login                                                  |
| GET /auth/csrf                 | C, GET может без Origin | —                                    | 200 csrf_token; не ротирует сессию, 401 нет действующей cookie                                                                 |
| POST /auth/refresh             | C                       | {}                                   | 200 access_token,csrf_token + rotated cookie; 409 одновременная ротация, поздний replay отзывает сессию                        |
| POST /auth/logout              | C                       | {}                                   | 204 + очистка cookie; отзыв сессии и access при следующей проверке                                                             |
| GET /profile                   | B                       | —                                    | 200 Profile                                                                                                                    |
| PATCH /profile                 | B                       | display_name                         | 200 Profile; avatar_url и поля идентичности запрещены                                                                          |

JWT только в JS памяти, refresh HttpOnly. No-store; sensitive страницы no-referrer/CSP. Reset и verification — отдельные 256-bit токены, SHA-256, 30 минут, одноразовые; не JWT.

## Публичный каталог

[Параметры, ProductCard и пагинация](catalog-api.md). GET не требует входа.

| Метод и URL       | Запрос                                                                   | Успех / ограничения                                                                |
| ----------------- | ------------------------------------------------------------------------ | ---------------------------------------------------------------------------------- |
| GET /health       | —                                                                        | 200 {status:"ok"}; состояние процесса, не глубокая проверка БД                     |
| GET /brands       | —                                                                        | 200 Brand[] только active                                                          |
| GET /brands/:slug | —                                                                        | 200 Brand с contacts/appearance/изображениями; 404 inactive/unknown                |
| GET /products     | category,brand_id,size,color,min_price,max_price,sort,page,limit в query | 200 ProductCard[]; X-Total-Count/X-Page/X-Limit/X-Total-Pages; 400 неверный фильтр |
| GET /products/:id | —                                                                        | 200 Product с colors/images/variants; 404 hidden/draft/inactive brand              |
| GET /projects     | —                                                                        | 200 фиксированный учебный список; legacy совместимость, frontend не использует     |

Фильтры AND, size+color относятся к одной комбинации в наличии. Limit 1–100, default20; sort id_asc/price_asc/price_desc/newest/oldest. Карточка включает preview_image_url, brand_name/slug, is_available, available_colors/sizes и цвета с собственными размерами. Общий размер не заменяет проверку конкретного цвета. Нулевой stock отображается, купить нельзя.

## Бренды и memberships

[Архитектура/RBAC и DTO](architecture.md). Brand: id,name,slug,description,logo_url,banner_url,status,contacts,appearance, timestamps. Membership содержит публичного участника/роль/статус без email/hash.

| Метод и URL                          | Доступ    | JSON                                      | Успех / ограничения                                                              |
| ------------------------------------ | --------- | ----------------------------------------- | -------------------------------------------------------------------------------- |
| POST /brands                         | B         | name,slug                                 | 201 draft Brand + owner в SQL                                                    |
| GET /me/brands                       | B         | —                                         | 200 [{brand,role}], только свои active                                           |
| GET /me/brand-invitations            | B         | —                                         | 200 свои pending invitations                                                     |
| GET /brands/:id/manage               | E         | —                                         | 200 Brand, включая draft                                                         |
| PATCH /brands/:id                    | O         | name,slug,description,contacts,appearance | 200 Brand; logo_url/banner_url запрещены, contacts/appearance заменяются целиком |
| POST /brands/:id/publish             | O         | {}                                        | 200 Brand; 409 неполные данные                                                   |
| POST /brands/:id/unpublish           | O         | {}                                        | 200 Brand; опубликованные товары теряют публичную доступность                    |
| GET /brands/:id/members              | O         | —                                         | 200 membership DTO[]                                                             |
| POST /brands/:id/invitations         | O         | username                                  | 201 приглашение существующего пользователя                                       |
| POST /brands/:id/invitations/accept  | адресат B | {}                                        | 200 membership; нельзя принять чужое приглашение                                 |
| POST /brands/:id/invitations/decline | адресат B | {}                                        | 200 membership                                                                   |
| DELETE /brands/:id/members/:user_id  | O         | —                                         | 204; owner отозвать нельзя, editor теряет права на новые операции                |
| GET /brands/:id/products/manage      | E         | —                                         | 200 Product[], включая draft/hidden                                              |

Внутренний wildcard GET назван :slug из-за Gin routing tree, но private endpoints выше принимают числовой ID. Чужой private ресурс даёт404, owner-only операция для editor —403, конфликт slug/перехода —409. Передача owner не реализована.

## Товары и PVM

[Полные DTO, 13 PVM маршрутов, ошибки и примеры](product-variant-management.md). Все операции требуют E. ProductInput: name,description,price_kopecks,category,materials; при создании также brand_id. Публикация требует active бренда и полного агрегата. Структурные изменения могут потребовать явного hide → edit → publish.

В таблице `P=/products/:id`, `C=P/colors/:color_id`, `V=C/variants/:variant_id`, `I=C/images/:image_id`; подставляются реальные IDs, не буквенные сокращения.

| Метод и URL           | JSON                                        | Успех / ограничения                                                           |
| --------------------- | ------------------------------------------- | ----------------------------------------------------------------------------- |
| POST /products        | brand_id + ProductInput                     | 201 draft Product                                                             |
| PATCH P               | частичный ProductInput, без brand_id/status | 200 Product                                                                   |
| POST P/publish        | {}                                          | 200 Product; 409 неполный агрегат                                             |
| POST P/hide           | {}                                          | 200 Product                                                                   |
| POST P/unpublish      | {}                                          | 200 Product                                                                   |
| POST P/colors         | color                                       | 201 ManagedColor                                                              |
| GET P/colors          | —                                           | 200 ManagedColor[]                                                            |
| PATCH C               | color                                       | 200 ManagedColor                                                              |
| DELETE C              | —                                           | 204; атомарно дочерние данные, не нарушает publication/inventory              |
| POST C/variants       | size,stock,sku                              | 201 ManagedVariant                                                            |
| GET C/variants        | —                                           | 200 ManagedVariant[]                                                          |
| PATCH V               | size/stock/sku                              | 200 ManagedVariant                                                            |
| DELETE V              | —                                           | 204; ограничения резервов/целостности                                         |
| POST C/images         | media_id, position                          | 200 ManagedImage; подтверждение ранее загруженного managed asset              |
| GET C/images          | —                                           | 200 ManagedImage[]                                                            |
| PATCH I               | media_id/position                           | 200 ManagedImage; внешние url отклоняются, привязка managed media проверяется |
| DELETE I              | —                                           | 204; ограничения публикации                                                   |
| POST C/images/reorder | image_ids полного списка                    | 200 ManagedImage[]; атомарная перестановка                                    |

ManagedColor: id,product_id,color,images. ManagedVariant: id,product_color_id,size,stock,sku. ManagedImage: id,product_color_id,url,position, опционально media_id. Stock неотрицателен; размер до20 символов; до10 изображений на цвет. Чужие вложенные ID404; некорректные значения400; дубликаты/нарушение публикации409. Media upload предпочтителен для новых изображений; произвольные приватные S3 URLs не поддерживаются.

## Media

[Полный контракт, проверки содержимого и GC/reconciliation](media-storage.md).

| Метод и URL                       | Доступ                                      | Запрос                                           | Успех / ограничения                                                                        |
| --------------------------------- | ------------------------------------------- | ------------------------------------------------ | ------------------------------------------------------------------------------------------ |
| POST /media/uploads               | B + права на target                         | purpose,content_type,size_bytes + target IDs     | 201 {upload,content_url}; target/purpose whitelist                                         |
| PUT /media/uploads/:id/content    | B + права на upload/target                  | raw JPEG/PNG/WebP, Content-Type и Content-Length | 200 upload; финальная нормализация/привязка, 411 без длины, 413 size, 429 обработка занята |
| GET /media/assets/:id/content     | public либо B для доступного private target | —                                                | 200 binary с безопасными MIME/headers; 404 без доступа                                     |
| DELETE /profile/avatar            | B                                           | —                                                | 204                                                                                        |
| DELETE /brands/:id/media/:purpose | O                                           | —                                                | 204; purpose только brand_logo/brand_banner                                                |
| POST /media/uploads/:id/complete  | B                                           | retired                                          | 410; direct S3 flow запрещён                                                               |

Purpose: user_avatar/brand_logo/brand_banner/product_image. Target IDs: brand_id либо product_id/color_id/image_id по назначению. Backend ограничивает размер/декодирование/пиксели, проверяет MIME по содержимому, повторно проверяет RBAC и не выдаёт приватные S3 credentials. Браузер отправляет File через Go, не напрямую в bucket. Legacy HTTPS и managed `/media/assets/:id/content` поддерживаются фронтом; ошибка изображения даёт placeholder.

## Покупатель

[DTO избранного, корзины и заказов, ошибки и конкуренция](buyer-shopping.md). Все маршруты B, владелец только из JWT. Клиентские цены/суммы/owner_id не принимаются.

| Метод и URL                   | JSON                        | Успех / ограничения                                       |
| ----------------------------- | --------------------------- | --------------------------------------------------------- |
| GET /favorites                | —                           | 200 [{product_id,product}], product nullable для скрытого |
| POST /favorites/:product_id   | без тела                    | 204 идемпотентно                                          |
| DELETE /favorites/:product_id | без тела                    | 204 идемпотентно                                          |
| GET /cart                     | —                           | 200 {items,total_kopecks}, серверные availability/цены    |
| POST /cart/items              | product_variant_id,quantity | 200 полная корзина; цвет/размер именно этого варианта     |
| PATCH /cart/items/:id         | quantity                    | 200 полная корзина; неизвестная/чужая404                  |
| DELETE /cart/items/:id        | —                           | 204                                                       |
| POST /orders                  | {}                          | 201 Order, атомарный **неоплаченный резерв на15 минут**   |
| GET /orders                   | —                           | 200 собственная история Order[]                           |
| GET /orders/:id               | —                           | 200 Order со snapshot items/cancel_before; чужой404       |
| POST /orders/:id/cancel       | {}                          | 200 Order; неоплаченный в срок, возврат stock атомарен    |

Корзина ничего не резервирует. При checkout backend повторно проверяет price/stock/publication и рассчитывает сумму. Ошибки availability/stock/transition409, bad quantity400. На истечении сервер переводит заказ и освобождает остатки; frontend только обновляет ответ. Оплаты, доставки и обработки оплаченного заказа нет. При неопределённом POST сначала проверяется история, автоматического повторного checkout нет.

## Вишлисты

[Полные DTO/JSON, лимиты, capability links и криптография](wishlists.md). Личные маршруты B, public GET использует только capability; cookie сама права не даёт.

| Метод и URL                          | JSON                                   | Успех / ограничения                                                |
| ------------------------------------ | -------------------------------------- | ------------------------------------------------------------------ |
| GET /wishlists                       | —                                      | 200 WishlistSummary[]                                              |
| POST /wishlists                      | title,description                      | 201 Wishlist                                                       |
| GET /wishlists/:id                   | —                                      | 200 Wishlist/items только владельцу                                |
| PATCH /wishlists/:id                 | title/description                      | 200 Wishlist                                                       |
| DELETE /wishlists/:id                | —                                      | 204                                                                |
| POST /wishlists/:id/items            | product_id,product_variant_id nullable | 201 полный Wishlist                                                |
| PATCH /wishlists/:id/items/:item_id  | product_variant_id nullable            | 200 полный Wishlist                                                |
| DELETE /wishlists/:id/items/:item_id | —                                      | 200 оставшийся Wishlist                                            |
| POST /wishlists/:id/items/reorder    | item_ids полного списка                | 200 список,409 stale set                                           |
| POST /wishlists/:id/share            | {}                                     | 200 новая owner share_path; предыдущий capability отозван          |
| DELETE /wishlists/:id/share          | —                                      | 200 private Wishlist, ссылка отозвана                              |
| GET /wishlists/shared/:token         | —                                      | 200 public DTO без владельца/private fields; недоступная ссылка404 |

До20 списков по200 элементов, один product на список. Foreign/hidden объекты не раскрываются, public выдача заново фильтрует hidden products/inactive brands. Hash capability хранится отдельно от encrypted owner recovery; ключ wishlist независим от Auth. URL capability нельзя журналировать; уже прочитанное гостем отозвать из его памяти невозможно.
