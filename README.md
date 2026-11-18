# SREZ

Локальный MVP маркетплейса независимых streetwear-брендов. Go API, PostgreSQL и React работают с настоящими товарами, вариантами, изображениями и аккаунтами. Интерфейс сохраняет стиль SREZ; фиктивный каталог вместо API не используется.

**Заказ сейчас — неоплаченный резерв на 15 минут. Оплаты, доставки и обработки оплаченных заказов пока нет.** Публикация исходников в GitHub не означает deployment приложения.

## Возможности

- Регистрация, подтверждение email, вход по email/username, профиль и managed avatar.
- Восстановление пароля через одноразовую ссылку; отзыв всех прежних сессий и access JWT.
- Каталог с фильтрами category/brand/size/color/price, сортировкой и пагинацией.
- Товар: галерея, конкретные цвета/размеры/остатки, материалы, бренд.
- Избранное, корзина выбранных вариантов, серверные цены и суммы.
- Неоплаченные резервы, история, отмена и автоматическое освобождение остатков.
- Несколько вишлистов: порядок, предпочтительный вариант, публичные ссылки, ротация/отзыв.
- Студия бренда: owner/editor, приглашения, публикация, PVM и загрузка изображений.
- Приватный SeaweedFS; браузер работает с файлами через Go backend.
- Mailpit для настоящего локального SMTP; письма не уходят в реальный почтовый ящик.

## Стек и архитектура

Go 1.27.2+, Gin, sqlx, PostgreSQL 16; React 19, TypeScript, Vite, React Router. JWT HS256 до 15 минут, Argon2id, opaque refresh sessions с абсолютным сроком 7 дней. Docker Compose, SeaweedFS, Mailpit. Проверки frontend: TypeScript, **Oxlint** (текущий lint проекта), Vitest и Playwright. Версии зависимостей закреплены go.mod/go.sum и package-lock.json.

HTTP Handler валидирует DTO и вызывает usecase. Usecase выполняет сценарий и проверяет доменные правила. Storage содержит SQL и транзакции; entities не сериализуются напрямую. В main собираются зависимости. Auth остаётся единым; verification и recovery используют общий MailSender/SMTP, отдельные токены и SQL-таблицы.

```text
cmd/app/                 конфигурация и запуск Go API
cmd/seed/                явный старый dev seed
internal/config/         конфигурация и проверка секретов
internal/entities/       доменные сущности/инварианты
internal/handler/        Gin, DTO, маршруты
internal/usecase/        Auth, каталог, PVM, shopping, media, wishlists
internal/storage/        PostgreSQL, SQL и транзакции
internal/security/       Argon2id, JWT/CSRF, share capabilities
internal/mailer/         общий SMTP, HTML/text письма
internal/objectstorage/  приватный S3-compatible SeaweedFS
migrations/              append-only SQL 001–015 и migration runner
frontend/src/            React UI, features, transport и auth state
frontend/e2e/            настоящие API/Mailpit/browser сценарии
frontend/docs/           план и отчёты предыдущих этапов
compose*.yaml            отдельные working/QA/Go-test окружения
deploy/                  запуск приватного SeaweedFS
scripts/                 безопасные команды запуска/QA/demo
demo/catalog/            необязательные изображения и manifest витрины
docs/                    актуальные доменные контракты
.github/workflows/ci.yml проверки PR, без deployment
```

[Архитектура](docs/architecture.md), [единая карта всех API](docs/api-reference.md), [ревизия и технический долг](docs/project-review.md), [план frontend](frontend/docs/IMPLEMENTATION_PLAN.md).

## Быстрый запуск на macOS

[Инструкция для Apple Silicon](docs/macos-development.md) описывает полный запуск в Docker Desktop, разработку с Vite, подтверждение почты через Mailpit и работу с локальными портами.

## Быстрый запуск: Windows PowerShell

Нужны Docker Desktop с Linux containers и Node 24 LTS (либо совместимая версия из frontend/package.json). Для Docker-запуска Go на хосте не нужен. Ниже команды из **корня этой рабочей копии**. Не запускайте другой Compose поверх чужой БД.

```powershell
# Например, после открытия своей локальной папки SREZ:
docker version
docker compose version
node --version
node .\frontend\scripts\prepare-local.mjs
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-integration.ps1
```

Скрипт сохраняет существующие значения `.env` и генерирует только отсутствующие ключи. Не копируйте `.env.example` поверх своей конфигурации. Docker собирает Go и React, запускает PostgreSQL/SeaweedFS/Mailpit, затем API применяет недостающие миграции. Готовый frontend обслуживает nginx, отдельный терминал Vite для обычного запуска не нужен.

