# SREZ — HTTP-сервер занятия №3

Учебный сервер на Go и Gin. `GET /projects` возвращает фиксированный список проектов из обработчика.

## Структура

```text
cmd/app/main.go                    точка входа: конфигурация и запуск
internal/config/config.go          HTTP_ADDR и адрес по умолчанию
internal/handler/handler.go        Gin, маршруты, обработчик и запуск HTTP
internal/handler/handler_test.go   проверка HTTP-ответа
internal/usecase/project.go        объявление пакета без логики
internal/storage/project.go        объявление пакета без логики
Dockerfile                        сборка контейнерного образа
SPEC.md                           черновик ТЗ
```

## Локальный запуск

Нужен Go 1.27.2 или новее. Команды ниже предназначены для PowerShell. Выполняйте их из корня проекта.

```powershell
go run ./cmd/app
```

Пока сервер работает, открой во втором терминале:

```powershell
curl.exe -i http://localhost:8080/projects
```

Или открой `http://localhost:8080/projects` в браузере. Ожидаемый статус — `200`, тело ответа:

```json
[{"id":1,"title":"Дело"},{"id":2,"title":"Витрина"}]
```

Порядок полей `id` и `title` внутри объекта не влияет на смысл JSON.

Для остановки сервера нажми `Ctrl+C` в его терминале.

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

Запусти Docker Desktop и дождись готовности движка Linux-контейнеров. Сборка впервые скачает базовый образ и зависимости, поэтому ей нужен интернет.

```powershell
docker build -t srez:lesson3 .
docker run --rm --name srez-lesson3 -p 8080:8080 srez:lesson3
```

Перед запуском контейнера останови локальный сервер, если он занимает порт 8080. В другом терминале проверь тот же URL:

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

Тест проверяет статус, тип содержимого и список проектов в HTTP-ответе.

## Путь запроса

`main` читает конфигурацию → `handler.New()` регистрирует маршрут → `Run` начинает слушать адрес → `GET /projects` вызывает `GetProjects` → обработчик отправляет JSON.

На этом этапе данные зашиты в обработчике. Бизнес-логика, база данных и остальные маршруты описаны как следующие этапы в `SPEC.md`.
