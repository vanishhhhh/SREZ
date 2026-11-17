# Восстановление доступа

Актуальный контракт Go/React. Подтверждение email переиспользуется, отдельного Auth или SMTP-сервиса нет. См. [общую карту API](api-reference.md) и [подтверждение email/Mailpit](email-verification.md).

## Пользовательский сценарий

1. На `/login` нажмите «Забыли пароль?».
2. На `/forgot-password` введите email.
3. Если адрес ранее подтверждён и лимиты позволяют, backend попробует отправить письмо «Сброс пароля — SREZ».
4. Откройте [локальный Mailpit](http://127.0.0.1:8025), найдите письмо по адресу и нажмите «Восстановить пароль».
5. `/reset-password` немедленно удаляет fragment из URL. Открытие страницы не расходует токен.
6. Введите новый пароль дважды и отправьте форму. Минимум 12 Unicode-символов, максимум 1024 UTF-8 байта; пробелы сохраняются.
7. После успеха войдите заново. Сброс не создаёт JWT/cookie/сессию.

**Восстановление доступно только для ранее подтверждённого email.** Legacy-аккаунты сохраняют вход без подтверждения, но сначала должны добровольно подтвердить почту. В публичном ответе forgot нельзя узнать состояние адреса. Reset не подтверждает email и не меняет его; username/email/user_id/password_hash нельзя передать вместо токена.

## API

Все auth POST требуют точный `Origin: SREZ_PUBLIC_ORIGIN`, `Content-Type: application/json`; неизвестные JSON-поля отклоняются. `Sec-Fetch-Site`, если присутствует, должен быть `same-origin`. В браузере пути имеют префикс `/api`; proxy удаляет его. Bearer не нужен.

### POST /auth/forgot-password

```json
{ "email": "reader@example.test" }
```

Адрес нормализуется `lower(trim)`, проверяются ASCII, формат и длина до 254 байт. Один и тот же ответ **202** возвращается для подходящего, неизвестного, неподтверждённого и подавленного account quota адреса:

```json
{
  "message": "If this address is eligible for recovery, a message will be attempted. Check your inbox or retry later.",
  "resend_after_seconds": 60
}
```

Это принятие запроса, **не обещание отправки или доставки**. Account suppression не раскрывается. Ошибки: 400 неверный JSON/email, 403 Origin/source, 415 Content-Type, 429 общий IP/auth limiter с Retry-After, 503 отсутствующая recovery конфигурация, 500 внутренняя ошибка без SQL/адресов/секретов.

### POST /auth/reset-password

```json
{ "token": "одноразовый-токен-из-письма", "password": "новый длинный пароль" }
```

Успех **200**, без сессии и Set-Cookie:

```json
{ "message": "password reset; sign in again" }
```

Неверный, истёкший, отозванный, использованный или токен другого назначения: одинаковый **400** `{"error":"invalid or expired reset link"}`. Невалидный пароль/лишние поля — 400; Origin/source — 403; Content-Type — 415; auth/IP limiter — 429; перегрузка Argon2 — 503. Ошибка не возвращает email, user_id, токен, хеш или SQL.

Клиентское повторение пароля служит проверкой UX; backend принимает только один `password` и независимо проверяет правила. Повтор POST после неопределённого сетевого ответа может дать 400, если первый запрос уже завершился. Тогда попробуйте войти новым паролем, а при необходимости запросите новую ссылку.

## Схема и токен

Миграция **015_password_recovery.sql** — только новые таблицы:

- `password_reset_tokens`: id, user_id/FK, token_hash BYTEA32 unique, created_at, expires_at, used_at, revoked_at; срок ровно 30 минут, частичный unique одного непогашенного токена, индексы user/time и expiry.
- `password_reset_ip_limits`: HMAC IP и постоянные минутные/часовые счётчики.

Токен — 32 байта `crypto/rand`, canonical Base64URL без padding (43 символа). В PostgreSQL только SHA-256, plaintext существует временно в памяти и в доставленном тестовом письме. Reset, verification, wishlist capability и refresh разделены таблицами и назначениями. Один тип токена не работает как другой.

Время определяет PostgreSQL. Новый выпуск отзывает старый. Одноразовый compare-and-update под блокировкой пользователя обеспечивает ровно один успешный reset при конкурентных запросах.

## Транзакция и отзыв

Argon2id вычисляется **до SQL-транзакции**, с тем же ограниченным пулом, что login/register. Затем:

1. Найти владельца по hash токена.
2. Заблокировать пользователя, проверить подтверждённый адрес.
3. Атомарно погасить только действующий неиспользованный токен.
4. Обновить password_hash и updated_at.
5. Отозвать все активные refresh-сессии пользователя и другие reset-токены.
6. Commit. При любой ошибке весь набор откатывается.

Общий порядок блокировок: **user → token/session**. Refresh берёт user-lock до session-lock; reset и refresh не образуют взаимную блокировку. Создание сессии дополнительно сверяет ожидаемый password_hash под user-lock: вход, проверивший старый пароль до reset, не может создать сессию после него. Конкурентный rehash может потребовать повторного входа, но не обходит сброс.

### Уже выданные access JWT

Bearer middleware сначала проверяет HS256/iss/aud/exp/iat/sub/sid/jti, затем через Auth usecase проверяет активную серверную сессию и email eligibility. **После commit reset старые JWT отклоняются сразу при следующей проверке**, не ожидая 15 минут. Это также усиливает logout и обнаруженный refresh replay.

Один индексированный SELECT/EXISTS по session PK и user PK на аутентифицированный запрос, без кеша отзыва; lookup ограничен 2 секундами. При отказе БД — безопасный 503, доступ не разрешается. Операция, уже прошедшая middleware до reset, не отменяется задним числом; чувствительные операции сохраняют собственные SQL-проверки прав. JWT срок по-прежнему до 15 минут, refresh absolute expiry 7 дней, rotation/grace/CSRF сохранены.

## Ограничения и SMTP

По аккаунту — один выпуск в 60 секунд и максимум 5 за скользящий час, включая неудачные доставки/отозванные токены. По IP — по умолчанию 10/минуту и 50/час в постоянных фиксированных окнах. HMAC имеет отдельное назначение `srez-password-reset-ip-v1`, raw IP не сохраняется. Unknown/pending запросы тоже расходуют IP quota; restart со стабильным CSRF key её не сбрасывает. Дополнительно работает существующий общий auth limiter.

В QA IP budget повышен до 200/1000 для набора тестов, account quota не отключена. Подмена X-Forwarded-For не работает без разрешённого trusted proxy.

Используется общий `MailSender.SendPasswordReset`, SMTP config и multipart builder. Письмо чёрно-белое, text+HTML, без внешней аналитики; ссылка только из доверенного frontend base URL. Отправка после commit, в памяти, таймаут 5 секунд и максимум 8 одновременных reset доставок. Ответ forgot не зависит от SMTP round trip. Полная постоянновременность SQL не заявляется: state-specific response/delivery timing устранены, но нужен дополнительный внешний review перед публичным запуском.

Durable plaintext outbox отсутствует. При падении процесса/SMTP письмо может не уйти; аккаунт не теряется, нужно повторить после cooldown. Логи содержат только `recovery_delivery_failed`, без письма/URL/email/token. Mailpit намеренно сохраняет письмо со ссылкой в приватном локальном volume.

## Frontend и приватность

`AuthFrame` переиспользуется verification/recovery страницами. Fragment читается до инициализации router, удаляется history.replaceState, сохраняется только в памяти. GET/preload не вызывает reset. После reload без токена снова откройте письмо. Нет localStorage/sessionStorage/cookie токенов или сторонних ресурсов на sensitive страницах. Nginx отдаёт no-store/no-referrer и CSP ресурсов своего origin; Vite редактирует секреты в диагностике.

После успешного reset локальное auth-состояние очищается, вкладки получают logout notification. Ответ от более раннего reset не стирает более новый login. Backend reset не меняет cookie другой личности, случайно открывшей ссылку; целевые сессии отзываются по владельцу токена в SQL.

## Запуск и тесты

Обычный локальный запуск из корня: `powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-integration.ps1`. Сайт — [localhost:5174](http://localhost:5174), Mailpit — [127.0.0.1:8025](http://127.0.0.1:8025), SMTP 1025. Внутри Docker sender использует mailpit:1025; GoLand на Windows — 127.0.0.1:1025. Полные команды — в [README](../README.md).

```powershell
# Только отдельные test DB/S3/Mailpit:
docker compose -p srez-mvp-tests -f compose.tests.yaml up -d --wait
docker build --target build -t srez-mvp-go-check:local .
node .\scripts\qa-go.mjs full
node .\scripts\qa-go.mjs vet
# Отдельный browser QA:
docker compose -p srez-mvp-qa -f compose.qa.yaml up -d --build --wait
node .\scripts\qa-browser.mjs password-recovery
node .\scripts\qa-browser.mjs all
```

PostgreSQL/HTTP tests проверяют hash/TTL/purpose, pending eligibility, rollback, 12 конкурентных reset, refresh race, stale login, старый/новый пароль, все старые JWT/refresh, cooldown/hour/IP persistence, strict JSON и Origin. Browser tests проходят реальное Mailpit письмо, форму, mismatch, rotation/expiry, два конкурентных POST и login/reload. Пользовательские строки не удаляются, timestamps меняются только у собственных QA fixtures.

Для внешнего провайдера сохраняется тот же MailSender: HTTPS frontend, TLS/STARTTLS, server-only credentials и verified sender/domain. Внешняя доставка, password history и production инфраструктура не входят в текущую локальную проверку.
