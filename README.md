# SREZ

SREZ — учебный командный backend-проект маркетплейса независимых streetwear-брендов. Он объединяет бренды и их товары в одном каталоге: покупатели могут знакомиться с брендами и смотреть карточки одежды с цветами, фотографиями, размерами и остатками. Для брендов проект предусматривает развитие сценариев управления ассортиментом.

Сейчас реализован HTTP API без frontend. Стек: **Go, Gin, PostgreSQL 16, sqlx, Docker Compose**.

## Текущее состояние

Реализованы:

- регистрация с Argon2id, login, HS256 access JWT и серверные refresh-сессии;
- CSRF-защита, refresh rotation/replay detection и управление собственным профилем;
- публичный каталог активных брендов и опубликованных товаров;
- подробные карточки с цветами, упорядоченными фотографиями и вариантами размеров;
- BrandMembership owner/editor, приглашения, отзыв прав и управление брендами;
- защищённое создание/редактирование draft, публикация готовых товаров;
- версионируемые миграции при старте, отдельный повторяемый dev seed;
- тесты домена, usecase и HTTP, интеграционные тесты PostgreSQL.

На следующих этапах: Product Variant Management, frontend, подтверждение email и восстановление доступа. Корзина, заказы и оплата не реализованы.

**Проект предназначен для локального обучения и разработки. Auth требует независимого AppSec-ревью перед публичным развёртыванием. Публикация исходников не означает готовность HTTP-сервера к production.**

## Быстрый запуск через Docker Compose

Нужен запущенный Docker Engine / Docker Desktop с Compose. На Apple Silicon образы собираются для arm64, принудительная эмуляция amd64 не требуется.

```sh
# Выполните копирование только если у вас ещё нет локального .env.
cp .env.example .env
# Два независимых случайных ключа; значения не выводятся в терминал.
umask 077
printf '\nSREZ_JWT_KEY=%s\nSREZ_CSRF_KEY=%s\n' "$(openssl rand -base64 32)" "$(openssl rand -base64 32)" >> .env
chmod 600 .env
docker compose up --build -d
curl -i http://127.0.0.1:8080/health
curl -i http://127.0.0.1:8080/brands
curl -i http://127.0.0.1:8080/products
```

Compose запускает `db` и `app`, передаёт приложению DATABASE_URL с внутренним адресом `db:5432` и ждёт готовности PostgreSQL. Приложение применяет миграции до открытия HTTP-порта. В свежей БД списки пользователей, брендов и товаров пусты (`[]`).

HTTP и PostgreSQL опубликованы только на `127.0.0.1` хоста (порты 8080 и 5432). Внутри контейнера Dockerfile явно задаёт `HTTP_ADDR=:8080`. Настройки Compose предназначены для локальной разработки: роль владельца БД, демонстрационный пароль и отключённый SSL.

`.env` игнорируется Git и исключён из Docker build context. Не добавляйте реальные секреты в исходники или `.env.example`. В примере используется локальный пароль `srez_dev`; для существующего volume из предыдущих этапов нужен фактический прежний пароль. Изменение POSTGRES_PASSWORD **не меняет пароль роли в уже созданной БД**. Для нового локального окружения допустим другой URL-safe пароль; согласуйте с ним DATABASE_URL при запуске Go на хосте.

Данные сохраняются в named volume `postgres_data`. Чтобы продолжать использовать прежнюю БД, сохраняйте имя Compose-проекта. Остановка с сохранением данных:

```sh
docker compose down
```

Не удаляйте volume и не используйте `down -v` для обновления проекта или миграций.

## Демонстрационные данные

После сборки и запуска приложения выполните seed явно:

```sh
docker compose run --rm --no-deps -e SREZ_DEV_SEED=true --entrypoint /seed app
```

Seed создаёт активные бренды `srez-studio` и `linea`, по опубликованному товару с двумя цветами и тремя вариантами. У размера L остаток равен нулю. URL и контакты демонстрационные; ID генерирует PostgreSQL, поэтому берите их из GET-ответов.

