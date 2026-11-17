# Ревизия проекта и приоритеты

17.11.2026. Источник истины — integration-repository и его актуальные Go/React исходники. Проверены структура, маршруты/DTO, импорты и зависимости, SQL/права/транзакции, auth/token flows, загрузка файлов, Compose, формы и ошибки, тестовые окружения и документация. Это engineering review с регрессией, а не независимый pentest или гарантия отсутствия уязвимостей.

## Инвентаризация и решения

| Область           | Состояние и принятое решение                                                                                                                           |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| cmd               | app собирает существующие зависимости; seed и qa-media-copy — явные операторские утилиты, не startup side effects                                      |
| internal/entities | закрытые доменные состояния, проверка publication/stock/timestamps; DTO остаются в handler                                                             |
| internal/handler  | Gin/strict JSON/статусы, без SQL; все реальные маршруты внесены в api-reference                                                                        |
| internal/usecase  | единый Auth, общие Argon2/MailSender; RBAC/media/shopping/wishlist сценарии сохранены                                                                  |
| internal/storage  | parameterized SQL, транзакционные parent/owner проверки, concurrency/rollback regression; новый reset соблюдает lock order                             |
| migrations        | 001–014 сохранены побайтно;015 добавочная; compatibility LF/CRLF не переписывает journal                                                               |
| frontend          | React/TS/Vite/Router пригодны для развития; rewrite не нужен; AuthFrame и PVM части выделены по ответственности                                        |
| ProductEditor     | был880 строк; размеры/остатки и изображения вынесены в features/product-editor без смены UX/API                                                        |
| Studio            | уже разделён на StudioPage/NewBrand/BrandSettings/Members/BrandStudio; дальнейшая разбивка возможна по мере роста, не сделана ради косметики           |
| Wishlists/Cart    | отдельные buyer/wishlist state/providers, API и pages; сохранены optimistic guards, серверные суммы и неопределённый checkout                          |
| Media Upload      | bounded File/Blob, object URL cleanup, AbortController, upload lock, безопасная неопределённость PUT, unsaved-file guard; больших изменений не требует |
| deploy/Compose    | приватный SeaweedFS, Mailpit loopback, named volumes; working/QA/Go-tests разделены                                                                    |
| scripts           | QA runners проверяют фактический exit status; backup переносим; native Go dev использует integration порты и не исполняет .env                         |
| docs              | README/API/PVM/recovery обновлены; старые отчёты помечены историческими; local proof/screenshots вне Git                                               |
| dependencies      | npm ci/audit, typecheck/Oxlint и Go tidy проверяются; MongoDB indirect не удалялся по одному имени без dependency graph                                |
| Git/CI            | публикация отдельной ветки после проверки staging; contents:read CI с реальными PostgreSQL/S3/Mailpit, без deploy                                      |

Поиск TODO/FIXME/HACK и небезопасного HTML не выявил отмеченных реализационных заглушек в runtime коде. Поля placeholder в формах и stubs в tests — нормальные конструкции, не фиктивный marketplace backend. `/projects` — намеренный legacy endpoint, `/users` отключены. Retired `/media/uploads/:id/complete` отвечает410, не используется новым frontend и сохранён для явного отказа старым клиентам.

## Подтверждённые проблемы и исправления

| Уровень | Файл/область                                                    | Дефект или риск                                                                                      | Исправление/проверка                                                                                                            |
| ------- | --------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| P1      | internal/handler/auth.go, storage/auth.go, security/security.go | Подписанный access JWT не проверял отзыв серверной сессии и мог пережить reset/logout                | Auth.Authenticate: проверка claims + indexed active session, 2s deadline, fail closed; реальные HTTP/JWT tests                  |
| P1      | usecase/auth.go, storage/auth.go                                | При добавлении reset возможна выдача новой сессии входом, который проверил старый пароль до reset    | Expected password_hash проверяется под user-lock в CreateSession; тест stale issuance                                           |
| P1      | storage/auth.go, storage/password_recovery.go                   | Новый reset блокирует user, затем sessions; прежний refresh брал обратный порядок                    | Единый user → session порядок; concurrent reset/refresh regression                                                              |
| P1      | migrations/migrate.go                                           | Windows CRLF и Linux LF одинакового SQL приводили к разным raw checksum                              | Принимаются только два EOL представления того же SQL; старые journal/file bytes сохранены; tampered SQL по-прежнему отклоняется |
| P1      | .github/workflows/ci.yml                                        | Не было общего PR gate для накопленного Go/React кода и инфраструктурных сценариев                   | Go/Frontend/Browser jobs, минимальные permissions, Docker test-only окружения                                                   |
| P2      | frontend/src/pages/ProductEditor.tsx                            | Большой файл смешивал orchestration и независимые PVM формы                                          | Выделены Variants.tsx и Images.tsx; seller/PVM/unsaved формы проходят реальные E2E                                              |
| P2      | frontend/src/pages/EmailVerification.tsx, PasswordRecovery.tsx  | Повторение sensitive page layout                                                                     | Общий AuthFrame, без внешних media/analytics                                                                                    |
| P2      | frontend/src/lib/http.ts                                        | Ошибка source предлагала старый жёсткий адрес5173                                                    | Сообщение ориентирует на настроенный адрес, без неверного порта                                                                 |
| P2      | docs/product-variant-management.md                              | Существовали подробные URL-write примеры/201, хотя актуальный Media API запрещает external URL write | Текущий managed upload/confirm200/replace/reorder, отдельная единая API карта                                                   |
| P2      | go.sum                                                          | CRLF давал технический tidy-diff в Linux без изменения набора зависимостей                           | Канонический LF, повторная tidy проверка; зависимости не обновлены ради форматирования                                          |
| P2      | .gitignore/.dockerignore, demo/catalog                          | В untracked были screenshots/QA JSON и одноразовые local checks                                      | Исключены из публикации, на компьютере сохранены; полезные demo assets/manifest и код остаются                                  |
| P2      | scripts/seed-showcase.mjs                                       | Fresh demo owner после verification больше не мог сразу login                                        | Явное сообщение открыть Mailpit и повторить после подтверждения, без bypass или дубликата аккаунта                              |

