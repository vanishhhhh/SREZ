# Product Variant Management (PVM)

PVM позволяет владельцу бренда и принятому редактору подготовить товар к публикации через защищённый HTTP API: создать цвета, добавить ссылки на фотографии, размеры и остатки. Ручной SQL и dev seed для этого не нужны.

## Маршруты и доступ

Все маршруты ниже требуют `Authorization: Bearer <access_token>`. Owner и editor должны иметь **активный membership именно бренда товара**. Invited, declined, revoked и отсутствие membership не дают доступа. Suspended-бренд запрещает управление товарами. Эти правила распространяются и на управляющие GET.

| Метод | Маршрут | Успешный ответ |
| --- | --- | --- |
| POST | `/products/:id/colors` | 201, созданный цвет |
| GET | `/products/:id/colors` | 200, массив цветов |
| PATCH | `/products/:id/colors/:color_id` | 200, изменённый цвет |
| DELETE | `/products/:id/colors/:color_id` | 204, без тела |
| POST | `/products/:id/colors/:color_id/variants` | 201, вариант |
| GET | `/products/:id/colors/:color_id/variants` | 200, массив вариантов |
| PATCH | `/products/:id/colors/:color_id/variants/:variant_id` | 200, вариант |
| DELETE | `/products/:id/colors/:color_id/variants/:variant_id` | 204, без тела |
| POST | `/products/:id/colors/:color_id/images` | 201, фотография |
| GET | `/products/:id/colors/:color_id/images` | 200, фотографии по позиции |
| PATCH | `/products/:id/colors/:color_id/images/:image_id` | 200, фотография |
| DELETE | `/products/:id/colors/:color_id/images/:image_id` | 204, без тела |
| POST | `/products/:id/colors/:color_id/images/reorder` | 200, новый порядок фотографий |

`:id` — ID товара. Все ID — положительные int64. Проверяется вся иерархия: цвет принадлежит товару, вариант/фотография — указанному цвету, товар — бренду с актуальными правами пользователя. Чужой дочерний ID не даёт доступа даже при наличии прав на другой товар того же бренда.

PVM использует существующий Bearer middleware. Cookie не заменяет заголовок Authorization. Origin/CSRF-проверки регистрации, login, refresh и logout сохранены; PVM не добавляет исключений в них. Управляющие ответы имеют `Cache-Control: no-store`.

## Форматы JSON и валидация

Тело — ровно один JSON-объект до 64 КиБ. Неизвестные поля, ID родителей, owner, status и прочие поля вне DTO запрещены. DELETE не требует тела. Пустой PATCH отклоняется. Как в существующих PATCH, `null` означает пропущенное поле; для очистки SKU передайте пустую строку. Пустые списки возвращаются как `[]`.

### Цвет

POST/PATCH:

```json
{"color":"Black"}
```

Ответ:

```json
{"id":11,"product_id":7,"color":"Black","images":[]}
```

Цвет должен быть непустым корректным UTF-8 после trim. Отображаемый регистр сохраняется. Ключ уникальности — `strings.ToLower(strings.TrimSpace(color))`: `Black` и ` BLACK ` конфликтуют внутри одного товара. В разных товарах одинаковые цвета допустимы.

Удаление цвета удаляет его фотографии и варианты **в той же транзакции**. При нарушении правил опубликованного товара удаление целиком откатывается, включая дочерние записи.

### Размер, остаток и SKU

POST требует `size` и `stock`; `sku` необязателен:

```json
{"size":"M","stock":12,"sku":"HOODIE-BLACK-M"}
```

PATCH принимает одно или несколько этих полей, например:

```json
{"stock":0}
```

Ответ:

```json
{"id":21,"product_color_id":11,"size":"M","stock":0,"sku":"HOODIE-BLACK-M"}
```

Размер — 1–20 Unicode-символов после trim. `One Size`, `44 EU` и другие обозначения разрешены. Размер уникален внутри цвета после lower/trim. Регистр отображаемого значения сохраняется. Один размер у разных цветов допустим.

Остаток — целое неотрицательное число, не превышающее вместимость Go `int` на текущей платформе; на amd64 это максимум 9223372036854775807. Переполнение JSON/int64 отклоняется, преобразование в `int` проверяется отдельно. Дробные числа и строки вместо числа запрещены. Нулевой остаток допустим и не препятствует публикации. Остаток заменяется абсолютным значением, это не операция списания или резервирования.