Команда требует `SREZ_DEV_SEED=true`; сервер её не вызывает. Повторный запуск не создаёт дубликаты. Существующий slug пропускается целиком: его бренд и товары не меняются, вручную изменённые примеры не восстанавливаются.

## Go / GoLand на хосте

Нужен Go **1.27.2 или новее**. В zsh/bash из корня проекта, после создания `.env` по примеру:

```sh
docker compose up -d --wait db
docker compose stop app
set -a
source .env
set +a
go run ./cmd/app
```

Source выполняет содержимое файла в shell: используйте только свой локальный `.env`, созданный из проверенного примера. Приложение само `.env` не читает.

Для GoLand задайте DATABASE_URL в Environment variables: `postgres://srez:srez_dev@localhost:5432/srez?sslmode=disable` (с фактическим локальным паролем). PostgreSQL должен быть доступен при старте. Если HTTP_ADDR не задан или пуст, сервер слушает **127.0.0.1:8080**. Явное значение сохраняется; другой локальный порт: `HTTP_ADDR=127.0.0.1:8081 go run ./cmd/app`.

Seed для Go с тем же локальным DATABASE_URL: `SREZ_DEV_SEED=true go run ./cmd/seed`.

## HTTP API

| Метод | Путь | Поведение |
| --- | --- | --- |
| GET | `/health` | состояние HTTP-процесса, не проверка БД |
| GET | `/brands` | активные бренды |
| GET | `/brands/:slug` | активный бренд или 404 |
| GET | `/products` | краткие карточки published-товаров активных брендов |
| GET | `/products/:id` | подробная карточка с цветами, изображениями и вариантами |
| POST | `/products` | Bearer JWT и активный owner/editor конкретного бренда; сохраняет draft, 201 |
| POST | `/auth/register` | регистрация, 201; email/username/display_name/password |
| POST | `/auth/login` | identifier (email или username) и password; access JWT, CSRF и refresh-cookie |
| GET | `/auth/csrf` | восстановление CSRF по действующей refresh-cookie |
| POST | `/auth/refresh` | атомарная ротация cookie и новый access JWT |
| POST | `/auth/logout` | отзыв текущей refresh-сессии, 204 |
| GET | `/profile` | собственный профиль/email по Bearer JWT |
| PATCH | `/profile` | собственные display_name/avatar_url по Bearer JWT |
| GET | `/projects` | фиксированный список предыдущего учебного этапа |

ID — положительный int64; неверное значение или переполнение дают 400. Slug: 3–60 строчных латинских букв, цифр или дефисов. Недоступные публично сущности дают 404. Ошибки имеют вид `{"error":"описание"}`; SQL-детали клиенту не передаются.

Все старые `/users` HTTP-маршруты отключены (404), чтобы исключить создание аккаунтов без пароля, перечисление пользователей и обход проверки владельца. Существующие записи сохраняются. Публичные DTO регистрации исключают email/хеш; собственный профиль включает email. PATCH принимает только display_name/avatar_url, непустой URL — HTTPS. Пользовательский ID в JSON не принимается.

### Управление брендами и товарами

Все маршруты ниже требуют Bearer JWT. Роли берутся из актуального membership в PostgreSQL для конкретного бренда, не из JSON или JWT. Owner может управлять настройками и участниками; owner/editor могут управлять товарами. Invited/declined/revoked прав не дают. Отзыв применяется к новым операциям сразу, даже со старым действующим access JWT.

