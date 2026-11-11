# Отчёт: PostgreSQL для каталога SREZ

Дата: 11 ноября 2026 года. Ветка `feat/catalog-postgres` создана от актуального `origin/main`, HEAD `d9ec9f94a6e0f2a0d4015bf0d780d7ef042d4579`. Перед началом рабочее дерево было чистым. Commit, push и merge не выполнялись, удалённый репозиторий не менялся.

## 1. Что реализовано

- Публичные GET /brands, /brands/:slug, /products и /products/:id работают с PostgreSQL.
- Списки и карточки скрывают неопубликованные/заблокированные бренды и draft/hidden товары.
- POST /products создаёт реальный draft через NewProduct и возвращает identity ID БД с HTTP 201. Проверяются существование и активность бренда; FK и повторная проверка под блокировкой защищают сохранение.
- Запись выключена по умолчанию (403); включается точным SREZ_DEV_CATALOG_WRITE=true только для локальной разработки.
- Сохранены DTO, строгая обработка JSON, лимит 64 КиБ, HTTP-валидация ID и slug. В ответах нет SQL-деталей.
- Добавлены версионируемые миграции, явный идемпотентный dev seed, Compose-настройки и инструкция для Mac.
- Новых зависимостей нет. Доменные сущности, User usecase/storage/handler и 001_users.sql не изменены. /profile остаётся фиксированной заглушкой.

## 2. Файлы

Созданы:

```text
.env.example
cmd/seed/main.go
internal/config/config_test.go
internal/devseed/data.go
internal/devseed/seed.go
internal/handler/catalog_fixtures_test.go
internal/handler/catalog_repository_test.go
internal/storage/brand.go
internal/storage/product.go
internal/storage/catalog_integration_test.go
internal/usecase/catalog.go
internal/usecase/catalog_test.go
migrations/002_catalog.sql
migrations/migrate.go
docs/catalog-postgres-report.md
```

Изменены:

```text
.dockerignore
.gitignore
Dockerfile
README.md
cmd/app/main.go
compose.yaml
internal/config/config.go
internal/handler/brand.go
internal/handler/product.go
internal/handler/handler.go
internal/handler/catalog_test.go
```

Удалён runtime-файл `internal/handler/catalog_mock.go`: демонстрационные данные перенесены в явный dev seed; HTTP-тесты используют отдельные тестовые адаптеры. Моков каталога в production-пути нет.

## 3. Архитектура

Handler → usecase → интерфейсы репозиториев → sqlx storage → PostgreSQL. Handler занимается HTTP, DTO и статусами. Usecase создаёт сущность NewProduct, проверяет бренд, координирует репозитории. Entities сохраняют валидацию/публикацию. Storage выполняет параметризованный SQL и Restore-конструкторы.

InsertDraft проверяет бренд повторно в транзакции с FOR SHARE: смена статуса/удаление бренда блокируется до завершения вставки. Подробная карточка загружается четырьмя пакетными запросами в REPEATABLE READ READ ONLY, включая полный набор цветов, отсортированных по позиции изображений и вариантов, связанных через ID цвета. Проверяется ValidatePublication. Список использует только собственные поля товара.

## 4. Схема и миграции

Пять таблиц: brands, products, product_colors, product_color_images, product_variants. BIGINT identity PK, FK без каскадного удаления, BIGINT-копейки и остатки, TIMESTAMPTZ, TEXT[] materials. Contacts/appearance разложены на колонки. CHECK защищают статусы, категории, неотрицательные значения, порядок дат и позиции изображений 0–9. Уникальны непустой slug, нормализованный цвет в товаре, размер в цвете, URL/позиция изображения в цвете. Индексы покрывают FK и публичные выборки.

color_key/size_key вычисляются в Go через strings.ToLower(strings.TrimSpace(value)), как в домене. Это исключает несовпадение локали PostgreSQL с Unicode-правилами Go; будущие писатели должны применять то же правило. Прямой SQL обязан передавать правильные ключи. Доменные проверки строк/публикации не переписаны целиком в SQL.

Минимальный runner использует существующий sqlx и стандартный go:embed, без отдельного сервиса/CLI-зависимости. При старте до HTTP получает транзакционную advisory lock, проверяет журнал schema_migrations и SHA-256, применяет недостающие файлы лексикографически и фиксирует SQL вместе с журналом одной транзакцией. Повторный запуск безопасен; ошибка откатывает новый набор; изменение применённого файла или версия задним числом блокирует запуск.

