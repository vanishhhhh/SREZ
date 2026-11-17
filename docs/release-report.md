# SREZ — Account Recovery и подготовка локального MVP к review

Дата:17.11.2026. Этот документ фиксирует реализованный код и локальную проверку. Фактические commit SHA/URL PR сообщаются после отправки ветки; собственный HEAD отчёт не встраивает, чтобы не создавать самоссылочную историю.

## A. Состояние

Работа выполнена в integration-repository поверх main5133b878d6610b041aaf2aa687988221ca30648b. Origin:https://github.com/vanishhhhh/SREZ.git. До изменений origin/main совпадал с HEAD, локальные frontend/MVP/email изменения не были опубликованы. Целевая ветка — feature/account-recovery-and-release-prep; main не является целью push/merge.

Исходная team-repository не использовалась как источник кода и не изменялась. Рабочая интеграционная копия доступна на localhost:5174, backend8081, Mailpit8025; обновление проверено в Docker без deployment.

## B. Функциональность

Существовали и сохранены: React инфраструктура/auth, публичный каталог и бренды, PVM, SeaweedFS Media, favorites/cart/unpaid orders, shareable wishlists, owner/editor студия, email verification. В ветку включаются полезные накопленные исходники этих этапов.

Добавлены forgot/reset backend и React, атомарный отзыв сессий и access JWT, общий SMTP reset message, миграция015, persistent quotas, regression tests. Исправлены stale-password login race, lock order refresh/reset, LF/CRLF migration portability, устаревшие API примеры/адреса и состав будущей публикации. Новые страницы переиспользуют текущие формы и стиль, PVM разделён на смысловые модули.

## C. Password Recovery

POST /auth/forgot-password принимает email и возвращает одинаковый202 для подходящих/неизвестных/неподтверждённых адресов и account suppression. POST /auth/reset-password принимает только token/password, погашает одноразовый challenge, меняет Argon2id hash и отзывает все сессии в транзакции; auto-login нет.

32 crypto random bytes → Base64URL43 → SHA-256 в отдельной password_reset_tokens. TTL30 минут, one-use, rotation/revoke, user lock и SQL CAS. Миграция015 добавляет также password_reset_ip_limits; применённые001–014 не изменены. Account60s/5hour, IP10minute/50hour, значимые лимиты переживают restart. Raw IP не сохраняется.

Тот же MailSender/config/Mailpit, text+HTML письмо «Сброс пароля — SREZ». Send после commit и с timeout; forgot202 не обещает доставку. Plaintext outbox отсутствует. Fragment очищается до router, token memory-only, GET не расходует ссылку, POST выполняется по форме. Новый пароль вводится дважды.

Старые access JWT теперь проверяются по активной серверной сессии: один indexed SELECT, timeout2s, bounded DB pool20, без stale cache. После commit reset/logout/replay следующий Bearer отклоняется; in-flight запрос не отменяется задним числом. CreateSession сверяет password_hash под user lock, refresh соблюдает общий lock order. [Подробный контракт](account-recovery.md).

## D. Review и рефакторинг

Проверены cmd/internal/migrations/frontend/docs/deploy/scripts/Compose/config/tests. Исторические документы сохранены с маркировкой; screenshots/logs/private backups не удаляются, но исключены из Git. Зависимости не удалялись по названию; npm ci/audit и Go tidy проверены. Новых runtime зависимостей нет.

ProductEditor880 строк разделён: Variants.tsx и Images.tsx; orchestration и данные/UX сохранены. Общий AuthFrame заменяет дублирование sensitive layout. Studio/Upload/Wishlist/Cart уже имели смысловые компоненты и сохранены без косметического rewrite. [Конкретные проблемы с уровнями и файлами](project-review.md).

## E. Документация и запуск

Обновлены README, frontend README/план, architecture, актуальный PVM. Добавлены единый api-reference, account-recovery, project-review и этот отчёт. Старые отчёты явно исторические. Windows инструкции не требуют чужого абсолютного пути или запуска shell из .env; native Go dev script явно выбирает integration15432 вместо старой БД5432.

```powershell
# Из текущего корня SREZ:
node .\frontend\scripts\prepare-local.mjs
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-integration.ps1
Start-Process 'http://localhost:5174'
Start-Process 'http://127.0.0.1:8025'
# Остановка с сохранением БД, медиа и писем:
docker compose -p srez-integration -f compose.integration.yaml stop
```

[Полные инструкции](../README.md), [все HTTP API](api-reference.md). GitHub Actions добавляет три jobs с contents:read: Go+real infrastructure, frontend и browser; production secrets/deployment нет. Реальный результат удалённого CI следует читать отдельно от локальных проверок.

## F. Выполненные проверки