| Метод | Путь | Доступ |
| --- | --- | --- |
| POST | `/brands` | любой аутентифицированный пользователь; создаёт draft и owner в одной транзакции |
| GET | `/me/brands` | свои бренды с активным membership и ролью |
| GET | `/brands/:id/manage` | активный owner/editor |
| PATCH | `/brands/:id` | owner; name/slug/description/logo_url/banner_url/contacts/appearance |
| POST | `/brands/:id/publish`, `/brands/:id/unpublish` | owner; тело `{}` |
| GET | `/brands/:id/members` | owner; только membership DTO, без email/хешей |
| POST | `/brands/:id/invitations` | owner; зарегистрированный username |
| GET | `/me/brand-invitations` | свои ожидающие приглашения, бренд и роль |
| POST | `/brands/:id/invitations/accept`, `/brands/:id/invitations/decline` | только адресат; тело `{}` |
| DELETE | `/brands/:id/members/:user_id` | owner; отзыв editor, owner отзывать нельзя |
| GET | `/brands/:brand_id/products/manage` | активный owner/editor; включая draft/hidden |
| POST | `/products` | активный owner/editor; draft для draft/active бренда |
| PATCH | `/products/:id` | активный owner/editor реального бренда товара |
| POST | `/products/:id/publish`, `/products/:id/hide`, `/products/:id/unpublish` | активный owner/editor; тело `{}` |

Непричастный пользователь получает 404 для приватного ресурса, editor при попытке owner-операции — 403. Некорректный запрос — 400, конфликт перехода/slug/неполная публикация — 409. Suspended бренд не допускает управление товарами; публикация товара требует active бренда. PATCH товара допускает только name/description/price_kopecks/category/materials, brand_id менять нельзя. Contacts/appearance в PATCH бренда заменяются целиком, а не сливаются по отдельным полям. Статусы меняются отдельными POST-операциями.

Пример после login (ACCESS_TOKEN владельца и EDITOR_TOKEN редактора получаются через описанные ниже auth endpoints):

```sh
# Создать бренд; ID возьмите из ответа.
curl -sS "$ORIGIN/brands" -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H 'Content-Type: application/json' -d '{"name":"Мой бренд","slug":"my-brand"}'
BRAND_ID=3 # замените на настоящий ID из ответа
curl -sS "$ORIGIN/brands/$BRAND_ID/invitations" -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H 'Content-Type: application/json' -d '{"username":"editor_username"}'
curl -sS "$ORIGIN/brands/$BRAND_ID/invitations/accept" -H "Authorization: Bearer $EDITOR_TOKEN" \
  -H 'Content-Type: application/json' -d '{}'
curl -sS "$ORIGIN/products" -H "Authorization: Bearer $EDITOR_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"brand_id\":$BRAND_ID,\"name\":\"Футболка\",\"price_kopecks\":250000}"
```

Dev-флаг создания товаров удалён и не предоставляет обхода. Legacy/seed-бренды не получают владельцев автоматически: они сохраняют публичное чтение, но не доступны для управления до отдельного доверенного назначения оператором. Публичного присвоения бренда или передачи владения нет.

Новые товары без цветов/изображений/вариантов пока нельзя публиковать. Product Variant Management ещё не реализован; публикация существующего полного агрегата использует данные БД и доменные правила, без клиентского `is_ready`.

POST принимает ровно один JSON-объект до 64 КиБ включительно; лимит измеряется по фактически прочитанным байтам, включая пробелы и chunked-тело. Превышение даёт 413. Неизвестные поля, клиентские ID/статус/даты и дополнительный JSON запрещены. Обязательны brand_id > 0 и name длиной 2–120 Unicode-символов после trim. Description до 5000 символов, цена — int64 в копейках >=0, категория пустая или `tshirt/hoodie/pants/outerwear/footwear/accessories/other`, materials — до 10 непустых строк без повторений после trim. Draft может быть неполным, автоматической публикации нет.

## Архитектура

`cmd/app` собирает зависимости. `handler` разбирает HTTP и формирует DTO; `usecase` выполняет сценарии и межсущностные проверки; `entities` задают инварианты и переходы состояний; `storage` выполняет параметризованный SQL через sqlx. HTTP-типы и SQL отделены от домена.

