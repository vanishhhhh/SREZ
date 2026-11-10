# SREZ — учебный HTTP-сервер

Учебный сервер на Go и Gin. Ручки пользователей работают через PostgreSQL. Также доступны исходный список проектов, проверка HTTP-процесса и тестовый каталог брендов и товаров. Доменные сущности находятся в `internal/entities`, HTTP-запросы и ответы используют отдельные DTO.

## Структура

```text
cmd/app/main.go                    точка входа: конфигурация и запуск
internal/config/config.go          HTTP_ADDR и DATABASE_URL
internal/handler/handler.go        Gin, маршруты, обработчик и запуск HTTP
internal/handler/handler_test.go   проверка HTTP-ответа
internal/handler/user.go           HTTP-обработчики пользователей
internal/handler/user_test.go      проверки ручек пользователей
internal/handler/health.go         проверка HTTP-процесса
internal/handler/product.go        моковые ручки товаров
internal/handler/brand.go          моковые ручки брендов
internal/handler/catalog_dto.go    DTO каталога и преобразование сущностей
internal/handler/catalog_mock.go   фиксированные доменные примеры
internal/handler/catalog_test.go   HTTP-тесты каталога и health
internal/entities/                сущности, валидация и доменные тесты
internal/usecase/user.go           сценарии работы с пользователями
internal/storage/user.go           запросы пользователей к PostgreSQL через sqlx
migrations/001_users.sql           создание таблицы пользователей
compose.yaml                       Go-сервер и PostgreSQL для локальной разработки
Dockerfile                        сборка контейнерного образа
SPEC.md                           исходный учебный пример каталога проектов
```

## Запуск через Docker Compose

Из корня проекта при запущенном Docker Desktop:

```powershell
docker compose up --build -d
curl.exe -i http://localhost:8080/health
curl.exe -i http://localhost:8080/users
```

Compose запускает PostgreSQL и Go-сервер. Сервер получает `DATABASE_URL` из Compose и запускается после готовности базы. При первом создании тома PostgreSQL применяет `migrations/001_users.sql`. Пустая таблица пользователей возвращается как `[]`.

Для остановки сервера и базы с сохранением данных:

```powershell
docker compose down
```

## Запуск Go из GoLand

Для отладки вне контейнера нужен Go 1.27.2 или новее. В PowerShell из корня проекта:

```powershell
docker compose up -d --wait db
$env:DATABASE_URL = "postgres://srez:srez_dev@localhost:5432/srez?sslmode=disable"
go run ./cmd/app
```

Для запуска из конфигурации GoLand нужно задать тот же `DATABASE_URL` в **Environment variables**. Сервер требует доступную базу при старте. Если на порту 8080 уже запущен сервис `app`, его можно остановить командой `docker compose stop app`.

## Ручки пользователей

Все маршруты `/users` используют PostgreSQL. `GET /profile` пока возвращает демонстрационный профиль, так как аутентификация ещё не добавлена.

| Метод | Путь | Ответ |
| --- | --- | --- |
| GET | `/users` | `200`, массив публичных DTO в порядке ID |
| GET | `/users/:id` | `200`, публичный DTO: `id`, `username`, `display_name`, `avatar_url` |
| POST | `/users` | `201`, публичный DTO с ID записи из PostgreSQL |
| PUT | `/users/:id` | `200`, обновлённый публичный DTO |
| DELETE | `/users/:id` | `204`, запись удалена, тело ответа пустое |
| GET | `/profile` | `200`, `id`, `email`, `username`, `display_name`, `avatar_url` |

ID должен быть положительным int64. Отсутствующий пользователь даёт `404`, некорректный ID или переполнение — `400`. Ошибки имеют вид `{"error":"описание"}`.

`POST /users` принимает только `email`, `username`, `display_name`:

```json
{"email":"roman@example.com","username":"roman_1","display_name":"Роман"}
```

