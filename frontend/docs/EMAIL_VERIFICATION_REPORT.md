# SREZ — подтверждение email с Mailpit

> Исторический отчёт завершённого этапа. Текущее состояние и результаты: [release report](../../docs/release-report.md). Логи, backup и screenshots остаются локальными, их отсутствие в Git ожидаемо.

Дата: 17.11.2026, Europe/Moscow. Рабочая папка: `C:\dev\SREZ`.

## 1. Реализовано

Новая регистрация создаёт неподтверждённый аккаунт и одноразовый токен в SQL-транзакции, затем отправляет настоящее multipart text/HTML письмо через SMTP. До подтверждения login не выдаёт JWT и refresh-cookie; session creation и refresh дополнительно проверяют требование в БД. Правильный пароль проверяется до раскрытия необходимости подтверждения. Argon2id, JWT, CSRF, Origin и refresh rotation сохранены.

Токен: 32 случайных байта `crypto/rand`, Base64URL без padding, SHA-256 в PostgreSQL, срок 30 минут по серверным часам. Проверка атомарна; повторная, просроченная, отозванная и конкурентная попытки отклоняются. Повторная отправка отзывает прежнюю ссылку.

Добавлены страницы `/verify-email/pending` и `/verify-email`, реальный статус почты в профиле, переход после регистрации и подсказка при запрещённом входе. Ссылка очищается до создания router, токен остаётся только в памяти. Подтверждение выполняется явной кнопкой POST и не создаёт сессию. Дизайн SREZ и существующие компоненты сохранены.

Лимиты хранятся в PostgreSQL: 60 секунд между выпусками, 5 выпусков за скользящий час на аккаунт, по умолчанию 10 запросов в минуту / 50 в час на IP. IP хранится как HMAC. Ответ resend одинаков для неизвестного, подтверждённого и подавленного аккаунта. SMTP имеет таймаут и ограничение параллелизма; при отказе аккаунт сохраняется, UI честно сообщает о недоставленном письме.

## 2. Файлы

Ниже изменения именно этого этапа. Более ранние незакоммиченные frontend, PVM, wishlist и каталог сохранены.

| Область                         | Файлы от корня integration-repository                                                                                                                                                                                                                              |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Модель/DTO                      | `internal/entities/user.go`, `internal/storage/user.go`, `internal/handler/user.go`                                                                                                                                                                                |
| Auth                            | `internal/usecase/auth.go`, `internal/storage/auth.go`, `internal/handler/auth.go`, `internal/handler/handler.go`                                                                                                                                                  |
| Email usecase/storage           | новые `internal/usecase/email_verification.go`, `internal/storage/email_verification.go`                                                                                                                                                                           |
| SMTP/config/startup             | новые `internal/config/mail.go`, `internal/mailer/smtp.go`; `cmd/app/main.go`                                                                                                                                                                                      |
| Схема                           | новая `migrations/014_email_verification.sql`, правило LF в `.gitattributes`                                                                                                                                                                                       |
| Окружения                       | `compose.yaml`, `compose.frontend.yaml`, `compose.integration.yaml`, `compose.qa.yaml`, `compose.tests.yaml`, `.env.example`, `scripts/start-integration.ps1`; в приватный `.env` добавлены только отсутствовавшие SMTP настройки                                  |
| React                           | новые `src/pages/EmailVerification.tsx`, `src/lib/verification-link.ts`; `src/App.tsx`, `src/pages/Auth.tsx`, `src/lib/auth-types.ts`, `src/lib/client.ts`, `src/styles.css`                                                                                       |
| Безопасные страницы/диагностика | `frontend/nginx.conf`, `frontend/vite.config.ts`, `frontend/scripts/redact-logs.ts`, `scripts/qa-browser.mjs`                                                                                                                                                      |
| Новые Go tests                  | `internal/entities/email_verification_test.go`, `internal/config/mail_test.go`, `internal/mailer/{smtp_test.go,mailpit_integration_test.go}`, `internal/storage/email_verification_integration_test.go`, `internal/handler/email_verification_integration_test.go` |
| Существующие test fixtures      | `internal/testsupport/email.go`, `internal/handler/email_test_helpers_test.go`; auth/catalog/PVM тесты проходят обычное подтверждение, без runtime bypass                                                                                                          |
| React/browser tests             | `frontend/src/lib/{verification-link.test.ts,email-auth.test.ts}`, обновлённые profile/redaction fixtures; `frontend/e2e/{email-verification.spec.ts,verification-fixture.ts}` и прежние live fixtures                                                             |
| Проверка сохранности            | `scripts/verify-email-preservation.mjs`, `scripts/email-working-smoke.mjs`; приватный backup вне репозитория                                                                                                                                                       |
| Документация                    | `docs/email-verification.md`, `README.md`, `docs/architecture.md`, `frontend/README.md`, `frontend/docs/IMPLEMENTATION_PLAN.md`, этот отчёт                                                                                                                        |