| Сервис       | Адрес на этом ПК             | В Docker       |
| ------------ | ---------------------------- | -------------- |
| SREZ         | http://localhost:5174        | frontend:8080  |
| API          | http://127.0.0.1:8081/health | app:8080       |
| PostgreSQL   | 127.0.0.1:15432              | db:5432        |
| SeaweedFS S3 | 127.0.0.1:18333, приватный   | seaweedfs:8333 |
| Mailpit UI   | http://127.0.0.1:8025        | mailpit:8025   |
| SMTP         | 127.0.0.1:1025               | mailpit:1025   |

Все опубликованные порты привязаны к loopback. Для auth используйте именно **localhost:5174**, как в PUBLIC_ORIGIN и FRONTEND_BASE_URL. Mailpit можно открыть через 127.0.0.1. Не меняйте hostname приложения посреди cookie-сессии.

```powershell
Invoke-RestMethod 'http://127.0.0.1:8081/health'
Invoke-RestMethod 'http://localhost:5174/api/health'
docker compose -p srez-integration -f compose.integration.yaml ps
# Остановка с сохранением БД, изображений и писем:
docker compose -p srez-integration -f compose.integration.yaml stop
# Возобновление:
docker compose -p srez-integration -f compose.integration.yaml start
```

Не используйте down -v, factory reset, удаление volumes или смену POSTGRES_PASSWORD ради исправления запуска. Изменение переменной не меняет пароль роли в существующей БД. Если порт занят, определите владельца процесса и выберите отдельное окружение; не завершайте неизвестные процессы.

## Конфигурация

`.env` приватный и не читается Go автоматически. Compose подставляет его значения; dev-скрипт читает файл как данные, не исполняет shell. Пример не содержит настоящих SMTP/API секретов.

| Настройка                               | Назначение                                                          |
| --------------------------------------- | ------------------------------------------------------------------- |
| POSTGRES_PASSWORD, DATABASE_URL         | пароль БД и native Go DSN; Compose задаёт внутренний db:5432        |
| HTTP_ADDR                               | адрес Go listener, native integration 127.0.0.1:8081                |
| SREZ_JWT_KEY / SREZ_CSRF_KEY            | независимые Base64 ключи минимум 32 байта                           |
| SREZ_WISHLIST_KEY                       | отдельный стабильный ключ owner recovery публичных ссылок           |
| SREZ_PUBLIC_ORIGIN                      | точный origin auth, integration http://localhost:5174               |
| SREZ_AUTH_LOCAL_HTTP                    | true только для локального loopback HTTP                            |
| SREZ_S3_*                               | endpoint, private access/secret keys, bucket/region; только backend |
| SREZ_SMTP_HOST / PORT                   | Docker mailpit:1025, native Windows 127.0.0.1:1025                  |
| SREZ_MAIL_FROM                          | sender, локально SREZ <no-reply@srez.local>                         |
| SREZ_FRONTEND_BASE_URL                  | доверенный origin ссылок, integration http://localhost:5174         |
| SREZ_SMTP_TLS                           | none только локально; starttls/tls для провайдера                   |
| SREZ_SMTP_USERNAME / PASSWORD           | необязательные private credentials реального SMTP                   |
| SREZ_EMAIL_IP_MINUTE_LIMIT / HOUR_LIMIT | defaults10/50; независимые persistent quotas verification/recovery  |

JWT, CSRF, wishlist и S3 ключи нельзя регенерировать при каждом запуске: это нарушит существующие сессии/доступ к данным. `.env`, `.secrets`, дампы, volumes и Mailpit mailbox должны оставаться приватными. В браузер не передаются server secrets; VITE_* для них не используется.

`compose.integration.yaml` — рекомендуемая полная локальная копия. Базовый `compose.yaml` остаётся backend-only конфигурацией на 8080/5432/8333; не применяйте его к старому проекту для обновления. `compose.frontend.yaml` — дополнительная локальная конфигурация прежнего workflow. Для текущей копии достаточно одного integration Compose.

## Разработка Go и React отдельно

Frontend сохраняет React/TS/Vite. Vite слушает localhost:5174, проксирует **только /api** к 127.0.0.1:8081 с удалением префикса и /media без изменения пути. /products и /brands принадлежат frontend routing и целиком не проксируются.

```powershell
# Если вместо Docker frontend нужен Vite:
docker compose -p srez-integration -f compose.integration.yaml stop frontend
npm.cmd --prefix .\frontend ci
npm.cmd --prefix .\frontend run dev
```

Для native Go нужен Go1.27.2+ в PATH. В другом PowerShell остановите только integration app; инфраструктура остаётся той же:

```powershell
docker compose -p srez-integration -f compose.integration.yaml up -d --wait db seaweedfs mailpit
docker compose -p srez-integration -f compose.integration.yaml stop app
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-dev-backend.ps1
```

