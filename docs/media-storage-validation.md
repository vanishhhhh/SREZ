# Media validation — Phase 2 и исправления независимого аудита

Актуальные результаты пяти исправлений, race и полного SHA-256 restart:
[отчёт исправлений](media-storage-audit-fixes.md). Ниже сохранена исходная матрица
Phase 2; её старый HEAD/GET restart test был недостаточен и заменён.

Ветка: `feat/media-storage-seaweedfs`, исходный main `f4edad2`.
Сохранено незакоммиченное состояние Phase 1; push/merge не выполнялись.
Добавлены 010 (Phase 2) и 011 (исправления GC); 001–010 не менялись при исправлении аудита.

## Обязательные проверки

| Команда / проверка | Результат |
| --- | --- |
| gofmt / gofmt -l internal cmd migrations | PASS, неформатированных Go-файлов нет |
| go test ./... -count=1 | PASS с реальными PostgreSQL и SeaweedFS |
| go vet ./... | PASS |
| git diff --check | PASS |
| docker compose config --quiet | PASS для основного и изолированного Compose |
| docker build -t srez-media-phase2-review . | PASS, linux/arm64 |
| Focused TestPostgresMediaSeaweedFlow | PASS, 0.84s |
| TestSeaweedPersistenceAfterRestart | Исходная проверка без полного body была недостаточна; заменена length + SHA-256 + ready reference |
| SeaweedFS logs: access/secret keys | PASS, ключей нет до/после restart |
| Read-only security review | Выполнен; подтверждённых открытых Critical/High нет |

Финальный изолированный проект: `srez-media-test-1794829303-67698`.
Случайные credentials, отдельные новые volumes, динамические loopback-порты и
временная PostgreSQL schema для каждого интеграционного теста. Teardown выполнил
только `down` своего проекта, без `-v`. Тестовые volumes сохранены; пользовательские
не монтировались, не мигрировались и не удалялись.

Финальные результаты пакетов:

```text
ok  srez/internal/config        0.509s
ok  srez/internal/entities      0.659s
ok  srez/internal/handler        6.976s
ok  srez/internal/objectstorage  0.520s
ok  srez/internal/security       1.721s
ok  srez/internal/storage        15.988s
ok  srez/internal/usecase        1.715s
```

cmd/app, cmd/seed, devseed и migrations не имеют собственных test-файлов;
их компиляция/использование в suite не выдаётся за отдельные unit-тесты.

## Матрица требований

| № | Сценарий | Реально выполнено |
| --- | --- | --- |
| 1 | JPEG/PNG/WebP | Реальный backend→S3 upload + SQL binding; правильные JPEG/alpha PNG выходы |
| 2 | >5 MiB до S3 | Early header и фактический oversized body: 413, Put counter не изменился, HEAD — объект отсутствует |
| 3 | Missing/false Content-Length | Реальные handler+DB/S3 негативные сценарии; chunked дополнительно unit 411 |
| 4 | Corrupt / неправильный MIME | Unit и реальный flow, без S3 записи |
| 5 | Pixel bomb / размеры | Реальный 4000×4000 PNG отклонён; 8193 side — unit, output writer cap — unit |
| 6 | Собственный avatar | Реальная привязка, profile URL, Bearer GET |
| 7 | Чужой avatar | Wrong uploader 404, guest/other GET 404, user_id mass assignment 400, SQL cross-user binding rejected |
| 8 | Logo/banner replacement | Реальные replace, старый asset больше не выдаётся, banner delete; обязательный active logo delete — 409 |
| 9 | Чужой brand | Owner чужого brand — 404; editor оформления — 403 по прежней policy |
| 10 | Revoke during operation | Реальный revoke после PUT и до финальной транзакции; 404, asset не привязан |
| 11 | Product/color image | Managed source в product_color_images, stable image_id и public detail |
| 12 | Cross-brand/product BOLA | Реальные create/upload/media_id/неверный color tests, 404 |
| 13 | Reorder | Managed и legacy регрессии, ID сохраняются |
| 14 | Последнее published фото | 409; ссылка/ready и отсутствие deletion queue подтверждены SQL |
| 15 | S3 failure / rollback | Injected Put/Delete adapter errors в реальном SQL/S3 flow: старая ссылка сохранена; DELETE retry успешен |
| 16 | SQL failure after S3 | Реальный BEFORE UPDATE failure trigger: rollback привязки, старый avatar сохранён, приватный candidate позже удалён GC |
| 17 | Повтор / конкуренция | Повтор PUT не пишет вновь; параллельные HTTP PUT дают одну S3 запись/ссылку; старый complete retry не восстанавливает заменённое фото |
| 18 | Незавершённые / cleanup | Expired processing и detached objects, DELETE failure/retry; 009→010 pending retirement и ready retention; tombstone GC |
| 19 | Private / pending delivery | Owner pending 404, avatar guest/other 404; draft brand guest 404; hidden product guest 404; anonymous S3 GET 403 |
| 20 | Auth / catalog / shopping / orders | Весь прежний suite с PostgreSQL: Auth/refresh/CSRF, RBAC/PVM, catalog, wishlist/cart, checkout/reservations/cancellation/history |

