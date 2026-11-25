# SREZ — локальный MVP

React + TypeScript + Vite сохранены и развиты. Реальные Go API обслуживают авторизацию, каталог, избранное, корзину, неоплаченные резервы, вишлисты и кабинет бренда. Изображения проходят через Go и приватный SeaweedFS. Новые регистрации требуют подтверждения email через локальный Mailpit. Старые аккаунты сохраняют вход, но не помечаются подтверждёнными автоматически. [Подтверждение email: архитектура и полная Windows-инструкция](../docs/email-verification.md).

Добавлено восстановление пароля `/forgot-password` → `/reset-password`, общий Mailpit и отзыв старых access/refresh. [Контракт recovery](../docs/account-recovery.md), [последний отчёт](../docs/release-report.md).

Актуальные документы: [отчёт MVP](docs/MVP_REPORT.md), [план](docs/IMPLEMENTATION_PLAN.md), [wishlists](../docs/wishlists.md), [buyer](../docs/buyer-shopping.md), [PVM](../docs/product-variant-management.md), [Media](../docs/media-storage.md). INTEGRATION_REPORT описывает предыдущий этап.

## Кнопки и шапка витрины

Интерактивная типографика использует общие CSS-токены: действия — 700, навигация и вкладки — 650, селекторы и вторичные действия — 600 в существующем вариативном Manrope. Убраны уменьшающие подписи адаптивные переопределения. Обычный текст сохраняет вес 500. [Полный аудит разделов, размеры, проверки и ограничения](docs/INTERACTIVE_TYPOGRAPHY.md).

На `/brands/:slug` компонент `HeaderWordmark` показывает текущий `brand.logo_url` через существующий `useImage` и Media API. Логотип вписывается с сохранением пропорций в 160×44 px, на экранах до 620 px — в 120×36 px. Альтернативный текст — название бренда; отсутствие файла, отказ доставки или ошибка декодирования дают текстовую замену. Ссылка по-прежнему ведёт на `/`. `useBrandHeader` регистрирует данные только для текущего пути через контекст `Outlet`; смена маршрута сразу скрывает предыдущий бренд. Остальные страницы используют SREZ.

