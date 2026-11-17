> Исторический документ предыдущего этапа. Текущее состояние и команды: [MVP_REPORT.md](MVP_REPORT.md) и [frontend README](../README.md).

> Исторический отчёт завершённого этапа. Текущее состояние и результаты: [release report](../../docs/release-report.md). Логи, backup и screenshots остаются локальными, их отсутствие в Git ожидаемо.

# SREZ: безопасная локальная интеграция и покупательский frontend

Дата: **17.11.2026**. Результат: актуальный Go backend и сохранённый React frontend объединены в независимой локальной копии. Покупательские API использованы существующие, Go-модули не переписывались. GitHub не изменялся; commit, push, PR и deploy не выполнялись.

## 1. Расположение и версии

| Параметр | Исходная копия                           | Интеграционная копия                     |
| -------- | ---------------------------------------- | ---------------------------------------- |
| Путь     | C:\dev\team-repository                   | C:\dev\SREZ                              |
| Ветка    | feature/product-variant-management       | main                                     |
| HEAD     | e8c9746cd041d14db8a534c2a552164ea324b02d | 5133b878d6610b041aaf2aa687988221ca30648b |
| Remote   | https://github.com/vanishhhhh/SREZ.git   | тот же, только чтение                    |
| Frontend | http://127.0.0.1:5173                    | http://localhost:5174                    |
| Go API   | http://127.0.0.1:8080                    | http://127.0.0.1:8081                    |

После проверки backup выполнен разрешённый fetch исходного origin. На момент синхронизации GitHub main и origin/main подтверждены на SHA `5133b878d6610b041aaf2aa687988221ca30648b`: 11 коммитов после исходного HEAD, 52 добавленных и 32 изменённых файла. Независимый clone создан с `--no-hardlinks --branch main`; исходная ветка и рабочие файлы не переключались.

Почему раньше не было buyer/Media: исходная feature-ветка осталась на версии с миграциями 001–005. Актуальный main уже включает shopping/orders, Media Storage и миграции 006–011. Реализовывать их повторно не требовалось.

## 2. Резервная копия и сохранность

Резервная копия:
`C:\dev\backups\2026-11-17-before-integration`.

| Проверка                                | Результат                                                                                                                       |
| --------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| Полная файловая копия исходного проекта | 347 файлов, включая .git, .env и весь frontend                                                                                  |
| Frontend                                | 62 файла, включая неотслеживаемые Git; node_modules/dist/временные отчёты исключены                                             |
| Локальные изменения                     | Сохранены compose.frontend.yaml, README, architecture, .dockerignore                                                            |
| Целостность                             | SHA-256 каждого файла сверены; files.sha256.json и VERIFIED.json                                                                |
| Git                                     | git-state.json: HEAD/ветка/refs/status/diff; копия .git                                                                         |
| PostgreSQL                              | Согласованный pg_dump custom-format, 27 795 байт; TOC проверен                                                                  |
| Реальное восстановление                 | pg_restore --single-transaction --exit-on-error в отдельную пустую интеграционную БД, затем проверка восстановленного состояния |
| Исходные файлы после работы             | 173 защищённых файла совпадают с backup; исходные HEAD/ветка/рабочий статус прежние                                             |
| Исходная рабочая БД                     | Доменные данные, последовательности и журнал миграций совпадают с backup; существующие auth-строки сохранены                    |

SHA-256 `postgres.dump`:
`e3db2eac6135f862680e2eef1920584f31446c4243477552226aa6e7fcf3b8b6`.

Доказательства: `RESTORE_VERIFIED.json`, `SOURCE_PRESERVED.json`, `DATABASE_PRESERVED.json` в backup. Значения секретов и строки пользовательских таблиц в отчёт не выводятся.

Исходный сервер оставался запущен. Его штатное обновление сессии добавило refresh-token и продвинуло допустимые auth timestamps после dump. Проверено сохранение всех прежних auth-строк; доменные таблицы и миграции не менялись. Проверяющий скрипт учитывает только такую естественную активность, не скрывая изменений бизнес-данных.

## 3. Изоляция окружения и миграций

Самостоятельный `compose.integration.yaml`, проект `srez-integration`:

| Сервис     | Порт хоста      | Named volume                    |
| ---------- | --------------- | ------------------------------- |
| Go app     | 127.0.0.1:8081  | —                               |
| PostgreSQL | 127.0.0.1:15432 | srez-integration_postgres_data  |
| SeaweedFS  | 127.0.0.1:18333 | srez-integration_seaweedfs_data |
| Vite       | localhost:5174  | —                               |

