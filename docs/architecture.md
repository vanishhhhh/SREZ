# Архитектура и хранение данных

SREZ использует Go, Gin, PostgreSQL и sqlx. Входные HTTP-данные и JSON-ответы представлены DTO, а не сериализованными доменными сущностями.

## Слои

| Путь | Ответственность |
| --- | --- |
| `cmd/app/main.go` | конфигурация, соединение, миграции, сборка зависимостей, HTTP |
| `internal/config` | HTTP_ADDR, DATABASE_URL и явный флаг локальной записи |
| `internal/handler` | маршруты, разбор запроса, DTO, HTTP-статусы |
| `internal/usecase` | сценарии, интерфейсы репозиториев, межсущностные проверки |
| `internal/entities` | инварианты, нормализация, создание/восстановление, переходы статусов |
| `internal/storage` | параметризованный SQL, преобразование строк БД, транзакции |
| `migrations` | встроенные версионируемые SQL и runner |
| `cmd/seed`, `internal/devseed` | отдельная явная загрузка демонстрационных данных |

Пользователи хранятся через Users usecase/repository. Каталог использует BrandRepository и ProductRepository, объявленные на уровне usecase. Пакеты `project.go` и GET /projects сохранены как часть предыдущего учебного задания.

## Чтение каталога и создание draft

Публичные запросы брендов фильтруют status=active. Запросы товаров выбирают только published у active-брендов. Подробная карточка загружается четырьмя запросами в read-only REPEATABLE READ: товар с брендом, цвета, изображения, варианты. Снимок согласован; изображения отсортированы по position, варианты связаны с цветом через ID. После восстановления проверяется полный ProductPublication. Краткий список не загружает дочерние сущности.

Создание draft через HTTP использует Management usecase: валидный Product из NewProduct сохраняется только после проверки актуального активного membership под блокировкой бренда. Draft разрешён для draft/active брендов, suspended запрещён. Старый dev-флаг удалён. Подробности — в разделе BrandMembership & RBAC Architecture.

## DTO каталога

Краткая карточка товара содержит id, brand_id, name, description, price_kopecks, category, materials, status, created_at и updated_at. Materials всегда возвращается массивом, включая пустой. Подробная карточка добавляет colors: у цвета есть id, color, images (массив HTTPS URL) и variants. У варианта — id, size, stock и sku; вариант вложен только в свой цвет. Нулевой stock сохраняется в JSON.

DTO бренда содержит id, name, slug, description, logo_url, banner_url, contacts (email, telegram_url, website_url), appearance (accent_color, background_color, text_color), status, created_at и updated_at. Даты передаются в RFC 3339. Данные учётных записей и membership напрямую не сериализуются.

## Схема

Все идентификаторы — BIGINT identity; цена хранится целым BIGINT в копейках, даты — TIMESTAMPTZ.

| Таблица | Данные и ограничения |
| --- | --- |
| `users` | профиль, Argon2id для регистрации, legacy-хеш ! сохранён; уникальные email/username |
| `brands` | данные бренда; contacts/appearance разложены на текстовые колонки; статусы draft/active/suspended; уникальный непустой slug |
| `products` | FK brand_id, цена >=0, materials TEXT[], категории и статусы draft/published/hidden |
| `product_colors` | FK product_id, color, уникальный `(product_id, color_key)` |
| `product_color_images` | FK цвета, URL, позиция 0–9; уникальные URL и позиция внутри цвета |
| `product_variants` | FK цвета, size, stock BIGINT >=0, sku; уникальный `(product_color_id, size_key)` |

CHECK также защищают порядок updated_at/created_at. Индексы покрывают FK и публичные выборки. Каскадного удаления нет. У нескольких draft-брендов может отсутствовать slug, как разрешает домен. Уникальность SKU не введена: такого доменного инварианта нет.

color_key и size_key вычисляются писателем как `strings.ToLower(strings.TrimSpace(value))`, точно как ключи уникальности в entities. Они отделены от отображаемого значения, не входят в DTO и избегают расхождений с локалью PostgreSQL. Текущий писатель связанных сущностей — seed. Будущие репозитории редактирования должны применять то же правило. При прямой записи SQL оператор отвечает за корректные ключи; такую запись следует ограничивать правами БД.