В Studio внешний заголовок остаётся SREZ, а шапка внутри изолированного предпросмотра использует тот же компонент с авторизованной доставкой изображения. Ссылки внутри предпросмотра по-прежнему не выполняют переходы. Логотип управляется в настройках бренда; отдельного блока логотипа в конструкторе нет. [Совместимость сохранённых страниц](../docs/brand-page-compositions.md#логотип-в-шапке-витрины).

`npx playwright test e2e/storefront-branding.spec.ts` проверяет шапку на 1440/390 px, пропорции широких/квадратных/высоких логотипов, отсутствие и ошибки файла, клиентские переходы, Studio и чтение V1–V3 с редакционными фото. Сценарии используют управляемые ответы HTTP и не записывают данные в backend. Для запуска нужен локальный frontend на тестовом origin из `playwright.config.ts` и установленный Chrome.

## macOS

[Инструкция для Apple Silicon](../docs/macos-development.md#react-через-vite) описывает запуск локального Vite с Docker API на том же origin, что и Docker frontend.

## Windows PowerShell: запуск

Рабочая папка: `C:\dev\SREZ`. Исходная `team-repository` и её БД не используются для запуска или миграций новой копии.

1. Запустите Docker Desktop, дождитесь Linux Engine. Проверьте ПО:

```powershell
Set-Location 'C:\dev\SREZ'
docker version
docker compose version
node --version
```

Нужен Node 24 либо 22.12+ для подготовки конфигурации и разработки. Go на Windows для запуска контейнеров не требуется. Для браузерных тестов нужен установленный Google Chrome.

2. Подготовьте `.env` без перезаписи существующего:

```powershell
node .\frontend\scripts\prepare-local.mjs
```

Если файла нет, скрипт создаст его из `.env.example`. Он заполнит только отсутствующие/пустые JWT, CSRF, wishlist и S3 ключи, сохранит существующие пароль БД и значения, не напечатает секреты. Некорректные существующие ключи вызовут отказ до перезапуска. JWT/CSRF/wishlist ключи должны отличаться. **Сохраните `.env` приватно вместе с backup БД; не регенерируйте ключи после каждого запуска.** POSTGRES_PASSWORD должен соответствовать уже существующему volume; смена env не меняет пароль PostgreSQL.

3. Обычный запуск всего приложения:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-integration.ps1
```

Скрипт собирает и запускает PostgreSQL, SeaweedFS, Go и frontend в Docker, затем проверяет HTTP. Интерфейс больше не зависит от фонового процесса Vite. При первом старте загрузка образов/сборка занимают время. Существующие volumes сохраняются. `npm ci` на хосте для этого способа не нужен.

Либо запустите компоненты последовательно для диагностики:

```powershell
docker compose -p srez-integration -f compose.integration.yaml up -d --wait db
docker compose -p srez-integration -f compose.integration.yaml up -d --wait seaweedfs
docker compose -p srez-integration -f compose.integration.yaml up -d --build app
docker compose -p srez-integration -f compose.integration.yaml up -d --no-deps --build frontend
```

Новые аддитивные миграции 012–013 проверены в изолированной БД до применения здесь. Backend применяет миграции при старте. Не изменяйте ранее применённые SQL и не удаляйте записи истории миграций.

| Компонент       | Рабочая копия         | QA приложение         | Go integration tests |
| --------------- | --------------------- | --------------------- | -------------------- |
| Frontend/Origin | http://localhost:5174 | http://localhost:5175 | —                    |
| Go API          | 127.0.0.1:8081        | 127.0.0.1:8082        | тестовый HTTP server |
| PostgreSQL      | 127.0.0.1:15432       | 127.0.0.1:15433       | 127.0.0.1:15434      |
| Seaweed S3      | 127.0.0.1:18333       | 127.0.0.1:18334       | 127.0.0.1:18335      |
| Compose project | srez-integration      | srez-mvp-qa           | srez-mvp-tests       |

Все volumes разделены по проектам. Не используйте стандартный compose.yaml поверх старой БД на 5432. Корневой исторический DATABASE_URL может указывать на старую БД: **не запускайте нативный Go, просто загрузив этот .env**. Compose передаёт собственный адрес `db:5432` внутри правильного проекта.

4. Проверьте API и страницы:

```powershell
Invoke-RestMethod http://127.0.0.1:8081/health
Invoke-RestMethod http://localhost:5174/api/products
Invoke-WebRequest -UseBasicParsing http://localhost:5174
Start-Process 'http://localhost:5174'
```

Используйте именно **localhost:5174**, а не 127.0.0.1:5174: `Origin` и host cookie должны совпадать. Старый проект использует 127.0.0.1:5173. Разные порты одного host не изолируют cookie; не входите в рабочее и QA приложения из одного обычного browser profile одновременно. Автотесты используют отдельные contexts.

5. Создайте собственный тестовый аккаунт через `/register`, подтвердите email через Mailpit и войдите. В студии создайте бренд → описание → логотип → публикация; затем товар → описание/материалы/цена → цвет → фото → размер/остаток → публикация. После этого он появится в каталоге. Это лучший способ получить полноценные managed media и проверить owner/editor. Пароль не короче 12 символов. Отдельный старый dev seed допустим только явно в QA:

```powershell
docker compose -p srez-mvp-qa -f compose.qa.yaml run --rm --no-deps -e SREZ_DEV_SEED=true --entrypoint /seed app
```

Seed не меняет существующий slug; его старые демонстрационные HTTPS-изображения зависят от внешних сайтов. Приложение само seed не запускает.

6. Остановка с сохранением данных:

```powershell
docker compose -p srez-integration -f compose.integration.yaml stop
# Повторный запуск без пересборки:
docker compose -p srez-integration -f compose.integration.yaml start
```

Не удаляйте volumes и не используйте down -v. При обновлении исходников повторите start-integration.ps1.

## Регистрация и Mailpit

После регистрации откроется `/verify-email/pending`. Откройте **http://127.0.0.1:8025**, найдите письмо «Подтвердите email — SREZ» по своему адресу и перейдите по кнопке. На странице `/verify-email` отдельно нажмите «Подтвердить почту», затем войдите с паролем. Открытие ссылки само по себе не активирует аккаунт; токен сразу исчезает из адресной строки.

Письма перехватываются локально, настоящий почтовый ящик не используется. До подтверждения новые аккаунты не получают JWT или refresh-cookie. При ошибке SMTP аккаунт остаётся: повторите отправку после60секунд. Лимит5 писем в час; resend не раскрывает существование аккаунта и не обещает доставку.

Mailpit запускается тем же start-integration.ps1, использует отдельный новый volume; SMTP1025/UI8025 доступны только на127.0.0.1. В QA mailbox другой:18025/11025. Значения .env сохраняются; для Mailpit SMTP пароль не нужен. Подробные команды/диагностика — в [email-verification.md](../docs/email-verification.md).

## Разработка через Vite

Контейнер frontend и Vite используют один порт, поэтому остановите только контейнер frontend. API/данные продолжают работать:

```powershell
docker compose -p srez-integration -f compose.integration.yaml stop frontend
Set-Location .\frontend
npm.cmd ci
npm.cmd run dev
```

Откройте localhost:5174. Терминал Vite нужно оставлять открытым; Ctrl+C завершает dev server. После разработки вернитесь в корень и выполните start-integration.ps1 для стабильного Docker-варианта. Не выполняйте npm ci одновременно с работающими Node-тестами/Vite; на Windows открытый журнал или модуль может блокировать переустановку. `npm.cmd` обходит блокировку npm.ps1 без смены системной политики.

Vite и nginx отправляют только `/api/*` в Go с удалением `/api`; `/media/*` проксируют без изменения пути. `/products` и `/brands` остаются страницами React. Origin не подменяется. Для Vite QA можно явно задать SREZ_API_TARGET=http://127.0.0.1:8082 и SREZ_FRONTEND_PORT=5175; соответствующий frontend-контейнер предварительно остановите.

## Страницы и сценарии

| Страница                                                                              | Функции                                                                     |
| ------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| `/`, `/catalog`, `/products/:productId`                                               | Реальный ассортимент, фильтры/sort/pagination, галерея, цвет/размер/наличие |
| `/brands`, `/brands/:slug`                                                            | Публичные бренды, контакты, товары                                          |
| `/register`, `/login`, `/account`                                                     | Вход, восстановление сессии, display_name, managed avatar, logout           |
| `/favorites`, `/cart`                                                                 | Независимое избранное; реальные variant ID, количество, серверные суммы     |
| `/orders`, `/orders/:orderId`                                                         | Неоплаченный 15-минутный резерв, история, срок, отмена, истечение           |
| `/wishlists`, `/wishlists/new`, `/wishlists/:id`                                      | Несколько подборок, вариант, порядок, share/revoke/rotate                   |
| `/w/:token`                                                                           | Гостевая подборка по действующей ссылке                                     |
| `/studio`, `/studio/brands/new`, `/studio/invitations`                                | Мои бренды, создание, принятие/отказ приглашений                            |
| `/studio/brands/:brandId`                                                             | Товары; `?tab=settings` оформление, `?tab=team` команда                     |
| `/studio/brands/:brandId/products/new`, `/studio/brands/:brandId/products/:productId` | Товар, цвета, фото, варианты, SKU, остатки, publish/hide                    |

Существующие маршруты `/studio` сохранены вместо дублирующего `/dashboard`. Owner управляет оформлением и командой; editor — ассортиментом в разрешённых backend пределах. Скрытые кнопки не заменяют серверную проверку прав. Изменение структуры опубликованного товара: явно скрыть → изменить → снова опубликовать. Формы предупреждают перед потерей несохранённых изменений; фотографии показывают локальный preview перед явной загрузкой.

## Auth, Media и резерв

Access JWT находится только в памяти JS; refresh-cookie HttpOnly. Refresh rotation, single-flight, Web Locks, 409/replay и абсолютный срок сессии сохранены. JWT/CSRF/Origin не отключаются. PATCH profile отправляет только display_name; logo_url/banner_url/avatar_url клиент больше не пишет.

Изображения: POST `/media/uploads` → PUT сырого File по `content_url` → managed media_id/URL. JPEG/PNG/WebP до 5 MiB, проверка реального содержимого на Go. Private media читается через Bearer Blob и освобождаемые object URLs. Public managed `/media/assets/:id/content` и legacy HTTPS поддерживаются. Прямых запросов браузера к S3 нет.

Корзина не резервирует товар. POST `/orders` принимает `{}` и создаёт **неоплаченный резерв на 15 минут**, без списания денег. Срок берётся из cancel_before; сервер меняет статусы и остатки. При неопределённом результате POST автоматического повтора нет: сначала история. Backend пока не имеет idempotency key.

## Автоматические проверки

```powershell
Set-Location 'C:\dev\SREZ\frontend'
npm.cmd ci
npm.cmd run typecheck
npm.cmd run lint
npm.cmd test
npm.cmd run build
npm.cmd run format:check
Set-Location ..
# Отдельные volumes, без изменений рабочей БД:
docker compose -p srez-mvp-tests -f compose.tests.yaml up -d --wait
docker build --target build -t srez-mvp-go-check:local .
node .\scripts\qa-go.mjs full
node .\scripts\qa-go.mjs vet
# QA application: отдельные БД/хранилище, реальные API:
docker compose -p srez-mvp-qa -f compose.qa.yaml up -d --build
node .\scripts\qa-browser.mjs all
```

Browser runner создаёт собственные тестовые аккаунты/товары, не удаляет пользовательские записи. Сценарий expiry изменяет срок только своего тестового заказа после проверки Docker label QA. Некоторые error/timeout UI-тесты намеренно подменяют HTTP-ответ, чтобы воспроизвести редкие ошибки; основные end-to-end сценарии работают с реальным API. В production исходниках моков нет. `qa-media-copy.mjs` — разовый операторский инструмент переноса отсутствующих объектов из integration в восстановленную QA-БД: источник только Head/Get, целевые файлы не перезаписываются, контроль SHA-256; не нужен для обычного запуска.

Логи тестов `mvp-*.log` локальные, capability URLs/JWT редактируются. Скриншоты `docs/previews/mvp-*.png` содержат только тестовые записи. Точные результаты и проверка сохранности — [MVP_REPORT](docs/MVP_REPORT.md).

## Диагностика

```powershell
docker compose -p srez-integration -f compose.integration.yaml ps
docker compose -p srez-integration -f compose.integration.yaml logs --tail 80 app
docker compose -p srez-integration -f compose.integration.yaml logs --tail 30 frontend
Get-NetTCPConnection -LocalPort 5174,8081,15432,18333 -State Listen -ErrorAction SilentlyContinue
```

- Docker unavailable: запустите Desktop/Linux Engine и проверьте docker version. Reset to factory defaults не нужен.
- Порт занят: определите владельца через OwningProcess/Get-Process. Не завершайте неизвестный процесс; остановите только известный сервер этой копии. Старые 5173/8080/5432 не занимайте.
- Миграции: читайте app logs; проверьте правильный Compose project, пароль и доступность БД. Не меняйте старый SQL/volume для обхода checksum.
- 400: проверьте поля и типы, slug, цену в копейках, variant_id, полноту публикации. Диагностика metadata выводит только безопасное имя поля.
- 401: повторный вход после исчерпания refresh; проверьте часы Windows/Docker. В storage браузера токенов нет по замыслу.
- 403: права owner/editor, точный Origin и CSRF cookie/header. Не отключайте защиту.
- 404: отсутствующий, чужой, скрытый объект либо отозванная ссылка; приватные детали не раскрываются.
- 409: повтор товара, изменившийся остаток/цена/состав, конкурентная операция; перечитайте серверное состояние.
- 500/502/503: API/storage недоступен или неверный DTO; проверьте health, app logs и БД/Seaweed. Vite ECONNREFUSED обычно означает незапущенный API или неверный target.
- Загрузка фото: до 5 MiB, JPEG/PNG/WebP; нажмите «Загрузить фотографию» после выбора. Проверьте оба запроса POST и PUT и Go logs. S3 list без подписи должен давать 403. Не делайте bucket публичным.
- Пустой каталог: опубликован ли бренд и товар, заполнены ли все цвета/фото/размеры, не остались ли фильтры.
- URL не журналируются ради секретов shared links. Proxy показывает статус/объём/время; подробную серверную ошибку ищите в безопасной классификации Go, не включайте логирование Authorization/cookie/полных URLs.

Ограничения локального MVP: нет реальной оплаты/доставки и восстановления пароля, production HTTPS/операционного мониторинга; локальные shared links работают только на этом ПК. Публичный запуск требует отдельного AppSec-ревью и конфигурации.