Исходный `team-repository_postgres_data` не удалялся, не пересоздавался, не восстанавливался поверх dump и не мигрировался. Он остаётся на 001–005. Новая БД первоначально восстановлена из dump, после чего только в ней применены 006–011. Тестовые данные и отдельные volumes сохранены.

Первые пять SQL-файлов перенесены из backup **с исходными байтами/окончаниями строк**, чтобы checksum применённых миграций совпал. Содержимое SQL и Go-кода не изменено. Git на Windows может показывать stat/CRLF-разницу для 005 до обновления индекса; содержательного diff нет. Индекс ради косметики не менялся.

Linux entrypoint SeaweedFS в новой копии приведён к LF; `.gitattributes` закрепляет LF только для shell-скриптов. Это требуется для bind mount на Windows. Backend origin/CSRF/JWT-проверки сохранены. Отдельный hostname localhost предотвращает смешивание host-only refresh-cookie с исходной копией на 127.0.0.1.

## 4. Устранённые несовместимости

- PATCH /profile отправляет только display_name; avatar_url больше не отправляется.
- PATCH /brands/:id не отправляет logo_url/banner_url; текст, контакты и оформление сохранены.
- Аватар, логотип, баннер, добавление/замена фото PVM используют POST /media/uploads и raw File PUT по проверенному content_url. Типы включают managed media_id, завершённый upload проверяется на complete/ready.
- Заменяемая фотография сохраняет стабильный image_id, получает новый media_id. Reorder, обложка и удаление существующего PVM сохранены.
- Приватные аватары/draft-изображения получают Bearer и отображаются через очищаемый Blob URL. Anonymous-доступ к аватару проверен: 404.
- Общее чтение managed media ограничено очередью, чтобы не превышать серверный лимит одновременной выдачи. Legacy HTTPS и placeholders сохранены. Прямых запросов к приватному S3 нет.
- После avatar upload профиль перечитывается без перемонтирования формы; загрузка brand media блокируется при несохранённых текстовых полях.
- Vite использует /api с удалением префикса и /media без rewrite; публичные React routes /brands и /products не проксируются целиком.
- Существующие in-memory access JWT, HttpOnly refresh, CSRF, Origin, восстановление сессии, single-flight и межвкладочные Web Locks сохранены.

## 5. Готовые функции покупателя

| Функция               | API и поведение                                                                                                                                     |
| --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| Избранное             | GET /favorites, POST/DELETE /favorites/:product_id; активные сердца, счётчик, страница, синхронизация вкладок; недоступные товары можно удалить     |
| Корзина               | GET /cart, POST /cart/items с настоящим product_variant_id, PATCH количества, DELETE; цвет/размер, stock, server totals, пустые/ошибочные состояния |
| Проверка checkout     | Повторный GET корзины и сравнение цен/позиций; при изменении требуется повторное подтверждение, POST не отправляется                                |
| Неоплаченный заказ    | POST /orders с {}; синхронная блокировка двойного клика; резерв на 15 минут, явное отсутствие оплаты                                                |
| Неизвестный результат | При обрыве после POST нет слепого повтора; предлагается проверить историю                                                                           |
| История/detail        | GET /orders и /orders/:id, server snapshots и суммы, таймер по cancel_before                                                                        |
| Отмена                | POST /orders/:id/cancel с {}; подтверждение, серверный статус и освобождение остатка                                                                |
| Истечение             | Poll только активных резервов, focus refresh; frontend ожидает серверный expired и не меняет stock/status сам                                       |
| UX                    | Toast, loading/error/empty, guest-login с возвратом, адаптивные страницы, счётчики в header                                                         |

Покупательский код не отправляет цены/итоги, не обращается к таблицам и не изменяет статусы заказа напрямую. Только специально изолированный тест expiry с проверкой Docker label сдвигает deadline **своего нового тестового заказа**, чтобы проверить настоящий worker без 15-минутного ожидания.

## 6. Проверки

| Проверка                                      | Результат                                                                                                          |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Установка npm ci                              | Успешно; 0 известных npm vulnerabilities на момент установки                                                       |
| TypeScript и Vite build                       | Успешно                                                                                                            |
| Frontend unit                                 | 78/78, 7 файлов                                                                                                    |
| Реальные и контролируемые браузерные сценарии | 25/25: 9 auth, 9 public, 7 buyer                                                                                   |
| Go + PostgreSQL + SeaweedFS, Linux            | Финальный последовательный полный прогон: 890 passed cases, 178 top-level tests, 7 packages; 1 opt-in test skipped |
| Docker build                                  | Успешно, текущие Go-исходники                                                                                      |
| Нативный Windows Go без S3                    | Успешно                                                                                                            |
| Визуальный просмотр                           | Главная, desktop/mobile cart, создание резерва, подтверждение и реальная отмена через UI                           |
| Данные/исходники                              | Backup восстановлен; исходный проект и доменные данные проверены отдельно                                          |