SKU сохраняется по действующим правилам `ProductVariant`: строка, без дополнительной нормализации, пустое значение допустимо. Глобальной уникальности SKU нет. Новые произвольные ограничения SKU не введены.

### Фотографии и порядок

POST:

```json
{"url":"https://example.com/front.jpg","position":0}
```

Ответ:

```json
{"id":31,"product_color_id":11,"url":"https://example.com/front.jpg","position":0}
```

`url` обязателен: HTTPS URL с hostname, без userinfo и пробельных символов, по существующей доменной проверке. Сервер хранит ссылку и не скачивает файл. Загрузка файлов, MinIO и S3 не реализуются. URL уникален внутри цвета; максимум 10 фотографий.

`position` — индекс в упорядоченной коллекции с нуля:

- POST без position добавляет фотографию в конец. При заданной позиции допустимо `0..n`, где n — текущее количество; существующие фотографии сдвигаются.
- PATCH меняет URL и/или позицию. Позиция допустима в `0..n-1`; перемещение на занятую позицию сдвигает соседние фотографии, а не создаёт конфликт.
- DELETE уплотняет оставшиеся позиции. Удалённые ранее пропуски в старых данных также уплотняются при изменении коллекции. Относительный порядок сохраняется.
- ID существующей фотографии не меняется при перемещении, смене URL или уплотнении.

Для перестановки всей коллекции отправьте **все существующие ID ровно по одному** в нужном порядке:

```http
POST /products/7/colors/11/images/reorder
Authorization: Bearer <access_token>
Content-Type: application/json

{"image_ids":[32,31,33]}
```

Ответ — массив фотографий с позициями 0, 1, 2. Перестановка атомарна. Дубли/неполный список — 400, чужой или отсутствующий ID — 404. Для пустой коллекции допустим `{"image_ids":[]}`.

## Опубликованный товар

После любого изменения связанного набора у published-товара usecase заново загружает **полные реальные коллекции** и вызывает `Product.ValidatePublication` до commit.

Каждый цвет должен сохранять хотя бы одну фотографию и один вариант. Должен остаться хотя бы один цвет. Остальные правила ProductPublication также сохраняются. Поэтому удаление последней фотографии, варианта или цвета возвращает 409 и откатывает операцию. Новый пустой цвет у опубликованного товара также отклоняется: он сразу нарушил бы требования публикации.

Чтобы подготовить новый цвет, явно выполните `POST /products/:id/hide` или `/unpublish` с `{}`, добавьте связанные данные, затем вызовите `/publish`. Draft/hidden допускают неполные коллекции. Автоматической смены статуса нет. Публикация требует активного бренда; готовить draft можно в draft/active-бренде. Editor может публиковать товар, но бренд публикует owner.

`updated_at` товара обновляется после успешной связанной записи. Ошибка не меняет ни данные, ни timestamp. Успешные изменения опубликованных товаров сразу видны в публичных GET, обложках, фильтрах и признаках доступности. Контракты этих GET сохранены: публичные `images` остаются массивами URL, а management-ответы содержат ID и позиции.

## Ошибки

Ответ имеет существующий формат `{"error":"описание"}`; SQL и секреты не раскрываются.

| Статус | Причина |
| --- | --- |
| 400 | Неверные ID/JSON/типы, неизвестные поля, пустой PATCH, отрицательный/переполненный stock, неправильный HTTPS URL, неправильная позиция или набор reorder |
| 401 | Нет действующего Bearer JWT |
| 403 | Бренд suspended; применяется существующая политика управления |
| 404 | Нет объекта, нет активного membership, чужой товар/цвет/вариант/фотография; invited/declined/revoked |
| 409 | Дубликат цвета/размера/URL, уже 10 фотографий, нарушение ProductPublication или конфликт состояния |
| 413 | Тело больше 64 КиБ |
| 503 | Management не настроен; штатное приложение настраивает его при старте |
| 500 | Неожиданная ошибка хранения, общий ответ без SQL |

Последний обязательный элемент опубликованного товара: `409 {"error":"resource is not ready for publication"}`. Дубликаты: `409 {"error":"state conflict"}`.

## Структуры Go, функции и слои