| Проверка                          | Результат                                                                                                                                                                    |
| --------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go fmt/build/full suite           | 926 passed,0 failed; один opt-in TestSeaweedPersistenceAfterRestart skipped                                                                                                  |
| Go vet                            | успешно                                                                                                                                                                      |
| Go mod tidy -diff                 | успешно после устранения только CRLF go.sum, состав зависимостей сохранён                                                                                                    |
| npm ci/audit                      | установка успешна,0 найденных npm vulnerabilities на момент запуска                                                                                                          |
| TypeScript/Oxlint                 | успешно; текущий проект использует Oxlint, отдельный ESLint не добавлялся                                                                                                    |
| Frontend unit                     | 96 passed,13 файлов                                                                                                                                                          |
| Frontend production/Docker builds | успешно                                                                                                                                                                      |
| Browser all                       | 36 passed:auth9,buyer7,email3,recovery2,public9,seller3,wishlists3                                                                                                           |
| Рабочий smoke                     | legacy owner login/reload и5 брендов, register→real SMTP verify→login→forgot→real reset mail→new password→login/reload; old password/access/refresh отвергнуты,0 page errors |
| Migration/data preservation       | 001–014 byte-identical;015 применена после отдельной QA;1092 прежние строки сохранены, новые только собственный smoke и auth/challenge/migration metadata                    |

Go тесты используют отдельную БД и случайные схемы, реальный SeaweedFS/Mailpit. Browser target жёстко ограничен localhost:5175, timestamp/SMTP outage проверки работают только со своими QA записями/контейнерами. Пользовательские строки ради тестов не удаляются. Тестов, которые якобы прошли без запуска, в результате нет.

Первые browser recovery assertions выявили дублирующее имя form/input и неоднозначный поиск inline+toast ошибки. Исправлены accessible имя и scope assertion; повторный полный прогон36/36 зелёный. Ранее существовавший Windows native worker fast-fail устраняется запуском каждого spec в отдельном процессе, без исключения тестов или автоматических retries.

Seaweed restart-persistence проверялся раньше и не запускался заново на этом этапе, поэтому не включён в passed. Нагрузочный benchmark, внешний SMTP и независимый AppSec не выполнялись. Remote CI не объявляется зелёным по локальному прогону.

Первый GitHub run успешно выполнил backend и frontend, но обнаружил ошибку в подготовке public/seller fixtures на чистом окружении: SREZ Origin наследовался запросами Mailpit. Исправлен отдельный почтовый APIRequestContext без наследования auth/Origin, без ослабления server checks; диагностика runner стала доступна в CI с редактированием секретов. Сценарий повторён на новых пустых test volumes. Результат следующего удалённого run фиксируется отдельно в итоговом сообщении/publication proof.

### Сохранность

Второй удалённый run прошёл backend/frontend и35/36 browser, выявив гонку seller fixture между reorder и выбором «фото1». Добавлено ожидание фактического нового cover URL перед заменой. Проверки не исключались, timeouts/retries не увеличивались; локальный seller сценарий повторяется перед отправкой исправления.

До изменений создан и проверен приватный backup390 файлов, включая116 frontend и untracked; node_modules/dist/logs исключены. PostgreSQL custom dump126281 байт, SHA-25638e8591fb1e358053c31b64b4ea154117aecba8e46d139087722fe0f60ad338d, TOC проверен. Резервная копия не входит в Git.

Все прежние значения .env сохранены, recovery не потребовала новых ключей. Old migration SQL и journal строки не переписаны. Четыре уже deleted media tombstones получили только штатное продвижение resweep_after существующим GC. Добавлена одна собственная recovery_smoke запись, она сохранена; пароль reset проверялся только на ней. Existing PostgreSQL/SeaweedFS/Mailpit volumes не пересозданы и не удалены.

Приватные доказательства: VERIFIED.json, RECOVERY_PRESERVATION_VERIFIED.json и RECOVERY_WORKING_SMOKE_VERIFIED.json в backup перед этапом. Никакие пароли/токены не включены в этот отчёт.

## G. Публикация исходников

Планируемая ветка: [feature/account-recovery-and-release-prep](https://github.com/vanishhhhh/SREZ/tree/feature/account-recovery-and-release-prep), PR в main без автоматического merge. Коммиты разделяют полезную функциональность и документацию/CI. Перед отправкой проверяются index, untracked, diff/cached, секреты и актуальная origin/main; blanket add всего корня не используется.

.env/.secrets/private data, dumps/volumes, dependencies/builds/logs, screenshots и execution proofs исключены. Исторические собственные секреты проверяются по Git blobs без вывода их значений. Локальная БД использует известный ранее опубликованный development password из примера; перед любым внешним использованием его требуется сменить. В этой задаче роль/пароль не менялись и наружу БД не открывалась.

Точные результаты push, commit IDs и ссылка на созданный PR фиксируются после выполнения в финальном сообщении и приватном publication proof. Если доступ отклонён, локальные коммиты остаются и сообщается фактическая ошибка; публикация не подменяется утверждением об успехе.

## H. Ограничения и следующий этап

Recovery пока только для ранее подтверждённой почты; legacy может сначала подтвердить адрес добровольно. In-memory SMTP доставка может потребовать повтора после сбоя. Нет password history/MFA, настоящей оплаты/доставки, checkout idempotency key, production HTTPS/monitoring или внешнего SMTP теста. Публичного deployment не выполнялось.

P0: незакрытых подтверждённых критических дефектов в выполненных проверках не осталось; это не сертификат безопасности. P1: внешний SMTP/HTTPS/ротация, независимый AppSec/load/monitoring/backups/retention и checkout idempotency. P2: session management/key rotation, дополнительные accessibility/browser checks и дальнейшая модульность по мере роста. P3: оплата/доставка/seller orders, guest cart и ownership transfer. [Полный roadmap](project-review.md#roadmap).