Новые runtime зависимости Go/React не понадобились. Два проверенных снимка интерфейса: `previews/email-confirmed-desktop.png`, `previews/email-expired-mobile.png`.

## 3. Миграция и сохранность

**014_email_verification.sql** добавляет `users.email_verified_at NULL`, `users.email_verification_required NOT NULL DEFAULT FALSE`, таблицы `email_verification_tokens` и `email_verification_ip_limits`, ограничения и индексы. Старые миграции 001–013 совпадают побайтно с резервной копией.

До изменений создан `..\backups\2026-11-17-before-email-verification`: 361 файл, включая 107 frontend, untracked, Git, `.env` и приватные настройки. `node_modules`, сборки и временные файлы исключены. Все SHA-256 повторно проверены. Custom PostgreSQL dump: 118103 байта, SHA-256 `dd320426069feaad13506242baa6b6504943a318df95a6e0497baf3b48bb4d76`; TOC проверен. Для сравнения архив декодируется в SQL только в test-контейнере без выполнения restore.

Миграция сначала проверена в изолированных PostgreSQL/Mailpit окружениях, затем применена к прежнему integration volume `srez-integration_postgres_data`. Read-only сравнение после обновления подтвердило сохранность **1071 прежней строки**, всех прежних колонок и последовательностей. Все **38 старых пользователей** имеют required=false и verified_at=NULL, их почта не объявлена подтверждённой. Каталог, изображения, корзины и заказы сохранены. У 13 уже удалённых media tombstones существующий GC продвинул только `resweep_after`; содержимое и прочие поля не менялись. Для проверки создан один отдельный `email_smoke_*` аккаунт, он сохранён; обычные login/refresh добавили auth-записи. Пользовательские данные не очищались.

Исходная `team-repository`, её БД и volume `team-repository_postgres_data` сохранены отдельно; read-only сравнение с исходным backup подтверждает прежние бизнес-данные. Прежние значения `.env` не изменены. Пять добавленных ключей: SMTP host/port/TLS, mail sender, frontend base URL; секретов в отчёте нет. Mailpit использует новый отдельный volume, старые PostgreSQL/SeaweedFS volumes не пересоздавались.

Доказательства в backup: `VERIFIED.json`, `BEFORE_WORKING_MIGRATION.json`, `EMAIL_PRESERVATION_VERIFIED.json`, `WORKING_SMOKE_VERIFIED.json`; итоговая проверка файлов/секретов — `EMAIL_FINAL_SAFETY.json`.

## 4. HTTP API

| Метод     | Маршрут                     | Изменение                                                                                                                       |
| --------- | --------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| POST      | `/auth/register`            | прежние поля; 201 public user + required=true, delivery status sent/unavailable и resend_after_seconds=60; без JWT/cookie/token |
| POST      | `/auth/verify-email`        | новый; `{ "token": "..." }`, 200 message; неверный/истёкший/повторный/отозванный — одинаковый 400                               |
| POST      | `/auth/resend-verification` | новый; `{ "identifier": "email или username" }`, одинаковый 202; IP limit — 429/Retry-After                                     |
| POST      | `/auth/login`               | новый pending аккаунт с правильным паролем получает 403; неверный пароль — прежний 401                                          |
| POST      | `/auth/refresh`             | прежняя ротация + проверка допуска аккаунта к новым credentials                                                                 |
| GET/PATCH | `/profile`                  | DTO расширен email_verified_at и email_verification_required; изменить их через PATCH нельзя                                    |

Origin/JSON middleware продолжают работать. Cookie-auth refresh/logout сохраняют CSRF. Registration conflict остаётся общим 409. Подробный контракт: [docs/email-verification.md](../../docs/email-verification.md).

## 5. Выполненные проверки

| Проверка                                         | Фактический результат                                                                                                                                  |
| ------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Go build в Docker                                | Успешно                                                                                                                                                |
| Полный Go suite, отдельные PostgreSQL/S3/Mailpit | **919 passed, 0 failed**; один opt-in `TestSeaweedPersistenceAfterRestart` пропущен в этом запуске                                                     |
| Email-focused Go suite                           | **18 passed**, включая настоящий Mailpit SMTP                                                                                                          |
| `go vet`                                         | Успешно                                                                                                                                                |
| Frontend typecheck/lint                          | Успешно                                                                                                                                                |
| Frontend unit                                    | **93 passed**, 12 файлов                                                                                                                               |
| Frontend production build и Docker npm ci/build  | Успешно                                                                                                                                                |
| Frontend format check                            | Успешно                                                                                                                                                |
| Полный browser QA                                | **34 passed**, 0 failed: auth 9, buyer 7, email 3, public 9, seller 3, wishlists 3                                                                     |
| Рабочая копия localhost:5174 + Mailpit:8025      | Успешно: legacy login/reload и 5 брендов владельца, UI register → письмо SMTP → явное подтверждение → login/profile/reload → logout; ошибок браузера 0 |