| Слой/файл | Назначение |
| --- | --- |
| `internal/handler/product_variants.go` | 13 маршрутов, отдельные request/response DTO, разбор всех ID, JSON и HTTP-статусы |
| `internal/usecase/product_variants.go` | Методы существующего Management: Create/Rename/DeleteColor, ProductColors, Create/Change/DeleteVariant, ProductVariants, Create/Change/DeleteImage, ProductImages, ReorderImages |
| `internal/usecase/management.go` | Существующие authorize/brand/WithinBrand; ManagementTx дополнен интерфейсом ProductVariantTx |
| `internal/entities/product_color.go` | Цвет и прежняя проверка полного списка URL |
| `internal/entities/product_variant.go` | Размер, остаток и SKU; действующие методы изменения |
| `internal/entities/product_image.go` | Новый ProductImage с закрытым состоянием: стабильный ID, цвет, URL и позиция; конструктор/восстановление/атомарные изменения |
| `internal/entities/product.go` | Прежний ValidatePublication; Touch обновляет дату связанного изменения без смены полей/статуса |
| `internal/storage/product_variants.go` | Реализация ProductVariantTx через sqlx в уже открытой brand-транзакции |

DTO — структуры для HTTP-формата. Сущности не сериализуются напрямую. Указатель в PATCH позволяет отличить отсутствие поля от `stock:0` или `sku:""`. Метод `withProduct` объединяет проверку прав и целостности, поэтому все CRUD используют одну политику.

```mermaid
sequenceDiagram
    participant C as Клиент с Bearer JWT
    participant H as Gin и PVM handler
    participant U as Management.withProduct
    participant S as ManagementTx / sqlx
    participant D as PostgreSQL
    C->>H: PATCH варианта
    H->>H: JWT, DTO, все ID
    H->>U: actor, product/color/variant ID, patch
    U->>S: WithinBrand
    S->>D: BEGIN; lock brand, membership, product, children
    U->>U: authorize, принадлежность, доменные методы
    U->>S: SaveVariant
    S->>D: UPDATE с параметрами
    U->>S: Publication (полный актуальный набор)
    U->>U: ValidatePublication для published; Touch
    U->>S: SaveProduct
    S->>D: COMMIT или ROLLBACK при ошибке
    H-->>C: DTO результата или HTTP-ошибка
```

### БД и миграция 005

Переиспользуются `products`, `product_colors`, `product_variants`, `product_color_images`; права читаются из `brand_memberships` и `brands`.

Раньше у фотографии не было отдельного ID: первичный ключ был `(product_color_id, position)`. Позиция меняется при сортировке, поэтому использовать её как image_id нельзя. `005_product_image_ids.sql` добавляет BIGINT identity с уникальностью для стабильного ID и делает прежний первичный ключ DEFERRABLE INITIALLY IMMEDIATE. Старые URL/позиции/связи сохраняются; прежние миграции не меняются.