001_users.sql сохранена и использует CREATE TABLE IF NOT EXISTS: поддерживается старая users-таблица без журнала миграций. Initdb-монтирование убрано из Compose, поэтому новые миграции работают и на старых volumes.

## 5. Запуск на Mac

Из корня проекта при работающем Docker Desktop:

```sh
cp .env.example .env  # если .env ещё нет
docker compose up --build -d
docker compose run --rm --no-deps -e SREZ_DEV_SEED=true --entrypoint /seed app
curl http://localhost:8080/brands
curl http://localhost:8080/products
```

Для Go/GoLand используйте DATABASE_URL с localhost:5432; в Compose URL собирается автоматически с db:5432. Подробные команды, включение временного POST и оговорка о пароле существующего volume — в README. Образы проверены на linux/arm64. Остановка с сохранением данных: docker compose down без флагов удаления volumes.

## 6. Действительно выполненные проверки

Успешно:

- go test ./...; отдельно полный прогон с TEST_DATABASE_URL, включающий PostgreSQL-тесты без skip.
- go vet ./...
- git diff --check
- docker compose config --quiet с POSTGRES_PASSWORD из .env.example (локальный .env не создавался).
- Docker Compose build/up на Apple Silicon, PostgreSQL 16 и app.
- Миграции на чистой схеме и старой users-схеме с контрольной записью, повторный/параллельный запуск, сохранность пользователя, checksum и rollback при ошибке DDL.
- Повторный seed без дубликатов; пропуск существующего slug без изменения его данных.
- FK, UNIQUE, CHECK, связи и порядок картинок, сохранение нулевого остатка, фильтрация draft/hidden и неактивных брендов, ошибки brand_id, повторная проверка статуса при вставке.
- HTTP: бренды и полные карточки, ID/slug, некорректный JSON/поля, тело >64 КиБ, два разных настоящих ID для draft и их отсутствие в публичном каталоге.
- HTTP User CRUD: list/get/create/update/delete, 409 username, 404 после удаления созданного тестом пользователя, заглушка /profile.
- Пересоздание тестовых контейнеров через down/up с сохранением volume: каталог, два draft и обновлённый пользователь сохранились; миграции не дублировались.
- Обычный запуск без флага записи: HTTP POST /products вернул 403.

Все изменяющие данные проверки выполнялись в отдельном проекте srez-catalog-review, на портах 15432/18080, с собственным volume srez-catalog-review_postgres_data. Интеграционные тесты создавали и очищали только свои уникальные схемы catalog_test_*. Существующие пользовательские базы/volumes не затрагивались. Проверка обновления старой схемы воспроизведена в изолированном окружении; реальная пользовательская БД намеренно не изменялась. Временные тестовые скрипты/Compose override находятся в /tmp, в репозиторий не добавлены. После проверки тестовые контейнеры остановлены; их volume и тестовые данные сохранены.

Начальный запуск Go в sandbox не смог записать cache/скачать модули; проверки затем успешно выполнены с разрешённым доступом. Docker также проверялся с разрешённым доступом к Docker Desktop socket.

## 7. Оставшиеся ограничения

Нет регистрации, JWT, сессий, RBAC/BrandMembership, управления брендами, редактирования или публикации товаров через HTTP. Seed пропускает весь существующий slug, не восстанавливает изменённые вручную примеры. URL демонстрационные. /health проверяет процесс, не готовность БД. Миграции только вперёд и только транзакционный DDL; startup timeout 30 секунд. DTO/контракты пользователей не менялись.

## 8. AppSec

- Существующий User CRUD не защищён авторизацией: возможно произвольное изменение/удаление пользователей при сетевой доступности. Это ограничение сохранено по условиям задачи, зафиксировано в README.
- Dev POST не проверяет владение брендом: флаг не является авторизацией. Он выключен по умолчанию; включать только локально. brand_id не доказывает права.
- /profile — фиксированный пример, не личность текущего посетителя.
- Локальный Compose использует роль владельца БД и SSL disabled. Production потребует TLS/секретов и разделения DDL-роли миграций и минимально привилегированной runtime-роли.
- Нормализованные ключи контролирует писатель; прямую запись SQL необходимо ограничить.

## 9. Следующий этап

Реализовать аутентификацию и настоящий /profile, затем проверки прав/BrandMembership; после них добавить сценарии управления брендами, редактирования и публикации полного агрегата товара в транзакциях. Подготовить production-конфигурацию ролей/секретов/TLS. Эти возможности в текущую задачу не включены.