В SQL не дублируются все проверки Unicode-длины, URL, материалов и публикации: их выполняют entities. Materials сравниваются после trim с учётом регистра. Правила публикации и межсущностные гарантии описаны в [документации домена](../internal/entities/README.md).

## Миграции

Файлы `NNN_name.sql` встроены через go:embed и применяются перед открытием HTTP-порта. Номера должны иметь фиксированную ширину: порядок лексикографический.

Runner использует существующий sqlx и стандартную библиотеку. Транзакционная advisory lock сериализует старты; schema_migrations хранит имя файла, SHA-256 и время применения. Все недостающие SQL и записи журнала фиксируются одной транзакцией; ошибка откатывает новый набор. Повторный старт пропускает применённые версии. Изменённый/удалённый применённый файл и версия, добавленная задним числом, блокируют запуск. Добавляйте новую миграцию, не редактируйте применённые.

001_users.sql использует CREATE TABLE IF NOT EXISTS и поддерживает users-таблицу, созданную старым Docker initdb без журнала. Нынешний Compose не использует initdb-монтирование: миграции запускаются приложением и для существующих volumes. Не удаляйте данные ради обновления схемы.

Runner рассчитан на транзакционный DDL; CREATE INDEX CONCURRENTLY внутри него не поддерживается. Автоматического downgrade нет. Startup timeout миграций — 30 секунд. Production потребует отделить DDL-роль от runtime-роли приложения.

## Dev seed

Команда seed требует SREZ_DEV_SEED=true, применяет миграции и под отдельной advisory lock вставляет демонстрационные бренды вместе с полным каталогом в одной транзакции. Сущности восстанавливаются/проверяются через домен; публикация проверяется Product.Publish.

Существующий slug пропускается полностью. Это делает повторный seed безопасным для чужих записей, но команда не восстанавливает вручную изменённые примеры. HTTP-каталог не зависит от наличия seed-кода или моков.

Команды запуска, HTTP API, тесты и ограничения безопасности — в [README](../README.md).

## Authentication & Security Architecture

### Решения и статус

| Компонент | Решение | Статус |
| --- | --- | --- |
| Password hashing | Argon2id, 19 MiB, 2 прохода, parallelism 1, случайная соль 128 бит | Implemented |
| Access token | HS256 JWT, 15 минут; обязательны sub/iss/aud/iat/exp/jti/sid | Implemented |
| JWT signing | независимый случайный ключ >=256 бит из окружения | Implemented |
| Access storage | только память будущего React; не cookie/localStorage/sessionStorage | Planned: frontend отсутствует |
| Refresh token | непрозрачный случайный токен 256 бит | Implemented |
| Refresh client storage | HttpOnly cookie | Implemented |
| Refresh server storage | SHA-256; история хешей до удаления оператором | Implemented |
| Refresh lifetime | абсолютные 7 дней, ротация не продлевает | Implemented |
| Refresh rotation | транзакция и блокировка строки сессии | Implemented |
| Refresh replay | 5 секунд: отказ 409 без отзыва; позже: отзыв всей сессии | Implemented |
| CSRF | HMAC отдельным ключом от ID сессии, Origin и Fetch Metadata | Implemented |
| Production cookie | __Host-srez_refresh, Secure, HttpOnly, SameSite=Strict, Path=/, без Domain | Implemented |
| Authentication | Bearer middleware, строгая проверка JWT | Implemented |
| User authorization | профиль только по доверенному sub; legacy /users отключены | Implemented |
| Frontend | отдельное React-приложение | Planned |
| Backend / production origin | отдельный API за reverse proxy на едином origin | API Implemented; proxy Planned |
| CORS | не нужен для единого origin; заголовки CORS не добавляются | Implemented |
| Logout | отзыв refresh-сессии, удаление cookie | Implemented |

### Пароли, аккаунты и адаптеры