Storage временно откладывает проверку уникальности позиций только внутри операции с фото, сохраняет полный порядок одним UPDATE через `unnest ... WITH ORDINALITY`, затем возвращает проверку в IMMEDIATE. Это позволяет поменять позиции 0 и 1 без удаления/пересоздания фотографий. Ограничения диапазона 0–9, URL и FK сохраняются. См. [PostgreSQL SET CONSTRAINTS](https://www.postgresql.org/docs/16/sql-set-constraints.html).

### Транзакции и конкурентные запросы

Порядок блокировок общий с существующим RBAC: brand → актуальный membership → product → цвета → изображения → варианты. Управление одним брендом сериализуется. Предварительный ProductBrand только находит бренд; право определяется повторно под блокировкой, а товар перечитывается с фактическим brand_id.

При отзыве editor операция либо фиксируется до revoke, либо получает 404 после него. Два удаления последних вариантов не могут оба пройти. Параллельные дубликаты дополнительно защищены UNIQUE в PostgreSQL. Ошибка post-write проверки вызывает rollback всей транзакции. Публичные чтения продолжают использовать собственный согласованный read-only snapshot и не видят промежуточного порядка.

## Ручная проверка в Windows PowerShell

### 1. Настройка и запуск

Открой PowerShell **в корне рабочей копии с этими изменениями**: рядом должны находиться `go.mod`, `compose.yaml` и `README.md`. Нужны Go 1.27.2+, Docker Desktop с Linux containers и доступный порт 8080. Docker Desktop должен показывать запущенный Engine.

Создай `.env`, если его ещё нет. Скрипт ниже генерирует два независимых ключа только для пустых строк шаблона, сохраняя уже заданные значения. Ключи не выводятся:

```powershell
if (-not (Test-Path -LiteralPath .env)) {
    Copy-Item -LiteralPath .env.example -Destination .env
}
$srezEnvLines = @(Get-Content -LiteralPath .env)
for ($i = 0; $i -lt $srezEnvLines.Count; $i++) {
    if ($srezEnvLines[$i] -match '^(SREZ_JWT_KEY|SREZ_CSRF_KEY)=[ \t]*$') {
        $srezKeyName = $Matches[1]
        $srezKeyBytes = New-Object byte[] 32
        $srezRng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
        try { $srezRng.GetBytes($srezKeyBytes) } finally { $srezRng.Dispose() }
        $srezEnvLines[$i] = $srezKeyName + '=' + [Convert]::ToBase64String($srezKeyBytes)
    }
}
Set-Content -LiteralPath .env -Value $srezEnvLines -Encoding UTF8
```

Проверь настройки шаблона, не публикуя ключи: `DATABASE_URL=postgres://srez:srez_dev@localhost:5432/srez?sslmode=disable`, issuer `srez`, audience `srez-api`, public origin `http://127.0.0.1:8080`, `SREZ_AUTH_LOCAL_HTTP=true`. Для старого volume используй фактический пароль БД. Изменение POSTGRES_PASSWORD не меняет пароль существующей роли.

Импортируй простой `.env` шаблона в текущее окно. Значения здесь записываются без кавычек; содержимое файла не исполняется:

```powershell
foreach ($line in Get-Content -LiteralPath .env) {
    if ($line -match '^([A-Z][A-Z0-9_]*)=(.*)$') {
        [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2], 'Process')
    }
}
$env:HTTP_ADDR = '127.0.0.1:8080'
docker compose up -d --wait db
docker compose stop app
go run ./cmd/app
```

Последняя команда занимает окно. В отдельном PowerShell выполни шаги ниже. Альтернатива Go на хосте: `docker compose up --build -d` поднимает app и db; отдельный `go run` тогда не нужен. Миграции применятся автоматически. Seed не нужен: создаём новый бренд с владельцем через API. У seed-брендов автоматически не появляется owner.

### 2. Owner, editor и JWT

Значения токенов оставляй в памяти окна и не выводи/не сохраняй в Git. Уникальный суффикс позволяет повторять пример без конфликтов аккаунтов:

```powershell
$api = 'http://127.0.0.1:8080'
$originHeaders = @{ Origin = $api }
$suffix = Get-Date -Format 'yyyyMMddHHmmss'
$ownerName = "pvm_$suffix"
$editorName = "pvm_editor_$suffix"
$demoPassword = 'local PVM password 123'

foreach ($name in @($ownerName, $editorName)) {
    $body = @{ email = "$name@example.com"; username = $name;
        display_name = $name; password = $demoPassword } | ConvertTo-Json
    Invoke-RestMethod -Method Post -Uri "$api/auth/register" `
        -Headers $originHeaders -ContentType 'application/json' -Body $body | Out-Null
}
$body = @{ identifier = $ownerName; password = $demoPassword } | ConvertTo-Json
$ownerLogin = Invoke-RestMethod -Method Post -Uri "$api/auth/login" `
    -Headers $originHeaders -ContentType 'application/json' -Body $body
$ownerHeaders = @{ Authorization = "Bearer $($ownerLogin.access_token)" }
$body = @{ identifier = $editorName; password = $demoPassword } | ConvertTo-Json
$editorLogin = Invoke-RestMethod -Method Post -Uri "$api/auth/login" `
    -Headers $originHeaders -ContentType 'application/json' -Body $body
$editorHeaders = @{ Authorization = "Bearer $($editorLogin.access_token)" }

$body = @{ name = 'PVM Brand'; slug = "pvm-$suffix" } | ConvertTo-Json
$brand = Invoke-RestMethod -Method Post -Uri "$api/brands" `
    -Headers $ownerHeaders -ContentType 'application/json' -Body $body
$body = @{ username = $editorName } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "$api/brands/$($brand.id)/invitations" `
    -Headers $ownerHeaders -ContentType 'application/json' -Body $body | Out-Null
Invoke-RestMethod -Method Post -Uri "$api/brands/$($brand.id)/invitations/accept" `
    -Headers $editorHeaders -ContentType 'application/json' -Body '{}' | Out-Null
```

Access живёт 15 минут. Если пример выполняется дольше, повтори login. Refresh-cookie и CSRF для Bearer PVM не нужны; для refresh/logout действует прежний auth-контракт.

