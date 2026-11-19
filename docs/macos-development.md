# Разработка SREZ на macOS

## Установка

Установите Docker Desktop для **Mac with Apple silicon** и откройте его один раз, чтобы завершить настройку и запустить Docker Engine. Через Homebrew установите Node.js 24 для подготовки окружения и Vite и Go для нативных тестов. Для редактирования подойдёт установленный отдельно VS Code.

```zsh
brew install node@24 go
export PATH="$(brew --prefix node@24)/bin:$PATH"
docker version
docker compose version
docker info --format '{{.OSType}}/{{.Architecture}}'
node --version
npm --version
go version
```

Добавьте строку `export PATH=…` в `~/.zshrc`, если хотите использовать Node 24 в новых терминалах VS Code. `docker version` должен показывать Client и Server. На Apple Silicon Docker Engine работает как `linux/aarch64`. Минимальная версия Go указана в `go.mod`. Сборка образов сама устанавливает нужные Go и Node внутри Docker.

## Первый запуск в Docker

Выполните из корня checkout. `--create-only` создаёт приватный `.env` с независимыми ключами и оставляет уже существующий файл побайтно прежним. Если Compose сообщает об отсутствующем параметре в существующем файле, добавьте только отсутствующее значение по `.env.example`. Сохраняйте ключи и пароль роли, с которыми уже инициализированы локальные данные.

```zsh
node frontend/scripts/prepare-local.mjs --create-only
docker compose -f compose.integration.yaml config --quiet
docker compose -f compose.integration.yaml up -d --build --wait --wait-timeout 120
curl -fsS http://127.0.0.1:8081/health
curl -fsS http://localhost:5174/api/health
```

| Сервис | Локальный адрес |
| --- | --- |
| SREZ | http://localhost:5174 |
| Go API | http://127.0.0.1:8081 |
| Mailpit | http://127.0.0.1:8025 |
| PostgreSQL | `127.0.0.1:15432` |
| SeaweedFS S3 | `127.0.0.1:18333` |
| SMTP | `127.0.0.1:1025` |

