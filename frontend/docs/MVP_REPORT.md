# SREZ — отчёт о локальном MVP

> Исторический отчёт завершённого этапа. Текущее состояние и результаты: [release report](../../docs/release-report.md). Логи, backup и screenshots остаются локальными, их отсутствие в Git ожидаемо.

Дата: **17.11.2026**, Europe/Moscow. Рабочая папка: `C:\dev\SREZ`. Базовый Git HEAD: `5133b878d6610b041aaf2aa687988221ca30648b`, ветка main. Это продолжение имеющегося frontend и backend, без синхронизации GitHub в текущей задаче.

## A. Реализовано

**Этап 5, Shareable Wishlists:** полный Go/PostgreSQL/React цикл: несколько именованных списков, создание/изменение/удаление, товар с nullable предпочтением variant, изменение варианта, перестановка/удаление элементов. Bookmark picker работает в каталоге и карточке товара; существующее избранное сохранено отдельно. Поделиться/скопировать/отозвать/перевыпустить; публичная подборка открывается без входа. Есть состояния загрузки/пустых данных/ошибки, подтверждения и мобильная вёрстка.

**Этап 6, Seller Dashboard:** сохранены /studio и существующие рабочие формы, добавлена единая адаптивная навигация с выбором бренда. Owner управляет брендом, оформлением, logo/banner, публикацией и командой; editor — ассортиментом по существующим RBAC. Приглашения/accept/decline/revoke, draft товаров, описание/категория/цена/материалы, цвета, размеры/остатки/SKU, фото/замена/порядок/обложка/удаление и publish/hide используют реальные API. Структурное изменение опубликованного товара требует явного hide → edit → publish. Добавлены preview перед upload, защита выбранного файла и несохранённых форм, обработка поздних GET.

**Этап 7, Integration/QA/Polish:** проверены auth, публичная витрина, избранное, cart, reserve/cancel/expiry, wishlist и seller. Фоновые процессы frontend заменены стабильным Docker frontend для обычного запуска; Vite сохранён для разработки. Исправлены воспроизведённые ошибки форм, clock metadata, HTML validation и CSP/fonts. Добавлены изолированные тестовые окружения, документация и скриншоты.

## B. Архитектура и сохранность

Backend: handler → usecase → entities/repository interfaces → PostgreSQL. Wishlist использует тот же ProductCard и пакетную загрузку публичного каталога; SQL не попадает в handler. Чтение — read-only REPEATABLE READ, N+1 отсутствует. Лимиты и операции списка сериализуются row locks; reorder атомарен. Ownership проверяется сервером во всех приватных операциях. Public DTO отдельно от owner DTO, без email/владельца/секретов/скрытых товаров.

Share token —256 случайных бит, поиск по SHA-256, восстановление владельцем из AES-GCM ciphertext. Независимый стабильный SREZ_WISHLIST_KEY сохранён в приватной .env-копии. URL/cookie/JWT не записываются proxy в логи; no-store/no-referrer и CSP включены. Открытая публичная подборка перечитывается раз в30 секунд и по focus, сервер проверяет доступ при каждом GET.

Frontend: существующие React19/TypeScript7/Vite8, in-memory JWT, HttpOnly refresh, CSRF/Origin, single-flight/Web Locks и API transport сохранены. Data Router нужен для общей защиты несохранённых форм. Новые features/wishlists и dashboard layout переиспользуют карточки, Media, уведомления и стили. Дублирующего buyer/backend API нет. Единственная новая frontend dev-зависимость — Oxlint; runtime библиотеки прежние. Принудительного разрешения конфликтов зависимостей не было.

Обычный frontend: Vite build → локальный nginx на5174, /api rewrite и /media без rewrite, исходный Origin сохраняется. Docker DNS upstream обновляется при смене адреса app. Шрифты локальные и совместимы с CSP. Nginx работает непривилегированным пользователем; журналы содержат только status/bytes/time, без URL. Диагностика приложения остаётся в безопасных Go logs.

### Резервные копии и доказательства