Argon2id использует память и замедляет подбор украденных хешей. Формат PHC хранит алгоритм, версию, параметры, соль и результат. Проверка сравнивает результат constant-time. Пароль: минимум 12 Unicode-символов, максимум 1024 байта; без trim/нормализации. Проверка хеша ограничивает входные параметры (память до 64 MiB, проходы до 4, потоки до 4). При успешном входе параметры ниже baseline обновляются через compare-and-update: новый m=max(старый m,19456), t=max(старый t,2), p=max(старый p,1). Поддерживаемые компоненты не уменьшаются, верхние границы m<=65536, t<=4, p<=4 сохраняются. Mixed hash 65536/1/1 обновляется до 65536/2/1; новая соль уникальна. Повторная проверка при upgrade и вычисления ограничены тем же семафором. Для отсутствующего пользователя/нерабочего хеша выполняется dummy Argon2id. Это уменьшает, но не устраняет различия во времени запросов к БД.

Новые email: ASCII, <=254 байт, lower(trim); username: lower(trim) и прежний доменный ASCII-формат. PostgreSQL обеспечивает уникальность lower(btrim(email)) и lower(username). Миграция 003 не изменяет старые записи; конфликт блокирует весь новый набор миграций. Оператор должен проверить конфликтующие группы отдельно и подтвердить владение перед любыми исправлениями. Автоматического merge/переименования нет. Записи с `!` не могут войти; регистрация не обновляет существующую запись, а отвечает конфликтом. Восстановление доступа не реализовано.

Security adapters находятся в internal/security; JWT использует golang-jwt/jwt/v5, Argon2id — уже используемый x/crypto. Usecase.Auth координирует проверку пароля, регистрацию, login, CSRF, refresh и logout. Storage.Auth реализует параметризованные запросы и транзакции. Entities.AuthSession задаёт допустимый абсолютный срок/активность. HTTP занимается DTO, cookie, Origin и проверкой Bearer.

Argon2id ограничен семафором (по умолчанию 2 вычисления); лишние запросы получают 503 без неограниченной очереди. Login/register имеют общий локальный лимит 10 попыток/IP/минуту, ограниченный словарь 10000 IP, очистку истёкших записей. Это не распределённая защита: несколько API-инстансов увеличивают общий бюджет, перезапуск сбрасывает счётчики, ботнет обходит лимит по одному IP. Для публичного production нужен общий limiter/ограничение на proxy. Trusted proxies по умолчанию отсутствуют; произвольный X-Forwarded-For не меняет ClientIP.

### Жизненный цикл

1. Register принимает строгий JSON, валидирует Origin и поля, сохраняет новый пользовательский Argon2id-хеш. Сессия автоматически не выдаётся.
2. Login проверяет пароль, создаёт независимую серверную сессию, случайный refresh и JWT, возвращает access_token/csrf_token в JSON и ставит HttpOnly refresh-cookie.
3. Middleware принимает только Bearer JWT, HS256, правильные issuer/audience и обязательные корректные claims. Время жизни не более 15 минут; sub — канонический положительный int64; jti/sid — случайные 256-битные идентификаторы. JWT не содержит email, паролей, refresh или прав на бренд.
4. GET /profile читает только sub проверенного JWT; PATCH /profile обновляет только display_name/avatar_url этого пользователя через существующий Users usecase. user_id/другие поля запрещены. Все /users HTTP-маршруты отключены, включая чтение списка. Старые методы сохранены для внутренней совместимости/тестов, но не зарегистрированы в рабочем роутере. Моковый профиль существует только в тестах.
5. После перезагрузки страницы access JWT утрачен. Будущий frontend запрашивает GET /auth/csrf с cookie, получает HMAC(ID сессии) и вызывает POST /auth/refresh с X-CSRF-Token. GET не требует access JWT, не меняет БД и выдаёт токен только для текущего действующего refresh. Все auth/profile ответы имеют Cache-Control: no-store.
6. Refresh сначала проверяет Origin и CSRF, затем под FOR UPDATE проверяет сессию и историю хеша, помечает прежний хеш использованным и вставляет новый. В транзакции существует только один текущий хеш; новый access выдаётся после commit. Абсолютный expires_at не меняется. При утрате ответа после commit клиент может потерять сессию и должен войти повторно: сервер не хранит исходный refresh для восстановления ответа.
7. Второй запрос со старым refresh в течение 5 секунд получает 409 без cookie/нового JWT и без отзыва. Поздний replay отзывает сессию, включая текущий refresh. Сервер не может надёжно отличить украденный токен от запоздавшего легитимного запроса. Grace-окно также не гарантирует обнаружения атаки внутри этих 5 секунд. Будущий frontend должен координировать refresh между вкладками и не повторять старую cookie бесконечно.
8. Logout проверяет ту же CSRF-привязку и отзывает только эту сессию. Повтор с той же cookie/CSRF даёт 204; после удаления cookie новый logout даёт 401. Отозванный refresh не работает. **Уже выданный access JWT остаётся действительным до 15 минут**: проверки отзыва в middleware нет.
9. Истёкшие сессии не ротируются; login создаёт новую сессию. Автоматической очистки истории пока нет. Для production потребуется retention job и лимит числа сессий пользователя.

