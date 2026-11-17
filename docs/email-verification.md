# Подтверждение email и локальный Mailpit

Актуально на17.11.2026. Изменения выполнены в `integration-repository`, без commit/push/PR. Авторизация, Argon2id, JWT, refresh rotation, CSRF и дизайн SREZ сохранены. Подтверждение встроено в существующий Auth, отдельного параллельного Auth-сервиса нет.

## Сценарий

1. POST /auth/register создаёт аккаунт с обязательным подтверждением и одноразовый challenge в одной транзакции.
2. После commit MailSender отправляет письмо через SMTP; SQL-транзакция во время SMTP не открыта.
3. Новому пользователю не выдаются JWT/cookie и не создаётся сессия.
4. Пользователь открывает письмо в Mailpit и переходит по ссылке.
5. Страница очищает адресную строку и ждёт нажатия «Подтвердить почту».
6. POST /auth/verify-email атомарно погашает токен и записывает реальную дату подтверждения.
7. Пользователь входит обычным способом с паролем. Подтверждение не создаёт сессию автоматически.

Верификация доказывает контроль адреса по полученному письму. Она не заменяет пароль, JWT или refresh. Verification token не является credential для API.

## Данные и совместимость

Новая аддитивная миграция **014_email_verification.sql**. SQL001–013 не меняется.

В users добавлены:

- email_verified_at: nullable timestamptz, до фактического подтверждения NULL.
- email_verification_required: bool NOT NULL DEFAULT FALSE.

Существующие аккаунты и сессии не блокируются и не получают выдуманную дату подтверждения. Новая регистрация явно устанавливает required=true. В профиле legacy-пользователя отображается «Email не подтверждён» и пояснение о сохранённом входе; он может подтвердить адрес добровольно.

В email_verification_tokens хранятся id, user_id, SHA-256 token_hash32байта, created_at, expires_at, used_at/revoked_at. Есть FK, уникальный hash, частичная уникальность одного непогашенного токена на пользователя, индексы по пользователю/времени и expiry, проверки срока ровно30минут и целостности дат. Plaintext-токена в PostgreSQL нет.

В email_verification_ip_limits сохраняются purpose-separated HMAC IP, счётчики и начала минутного/часового окон. Ключ HMAC выводится по отдельному назначению из стабильного CSRF key. Сырые IP не сохраняются. Клиентский IP берётся из Gin ClientIP с существующим trusted-proxy allowlist, а не из произвольного X-Forwarded-For.

Пользовательские поля owner/user_id/email_verified/required не принимаются через JSON регистрации, проверки или профиля. PATCH profile по-прежнему изменяет только разрешённые поля.

## Токен и транзакции

Каждый токен —32байта crypto/rand, canonical Base64URL без padding (43символа),256бит энтропии. Это новый случайный секрет, а не JWT/refresh. Для поиска сохраняется только SHA-256. Срок30минут считается по времени PostgreSQL, не по часам браузера.

Проверка сначала определяет пользователя по hash, затем блокирует строку пользователя. В одной транзакции проверяются статус, срок и непогашенность, обновляются used_at и email_verified_at. Повторный и конкурентный запросы не получают второй успех. Ошибка обновления пользователя откатывает погашение токена.

Resend берёт ту же user-row lock, проверяет состояние/квоты, отзывает предыдущий токен и создаёт новый. Поэтому конкурентные verify/resend сериализованы и старый адрес после перевыпуска не работает. У подтверждённого аккаунта новые письма не создаются.

Login проверяет требование email только после корректного пароля. Для неверного пароля/неизвестного пользователя сохраняется прежний401. Session INSERT и refresh rotation дополнительно проверяют eligibility в БД: pending-аккаунт не получает новые access/refresh credentials через другой путь. Logout остаётся разрешённым для отзыва существующей cookie.

## SMTP и обработка отказов

`internal/usecase/email_verification.go` содержит интерфейс MailSender и сценарии. `internal/storage/email_verification.go` — SQL/блокировки/квоты. `internal/mailer/` — SMTP и multipart/alternative письмо с текстовой и HTML-версией. SMTP используется через стандартную библиотеку Go; JWT/хеширование не переписывались.

Тема: **Подтвердите email — SREZ**. Чёрно-белое письмо содержит кнопку, объяснение срока и просьбу игнорировать неожиданную регистрацию; внешних изображений/аналитики нет.

Ссылка строится только из доверенного SREZ_FRONTEND_BASE_URL, не из Host/Origin запроса. В текущей рабочей копии это `http://localhost:5174/verify-email#token=...`. Исходный проект на127.0.0.1:5173 сохранён отдельно; подменять host/port текущего приложения не нужно.