Откройте [регистрацию](http://localhost:5174/register), выберите локальный тестовый email и пароль от 12 символов. Найдите письмо по адресу получателя в [Mailpit](http://127.0.0.1:8025), откройте ссылку, нажмите кнопку подтверждения в SREZ и войдите. Для восстановления пароля используйте подтверждённый адрес. Для cookie, CSRF и ссылок из писем используйте `localhost` для UI на всём протяжении сценария.

Регистрация, повторная отправка подтверждения и запрос восстановления пароля могут создать письмо. Обычный запуск Go API писем не создаёт. Integration направляет SMTP в `mailpit:1025` внутри Docker в обоих режимах frontend. Для локальной проверки открывайте Mailpit в браузере. Если письма появляются во внешнем ящике, проверьте, какой Go API обслуживает приложение. Vite по умолчанию проксирует в `127.0.0.1:8081`, а `SREZ_API_TARGET` может указывать на другой экземпляр. При использовании нативного Go с отдельной SMTP-конфигурацией сначала проверьте её назначение.

API при старте применяет недостающие миграции к БД `srez-integration`. Сверьте принадлежность существующего проекта и его volumes перед первым `up`. Для проверки миграций используйте `compose.tests.yaml`. В `compose.integration.yaml` уже подключены все пять сервисов. `compose.frontend.yaml` относится к прежнему workflow.

## React через Vite

В первом терминале запустите инфраструктуру и API. Если Docker frontend ещё не создан, `stop frontend` можно пропустить.

```zsh
docker compose -f compose.integration.yaml stop frontend
docker compose -f compose.integration.yaml up -d --build --wait --wait-timeout 120 app
curl -fsS http://127.0.0.1:8081/health
```

Во втором терминале из корня репозитория запустите Vite и откройте `http://localhost:5174`.

```zsh
npm --prefix frontend ci
npm --prefix frontend run dev
```

Изменения TypeScript и CSS в VS Code подхватывает HMR. И Vite, и nginx отправляют `/api/…` в Go без префикса `/api`, а `/media/…` передают с исходным путём. Страницы `/products/…` и `/brands/…` обслуживаются React. Vite и Docker frontend используют один порт, поэтому переключайте их последовательно. Для возврата нажмите `Ctrl+C` в терминале Vite, затем выполните полный Docker-запуск.

## Логи, остановка и порты

```zsh
docker compose -f compose.integration.yaml ps
docker compose -f compose.integration.yaml logs --tail 80 app frontend
docker compose -f compose.integration.yaml logs -f --tail 30 app
docker compose -f compose.integration.yaml stop
# Возобновить полный запуск с текущими настройками и сборкой:
docker compose -f compose.integration.yaml up -d --build --wait
```

`Ctrl+C` завершает просмотр логов. `stop` сохраняет БД, письма и объекты в named volumes проекта. `start` возобновляет уже созданные контейнеры с прежними настройками. При смене конфигурации используйте `up`. Оставляйте имя `srez-integration` неизменным при переключении Docker/Vite. В отладочных выводах используйте `config --quiet`, чтобы Compose не распечатал значения переменных.

Чтобы выяснить, кому принадлежит занятый порт, выполните, например, `lsof -nP -iTCP:5174 -sTCP:LISTEN` и `docker ps --format '{{.Names}} {{.Ports}}'`. Если это свой Vite или Docker frontend, остановите его описанным выше способом. Если порт принадлежит другому приложению, выберите свободный порт. Для integration есть переменные `SREZ_FRONTEND_PORT`, `SREZ_API_PORT`, `SREZ_DB_PORT`, `SREZ_MEDIA_PORT`, `SREZ_MAILPIT_PORT` и `SREZ_SMTP_PUBLISH_PORT`. Экспортируйте только нужные значения до `up`. Compose синхронизирует порт UI с origin и ссылками из писем, SMTP внутри Docker остаётся на `mailpit:1025`.

Например, если заняты UI и API, используйте в двух терминалах одинаковые переменные. Vite загружает настройки из каталога `frontend`, поэтому передайте ему параметры портов через окружение терминала. Проверьте выбранные порты перед запуском.

```zsh
export SREZ_FRONTEND_PORT=5184 SREZ_API_PORT=8181
# Только для терминала Vite, если изменён опубликованный порт API:
export SREZ_API_TARGET="http://127.0.0.1:$SREZ_API_PORT"
```

Откройте `http://localhost:5184` и запросите новое письмо для действий, которые ещё не подтверждены. Проверки HTTP должны использовать новые порты. Для Windows PowerShell-скриптов используйте значения по умолчанию, на которых проверяются их health-запросы.

## Тесты

Запускайте команды из корня репозитория. Для обычного `go test` используйте терминал без `TEST_DATABASE_URL`, если инфраструктурные тесты должны пропускаться. Параметр `full` ниже запускает Go-тесты в Docker с отдельными PostgreSQL, SeaweedFS и Mailpit.

```zsh
go test ./...
npm --prefix frontend ci
npm --prefix frontend run typecheck
npm --prefix frontend run lint
npm --prefix frontend test
npm --prefix frontend run build

docker compose -f compose.tests.yaml up -d --wait
docker build --target build -t srez-mvp-go-check:local .
node scripts/qa-go.mjs full
docker compose -f compose.tests.yaml stop
```

Для браузерных тестов нужен Google Chrome. Используйте `compose.qa.yaml` и `node scripts/qa-browser.mjs all` из корня, как в [общей инструкции](../README.md#тестирование-и-ci). Playwright пишет тестовые данные в отдельное QA-окружение на `localhost:5175`. Остановите QA командой `docker compose -f compose.qa.yaml stop`.

## Отчёт о проверке на Apple Silicon

До этой работы готовые команды запуска были написаны для PowerShell. `start-integration.ps1` вызывает Node и Compose и проверяет readiness через `Invoke-RestMethod`/`Invoke-WebRequest`. `start-dev-backend.ps1` запускает нативный Go с явно заданными integration-портами. Проекты Compose, Dockerfile Go и React, nginx и Vite уже подходили для ARM64. На Mac достаточно Node и последовательности Compose/Vite из этой инструкции. [start-integration.ps1:7](../scripts/start-integration.ps1#L7), [start-dev-backend.ps1:5](../scripts/start-dev-backend.ps1#L5), [Dockerfile:1](../Dockerfile#L1), [frontend/Dockerfile:1](../frontend/Dockerfile#L1)

Режим `--create-only` использует существующую генерацию JWT/CSRF/wishlist и S3, а новый файл создаёт эксклюзивно с режимом `0600`. Настройки Auth, wishlist и SMTP остаются backend-конфигурацией. Integration подставляет DNS и порт PostgreSQL/S3/SMTP внутри проекта, отдельно от опубликованных loopback-портов. Изменение frontend-порта синхронно меняет разрешённый auth origin и адрес ссылок из писем. [prepare-local.mjs:7](../frontend/scripts/prepare-local.mjs#L7), [prepare-local.mjs:55](../frontend/scripts/prepare-local.mjs#L55), [compose.integration.yaml:5](../compose.integration.yaml#L5), [auth.go:20](../internal/config/auth.go#L20), [wishlist.go:11](../internal/config/wishlist.go#L11), [mail.go:18](../internal/config/mail.go#L18), [media.go:9](../internal/config/media.go#L9)

| Проверка | Результат 18 ноября 2026 |
| --- | --- |
| Среда | macOS `darwin/arm64`, Docker CLI 29.9.2, Compose 5.6.1, Docker Engine `linux/aarch64`, Go 1.27.3, Node 26.13.0. Docker frontend собран на Node 24 из Dockerfile. |
| Compose и образы | `config --quiet` прошёл для integration, QA и Go tests. Собраны backend/frontend. Все пять integration-контейнеров использовали `linux/arm64` или `linux/arm64/v8`, включая закреплённые образы SeaweedFS и Mailpit. |
| Миграции и Docker HTTP | В новой локальной integration БД записаны 15 миграций. Прямой Go `/health` и проксированный `/api/health` вернули `status: ok`. Frontend и Mailpit вернули HTTP 200. |
| Vite | API proxy и клиент Vite ответили. В Chromium проверено применение изменения CSS без перезагрузки документа, шрифты вернули HTTP 200. После остановки Vite nginx снова обслуживал `/api/health` на 5174. [vite.config.ts:16](../frontend/vite.config.ts#L16), [nginx.conf:39](../frontend/nginx.conf#L39) |
| Media | На тестовом QA-изображении Go напрямую, nginx и локальный Vite вернули HTTP 200, MIME `image/*` и одинаковые байты. В integration отсутствующий объект через Vite и Go вернул одинаковый JSON 404. |
| Конфликты портов | Проверены альтернативные UI 5184, API 8181, Mailpit 8125 и опубликованный SMTP 1125. Запрос входа с origin 5184 дошёл до проверки credentials и вернул ожидаемый 401. После проверки возвращены стандартные порты. |
| `.env` и bundle | Проверены `0600`, пять разных сгенерированных ключей и побайтное сохранение существующего `.env` с BOM и CRLF. В HTML/JS/CSS из nginx-образа проверены пять тестовых ключей и пароль БД, включая имена переменных. Совпадений и source maps нет. |
| Тесты | Нативный `go test ./...` прошёл. `qa-go.mjs full` прошёл 926 тестов с PostgreSQL/SeaweedFS/Mailpit. Отдельный `TestSeaweedPersistenceAfterRestart` прошёл в два этапа с остановкой и запуском SeaweedFS между записью и чтением. Frontend typecheck, lint, format:check, 96 unit-тестов и build прошли. В QA прошли 36 Playwright-тестов, включая регистрацию, подтверждение почты и восстановление. Browser channel изменён на Chromium только во временной копии. |
| Windows | PowerShell-файлы, порты по умолчанию и Node-вызов без флага сохранены. Разрешённая Compose-конфигурация со стандартными переменными побайтно совпала с версией из `origin/main`. Запуск Windows/PowerShell на этой машине не выполнен. |

Проверочные проекты подняты из временной копии только после подтверждения отсутствия этих контейнеров и volumes. После проверки сервисы остановлены, созданные volumes сохранены. В integration осталась схема с миграциями, а демо- и E2E-данные находятся в QA. Корневой `.env` рабочей копии не создавался и не менялся. Для обычного запуска из checkout используйте `up` по инструкции выше, поскольку `start` повторно запускает контейнеры с их прежними параметрами.

## Основной стенд и изолированные проверки

Постоянный режим на Mac использует один Compose project `srez-integration` с Nginx/frontend, Go API, PostgreSQL, SeaweedFS и Mailpit. Из рабочего дерева с актуальным кодом запустите `docker compose -f compose.integration.yaml up -d --build --wait --wait-timeout 120`. Открывайте [SREZ](http://localhost:5174/). API доступен на `127.0.0.1:8081`, интерфейс Mailpit на `127.0.0.1:8025`. Используйте существующий `.env` для этого экземпляра.

Для QA запускайте `docker compose -f compose.qa.yaml up -d --build --wait`, затем `node scripts/qa-browser.mjs all`. После прогона достаточно `docker compose -f compose.qa.yaml stop`. Браузерные тесты используют только QA на `localhost:5175`, со своими PostgreSQL, SeaweedFS и Mailpit. Изолированные Go-тесты используют `compose.tests.yaml`. Логи Go-раннеров пишутся в рабочее дерево, одноразовые контейнеры автоматически удаляются после завершения.

В локальном HTTP cookie основного стенда называется `srez_refresh_local`, на QA используется `srez_refresh_local_qa`. Cookies в браузере общие для одного хоста на разных портах. Явный `SREZ_AUTH_LOCAL_COOKIE_SCOPE` разводит локальные окружения и доступен только при loopback HTTP. Для уже перезаписанной старой сессии основного стенда требуется обычный повторный вход. Используйте настроенный `localhost` в браузере, поскольку Origin проверяется целиком.