Native backend работает с Vite frontend; Docker nginx ожидает контейнер app. Dev-скрипт временно загружает .env, затем явно задаёт integration DSN на **15432**, HTTP8081, S318333, SMTP1025 и origin localhost:5174. Исходный .env не меняется. GoLand Run Configuration должен использовать те же значения и существующие ключи; исторический DATABASE_URL на5432 не подходит. После завершения native процессов вернитесь к start-integration.ps1.

## PostgreSQL и миграции

SQL001–015 встроены в бинарник. Startup выполняет миграции до HTTP listener под advisory lock, в транзакции. Изменённый применённый SQL блокирует startup; добавлять нужно только новую версию. Старые файлы и migration journal не переписываются.

Migration runner признаёт одинаковый SQL с LF/CRLF, чтобы Windows checkout и Linux clone не давали ложный checksum conflict; другие изменения по-прежнему отклоняются. Новые journal checksums canonical LF, старые сохранены.

Данные находятся в named volumes проекта: postgres_data, seaweedfs_data, mailpit_data. Сохраняйте имя Compose-проекта. Миграции сначала проверяются на отдельной тестовой БД, затем допустимы в working. Резервная копия — приватные исходники/config + custom pg_dump с проверкой SHA-256/TOC, не файл в Git.

## Mailpit, регистрация и восстановление

Откройте `/register`, создайте аккаунт с паролем от12 символов. До подтверждения почты JWT/refresh не выдаются. Откройте Mailpit8025, найдите письмо по `to:ваш-email`, перейдите по кнопке и отдельно подтвердите адрес на странице SREZ. Затем войдите. Legacy-аккаунты сохраняют вход с required=false и реальным verified_at=NULL; UI не считает их подтверждёнными.

На login есть «Забыли пароль?». Для ранее подтверждённой почты можно запросить ссылку, открыть письмо Mailpit и установить новый пароль дважды. Все старые сессии и JWT отзываются; автоматического входа нет. Неизвестный/неподтверждённый адрес получает такой же нейтральный ответ.

Оба challenge используют отдельные таблицы, 32 random bytes, SHA-256, срок30 минут, одноразовость и ротацию. Account quota:60 секунд/5 выпусков в час. SMTP только после SQL commit; при отказе аккаунт не теряется. Асинхронный resend/forgot не обещает доставку, после сбоя требуется повторить позже. Plaintext durable outbox не используется.

[Verification API и пошаговая Windows-инструкция](docs/email-verification.md), [Recovery API и отзыв JWT](docs/account-recovery.md). Внешний провайдер потребует HTTPS, TLS, sender/domain и отдельной проверки доставки; сейчас письма остаются локальными.

## Каталог, PVM, изображения и роли

Каталог и бренды публичны; новые пустые БД не заполняются приложением автоматически. Цена — копейки, пагинация — HTTP заголовки. Цвет+размер выбирают один реальный product_variant_id, отсутствующие сочетания нельзя купить.

В `/studio` owner создаёт бренд, описание/логотип и публикует. Owner/editor создают draft товар, заполняют материалы/цену/категорию, цвета, фото и варианты с остатками; затем явно публикуют. Структурные изменения опубликованного товара могут потребовать hide → edit → publish. Owner управляет приглашениями; editor не меняет owner-настройки. Отзыв роли действует через актуальный SQL membership.

Файлы загружаются POST /media/uploads → PUT content_url через Go. Поддерживаются JPEG/PNG/WebP до5 MiB с проверкой фактического содержимого, размеров, нормализацией и повторным RBAC. S3 private. Legacy HTTPS URLs читаются, но новые avatar/logo/banner и PVM photos загружаются через Media API; PATCH внешних URL запрещён. Managed URL имеет вид /media/assets/:id/content, ошибка изображения даёт placeholder.

[Каталог](docs/catalog-api.md), [актуальный PVM](docs/product-variant-management.md), [Media](docs/media-storage.md), [GC](docs/media-storage.md).

Необязательный `demo/catalog` содержит пять разных брендов,21 позицию (по3 в каждой категории), различные PNG. `node scripts/seed-showcase.mjs` сначала только проверяет manifest; запись требует явного --apply. Новый demo owner проходит обычное подтверждение Mailpit, после чего скрипт запускается повторно. Частные credentials/state остаются в .secrets. Не запускайте seed ради исправления пользовательской БД. Старый cmd/seed также требует явного SREZ_DEV_SEED=true и не перезаписывает существующие slug.

## Покупатель и вишлисты

`/favorites` синхронизируется с карточками; `/cart` хранит реальные варианты, количество и суммы из API. Корзина ничего не резервирует. POST /orders принимает {}, перепроверяет stock/price и создаёт неоплаченный резерв15 минут. История и cancel_before берутся с сервера. Отмена/истечение освобождают остатки атомарно. При неопределённом POST сначала проверьте историю — повторная отправка автоматически не выполняется.