### 3. Editor создаёт draft, цвет, фото и размеры

```powershell
$body = @{ brand_id = $brand.id; name = 'PVM Hoodie'; description = 'Cotton hoodie';
    price_kopecks = 250000; category = 'hoodie'; materials = @('cotton') } | ConvertTo-Json
$product = Invoke-RestMethod -Method Post -Uri "$api/products" `
    -Headers $editorHeaders -ContentType 'application/json' -Body $body
$productUrl = "$api/products/$($product.id)"
$color = Invoke-RestMethod -Method Post -Uri "$productUrl/colors" `
    -Headers $editorHeaders -ContentType 'application/json' -Body '{"color":"Black"}'
$colorUrl = "$productUrl/colors/$($color.id)"
$image = Invoke-RestMethod -Method Post -Uri "$colorUrl/images" `
    -Headers $editorHeaders -ContentType 'application/json' `
    -Body '{"url":"https://example.com/hoodie-black.jpg","position":0}'
$variant = Invoke-RestMethod -Method Post -Uri "$colorUrl/variants" `
    -Headers $editorHeaders -ContentType 'application/json' `
    -Body '{"size":"M","stock":12,"sku":"HOODIE-BLACK-M"}'
Invoke-RestMethod -Method Post -Uri "$colorUrl/variants" `
    -Headers $editorHeaders -ContentType 'application/json' `
    -Body '{"size":"One Size","stock":0}' | Out-Null
```

### 4. Публикация и чтение

Сначала owner завершает и публикует бренд, затем editor публикует готовый товар:

```powershell
$body = @{ description = 'Independent streetwear';
    logo_url = 'https://example.com/brand-logo.jpg' } | ConvertTo-Json
Invoke-RestMethod -Method Patch -Uri "$api/brands/$($brand.id)" `
    -Headers $ownerHeaders -ContentType 'application/json' -Body $body | Out-Null
Invoke-RestMethod -Method Post -Uri "$api/brands/$($brand.id)/publish" `
    -Headers $ownerHeaders -ContentType 'application/json' -Body '{}' | Out-Null
Invoke-RestMethod -Method Post -Uri "$productUrl/publish" `
    -Headers $editorHeaders -ContentType 'application/json' -Body '{}' | Out-Null
Invoke-RestMethod -Uri $productUrl | ConvertTo-Json -Depth 10
Invoke-RestMethod -Uri "$api/products?brand_id=$($brand.id)&color=black&size=M"
```

### 5. Остатки, редактирование и перестановка

```powershell
Invoke-RestMethod -Method Patch -Uri "$colorUrl/variants/$($variant.id)" `
    -Headers $editorHeaders -ContentType 'application/json' -Body '{"stock":0}' | Out-Null
Invoke-RestMethod -Uri $productUrl | ConvertTo-Json -Depth 10
# M больше не совпадает с фильтром; One Size тоже имеет нулевой остаток.
Invoke-RestMethod -Uri "$api/products?brand_id=$($brand.id)&size=M"

$secondImage = Invoke-RestMethod -Method Post -Uri "$colorUrl/images" `
    -Headers $editorHeaders -ContentType 'application/json' `
    -Body '{"url":"https://example.com/hoodie-back.jpg"}'
$body = @{ image_ids = @($secondImage.id, $image.id) } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "$colorUrl/images/reorder" `
    -Headers $editorHeaders -ContentType 'application/json' -Body $body
Invoke-RestMethod -Method Patch -Uri "$colorUrl/images/$($image.id)" `
    -Headers $editorHeaders -ContentType 'application/json' -Body '{"position":0}'
Invoke-RestMethod -Uri $productUrl | ConvertTo-Json -Depth 10
```

Для отрицательной проверки попробуй удалить оба изображения по очереди: первое удалится с 204, второе вернёт 409. Без Authorization маршруты управления возвращают 401. Другой зарегистрированный пользователь без membership получает 404. После отзыва editor owner-запросом `DELETE /brands/:id/members/:user_id` прежний JWT editor также получает 404.

Остановка Go — Ctrl+C. Обычный `docker compose down` сохраняет volume. Не используй `down -v` и не удаляй данные ради миграции.

## Автоматические проверки

Без `TEST_DATABASE_URL` PostgreSQL-тесты пропускаются, это не подтверждение интеграционной проверки. Нужна отдельная тестовая PostgreSQL, не рабочая БД. Тесты создают и удаляют только собственные уникальные схемы.