Сценарий создания вызывает `entities.NewUser` для проверки email, `username` и `display_name`. В PostgreSQL `username` должен быть уникальным. Повторное создание с тем же именем возвращает `409`. Создаётся запись профиля с маркером отключённого пароля `!`. Аутентификация пока отсутствует, поля `password` и `password_hash` в запросе запрещены. Публичный DTO исключает email и пароль.

`PUT /users/:id` допускает только `display_name` и `avatar_url`, одно или оба:

```json
{"display_name":"Рома","avatar_url":"https://example.com/avatar.png"}
```

Пропущенные поля сохраняются. Пустая строка в `avatar_url` удаляет аватар, а непустой URL должен использовать HTTPS. Пустое обновление, неверные поля или неверный JSON дают `400`.

Проверка CRUD в PowerShell при запущенном сервере:

```powershell
$base = "http://localhost:8080"
$body = @{ email = "roman@example.com"; username = "roman_1"; display_name = "Роман" } | ConvertTo-Json
$user = Invoke-RestMethod -Method Post -Uri "$base/users" -ContentType "application/json; charset=utf-8" -Body ([System.Text.Encoding]::UTF8.GetBytes($body))
$card = "$base/users/$($user.id)"
Invoke-RestMethod -Method Get -Uri "$base/users"
Invoke-RestMethod -Method Get -Uri $card
$update = @{ display_name = "Рома" } | ConvertTo-Json
Invoke-RestMethod -Method Put -Uri $card -ContentType "application/json; charset=utf-8" -Body ([System.Text.Encoding]::UTF8.GetBytes($update))
Invoke-RestMethod -Method Delete -Uri $card
curl.exe -i $card
```

Последний GET возвращает `404`.

`SPEC.md` относится к исходному учебному примеру каталога проектов. Актуальное ТЗ маркетплейса SREZ ведётся отдельно; `SPEC.md` не является его актуальной спецификацией.

## Бренды, товары и health

| Метод | Путь | Ответ |
| --- | --- | --- |
| GET | `/health` | `200`, `{"status":"ok"}` |
| GET | `/products` | `200`, JSON-массив двух фиксированных опубликованных товаров |
| GET | `/products/:id` | `200`, подробная карточка товара с ID `1` или `2`, цветами, фото и размерами |
| POST | `/products` | `201`, пример черновика с условным ID `3` |
| GET | `/brands` | `200`, JSON-массив двух фиксированных активных брендов |
| GET | `/brands/:slug` | `200`, бренд `srez-studio` или `linea` |

`GET /products/:id` принимает положительный int64: некорректный ID, ноль,
отрицательное значение или переполнение дают `400`, неизвестный положительный
ID — `404`. Slug должен содержать 3–60 строчных латинских букв, цифр или дефисов:
нарушение формата даёт `400`, корректный отсутствующий slug — `404`.
Ошибки новых маршрутов имеют вид `{"error":"описание"}`.

Товар в ответе содержит `id`, `brand_id`, `name`, `description`, `price_kopecks`,
`category`, `materials`, `status`, `created_at`, `updated_at`. `materials` всегда
возвращается массивом, включая пустой `[]` у черновика. Бренд содержит `id`,
`name`, `slug`, `description`, `logo_url`, `banner_url`, публичные `contacts`
(`email`, `telegram_url`, `website_url`), `appearance` (`accent_color`,
`background_color`, `text_color`), `status`, `created_at`, `updated_at`.
Даты передаются в RFC 3339. `GET /products` остаётся кратким списком без галерей
и вариантов. `GET /products/:id` дополнительно возвращает массив `colors`:
у каждого цвета есть `id`, `color`, `images` (массив HTTPS URL) и `variants`.
Каждый вариант содержит `id`, `size`, `stock`, `sku` и вложен только в свой цвет.
В фиксированных карточках по два цвета с разными галереями; чёрный содержит
размеры M и L, белый — One Size. Нулевой остаток L возвращается как `stock: 0`.
Карточка использует те же доменные моки, которые проверяются при публикации;
проверки правил публикации остаются в entities.
Сущности, membership и данные учётных записей напрямую не сериализуются.

