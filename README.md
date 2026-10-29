# SREZ — учебный HTTP-сервер

Учебный сервер на Go и Gin. Помимо исходного списка проектов и моковых ручек пользователей, доступны проверка HTTP-процесса и моковый каталог брендов и товаров. Доменные сущности находятся в `internal/entities`, HTTP-запросы и ответы используют отдельные DTO.

## Структура

```text
cmd/app/main.go                    точка входа: конфигурация и запуск
internal/config/config.go          HTTP_ADDR и адрес по умолчанию
internal/handler/handler.go        Gin, маршруты, обработчик и запуск HTTP
internal/handler/handler_test.go   проверка HTTP-ответа
internal/handler/user.go           моковые ручки пользователей
internal/handler/user_test.go      проверки ручек пользователей
internal/handler/health.go         проверка HTTP-процесса
internal/handler/product.go        моковые ручки товаров
internal/handler/brand.go          моковые ручки брендов
internal/handler/catalog_dto.go    DTO каталога и преобразование сущностей
internal/handler/catalog_mock.go   фиксированные доменные примеры
internal/handler/catalog_test.go   HTTP-тесты каталога и health
internal/entities/                сущности, валидация и доменные тесты
internal/usecase/project.go        объявление пакета без логики
internal/storage/project.go        объявление пакета без логики
Dockerfile                        сборка контейнерного образа
SPEC.md                           исходный учебный пример каталога проектов
```

## Локальный запуск

Требуется Go 1.27.2 или новее. Команды ниже предназначены для PowerShell и выполняются из корня проекта.

```powershell
go run ./cmd/app
```

Проверка работающего сервера из второго терминала:

```powershell
curl.exe -i http://localhost:8080/projects
```

Альтернативный способ проверки — переход по адресу `http://localhost:8080/projects` в браузере. Ожидаемый статус — `200`, тело ответа:

```json
[{"id":1,"title":"Дело"},{"id":2,"title":"Витрина"}]
```

Порядок полей `id` и `title` внутри объекта не влияет на смысл JSON.

Остановка сервера выполняется сочетанием `Ctrl+C` в терминале, в котором он запущен.

## Ручки пользователей

Это заглушки без базы данных, регистрации, аутентификации и проверки прав.
Все данные вымышленные. Каждый запрос получает свежий доменный `entities.User`;
`POST` и `PUT` возвращают пример ответа, но ничего не сохраняют.
`GET /profile` — временный тестовый собственный профиль **без авторизации**:
он всегда показывает пользователя `1`, не определяя текущего пользователя.
JWT и сессии отсутствуют.

| Метод | Путь | Ответ |
| --- | --- | --- |
| GET | `/users/1` | `200`, публичный DTO: `id`, `username`, `display_name`, `avatar_url` |
| POST | `/users` | `201`, публичный DTO с условным `id: 2` |
| PUT | `/users/1` | `200`, обновлённый публичный DTO с `id: 1` |
| GET | `/profile` | `200`, `id`, `email`, `username`, `display_name`, `avatar_url` |

Для `GET /users/:id` и `PUT /users/:id` доступен только тестовый пользователь `1`.
ID должен быть положительным int64. Другой положительный ID даёт `404`,
некорректное значение или переполнение — `400`. Ошибки имеют вид `{"error":"описание"}`.

`POST /users` принимает только `email`, `username`, `display_name`:

```json
{"email":"roman@example.com","username":"roman_1","display_name":"Роман"}
```

Обработчик вызывает `entities.NewUser`. Email проверяется доменом, username
содержит 3–30 латинских букв, цифр или underscore; display_name обязателен,
краевые пробелы обрезаются. Внутри используется фиксированный тестовый
PasswordHash только для демонстрации доменной модели. Это не настоящая
регистрация: `password` и `password_hash` в запросе запрещены. Хеш никогда
не возвращается; email присутствует только в ответе `/profile`.

`PUT /users/:id` допускает только `display_name` и `avatar_url`, одно или оба:

```json
{"display_name":"Рома","avatar_url":"https://example.com/avatar.png"}
```

Пропущенное поле сохраняет значение тестового пользователя. Пустая строка
`avatar_url` удаляет аватар; непустой URL должен быть HTTPS. Пустой display_name
после обрезки пробелов запрещён. Используются методы `ChangeDisplayName` и
`ChangeAvatar`; смены email, username и пароля нет. Пустое обновление даёт `400`.
Если одна из проверок не прошла, ответ с частично обновлённым DTO не возвращается.

POST и PUT требуют один JSON-объект: неизвестные поля, неверные типы,
ошибки JSON, дополнительные объекты и доменные ошибки дают `400`.
Старое поле `name` больше не поддерживается. Для PUT сначала проверяются ID
и наличие тестового пользователя, затем тело запроса.

Каждый POST возвращает условный ID `2` только в DTO; новая сущность сохраняет
ID `0`. Последующий `GET /users/2` вернёт `404`. После любого POST или PUT
`GET /users/1`, `GET /profile` и следующее обновление снова используют исходный
пример. Уникальность email и username не проверяется без storage/usecase.

Проверка в PowerShell при запущенном сервере:

```powershell
curl.exe -i http://localhost:8080/users/1
curl.exe -i http://localhost:8080/profile

$createUserBody = @{ email = "roman@example.com"; username = "roman_1"; display_name = "Роман" } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri http://localhost:8080/users -ContentType "application/json; charset=utf-8" -Body ([System.Text.Encoding]::UTF8.GetBytes($createUserBody))
$updateUserBody = @{ display_name = "Рома"; avatar_url = "" } | ConvertTo-Json
Invoke-RestMethod -Method Put -Uri http://localhost:8080/users/1 -ContentType "application/json; charset=utf-8" -Body ([System.Text.Encoding]::UTF8.GetBytes($updateUserBody))
```

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

## Docker

Для сборки и запуска требуется работающий Docker Desktop с готовым движком Linux-контейнеров. При первой сборке загружаются базовый образ и зависимости; требуется подключение к интернету.

```powershell
docker build -t srez:lesson3 .
docker run --rm --name srez-lesson3 -p 8080:8080 srez:lesson3
```

Для запуска контейнера порт 8080 должен быть свободен. Если порт занят локальным сервером, его необходимо остановить. Проверка ответа контейнера выполняется из другого терминала:

```powershell
curl.exe -i http://localhost:8080/projects
```

Остановка контейнера:

```powershell
docker stop srez-lesson3
```

Dockerfile собирает Linux-бинарник в первом этапе, а во второй образ копирует только готовую программу. Базовая версия Go совпадает с `go.mod`. Порт контейнера — 8080; переменную `HTTP_ADDR` можно переопределить через `docker run -e`.

## Проверки

```powershell
go test ./...
go vet ./...
go build -o ./bin/srez.exe ./cmd/app
```

Тесты проверяют проекты, существующие User endpoints, health и каталог:
HTTP-статусы, JSON-контракты, ошибки параметров и тела запроса, доменную валидацию
и отсутствие сохранения после POST. Доменные сущности проверяются отдельно.

## Путь запроса

`main` читает конфигурацию → `handler.New()` регистрирует маршрут → `Run` начинает слушать адрес → `GET /projects` вызывает `GetProjects` → обработчик отправляет JSON.

На этом этапе данные зашиты в обработчике. Полноценная бизнес-логика, хранение
и авторизация относятся к следующим этапам отдельно ведущегося ТЗ маркетплейса;
`SPEC.md` сохраняет описание исходного учебного примера.