```powershell
# Пример отдельного контейнера, если имя и порт свободны.
docker run --rm -d --name srez-pvm-tests -e POSTGRES_USER=pvm_test `
    -e POSTGRES_PASSWORD=pvm_test_local -e POSTGRES_DB=pvm_test `
    -p 127.0.0.1:15432:5432 postgres:16-alpine
# Дождись "accepting connections"; при ошибке запуска контейнера не продолжай.
docker exec srez-pvm-tests pg_isready -h 127.0.0.1 -U pvm_test -d pvm_test
$env:TEST_DATABASE_URL = 'postgres://pvm_test:pvm_test_local@localhost:15432/pvm_test?sslmode=disable'
go test ./... -count=1
go vet ./...
git diff --check
# Только контейнер, созданный для этого примера; рабочий Compose не останавливается.
docker stop srez-pvm-tests
Remove-Item Env:TEST_DATABASE_URL
```

Новые тесты проверяют DTO/ID/JSON/JWT, доменные ошибки, нормализацию и дубликаты, нулевой stock, полный HTTP-путь owner/editor без SQL-подготовки товара, BOLA, invited/declined/revoked, rollback последних элементов, сохранение старых фотографий при миграции, конкурентную уникальность/удаление/reorder и ожидающий запрос editor после revoke. Прежний query-count тест продолжает проверять три SQL-запроса публичного каталога.

## Ограничения и развитие

Сохраняются общие ограничения Auth/RBAC из architecture.md: JWT не отзывается немедленно при logout, операции одного бренда сериализуются, production HTTP/DB timeouts и отдельное AppSec-ревью остаются следующими задачами. Смена stock — абсолютная запись; параллельные корректные записи сериализуются, более поздняя заменяет значение. Резервов, списаний, корзины и заказов нет.

При будущем развитии сохраняй единую транзакционную границу и порядок блокировок. У загрузки файлов должен быть отдельный адаптер, но подтверждённый URL должен проходить текущие доменные проверки. При появлении ссылок корзины/заказов на variant_id нужно отдельно определить политику удаления вариантов и исторических данных; сейчас таких таблиц нет. Не добавляй обход прав через служебный флаг и не меняй публичные DTO без согласованного контракта.

## Фактическая проверка реализации

Проверено 14 ноября 2026 года на актуальной базе кода `e968e35`, в локальной ветке `feature/product-variant-management`:

- `go test -v -count=1 -timeout 180s ./...` с отдельной временной PostgreSQL 16: все пакеты прошли, PostgreSQL-тесты не пропущены (116 тестовых функций верхнего уровня и их подслучаи).
- `go vet ./...` и сборка `cmd/app`/`cmd/seed`: прошли.
- `git diff --check`, gofmt изменённых Go-файлов и локальные ссылки документации: проверены.
- Docker-образ собран; отдельные временные app/db и сеть проверены реальными HTTP-запросами из PowerShell: регистрация owner/editor, приглашение, полный draft-агрегат, публикация, публичная карточка, reorder/обложка, stock/фильтр, отказ чужому пользователю и editor после revoke.

Тестовые контейнеры и временная сеть удалены после проверки; рабочие данные/volumes не изменялись. Проверка выполнена локально, публичного развёртывания приложения нет.

## Полный список изменённых файлов

Новые файлы:

- `migrations/005_product_image_ids.sql`
- `internal/entities/product_image.go`
- `internal/entities/product_image_test.go`
- `internal/usecase/product_variants.go`
- `internal/usecase/product_variants_test.go`
- `internal/storage/product_variants.go`
- `internal/storage/product_variants_integration_test.go`
- `internal/handler/product_variants.go`
- `internal/handler/product_variants_test.go`
- `internal/handler/product_variants_integration_test.go`
- `docs/product-variant-management.md`

Обновлённые файлы:

- `internal/entities/product.go` — Touch для даты связанного изменения.
- `internal/usecase/management.go` — ProductVariantTx в существующей транзакционной границе.
- `internal/handler/management.go` — регистрация PVM через прежний middleware.
- `internal/storage/catalog_integration_test.go` — ожидаемое число миграций 5.
- `internal/storage/management_integration_test.go` — монотонное время в прямой SQL-фикстуре revoke, чтобы часы PostgreSQL VM не нарушали CHECK относительно времени Go.
- `README.md`
- `docs/architecture.md`
- `docs/catalog-api.md`
- `internal/entities/README.md`