Дополнительно: unexpired presigned staging PUT заблокирован 403, положительный
signing control — 200; backend HTTP больше не возвращает presigned URLs;
URL-writer обходы закрыты; DB запрещает NULL target и ready-orphan; отдельные delivery slots
удерживаются до окончания response write; после restart проверяются HEAD, полный bounded GET, длина, SHA-256 и SQL reference.

Неоднозначный **успешный** COMMIT моделируется unit-тестом: SQL state committed,
caller получил error; retry не повторяет PUT, GC сохраняет live asset. Это не
фактический TCP-разрыв реального PostgreSQL в момент COMMIT. S3 failure здесь —
явная fault injection, а не остановка пользовательского/тестового daemon.

Единственный намеренный skip в первом полном suite — restart-only тест без
TEST_S3_VERIFY_RESTART. Скрипт затем перезапускает SeaweedFS, заново читает host
port и запускает этот тест явно: PASS. Другие service-dependent тесты получили
TEST_DATABASE_URL и TEST_S3_ENDPOINT. При обычном запуске без этих переменных
они будут skipped; такой запуск сам по себе не заменяет данный интеграционный прогон.

Логи: `/tmp/srez-phase2-final-tests.log`, `/tmp/srez-phase2-build.log`.
Команда воспроизведения: `scripts/test-media.sh`.

## Изменённые области для ревью

* Новая 010 и `media_migration_integration_test.go` — ownership, refs, lifecycle,
  контроль ready-состояния и переход с неизменённой 009.
* Media handler/usecase/storage, S3 adapter — backend stream, нормализация,
  auto-bind, private serving, legacy staging deny и GC.
* Domain User/Brand/ProductImage/ProductColor/Publication — два явных источника,
  без ослабления HTTPS URL checks.
* PVM SQL/DTO/read models — media_id, stable image_id/position, относительные URL,
  сохранённые post-write publication checks и legacy read.
* Старые profile/brand/product URL routes — запрет нового внешнего URL input.
* HTTP server — read/header/write/idle deadlines и media response slot lifetime.
* Регрессионные fixtures — исторические URL создаются доверенным setup, не через
  закрытый URL API; fixture до 010 использует историческую SQL shape.
* README, architecture, media/PVM guide и security review — актуальный контракт.

[API, модель и ограничения](media-storage.md) ·
[Read-only security review](media-storage-security-review.md).

## Финальная интеграция в Git — 2026-11-16

Перед коммитами повторно получен origin/main: он остался на f4edad2, конфликтов
не было. Проверены все 56 файлов реализации/интеграции/документации, включая
untracked; .env, credentials, логи, volumes и временные артефакты не включены.
Миграции 001–008 совпадают с origin/main; 009–011 сохранены без изменения checksum.

При повторном финальном прогоне обнаружен сбой test harness: повторный Compose up
после restart SeaweedFS пересоздал PostgreSQL и изменил динамический host port,
а restart-проверка использовала старый DSN. Скрипт исправлен: ожидание готовности
использует --no-recreate и перечитывает host endpoints обоих сервисов. Это не
изменяет production API, миграции или пользовательские данные. Полный suite,
focused regressions, race и проверка полного SHA-256 повторены: PASS.
Финальный проект: srez-media-test-1794843368-73005; файл 4 843 299 bytes,
SHA-256 до/после совпал с указанным в отчёте исправлений.

Проверены gofmt, go vet, diff --check, оба Compose config с временным тестовым env
и Docker build. Сервисы тестов изолированы; teardown только своего проекта без -v.
GitHub Actions workflows в исходном main отсутствуют: локальные результаты
не объявляются GitHub CI. Rules/status/reviews проверяются на фактическом PR до merge.

Исторические отчёты выше и media-storage-audit-fixes.md описывают состояние до
разрешения пользователем финальных commit/PR/merge; точный итог Git фиксируется в PR.