Браузерные сценарии проверяют:

- регистрацию/login/reload/profile/logout/re-login, отсутствие токенов в localStorage/sessionStorage, 401/403/409 и сеть;
- реальные каталог/фильтры/пагинацию, цвет/размер/галерею, empty/loading/error, адаптивность;
- полный buyer flow, два покупателя на последнюю единицу (201/409), чужой заказ (404), quantity 0 (400);
- двойное создание, обрыв уже выполненного POST, изменение цены до оформления;
- upload/private media/PVM replacement/reorder и удалённые URL writers;
- backend expiry, запрет поздней отмены, возврат остатка, межвкладочную синхронизацию и скрытый товар.

Во время проверки выявлены особенности существующих Go-тестов: Windows + S3 тест ограничения gallery зависит от TCP buffering; первый параллельный Linux-прогон однажды получил fixture time parse error в rollback/stock. Финальный Linux-прогон с `-p 1 -count=1` прошёл весь набор. Backend ради этих тестов не менялся. Отдельный opt-in `TestSeaweedPersistenceAfterRestart` не запускался; перезапуск storage как самостоятельный сценарий не заявляется проверенным.

Логи Go: `go-validation.log`, `go-media-validation.log`, `go-linux-serial-validation.log` в новой копии, локально и вне Git. Превью ручной проверки: локальные reservation-desktop.jpg и reservation-cancelled.jpg (не входят в Git).

## 7. Изменённые файлы

| Группа                   | Файлы                                                                                                                                                                      |
| ------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Запуск/изоляция          | compose.integration.yaml, compose.frontend.yaml, .gitattributes, .dockerignore, scripts/start-integration.ps1                                                              |
| Защита исходной БД       | scripts/verify-original-data.mjs                                                                                                                                           |
| Клиент и типы            | frontend/src/lib/http.ts, client.ts, api.ts, types.ts, auth-types.ts                                                                                                       |
| Media                    | frontend/src/features/media/api.ts, UploadField.tsx, useImage.ts, media.test.ts                                                                                            |
| Buyer                    | frontend/src/features/buyer/types.ts, api.ts, store.ts, Buyer.tsx, Pages.tsx, buyer.test.ts                                                                                |
| Существующие страницы/UI | frontend/src/App.tsx, components/Layout.tsx, components/ui.tsx, pages/Auth.tsx, pages/Studio.tsx, pages/ProductEditor.tsx, features/storefront/ProductPage.tsx, styles.css |
| Vite и утилиты           | frontend/vite.config.ts, .env.example, scripts/prepare-local.mjs, scripts/start-dev.ps1, package.json                                                                      |
| Браузерные тесты         | frontend/e2e/auth.spec.ts, public.spec.ts, buyer.spec.ts, media-fixture.ts, marketplace.spec.ts, playwright*.config.ts                                                     |
| Документация             | README.md, docs/architecture.md, frontend/README.md, frontend/docs/IMPLEMENTATION_PLAN.md, INTEGRATION_REPORT.md; исторические отчёты отмечены архивными                   |

Исходный frontend перенесён целиком и развивается на прежнем стеке, без полного rewrite. Новые библиотеки не добавлялись. Изменения остаются незакоммиченными только в интеграционной копии.

## 8. Запуск и дальнейшая работа

```powershell
Set-Location 'C:\dev\SREZ'
npm ci --prefix .\frontend
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-integration.ps1
```

Открыть http://localhost:5174. Подробные команды проверок и раздельного запуска находятся в [frontend README](../README.md). Проверить сохранность исходной БД повторно можно командой `node .\scripts\verify-original-data.mjs` из нового корня: это чтение исходной БД и обновление файла доказательства в backup, без вывода данных.

Остаются отдельными задачами: реальная оплата и доставка, server idempotency key для checkout, гостевая корзина, централизованная защита несохранённых форм студии, server facets размеров/цветов, deployment hardening и opt-in restart-тест SeaweedFS. Это не препятствует проверенному локальному неоплаченному резервированию.

Исходную рабочую БД пока не переводить на новые миграции автоматически: продолжать разработку в интеграционной копии. Возврат к прежнему проекту не требует Git-операций: он сохранён в своей исходной папке со своим volume и портами.