Вишлисты отделены от избранного: до20 списков по200 товаров, предпочтительный variant nullable, полный reorder. Share выдаёт capability, revoke/rotate запрещает старую ссылку при следующем запросе. Публичный DTO исключает скрытые товары/private profile; право не берётся из cookie. Уже прочитанные гостем данные невозможно отозвать из памяти.

[Buyer API и сценарии](docs/buyer-shopping.md), [Wishlists API](docs/wishlists.md). `/projects` сохранён только для учебной совместимости; /users HTTP отключены.

## Тестирование и CI

Тесты не должны использовать рабочую БД. Существующие окружения разделены:

| Проект           | Frontend/API | PostgreSQL | S3    | Mailpit UI/SMTP |
| ---------------- | ------------ | ---------- | ----- | --------------- |
| srez-integration | 5174/8081    | 15432      | 18333 | 8025/1025       |
| srez-mvp-qa      | 5175/8082    | 15433      | 18334 | 18025/11025     |
| srez-mvp-tests   | нет          | 15434      | 18335 | 28025/12025     |

```powershell
# Frontend:
npm.cmd --prefix .\frontend ci
npm.cmd --prefix .\frontend run typecheck
npm.cmd --prefix .\frontend run lint
npm.cmd --prefix .\frontend test
npm.cmd --prefix .\frontend run build
npm.cmd --prefix .\frontend run format:check
# Go с реальными изолированными PostgreSQL/SeaweedFS/Mailpit:
docker compose -p srez-mvp-tests -f compose.tests.yaml up -d --wait
docker build --target build -t srez-mvp-go-check:local .
node .\scripts\qa-go.mjs full
node .\scripts\qa-go.mjs vet
# Browser, только изолированный localhost:5175:
docker compose -p srez-mvp-qa -f compose.qa.yaml up -d --build --wait
node .\scripts\qa-browser.mjs all
# Сервисная остановка не удаляет volumes:
docker compose -p srez-mvp-qa -f compose.qa.yaml stop
```

Обычные go build ./..., go vet ./..., go test ./... доступны на хосте. Без TEST_DATABASE_URL интеграционные tests явно skipped; такой прогон не доказывает работу PostgreSQL. Полный QA runner передаёт test DSN/S3/Mailpit через env без вывода секретов. Browser runner открывает отдельный процесс на spec, автоматически обнаруживает все tests, не исключает recovery и не повторяет упавшие автоматически. Требуется установленный Chrome; на новом компьютере `npm.cmd --prefix frontend exec -- playwright install chrome`.

Перед каждым browser spec runner проверяет Compose-метки и перезапускает только `srez-mvp-qa-app-1`, затем ждёт `/api/health` через frontend proxy. Это изолирует общий минутный auth/IP-бюджет между наборами: быстрый CI иначе превышает 100 запросов и получает 429 в последних tests. Backend-лимиты не меняются; PostgreSQL, постоянные email-квоты, сессии и volumes сохраняются.

CI проверяет Go format/build/vet/tidy и полный suite с реальной test инфраструктурой; отдельно npm ci/typecheck/Oxlint/format/unit/build и browser QA. Actions имеют contents:read, не используют production secrets и не выполняют deployment. Первый удалённый run нужно оценивать отдельно от локальных результатов.

[Итог этого этапа](docs/release-report.md), [review и приоритеты](docs/project-review.md). Исторические frontend отчёты описывают свои этапы, их старые количества тестов не являются текущими.

## Безопасность и границы

JWT только в памяти JS, refresh HttpOnly/Strict, точный Origin и CSRF для cookie-операций. Bearer проверяет активную сессию одним индексированным запросом: reset/logout/replay отзывают access при следующей проверке. Кеш отзыва не используется; DB pool ограничен20 соединениями, auth lookup имеет deadline2 секунды. При недоступной БД доступ не разрешается. Уже выполняющийся запрос не отменяется задним числом.

SQL параметризован, user_id/роль/цены не принимаются вместо серверных данных, RBAC/IDOR/private media проверяются повторно. Sensitive страницы очищают URL и используют no-store/no-referrer/CSP; логи не содержат credentials/capability URL. В Git не входят .env, .secrets, дампы, базы, node_modules, сборки, логи и локальные screenshots. Резервные копии остаются на компьютере.

Это проверенный локальный MVP, не независимая сертификация безопасности. Нет настоящей оплаты/доставки, password history, production monitoring/HTTPS/DNS, автоматической безопасной ротации всех ключей и retention jobs. Checkout idempotency key, каталожные facets, guest cart/merge и transfer ownership остаются отдельными задачами. Подробный приоритетный roadmap — в review. Публичный запуск требует отдельного разрешения и эксплуатации.