Новая recovery форма и тесты дополнительно выявили одинаковое accessible имя формы/поля и неоднозначные assertions inline+toast. Форма получила отдельное имя, tests проверяют правильный form scope; ошибки не скрывались и сценарий повторён.

## Проверенные security границы

- Auth: password gate, dummy Argon2, JWT claims, refresh single-flight/rotation/grace/replay/absolute expiry, HttpOnly cookie, exact Origin/CSRF, logout/recovery revocation.
- Verification/recovery: 32 crypto bytes, hash-only DB, отдельные назначения, TTL/one use, SQL rollback, concurrent requests, persistent quotas, no auto-login и общие нейтральные ответы.
- RBAC/IDOR: foreign brand/product/variant/media/wishlist/cart/order IDs, user_id/role/price mass assignment; invited/revoked editor права не получает.
- Media: MIME sniff/full decode/пиксели/output cap, Content-Length/deadlines, повторный RBAC, private source/key, публикация/отзыв и GC/reconciliation.
- Shopping: variant-specific availability, server money, last-stock concurrency, cancel/expiry atomic release, lost checkout response без автоматического повторения.
- Share links: capability lookup hash + encrypted owner recovery, отдельный ключ, hidden catalog filtering, revoke/rotate и отсутствие URL в логах.
- Frontend: React escaping вместо user HTML, form locks, error/loading/empty, media Abort/object URL cleanup, очищенный fragment, no token persistence, no-referrer/no-store/CSP.

Проверки не означают полную постоянновременность SQL, DDoS-защиту или отсутствие XSS при будущих изменениях. Запрос, прошедший middleware до reset, не отменяется ретроспективно. Нагрузочный benchmark и независимый AppSec-review не проводились.

## Что сохраняется локально и не публикуется

`.env`, `.secrets`, pg_dump/volumes, node_modules/dist/build, _.log, test-results/playwright-report, screenshots, mvp/recovery results JSON и demo execution proofs. Одноразовые scripts/verify-_-data.mjs, email preservation/smoke/safety scripts с жёсткими датами относятся к локальной истории, не к переносимому запуску; они не удалены. Универсальные QA, backup, seed и dev scripts публикуются. Резервная копия до recovery проверена отдельно; её содержимое не добавляется в Git.

## Roadmap

### P0

Подтверждённых незакрытых критических ошибок в выполненных проверках не осталось. Это не отменяет необходимость независимого review перед внешним запуском.

### P1 — до реального публичного использования

- Проверяемый внешний SMTP, HTTPS/domain/sender, SPF/DKIM/DMARC и операционная ротация production secrets; публичные demo значения не использовать.
- Independent AppSec, нагрузочные проверки новой session lookup, общая distributed защита login/forgot и trusted proxy конфигурация.
- Monitoring/alerting, приватные backup/restore учения, retention для auth/challenges/IP keys/Mailpit без удаления нужных пользовательских данных.
- Надёжная защищённая доставка уведомлений без plaintext durable outbox. Сейчас send/resend/forgot могут потребовать повторения после сбоя.
- Idempotency key для checkout вместо только клиентской проверки истории при потерянном ответе.

### P2 — технический долг

- Дальнейшая модульность Studio/Wishlist pages по мере роста, отдельные API facets каталога.
- Автоматизированная key rotation с миграцией encrypted wishlist recovery и session management UI.
- Уведомление о смене пароля, продуктовая политика password history/MFA и согласованный recovery для старых аккаунтов без ранее подтверждённого email.
- Полноценный concurrency/load benchmark, дополнительные accessibility/keyboard tests и свежие cross-browser проверки.

### P3 — продуктовые возможности

- Оплата, доставка, seller order processing/возвраты после настоящей оплаты.
- Гостевая корзина и merge после login, ownership transfer по согласованному безопасному процессу.
- Public hosting/HTTPS и CDN только как отдельный этап с разрешением на deployment.

Фактические результаты и публикация исходников — [release report](release-report.md). Правила recovery сверены с [OWASP Forgot Password](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html), CI — с [официальным Playwright](https://playwright.dev/docs/ci-intro). Эти источники задают рекомендации, а не сертифицируют реализацию.