- До изменений создан `..\backups\2026-11-17-before-mvp`:267 файлов, включая78 frontend, untracked, конфигурацию, Git и .env; node_modules/dist/cache исключены. Все267 SHA-256 повторно проверены в конце.
- PostgreSQL custom dump:96094 байта; SHA-256 `67e3bc990db5ed9cb69b000685e3cda5dcaaecbe847a126683019ff999679714`. TOC проверен, архив восстановлен только в пустую QA БД до новых миграций.
- Для восстановленных SQL-ссылок QA отдельно скопированы87 отсутствующих media объектов (397011 байт),108 уже существующих оставлены; отсутствующих источников0, SHA-256 проверены. Рабочее S3 читалось только Head/Get; целевой PutIfNoneMatch не заменяет существующее. Браузер продолжает обращаться к Go, не к S3.
- Новые012–013 сначала прошли на QA; затем применены к существующему volume integration. Все11 ранее применённых SQL совпадают побайтно с backup до MVP. Исходная БД старой копии остаётся001–005; integration и QA —001–013.
- Исходные167 не-Git файлов team-repository из старого manifest совпадают с backup. Её branch feature/product-variant-management и HEAD e8c9746cd041d14db8a534c2a552164ea324b02d сохранены.
- Read-only сравнение исходной БД: бизнес-данные/последовательности совпадают, auth сохранён; одна штатная refresh-строка добавилась работающим приложением.
- Read-only сравнение integration с текущим backup: все прежние бизнес-строки/колонки/последовательности и миграции сохранены. Отдельно подтверждены только штатные изменения: одна refresh-строка и продвижение resweep_after у9 уже удалённых media tombstones существующим GC. Контент/ссылки/состояния остальных объектов совпадают; новые wishlist таблицы пусты в рабочей БД после миграции.
- Все прежние значения .env сохранены. Добавлен только недостающий wishlist key; текущая приватная копия `env-after-mvp.private` проверена. Секреты в отчёт/исходники не включены.
- Доказательства: VERIFIED.json, QA_RESTORE_VERIFIED.json, QA_MEDIA_COPY_VERIFIED.json, INTEGRATION_DATABASE_PRESERVED.json, FINAL_SAFETY.json в backup; DATABASE_PRESERVED.json и MVP_FILES_PRESERVED.json в старом backup.

Docker volumes не удалялись/не пересоздавались. Исходная team-repository не обновлялась. Изолированные тесты могут создавать/удалять только свои тестовые схемы и записи; пользовательские данные для этого не использовались.

## C. Новые API и страницы

Все пути Go ниже без браузерного /api. Кроме shared GET требуется Bearer. JSON строго валидируется, чужие объекты404.

| Метод/путь                                 | Назначение                                        |
| ------------------------------------------ | ------------------------------------------------- |
| POST /wishlists; GET /wishlists            | Создать/перечислить собственные списки            |
| GET/PATCH/DELETE /wishlists/:id            | Детали/переименование/удаление                    |
| POST /wishlists/:id/items                  | Добавить product_id и optional product_variant_id |
| PATCH/DELETE /wishlists/:id/items/:item_id | Изменить предпочтение/удалить                     |
| POST /wishlists/:id/items/reorder          | Полный атомарный item_ids порядок                 |
| POST /wishlists/:id/share                  | Включить либо перевыпустить публичную ссылку      |
| DELETE /wishlists/:id/share                | Отозвать доступ                                   |
| GET /wishlists/shared/:token               | Гостевое чтение действующей подборки              |

Лимиты20 списков/200 товаров; название100, описание2000 символов. Один товар на список; вариант можно изменить. DELETE списка204, DELETE элемента200 с оставшимся списком. Полные DTO/ошибки/политика токенов — [docs/wishlists.md](../../docs/wishlists.md).

React: /wishlists, /wishlists/new, /wishlists/:id, /w/:token. Dashboard сохраняет /studio, /studio/invitations, /studio/brands/new, /studio/brands/:brandId (tab=settings/team), product new/editor. Buyer/Auth/Media/Brand/PVM endpoints переиспользованы, не реализованы повторно.

## D. Основные файлы

| Область           | Создано/изменено                                                                                                                                                                                                            |
| ----------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Schema            | migrations/012_shareable_wishlists.sql, 013_wishlist_share_integrity.sql; прошлые SQL сохранены                                                                                                                             |
| Wishlist backend  | internal/entities/wishlist.go, usecase/wishlists.go, storage/wishlists.go, handler/wishlists.go, security/wishlist_links.go, config/wishlist.go и тесты                                                                     |
| Wiring            | cmd/app/main.go, handler/auth.go, handler/handler.go, .env.example, compose.yaml                                                                                                                                            |
| Regression fixes  | usecase/mutation_time.go, usecase/management.go, handler/management.go, management_time_integration_test.go, management_logging_test.go; security/security.go, issuance_clock_test.go                                       |
| Wishlist frontend | src/features/wishlists/{api.ts,Picker.tsx,Pages.tsx,_.test._}, storefront/shared.tsx и ProductPage.tsx                                                                                                                      |
| Seller/forms      | src/features/dashboard/Layout.tsx, pages/Studio.tsx, pages/ProductEditor.tsx, components/UnsavedChanges.tsx, Modal.tsx, ui.tsx, lib/hooks.ts, media/UploadField.tsx                                                         |
| Common            | App.tsx, pages/Auth.tsx, styles.css, lib/public-data.ts, lib/studio.ts, vite.config.ts, index.html, package.json/package-lock, .oxlintrc.json                                                                               |
| Local runtime     | frontend/Dockerfile, nginx.conf, .dockerignore; compose.integration.yaml, compose.qa.yaml, compose.tests.yaml; scripts/start-integration.ps1                                                                                |
| QA                | e2e/mvp-fixtures.ts, wishlists.spec.ts, seller.spec.ts, auth.spec.ts, buyer.spec.ts; scripts/qa-go.mjs, qa-browser.mjs, qa-media-copy.mjs; cmd/qa-media-copy/main.go; verify-original-data.mjs, verify-integration-data.mjs |
| Docs              | frontend/README.md, docs/IMPLEMENTATION_PLAN.md, MVP_PROGRESS.md, MVP_REPORT.md, previews/mvp-*.png; root README, docs/architecture.md, docs/wishlists.md                                                                   |