Подробности схемы, транзакций и миграций — в [docs/architecture.md](docs/architecture.md), доменные правила — в [internal/entities/README.md](internal/entities/README.md). Старое ТЗ каталога проектов сохранено в [docs/legacy/SPEC.md](docs/legacy/SPEC.md) для преподавателя; оно не описывает текущий маркетплейс.

## Проверки

```sh
go test ./...
go vet ./...
git diff --check
docker compose config
```

Compose config требует локальный `.env` или POSTGRES_PASSWORD в окружении; вывод команды включает разрешённые значения переменных, не публикуйте его с реальными секретами.

Без TEST_DATABASE_URL PostgreSQL-интеграционные тесты помечаются skipped. Для их запуска используйте **отдельную тестовую БД**, например на порту 15432:

```sh
TEST_DATABASE_URL='postgres://srez:srez_dev@localhost:15432/srez?sslmode=disable' \
  go test ./internal/storage -run Postgres -v -count=1
```

Порт приведён как пример; тестовая PostgreSQL должна быть запущена отдельно. Каждый тест создаёт уникальную схему `catalog_test_*`, задаёт search_path и удаляет только собственную тестовую схему. Проверяются миграции на чистой/существующей users-схеме, rollback, параллельный старт, seed, ограничения БД, публичность, связи цветов/вариантов и User CRUD. RBAC-тесты проверяют матрицу прав, BOLA, mass assignment, legacy-бренды, атомарность создания owner, приглашения/отзыв и конкурентные операции. Остальные тесты проверяют домен, usecase, DTO, HTTP-ошибки и лимит тела.

## Authentication: окружение и проверка

Auth включён всегда; отсутствие корректных ключей блокирует запуск. В `.env.example` ключи пусты намеренно. Создайте их командами быстрого запуска, не коммитьте `.env`.

| Переменная | Назначение |
| --- | --- |
| SREZ_JWT_KEY / SREZ_CSRF_KEY | независимые base64-ключи минимум 32 случайных байта каждый |
| SREZ_JWT_ISSUER / SREZ_JWT_AUDIENCE | обязательные issuer/audience |
| SREZ_PUBLIC_ORIGIN | точный браузерный origin без пути/слеша; локально http://127.0.0.1:8080 |
| SREZ_AUTH_LOCAL_HTTP | только true разрешает HTTP-cookie без Secure на loopback; production false/HTTPS |
| SREZ_AUTH_ATTEMPTS | общий локальный login/register лимит на IP в минуту; по умолчанию 10 |
| SREZ_AUTH_HASH_WORKERS | одновременно Argon2id; по умолчанию 2, максимум 8 |
| SREZ_TRUSTED_PROXIES | явно доверенные IP/CIDR proxy через запятую; пусто по умолчанию |

Access TTL фиксирован 15 минут, абсолютный refresh TTL — 7 дней. Ключи отсутствуют в образе; передаются окружением при запуске. При смене локального HTTP-порта меняйте также SREZ_PUBLIC_ORIGIN. Будущий frontend должен использовать dev proxy и хранить access только в памяти JavaScript; React и reverse proxy сейчас не реализованы.

POST auth endpoints требуют точный Origin, в том числе curl, и application/json. GET /auth/csrf допускает отсутствующий Origin: обычный браузерный same-origin fetch может его не отправлять. Если Origin указан, он должен точно совпадать с SREZ_PUBLIC_ORIGIN. Для всех auth endpoints при наличии Sec-Fetch-Site разрешён только same-origin; cross-site, same-site и другие значения отклоняются. Без обоих заголовков GET допустим для совместимости с клиентами без Fetch Metadata: доступ всё равно требует действующую refresh-cookie, чтение сторонним браузерным сайтом ограничено Same-Origin Policy, CORS не включён. GET не изменяет сессию и возвращает Cache-Control: no-store. Refresh/logout дополнительно требуют X-CSRF-Token и cookie. Bearer GET/PATCH `/profile` не использует cookie и не требует Origin.

Пример для локального окружения (curl, Python 3). Файлы с cookie/токенами создаются в отдельном временном каталоге с правами только владельца; не добавляйте их в Git и не публикуйте:

```sh
ORIGIN=http://127.0.0.1:8080
AUTH_TMP=$(mktemp -d)
chmod 700 "$AUTH_TMP"
curl -sS -X POST "$ORIGIN/auth/register" -H "Origin: $ORIGIN" \
  -H 'Content-Type: application/json' \
  -d '{"email":"demo-auth@example.com","username":"demo_auth","display_name":"Demo","password":"local demo password 123"}'
curl -sS -c "$AUTH_TMP/cookies" "$ORIGIN/auth/login" -H "Origin: $ORIGIN" \
  -H 'Content-Type: application/json' \
  -d '{"identifier":"demo_auth","password":"local demo password 123"}' > "$AUTH_TMP/login.json"
ACCESS_TOKEN=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["access_token"])' "$AUTH_TMP/login.json")
curl -sS "$ORIGIN/profile" -H "Authorization: Bearer $ACCESS_TOKEN"
curl -sS -b "$AUTH_TMP/cookies" "$ORIGIN/auth/csrf" > "$AUTH_TMP/csrf.json"
CSRF_TOKEN=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["csrf_token"])' "$AUTH_TMP/csrf.json")
curl -sS -b "$AUTH_TMP/cookies" -c "$AUTH_TMP/cookies" "$ORIGIN/auth/refresh" \
  -H "Origin: $ORIGIN" -H "X-CSRF-Token: $CSRF_TOKEN" \
  -H 'Content-Type: application/json' -d '{}' > "$AUTH_TMP/refresh.json"
curl -i -b "$AUTH_TMP/cookies" "$ORIGIN/auth/logout" -H "Origin: $ORIGIN" \
  -H "X-CSRF-Token: $CSRF_TOKEN" -H 'Content-Type: application/json' -d '{}'
unset ACCESS_TOKEN CSRF_TOKEN
```

Для HTTP используется cookie srez_refresh_local; production: __Host-srez_refresh, Secure, HttpOnly, SameSite=Strict, Path=/, без Domain. CSRF сохраняет привязку к сессии при rotation. GET /auth/csrf позволяет восстановить его после перезагрузки страницы. Старый refresh в первые 5 секунд после ротации получает 409 без новых токенов; более поздний replay отзывает сессию. Координируйте refresh между вкладками. После logout access JWT может действовать до истечения 15 минут.

Миграция 003 добавляет регистронезависимую уникальность email/username. При конфликтах старых данных запуск остановится, данные не меняются. Нужен разбор оператором с подтверждением владения; не удаляйте volume. Старые аккаунты с `!` не могут войти, не получают пароль через регистрацию и не имеют автоматического восстановления.

## Ограничения безопасности

- Нужны независимое AppSec-ревью, HTTPS и production-инфраструктура. Локальный Compose использует demo-пароль PostgreSQL, sslmode=disable и роль владельца БД.
- Rate limiting локальный, по IP: несколько инстансов/ботнет обходят общий лимит. Потребуется распределённая защита. Argon2id ограничен семафором, но это не полная защита от DoS.
- JWT проверяется без запроса отзыва сессии; logout не отзывает access немедленно. XSS может украсть access из памяти и инициировать запросы с HttpOnly cookie. CSP/frontend-защита ещё не реализованы.
- Пять секунд терпимости к replay не позволяют отличить все атаки от гонок; потеря ответа после rotation может потребовать нового login.
- Нет email verification/recovery, retention job истории refresh и ограничения числа активных сессий. Register раскрывает занятость аккаунта ответом 409; login использует общую ошибку.
- Все операции управления требуют Bearer и актуальный membership. Прямая SQL-запись оператором обходит прикладные проверки; доступ к БД должен быть ограничен.
- Публичный каталог сохранён. Полный CRUD цветов/вариантов, ownership transfer и frontend пока отсутствуют.

Полная модель угроз, жизненный цикл и компромиссы — в [docs/architecture.md](docs/architecture.md).