```mermaid
sequenceDiagram
    participant C as Browser
    participant A as Go API
    participant D as PostgreSQL
    C->>A: POST login (JSON, Origin)
    A->>D: create session + refresh hash
    A-->>C: access JWT + CSRF; HttpOnly cookie
    C->>A: POST refresh (cookie, Origin, CSRF)
    A->>D: lock session; mark old; insert new hash; commit
    A-->>C: new JWT and rotated cookie
    C->>A: POST logout (cookie, Origin, CSRF)
    A->>D: revoke session
    A-->>C: 204 + clear cookie
```

### Origin, cookie, CORS

Для POST auth endpoints Origin обязателен и должен точно совпадать с SREZ_PUBLIC_ORIGIN, в том числе для CLI. Для read-only GET /auth/csrf отсутствие Origin разрешено: браузеры могут не отправлять его в обычном same-origin fetch. Явно указанный Origin (включая null) обязан точно совпадать с конфигурацией. Исключение зарегистрировано только на этом GET-маршруте и не ослабляет POST. При наличии Sec-Fetch-Site разрешён только same-origin; cross-site, same-site, none и неизвестные значения отклоняются, независимо от Origin. Отсутствующий Fetch Metadata допустим: при отсутствии обоих заголовков GET использует действующую refresh-cookie, а доступ стороннего браузерного JS к ответу ограничен Same-Origin Policy без CORS. Это совместимость со старыми/небраузерными клиентами, а не доказательство происхождения. GET не изменяет состояние сессии, не обновляет cookie и сохраняет Cache-Control: no-store. Bearer GET/PATCH /profile не требует Origin и не использует refresh-cookie. Для POST auth требуется application/json. Refresh/logout дополнительно требуют refresh-cookie и сессионный X-CSRF-Token. Origin не является доказательством личности для небраузерного клиента; доказательства — секретные токены.

Локальный HTTP разрешается только точным SREZ_AUTH_LOCAL_HTTP=true и origin на localhost/loopback; имя cookie тогда srez_refresh_local, Secure=false. Иначе допустим только HTTPS, cookie использует __Host- и Secure. Очистка повторяет имя/Path/Domain/Secure/SameSite. Срок cookie — оставшийся абсолютный срок сессии. Продакшен-значений ключей/fallback в исходниках нет, отсутствующие/некорректные ключи блокируют запуск приложения.

Same-Origin Policy ограничивает чтение ресурсов браузерным JS; CORS регулирует исключения для такого чтения. SameSite ограничивает отправку cookie между сайтами, но сайт не равен origin. CSRF-защита проверяет конкретный запрос на сервере; JWT аутентифицирует Bearer. CORS/SameSite не заменяют CSRF или авторизацию. План: https://srez.ru/ — React, /api/* — Go за reverse proxy; браузер видит один origin, CORS не требуется. В локальной разработке будущий React может использовать dev proxy. Ни frontend, ни proxy сейчас не реализованы.

### Threat Model