SMTP имеет ограничение подключения3секунды и всей операции5секунд, поддерживает отмену context. Не более8 одновременных отправок. Ошибки не включают адрес, письмо, URL или секреты в application logs. Успех означает, что SMTP-сервер принял DATA, а не гарантию доставки провайдером во «Входящие».

Регистрация отправляет синхронно после commit: при ошибке SMTP **аккаунт сохраняется**, ответ201 содержит email_delivery_status=unavailable; UI прямо сообщает об ошибке. Повторная регистрация не нужна.

Resend возвращает одинаковое202 для неизвестного, подтверждённого, подавленного account quota и подходящего аккаунта. Текст означает **«запрос принят, отправка будет предпринята при необходимости»**, а не «письмо отправлено». SMTP выполняется асинхронно в памяти, чтобы ответ не зависел от сетевого времени конкретного аккаунта. Plaintext durable outbox отсутствует. При падении процесса письмо может не уйти; аккаунт остаётся, восстановление — новый resend после cooldown. Это сознательная граница локального MVP.

## API

Пути Go ниже; браузер использует /api prefix через существующий proxy. Все POST требуют точный Origin, допустимый Sec-Fetch-Site и application/json; неизвестные JSON-поля отклоняются. Cookie-auth refresh/logout сохраняют прежнюю CSRF-защиту. Verify/resend не используют cookie для авторизации и не выдают credentials.

| Метод/маршрут                  | JSON                                                | Результат                                                                                                                                          |
| ------------------------------ | --------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| POST /auth/register            | email,username,display_name,password (прежние поля) | 201: прежний public user + email_verification_required:true, email_delivery_status:sent/unavailable, resend_after_seconds:60; без JWT/cookie/token |
| POST /auth/login               | identifier,password                                 | 403 email verification required только после правильного пароля pending-аккаунта; иначе прежний auth flow                                          |
| POST /auth/verify-email        | `{"token":"..."}`                                   | 200 `{"message":"email verified"}`; без сессии                                                                                                     |
| POST /auth/resend-verification | `{"identifier":"email или username"}`               | 202 с одинаковым generic message и resend_after_seconds:60                                                                                         |
| GET/PATCH /profile             | Bearer; PATCH whitelist прежний                     | Добавлены email_verified_at:null/RFC3339 и email_verification_required:boolean                                                                     |

Неверный/истёкший/отозванный/использованный токен:400 с одинаковым безопасным сообщением. Нет email/ID/SQL/hashes в ошибке. Неверный Origin403, Content-Type415, лимит IP429 с Retry-After60, недоступная конфигурация503. Конфликт регистрации409 остаётся прежним общим account unavailable.

## Лимиты и защита от массовых писем

- Одному аккаунту — не чаще одного выпуска в60секунд, включая первую регистрацию.
- Не более5 выпусков за скользящий последний час, включая неуспешные отправки и отозванные токены.
- По IP —10 запросов в минутное и50 в часовое окно в обычной рабочей конфигурации. Unknown/verified resend тоже расходует IP quota.
- Account suppression не раскрывается отдельно:202/тело одинаковы. Общий IP limit даёт429 независимо от существования адреса.
- Значимые квоты находятся в PostgreSQL и сохраняются после перезапуска. Обычный in-memory auth limiter остаётся дополнительной защитой.
- QA явно использует200/1000 по IP для серии тестовых аккаунтов; account5/hour и60seconds не отключены. Такие QA значения не предназначены для публичного сервиса.

Минутный/часовой IP limiter использует фиксированные окна; account cap — SQL rolling hour. Ротация CSRF key меняет HMAC IP namespace (как и существующие сессии); обычный restart со стабильным .env квоты не сбрасывает.

## Frontend и утечки

/verify-email/pending показывает маскированный адрес, факт отправки либо честный SMTP failure, resend/cooldown и ссылку ко входу. При reload не восстанавливает идентификатор из persistent storage: можно ввести его снова.

/verify-email читает fragment (дополнительно поддержан query для совместимости) до создания router, немедленно удаляет секрет через history.replaceState, держит его только в памяти страницы. StrictMode учтён. GET, preview и preload не подтверждают адрес. POST выполняется только по кнопке. После reload без токена нужно снова открыть письмо; после успеха токен очищается.

Verification token не попадает в localStorage/sessionStorage/cookie, router state, ответы API или analytics. JWT остаётся только в памяти; refresh HttpOnly. Верификация не вызывает login автоматически. Profile показывает реальные date/required, а не выводит статус из наличия сессии.