Пути src/e2e в таблице относительны frontend. Полный текущий локальный список: локальный MVP_GIT_STATUS.txt (не входит в Git). Frontend уже был untracked до этой задачи; наличие ?? не означает, что все его файлы созданы заново. Сохранён исходный backup со списком прежних изменений.

## E. Тестирование

| Проверка                                                           | Фактический результат                                                                               |
| ------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------- |
| Docker Go build                                                    | server и seed собраны; Go QA image содержит весь текущий код                                        |
| node scripts/qa-go.mjs full                                        | **900 passed**, включая subtests; failed0. Opt-in restart test в общем запуске пропущен намеренно   |
| node scripts/qa-go.mjs vet                                         | exit0                                                                                               |
| persistence-write + restart test-only Seaweed + persistence-verify | По1 passed в каждом запуске, реальный файл/длина/SHA-256 сохранились                                |
| npm ci                                                             | 57 пакетов, audit58, vulnerabilities0 на момент запуска                                             |
| npm run typecheck / lint                                           | exit0, предупреждений lint0                                                                         |
| npm test                                                           | **88 passed**,10 файлов                                                                             |
| npm run build                                                      | exit0; JS474.92KB (gzip140.61KB), CSS61.58KB (gzip12.83KB)                                          |
| npm run format:check                                               | exit0                                                                                               |
| node scripts/qa-browser.mjs all                                    | **31 passed**, Chrome, один worker, реальные QA Go/Postgres/Seaweed                                 |
| Desktop/mobile/16:10                                               | Browser assertions (включая768/1024 и Tab/Escape в dialog) плюс визуальный осмотр4 новых скриншотов |

Основные реальные сценарии: register → login → profile → reload → concurrent tabs → logout/relogin; catalog filters/sort/pagination/gallery; favorite → chosen variant → cart quantity/remove/re-add → unpaid reserve → cancel → restored stock/history; price/stock conflicts, последняя единица/expiry; wishlist create/add/reorder/edit → guest/share/revoke/rotate; seller brand/logo/banner/product/PVM/images/publish → editor invite/accept/edit/revoke. Проверены IDOR/BOLA/mass assignment, скрытые товары, чужие media, приватная выдача и анонимный S3 list403.

Go содержит отдельные regression cases времени метаданных и JWT, логов без пользовательского текста, SQL constraints и concurrent wishlist limits/reorder. Управляемые auth/public transport tests воспроизводят timeout/401/403/409/неверный DTO и не заменяют реальные end-to-end сценарии. Ни один тест не удалён для получения зелёного результата.

Логи: mvp-go-full.log, mvp-go-vet.log, mvp-go-persistence-write.log, mvp-go-persistence-verify.log, mvp-browser-all.log; generated reports игнорируются Git. Секретные URL/JWT текстового browser output редактируются; traces/video выключены. Проверка безопасности отклонила старый способ чтения backup внутри исходного DB-контейнера: архив теперь декодируется в тестовом контейнере без --dbname, исходная БД только pg_dump с read-only настройкой.

Рабочий запуск start-integration.ps1 завершился успешно. Дополнительно отдельная гостевая Chrome-сессия проверила рабочие /, /catalog, /brands, /login и защиту /wishlists: по одному h1, без горизонтального переполнения и JS-ошибок. Health/catalog200, пять managed media200. Доказательство: MVP_RUNTIME_CHECK.json. Секреты JWT/CSRF/wishlist/S3 из .env не найдены в собранных JS/CSS: MVP_BUNDLE_CHECK.json.

## F. Исправленные ошибки