| Угроза | Механизм | Ограничения |
| --- | --- | --- |
| XSS | access только в памяти; refresh HttpOnly | XSS может читать access и CSRF, инициировать запросы; frontend escaping/CSP ещё Planned |
| CSRF / login CSRF | точный Origin, JSON, Fetch Metadata; session HMAC для cookie-операций | same-origin XSS обходит; CLI может задать Origin, но должен владеть токенами |
| JWT tampering / algorithm confusion | JWT library, HS256 allowlist, подпись, обязательные claims | компрометация ключа позволяет подделку |
| Token theft | TLS в production, HttpOnly/Secure/Strict cookie, refresh только SHA-256 в БД | JWT доступен до TTL; клиентская машина может быть скомпрометирована |
| Refresh replay | история хешей, rotation, отзыв после 5 секунд | задержанный легитимный replay тоже может отозвать сессию |
| Brute force | Argon2id, локальный IP limiter, bounded hashing | нет распределённого/аккаунтного limiter; production требует усиления |
| User enumeration | одинаковая ошибка login, dummy hash; /users отключён | register конфликт раскрывает занятость; timing не абсолютно одинаков |
| IDOR/BOLA | профиль только по sub; legacy write отключены | права проверяются по БД; управление вариантами — следующий этап |
| Race conditions | row lock, unique current refresh, DB unique аккаунтов | frontend обязан координировать вкладки; возможна потеря ответа |
| Session fixation | новая random session/refresh при login | несколько активных сессий допустимы |
| Небезопасные cookie | production __Host-, Secure, HttpOnly, Strict; явный локальный режим | защита требует корректного HTTPS/proxy |
| Ошибки CORS | заголовки CORS не выдаются, точный Origin | reverse proxy не должен добавлять wildcard/credentials |
| Signing key compromise | независимые ключи из окружения, нет fallback | смена JWT-ключа инвалидирует access; смена CSRF-ключа требует получения нового CSRF; ротация операционная |
| Утечки логов | тела/пароли/токены не логируются, ошибки SQL не выдаются клиенту | оператор не должен логировать Authorization/Cookie на proxy или выводить env |

### Компромиссы и границы

JWT упрощает отдельный API и stateless access-проверку, но не даёт немедленного logout и не подтверждает права на бренд. Серверные сессии без JWT остаются допустимой более простой альтернативой. Refresh непрозрачный, потому что все нужные состояние/отзыв уже в БД; SHA-256 подходит только для случайных высокоэнтропийных токенов, не для паролей. HttpOnly не мешает JS отправлять запросы. LocalStorage/sessionStorage сохраняют украденный JS-токен дольше и не используются; память также не устраняет XSS.

Implemented означает покрытие существующими автоматическими проверками, но не независимую сертификацию безопасности. Planned: React, reverse proxy, CSP, email verification/recovery, распределённый limiter, cleanup/retention, управление сессиями и Product Variant Management. Неподтверждённый email регистрации пока нельзя использовать как доказательство владения для восстановления доступа. Публичный запуск до отдельного AppSec-ревью не рекомендуется.