Ответы имеют no-store/no-referrer; документ /verify-email имеет CSP с ресурсами только своего origin, без сторонних images/fonts/scripts/connect. Страница использует отдельную минимальную рамку без внешних аватаров. Go и nginx не журналируют URL/тела/Authorization. Vite и browser runner редактируют token= в диагностике. Скриншоты сохраняются без адресной строки и секретов; traces/video выключены.

Mailpit хранит полученное письмо, включая ссылку: это назначение тестового почтового ящика. Его volume, UI и backup нужно хранить приватно, как .env и дамп БД. Не включайте публичный SMTP relay.

## Конфигурация

| Переменная                                            | Назначение                                                                |
| ----------------------------------------------------- | ------------------------------------------------------------------------- |
| SREZ_SMTP_HOST                                        | В Docker mailpit, при Go/GoLand на Windows127.0.0.1                       |
| SREZ_SMTP_PORT                                        | 1025                                                                      |
| SREZ_MAIL_FROM                                        | SREZ <no-reply@srez.local>                                                |
| SREZ_FRONTEND_BASE_URL                                | Доверенный frontend origin; integration localhost:5174, QA localhost:5175 |
| SREZ_SMTP_TLS                                         | none для локального Mailpit; starttls либо tls для реального провайдера   |
| SREZ_SMTP_USERNAME / SREZ_SMTP_PASSWORD               | Необязательные SMTP credentials; только server env                        |
| SREZ_EMAIL_IP_MINUTE_LIMIT / SREZ_EMAIL_IP_HOUR_LIMIT | По умолчанию10/50; QA200/1000                                             |

Plaintext SMTP разрешён только для loopback/mailpit и без AUTH. STARTTLS обязателен в режиме starttls, TLS-проверка сертификата не отключается, minimum TLS1.2. Frontend URL не принимает credentials/query/fragment/path; HTTP допустим только для localHTTP и loopback. Неверная конфигурация останавливает startup до миграций.