`POST /products` принимает один JSON-объект размером до **64 КиБ (65536 байт)**
включительно. При превышении возвращается `413` с JSON-ошибкой. Лимит проверяется
по фактически прочитанным байтам, включая краевые пробелы, в том числе без
Content-Length. Некорректный запрос в пределах лимита по-прежнему даёт `400`.
Обязательны положительный `brand_id`
и `name` длиной 2–120 Unicode-символов после обрезки краевых пробелов.
Допустимы также:

* `description` — строка до 5000 символов;
* `price_kopecks` — целое int64, не меньше нуля, по умолчанию `0`;
* `category` — пустая строка либо `tshirt`, `hoodie`, `pants`, `outerwear`,
  `footwear`, `accessories`, `other`;
* `materials` — до 10 непустых строк без повторений после обрезки краевых пробелов.

Черновик может не содержать описания, категории и материалов. Обработчик вызывает
`entities.NewProduct`; нормализация и ограничения полей определяются доменом.
Некорректный JSON, типы полей, доменные значения, неизвестные поля и дополнительный
JSON после первого объекта дают `400`. ID, статус и даты нельзя задать запросом.
Настройки декодирования существующих User endpoints не изменены.

Пример создания:

```powershell
$productBody = @{
    brand_id = 1
    name = "Футболка"
    description = "Базовая хлопковая футболка"
    price_kopecks = 250000
    category = "tshirt"
    materials = @("хлопок")
} | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri http://localhost:8080/products -ContentType "application/json; charset=utf-8" -Body ([System.Text.Encoding]::UTF8.GetBytes($productBody))

curl.exe -i http://localhost:8080/health
curl.exe -i http://localhost:8080/products
curl.exe -i http://localhost:8080/products/1
curl.exe -i http://localhost:8080/brands
curl.exe -i http://localhost:8080/brands/srez-studio
curl.exe -i http://localhost:8080/products/3
```

Последний запрос вернёт `404`, в том числе после успешного POST. Каждый POST
возвращает тот же условный ID `3` только в DTO ответа: новая сущность остаётся
с ID `0`, ничего не сохраняется. Статус всегда `draft`; дата создания берётся
из текущего времени. Списки и отдельные GET-объекты после POST не меняются.
Их данные и даты фиксированы, URL изображений и контакты демонстрационные.

Здесь нет БД, in-memory storage, авторизации и проверки прав. POST проверяет
только локальные правила товара: существование и активность бренда по `brand_id`
пока не проверяются, поэтому любой положительный ID бренда допустим. Это останется
за будущим usecase вместе с сохранением и межобъектными операциями.
`/health` проверяет доступность HTTP-процесса, не подключение к внешним сервисам.

## Другой порт

```powershell
$env:HTTP_ADDR = ":8081"
go run ./cmd/app
```

Адрес проверки: `http://localhost:8081/projects`. Чтобы вернуть адрес по умолчанию после остановки сервера:

```powershell
Remove-Item Env:HTTP_ADDR
```

## Проверки

```powershell
go test ./...
go vet ./...
go build -o ./bin/srez.exe ./cmd/app
```

Автотесты используют тестовое хранилище и проверяют HTTP-ответы и правила доменных сущностей. Полный цикл через PostgreSQL можно проверить командами выше.

## Путь запроса

`main` читает конфигурацию и собирает хранилище, сценарии и HTTP-обработчик. При `GET /users/:id` обработчик разбирает ID, `usecase` запрашивает пользователя у хранилища, а `storage` выполняет `GetContext` с параметром `$1`. Обработчик формирует JSON и выбирает HTTP-статус. Для POST сценарий проверяет данные через `entities.NewUser`, а PostgreSQL атомарно проверяет уникальность `username` при вставке.