1. Revoke передавал typed nil []byte в lib/pq как пустой bytea, нарушая private integrity: теперь SQL NULL; добавлены regression checks.
2. SQL CHECK публичного hash допускал UNKNOWN при NULL: новая миграция013 с явными NOT NULL, без правки012.
3. Валидная публикация иногда получала400 time при stored updated_at чуть впереди часов: Management теперь не уменьшает metadata time; полный HTTP regression воспроизводит clock step. Сроки orders/refresh не изменены.
4. Свой только что выпущенный JWT мог получить401 при субсекундном обратном шаге часов. iat и exp сдвинуты вместе на1секунду назад при выпуске; exp не продлён, max15 минут, строгая проверка будущего iat/истечения сохранена, leeway не добавлен. Отдельный детерминированный test проверяет границы.
5. Поздний GET/обновление media сбрасывало набранную форму: useFormDraft сохраняет изменённый draft. AuthPage разделён ключом по login/register без стирающего mount effect.
6. useBlocker требовал Data Router: routing адаптирован, все прежние страницы сохранены. Предупреждения работают для links/back/query/unload и выбранных файлов.
7. Поле формы включало подсказки/options в доступное имя: исправлены id/htmlFor/aria-describedby; браузерный ввод и labels стабильны.
8. После перестановки фото upload мог выбрать другой widget: стабильный useId внешнего блока вместо индекса; замена сохраняет image ID.
9. HTML pattern slug несовместим с современным /v из-за неэкранированного дефиса: исправлен и проверен unit/browser.
10. Vite встраивал маленький шрифт data URL, блокируемый CSP: локальные отдельные font assets и правильное семейство Manrope Variable; JS console/CSP ошибок в завершённых сценариях нет.
11. QA восстановленная БД ссылалась на отсутствующие объекты: исправлено проверенным readonly source copy, без подстановки fake images.
12. Windows background Vite прекращал работу и блокировал переустановку зависимостей открытым журналом: стабильный Docker frontend, документированный dev lifecycle; npm ci и browser tests выполняются последовательно.
13. Logout browser test навигировал до завершения асинхронного запроса: ожидает завершение server logout/navigation; проверки backend не ослаблены.
14. Startup ожидал слишком коротко: теперь ждёт Docker health и отдельно проверяет frontend и /api/health; proxy отслеживает изменение IP backend через Docker DNS.

## G. Известные ограничения

- Это локальный MVP. Нет настоящей оплаты, доставки и обработки оплаченных заказов; заказ — неоплаченный15-минутный резерв.
- Shared links доступны только на этом ПК; публичный домен/HTTPS/deploy не настраивались. Видевший подборку посетитель может сохранить её данные; revoke запрещает следующие запросы.
- В checkout нет server idempotency key: неизвестный результат POST требует проверки истории перед повтором.
- Нет email verification/recovery, гостевого merge cart, transfer ownership, server catalog facets; ничего из этого не имитируется.
- Независимое production AppSec-ревью, нагрузочные тесты, multi-node сценарии и эксплуатационный monitoring не выполнялись. Локальный limiter100 нужен для серии QA аккаунтов, не для production.
- После срока резерв серверный worker может обновить статус с небольшой задержкой; frontend перечитывает сервер, не правит статус локально.
- Backup содержит .env и БД: хранить приватно. Wishlist key нужно сохранять; его случайная смена ломает восстановление старых owner links.
- Nginx intentionally не пишет request URL/error log, чтобы не раскрывать capability; диагностика ограничена status/time и безопасными Go logs.

## H. Windows PowerShell

Полная инструкция с11 шагами и диагностикой кодов — [frontend/README.md](../README.md).

```powershell
Set-Location 'C:\dev\SREZ'
docker version
node --version
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-integration.ps1
Invoke-RestMethod http://127.0.0.1:8081/health
Start-Process 'http://localhost:5174'
# Остановка без удаления данных:
docker compose -p srez-integration -f compose.integration.yaml stop
```

Go на хосте и npm ci для обычного контейнерного запуска не требуются. Скрипт использует Node для безопасного дополнения отсутствующих ключей. Не используйте старый DATABASE_URL5432 для нативного Go этой копии. .env не перезаписывайте.

## I. Git status

HEAD до и после: **5133b878d6610b041aaf2aa687988221ca30648b**, branch **main**. Все локальные refs совпадают с backup. Никаких commit/push/PR/fetch-sync/reset/clean/удаления веток/deploy в этой задаче. .env и результаты сборок/логи игнорируются. Исходная feature-ветка e8c9746 и её папка сохранены. Полный status — локальный MVP_GIT_STATUS.txt (не входит в Git).