Текущий официальный образ: **axllent/mailpit:v1.34.0**, закреплён digest sha256:23a2ef85bc50853ab3093645d1d64679afd8887e89da5e48a88940c172591a4e. [Официальный Docker образ](https://mailpit.axllent.org/docs/install/docker/), [релиз](https://github.com/axllent/mailpit/releases/tag/v1.34.0), [параметры](https://mailpit.axllent.org/docs/configuration/runtime-options/).

Mailpit не пересылает письма настоящим адресатам. Он принимает SMTP и хранит сообщения в собственном новом mailpit_data volume. Оба host-порта привязаны только к127.0.0.1. Automatic version check выключен, auto-deletion писем выключен; старые volumes не удаляются.

| Окружение                | Frontend       | SMTP host-порт  | Mailpit UI             |
| ------------------------ | -------------- | --------------- | ---------------------- |
| Рабочее srez-integration | localhost:5174 | 127.0.0.1:1025  | http://127.0.0.1:8025  |
| Browser QA srez-mvp-qa   | localhost:5175 | 127.0.0.1:11025 | http://127.0.0.1:18025 |
| Go tests srez-mvp-tests  | —              | 127.0.0.1:12025 | http://127.0.0.1:28025 |

Внутри каждого Docker network SMTP всегда mailpit:1025. Это три разных mailbox volumes. Стандартный compose.yaml тоже содержит Mailpit1025/8025, поэтому его не следует одновременно запускать с integration на тех же портах.

## Windows PowerShell: полный запуск и проверка

1. Запустите Docker Desktop с Linux Engine. В PowerShell:

```powershell
Set-Location 'C:\dev\SREZ'
docker version
docker compose version
node --version
```

2. Сохраните имеющийся .env; не копируйте поверх него .env.example. Скрипт подготавливает только отсутствующие ключи:

```powershell
node .\frontend\scripts\prepare-local.mjs
```

Новые SMTP параметры добавлены в .env.example; текущий .env дополнен без изменения прежних значений. Compose явно задаёт mailpit и правильный frontend origin. Реальный SMTP пароль не требуется.

3. Запустите Mailpit и инфраструктуру:

```powershell
docker compose -p srez-integration -f compose.integration.yaml up -d --wait mailpit
docker compose -p srez-integration -f compose.integration.yaml up -d --wait db seaweedfs
```

4. Соберите Go и React:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-integration.ps1
Invoke-RestMethod http://127.0.0.1:8081/health
Invoke-RestMethod http://localhost:5174/api/health
```

Скрипт запускает весь Compose, включая Mailpit. Frontend собирается Vite и выдаётся Docker nginx; Go/Node dev server на хосте для обычного запуска не нужен. Node используется скриптом конфигурации. Backend применяет014 автоматически; сначала она проверена в изолированной БД.

5. Откройте http://localhost:5174/register и зарегистрируйте новый email/username. Откроется pending page. Войти с этим паролем до подтверждения нельзя.

6. Откройте Mailpit:

```powershell
Start-Process 'http://127.0.0.1:8025'
```

7. Найдите письмо по своему email (поиск `to:адрес`) и теме «Подтвердите email — SREZ». Откройте HTML-письмо, нажмите «Подтвердить почту».

8. На странице SREZ нажмите кнопку подтверждения. Проверьте, что токен исчез из адресной строки. Появится сообщение об успехе; простое открытие письма/страницы не расходует ссылку.

9. Перейдите ко входу, введите email либо username и пароль. В /account появится «Email подтверждён». Reload и refresh продолжают работать как прежде.

10. При ошибке отправки проверьте Mailpit и повторите запрос после60секунд. Аккаунт уже создан, повторная регистрация не нужна. Старая ссылка после resend больше не действует. Проверьте последний message и5/hour cap.

11. Остановка без удаления данных:

```powershell
docker compose -p srez-integration -f compose.integration.yaml stop
# Повторный запуск:
docker compose -p srez-integration -f compose.integration.yaml start
```

Не используйте down -v, factory reset или удаление volume ради SMTP. Диагностика:

```powershell
docker compose -p srez-integration -f compose.integration.yaml ps
docker compose -p srez-integration -f compose.integration.yaml logs --tail 60 app mailpit
Get-NetTCPConnection -LocalPort 1025,8025 -State Listen -ErrorAction SilentlyContinue
```

Не завершайте неизвестный процесс при занятом порте. В текущем проекте порты проверены перед запуском. Для временной проверки используйте QA с отдельными11025/18025, не меняйте пользовательские volumes.

### GoLand/Go непосредственно в Windows

Mailpit остаётся в Docker, SMTP=127.0.0.1:1025. В Run Configuration задайте SREZ_PUBLIC_ORIGIN и SREZ_FRONTEND_BASE_URL=http://localhost:5174, SREZ_AUTH_LOCAL_HTTP=true, SREZ_SMTP_TLS=none. JWT/CSRF/wishlist/S3 значения берите из собственного .env, не из примеров. Укажите DATABASE_URL на **15432**, а не на старую исходную БД5432. Сначала остановите только integration app, чтобы не занять тот же HTTP-порт; не загружайте исторический DATABASE_URL из .env без исправления адреса для Run Configuration.

## Тесты и дальнейший SMTP-провайдер

```powershell
# Из корня проекта, отдельные test DB/S3/Mailpit:
docker compose -p srez-mvp-tests -f compose.tests.yaml up -d --wait
docker build --target build -t srez-mvp-go-check:local .
node .\scripts\qa-go.mjs email
node .\scripts\qa-go.mjs full
node .\scripts\qa-go.mjs vet
# Browser QA:
docker compose -p srez-mvp-qa -f compose.qa.yaml up -d --build
node .\scripts\qa-browser.mjs email-verification
node .\scripts\qa-browser.mjs all
# Frontend:
npm.cmd --prefix .\frontend run typecheck
npm.cmd --prefix .\frontend run lint
npm.cmd --prefix .\frontend test
npm.cmd --prefix .\frontend run build
```

Тестовые аккаунты проходят реальный verify через in-memory MailSender в PostgreSQL/HTTP tests либо через отдельный настоящий Mailpit в browser tests. Публичного test bypass/verification-off флага нет. SMTP outage/restart и SQL expiry воздействуют только на созданные тестом записи и QA контейнер с проверенным label. Пользовательские строки и mailboxes не очищаются. Результаты — в frontend/docs/EMAIL_VERIFICATION_REPORT.md.

Для настоящего провайдера замените server SMTP host/port, sender и TLS mode, добавьте credentials через приватную конфигурацию, задайте HTTPS frontend URL и согласованный PUBLIC_ORIGIN. Понадобятся подтверждённый sender/domain, SPF/DKIM/DMARC и независимая эксплуатационная проверка. Существующий .env и ключи не регенерируйте. Изменение MailSender не требует переписывания Auth/token storage/frontend-проверки. Реальный внешний SMTP в этой задаче не использовался и не проверялся; публикации в интернет не было.