Проверены strict JSON, неверный Origin, пароль до раскрытия email-статуса, подмена полей, хеш вместо plaintext, expiry, reuse, rotation, 12 конкурентных PostgreSQL verify и два HTTP verify (один успех), SQL rollback, cooldown/hour cap/persistent IP quota, совместимость со старыми аккаунтами и refresh/session gate.

Реальный browser email сценарий открыл Mailpit UI, нашёл письмо и взял ссылку с HTML-кнопки. Проверены немедленная очистка URL, отсутствие токена в storage/history/cookie, no-store/no-referrer, явная кнопка без auto-login, desktop/mobile. SMTP outage тест временно останавливал только QA Mailpit с проверенным Compose label, затем запускал тот же контейнер/volume; аккаунт сохранился и после resend успешно подтверждён.

В длинном едином Windows worker процессе возникал native fast-fail 3221226505 без assertion failure, в разных местах suite. Runner теперь автоматически обнаруживает все активные spec и последовательно запускает каждый в отдельном процессе; **тесты не исключаются, автоматические повторы не добавлены**. Итоговый полный запуск зелёный: `mvp-browser-all.results.json`, подробности `mvp-browser-all.log`. Причина самого Windows fast-fail не установлена.

Логи Go: `mvp-go-full.log`, `mvp-go-email.log`, `mvp-go-vet.log`. Restart-persistence Seaweed проверялся на предыдущем этапе; в email suite он заново не выполнялся и не включён в число успешных проверок этого этапа.

## 6. Известные ограничения

- Mailpit — приватный локальный почтовый ящик, а не отправка в реальный Gmail/Yandex. Письма со ссылками сохраняются в его volume; доступ к ПК/backup должен быть приватным.
- Resend выполняет SMTP асинхронно в памяти, без durable plaintext outbox. При падении процесса или SMTP-ошибке нужна новая попытка после cooldown; одинаковый 202 не обещает доставку. Registration сообщает известный результат SMTP честно.
- Старые аккаунты временно допускаются без подтверждения по явному требованию совместимости. UI не выдаёт их за подтверждённые.
- IP quota использует фиксированные окна, аккаунт — скользящий час. В QA лимит IP повышен до 200/1000 ради серии тестов, account cap и cooldown сохранены.
- Внешний SMTP/провайдер, production HTTPS/DNS/SPF/DKIM/DMARC и доставляемость в реальную почту не проверялись. SMTP реализация поддерживает проверяемый TLS/STARTTLS и server-only credentials для последующей замены.
- После очистки URL и перезагрузки страницы подтверждения нужно снова открыть письмо: секрет намеренно не сохраняется. Сброс пароля не входил в этот этап.

## 7. Запуск в Windows PowerShell

Docker Desktop должен быть запущен с Linux containers. Не используйте базовый compose поверх исходной БД.

```powershell
Set-Location 'C:\dev\SREZ'
docker version
docker compose version
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-integration.ps1
Start-Process 'http://localhost:5174/register'
Start-Process 'http://127.0.0.1:8025'
```

Зарегистрируйтесь, найдите письмо в Mailpit по `to:ваш-email`, откройте ссылку, нажмите «Подтвердить почту», затем войдите и проверьте профиль. SMTP: `127.0.0.1:1025`, API: `127.0.0.1:8081`, PostgreSQL: `127.0.0.1:15432`; внутри Docker SMTP — `mailpit:1025`.

```powershell
# Диагностика и остановка с сохранением всех volumes/писем:
docker compose -p srez-integration -f compose.integration.yaml ps
docker compose -p srez-integration -f compose.integration.yaml stop
# Возобновление:
docker compose -p srez-integration -f compose.integration.yaml start
```

Go/GoLand на Windows: SMTP host `127.0.0.1`, port `1025`, TLS `none`; Run Configuration требует DATABASE_URL именно integration БД на **15432**, согласованные PUBLIC_ORIGIN/FRONTEND_BASE_URL и существующие секреты. Полная пошаговая инструкция, отдельный запуск Mailpit и команды QA — [email-verification.md](../../docs/email-verification.md).

## 8. GitHub и локальность

Commit, push, PR, deployment, reset/clean, удаление пользовательских строк или Docker volumes не выполнялись. HEAD/branch/refs integration не изменены: **main @ 5133b878d6610b041aaf2aa687988221ca30648b**. Исходная копия остаётся **feature/product-variant-management @ e8c9746cd041d14db8a534c2a552164ea324b02d**. Изменения находятся только в локальных файлах и локальных интеграционном/тестовых окружениях; GitHub не менялся.