Источники решений: [OWASP Password Storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html), [JWT library API](https://pkg.go.dev/github.com/golang-jwt/jwt/v5).

## BrandMembership & RBAC Architecture

### Роли и статус реализации

**Implemented:** PostgreSQL memberships, private endpoints, приглашения, отзыв, транзакционная object-level authorization. JWT подтверждает личность, а не текущие полномочия бренда. В JWT роли брендов не добавлены. Все private endpoints используют Bearer middleware; usecase повторно проверяет роль/статус в БД внутри транзакции.

| Операция | Guest | User без membership | Active editor | Active owner |
| --- | --- | --- | --- | --- |
| Публичный каталог | да | да | да | да |
| Создать новый бренд | нет | да | да | да |
| Свои бренды / manage | нет | только свои memberships | свой бренд | свой бренд |
| Настройки/publish/unpublish бренда | нет | нет | нет | да |
| Пригласить/отозвать editor, список участников | нет | нет | нет | да |
| Draft/редактирование/публикация товаров | нет | нет | свой бренд | свой бренд |

Invited/declined/revoked не дают editor-прав. Для suspended бренда операции управления товарами запрещены. Manage бренда позволяет участнику увидеть состояние блокировки; изменение настроек/publish/unpublish suspended запрещено. Публикация товара требует active бренда, draft можно готовить в draft/active бренде.

### Схема и жизненный цикл

004_brand_memberships.sql добавляет PK(user_id,brand_id), FK пользователей/бренда/пригласившего, CHECK role/status/временных полей. PK покрывает поиск по user_id, отдельный индекс — brand_id/user_id. Частичный UNIQUE(brand_id) для active owner запрещает двух владельцев. CHECK не позволяет owner стать revoked/declined. Модель допускает исторические приглашения/бывшего owner-editor как существующий домен, но API смены ролей нет.

POST /brands создаёт Brand.NewBrand со статусом draft и NewOwnerMembership в одной транзакции. При ошибке membership вся вставка откатывается. Owner нельзя назначить клиентским JSON. API не создаёт orphan-бренды; SQL unique обеспечивает максимум одного owner, а exactly-one при создании обеспечивает транзакционный сценарий. Удаления/отзыва owner и ownership transfer нет: они требуют отдельного многострочного сценария и ревью.

Owner приглашает зарегистрированный username: NewEditorInvitation → invited. Только адресат выполняет Accept → active или Decline → declined. Owner делает Revoke для invited/active editor → revoked. Повторное приглашение revoked/declined вызывает Reinvite и требует нового Accept. Повтор active/invited даёт 409; PK предотвращает дубли. Приглашение самого себя/владельца запрещено. Все переходы используют существующие методы домена.

GET /me/brands возвращает active memberships; GET /me/brand-invitations — invited. DTO включает бренд/роль. GET members owner-only отдаёт user_id, brand_id, роль, статус и даты/метаданные приглашения без данных учётной записи. Объекты entities напрямую не сериализуются.

### Транзакции, блокировки и TOCTOU

ManagementRepository.WithinBrand — небольшая транзакционная граница для usecase callback, без универсального permission engine. Единый порядок:

1. Строка бренда FOR UPDATE.
2. Membership текущего пользователя, затем целевого участника, если нужен переход.
3. Товар FOR UPDATE с проверкой его фактического brand_id.
4. Для публикации: цвета по ID FOR UPDATE, изображения по color_id/position FOR UPDATE, варианты по ID FOR UPDATE.
5. Доменные методы, SQL-запись и commit; при любой ошибке rollback.

Все операции одного бренда, включая принятие/отзыв приглашения, сериализуются строкой бренда. Поэтому разные порядки доступа к двум memberships не образуют цикл: в один момент только одна такая транзакция владеет брендом. Между брендами операции не берут вторую блокировку бренда. Частные чтения также используют эту границу для простоты согласованной политики.

При revoke vs update возможны два корректных порядка: операция editor фиксируется до revoke либо после revoke получает 404. После завершившегося revoke старый JWT не предоставляет права. Проверка отдельно от commit не используется. Защита проверена интеграционно, включая детерминированный ожидающий update за блокировкой с последующим revoke.

Для PATCH/publish/hide/unpublish бренд сначала определяется по существующему product_id, затем блокируется, товар перечитывается с условием brand_id и блокируется. Предварительное чтение brand_id не является проверкой прав: всё решение принимается под блокировками. PATCH не принимает brand_id. При публикации загружается полный реальный агрегат и вызывается Product.Publish; client is_ready/коллекции не принимаются. Блокировки родителей также блокируют конкурентные вставки через FK, блокировки дочерних строк — их изменения до commit. Public GET использует тот же загрузчик коллекций без write-lock в своём read-only snapshot.

**Limitations:** операции одного бренда сериализуются, это уменьшает пропускную способность, но упрощает доказательство актуальности прав. Транзакции READ COMMITTED. Управляющие запросы должны быть короткими; нагрузочное тестирование и production HTTP/DB timeouts — отдельная инфраструктурная работа. Прямые административные SQL-записи не являются безопасным API: будущие редакторы вариантов обязаны соблюдать тот же порядок и заново валидировать полный агрегат. Нельзя считать приложение защищённым от оператора с произвольными правами SQL.

### Ошибки и защита объектов

401 — нет JWT. Для отсутствующего membership или invited/declined/revoked private ресурс скрывается как 404, равно как отсутствующий объект. Editor, уже имеющий доступ к бренду, при owner-only действии получает 403. Suspended продуктовые операции — 403. Некорректные поля/ID — 400, состояние/slug/повторный переход/неполная публикация — 409. DTO не раскрывают SQL, пароль или токены.

Strict JSON и лимит 64 КиБ сохраняются. POST создания бренда принимает только name/slug; PATCH бренда только предусмотренные поля. Contacts/appearance заменяются целиком при передаче объекта. PATCH товара принимает только name/description/price_kopecks/category/materials. POST-переходы принимают `{}`; изменения не выполняются через GET. Все эти операции аутентифицированы Bearer, cookie не является источником полномочий, поэтому CSRF-контракт auth-cookie операций не изменён.

### Legacy brands и доверенный backfill

Миграция не назначает владельцев старым/seed-брендам. Публичные чтения сохраняются, но пользователь не может присвоить такой бренд. Отдельного backfill HTTP endpoint нет.

Операторский процесс (не автоматическая миграция): получить подтверждение владения вне приложения, проверить конкретные user_id/brand_id и согласование команды, сделать резервную копию, открыть транзакцию, заблокировать конкретный бренд FOR UPDATE, убедиться в отсутствии owner, вставить валидный active owner с joined_at/created_at/updated_at и зафиксировать проверяемый аудит назначения. При конфликте rollback; никогда не использовать массовый backfill на «первого пользователя». Прямая запись допускается только доверенному оператору и не является частью пользовательского API. Интеграционные тесты назначают owner seed-бренду исключительно внутри своей изолированной схемы для проверки полного агрегата.

### Threat model

| Угроза | Механизм | Ограничение |
| --- | --- | --- |
| BOLA/IDOR | поиск фактического бренда товара, membership того же бренда, проверка под lock | доверенный SQL-оператор вне модели API |
| BFLA | отдельные owner-only действия и editor/owner операции, deny by default | глобальных admin/seller ролей нет |
| Privilege escalation | owner создаётся только с новым брендом; роли/ownership не принимаются в JSON | ownership transfer Planned, не скрытый endpoint |
| Mass assignment | отдельные DTO, unknown fields запрещены, статусы отдельными действиями | null-поля PATCH трактуются как пропущенные |
| TOCTOU / revoke race | brand → membership → product → children, одна транзакция | сериализация операций бренда снижает параллелизм |
| Гонки приглашений/публикации | domain transitions, locks, PK/unique | проигравший запрос получает конфликт состояния |
| Захват legacy brand | отсутствие membership/backfill API и автоматического владельца | нужен ручной доверенный процесс |

### Следующий этап

**Planned:** полный CRUD цветов, изображений и вариантов с теми же locks/проверкой ProductPublication; frontend для координации токенов и приглашений; отдельный аудит Auth+RBAC. **Limitations:** новый draft без дочернего агрегата пока нельзя опубликовать. Нет ownership transfer, email-рассылки или глобального admin. Существующие ограничения JWT/logout, refresh grace и локального auth limiter остаются в разделе Authentication.


### Финальные меры аудита и отложенный deployment hardening

Recovery и access logging не печатают сырые URL/query, заголовки, cookies, тела или значения panic. Recovery пишет только фиксированный тип события и stack frames без аргументов/исходных строк; клиент получает обобщённую ошибку, broken connection не вызывает повторной записи ответа. Это проверяется синтетическими секретами в debug/release, включая EPIPE/ECONNRESET.

R2 остаётся deployment risk, не production-ready: необходимо настроить HTTP ReadHeader/Read/Idle deadlines, DB pool budget, context/lock/statement timeouts, proxy connection/body limits и распределённый limiter. Locking protocol RBAC сохранён. Возможный deny-only precheck membership до ожидания lock не должен заменять авторитетную проверку внутри транзакции. Нагрузочное тестирование и production proxy/TLS вне текущего этапа.
