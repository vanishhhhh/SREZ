> Исторический документ предыдущего этапа. Текущее состояние и команды: [MVP_REPORT.md](MVP_REPORT.md) и [frontend README](../README.md).

# SREZ: frontend и авторизация

> Исторический отчёт auth-этапа. Для новой интеграционной копии используйте localhost:5174 и API8081; авторизация сохранена. Актуальный запуск, Media и buyer: [INTEGRATION_REPORT.md](INTEGRATION_REPORT.md).

Этап выполнен локально 16.11.2026. Используются прежние React + TypeScript + Vite и существующие Go auth endpoints. Код каталога, студии и PVM на этом этапе не менялся. Go-исходники, миграции и рабочие данные сохранены.

## Устройство

```text
Layout + router
  AuthProvider / RequireAuth
    AuthState: checking → authenticated / anonymous / unavailable
      SessionClient: identity, private in-memory access, cookie-operation queue
        jsonRequest: /api, JSON, cookies, HTTP errors, abort/timeout
          Vite → Go → PostgreSQL
```

- `http.ts` отвечает за same-origin JSON transport, ответ 204, отмену/20-секундный timeout, статусы и безопасные сообщения ошибок. Произвольные внешние URL не принимаются.
- `auth-types.ts` содержит реальные DTO регистрации, login, профиля и разрешённого PATCH. `types.ts` сохраняет совместимые type exports для прежнего приложения.
- `SessionClient` хранит access JWT только в закрытом состоянии экземпляра. React snapshot содержит пользователя/статус/сообщение, но не токены. Нет записи токенов в localStorage, sessionStorage, IndexedDB, URL или логи.
- `AuthProvider` подписывается на snapshot; `RequireAuth` ждёт проверки, показывает recovery при недоступности и перенаправляет на login только при подтверждённом anonymous/expired. После входа восстанавливается безопасный исходный URL.
- Общие `InputField`/`PasswordField` связывают label, hint и input через ID, поддерживают aria-invalid/description и autocomplete. Используются существующие кнопки, уведомления и визуальная система SREZ.
- `PageBoundary` охватывает содержимое страницы: header/footer остаются при ошибке, переход на другой URL позволяет восстановиться. Неизвестный маршрут показывает 404. `/unavailable` — отдельная страница подключения; она также используется guard без потери приватного URL.

## Контракты auth

| Метод и путь Go API   | Действие клиента                                                                                                                                                                                             |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| POST `/auth/register` | Передаёт email/username/display_name/password, получает публичного пользователя. Затем отдельно вызывает login. Если login не удался, форма помнит, что аккаунт уже создан, и не регистрирует его второй раз |
| POST `/auth/login`    | Передаёт identifier и пароль без trim пароля; принимает access/CSRF и серверную HttpOnly cookie, загружает собственный профиль                                                                               |
| GET `/auth/csrf`      | Получает актуальный CSRF для текущей browser-managed cookie                                                                                                                                                  |
| POST `/auth/refresh`  | Передаёт `{}` и X-CSRF-Token, принимает ротацию cookie и новый access                                                                                                                                        |
| POST `/auth/logout`   | Получает текущий CSRF, отзывает сессию. Только при успехе/уже недействующей сессии очищает память. Сетевой сбой не изображается успешным выходом                                                             |
| GET `/profile`        | Загружает собственный профиль по Bearer JWT                                                                                                                                                                  |
| PATCH `/profile`      | Явно передаёт только display_name/avatar_url; username/email/id/role в тело не попадают                                                                                                                      |

После reload выполняются CSRF → refresh → profile. 401 без cookie при первом посещении означает обычного гостя. Завершившаяся активная сессия показывает сообщение о повторном входе. Network/5xx, 403 проверки cookie и повторный 409 переводят проверку в `unavailable`, а не в притворный logout; приватные страницы до восстановления закрыты. При сбое только profile повторная проверка может использовать уже полученный access без лишней ротации.

## Конкурентность и срок жизни

1. В одной вкладке параллельные 401 используют один `refreshFlight`; повтор исходного запроса выполняется максимум один раз. Обычные 403/409 mutation не запускают refresh.
2. Все операции с cookie — login, refresh, logout — проходят локальную очередь. Web Locks с именем `srez-cookie-session` дополнительно сериализуют их между вкладками одного origin.
3. При первом refresh 409 клиент ждёт 200 мс и заново получает CSRF для текущей cookie. Максимум два POST refresh на одну попытку. Бесконечного replay/retry нет, старый refresh JavaScript не захватывает и не воспроизводит.
4. При refresh с известной личностью профиль дополнительно перепроверяется: смена user ID отменяет старый запрос, а приватная страница монтируется заново. Generation guard отбрасывает успешные и ошибочные ответы старой личности после logout/нового login. Запоздавший 401 не запускает восстановление новой cookie и не очищает новый вход.
5. BroadcastChannel передаёт только `logout`/`session-changed`. При входе в другой вкладке старая личность сбрасывается и заново проверяется по текущей cookie. Токены и профиль через канал не пересылаются.
6. Клиент не продлевает срок cookie и не меняет backend TTL. Access живёт 15 минут и обновляется по первому 401; refresh имеет абсолютный срок 7 дней. Ротация не превращает его в скользящий срок. Две обычные вкладки используют одну browser cookie-сессию; для разных аккаунтов нужны разные профили/приватные контексты.

HttpOnly/SameSite, exact Origin, CSRF и проверка JWT остаются в Go без изменений. Клиент не подставляет поддельный Origin и не включает CORS со звёздочкой.

## Proxy и окружение

Vite dev/preview слушают только `127.0.0.1:5173`, `strictPort:true`. `/api/...` проксируется на `127.0.0.1:8080/...` с удалением `/api`. `/media/...` проксируется туда же **без rewrite**. `changeOrigin:false` сохраняет Origin. `/brands` и `/products` целиком не проксируются: это маршруты React.

Go API в этой копии ещё не имеет Media Storage endpoint. Proxy `/media` подготовлен, но ответ 404 от Go не является работающей загрузкой файлов. JSON transport и HTTPS URL профиля сохранены; multipart/upload — отдельный будущий этап.

Backend для frontend требует `SREZ_PUBLIC_ORIGIN=http://127.0.0.1:5173` и существующий локальный режим `SREZ_AUTH_LOCAL_HTTP=true`. Это задаёт `compose.frontend.yaml`. Ключи JWT/CSRF и пароль БД не должны находиться в `VITE_*`. Адрес `localhost` вместо `127.0.0.1` использовать нельзя без согласованной смены всех настроек.

## Обычный запуск: Windows PowerShell

Откройте Docker Desktop и дождитесь готовности Engine. Первое окно:

```powershell
Set-Location 'C:\dev\team-repository'
node .\frontend\scripts\prepare-local.mjs
docker compose -f compose.yaml -f compose.frontend.yaml up --build -d
Invoke-RestMethod http://127.0.0.1:8080/health
```

Скрипт сохраняет существующий `.env` и его значения, генерирует только пустые независимые ключи. Для существующей базы сохраните её пароль и прежнее имя Compose-проекта; при использовании `-p ИМЯ` добавляйте его к каждой Compose-команде. Volumes не пересоздавайте.

Второе окно:

```powershell
Set-Location 'C:\dev\team-repository\frontend'
npm install
npm run dev
```

Открыть `http://127.0.0.1:5173/login`, `/register`, `/account`, `/unavailable`. Для 404 откройте любой неизвестный адрес. Если PowerShell блокирует npm.ps1, используйте `npm.cmd` с теми же аргументами.

Остановка: `Ctrl+C` в окне React; затем из корня `docker compose -f compose.yaml -f compose.frontend.yaml stop`. Контейнеры и данные сохраняются. Флаг удаления volumes не нужен.

## Проверки и отдельная тестовая база

```powershell
# В frontend:
npm test
npm run build
npm run format:check
```

Для UI ошибок без реального API: при работающем Vite выполнить `npm run test:e2e:auth`. Будут выполнены 8 браузерных тестов с контролируемыми ответами; реальный тест будет явно пропущен без opt-in флага. Эти ответы существуют только в тестах, runtime приложение их не использует.

Для настоящего Go/PostgreSQL E2E освободите 8080, остановив только известный рабочий app с сохранением базы:

```powershell
# Из корня, если app уже работает:
docker compose -f compose.yaml -f compose.frontend.yaml stop app
Set-Location .\frontend
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\auth-test-api.ps1
```

Этот процесс собирает текущий локальный Go backend и создаёт отдельные именованные контейнеры/сеть/volume. Рабочий volume не подключается. Ключи независимы и не печатаются. Auth rate limit в тестовом контейнере повышен до 100, остальные JWT/Origin/CSRF проверки включены. Дождитесь `READY` и оставьте окно открытым. `ExecutionPolicy Bypass` относится только к этому процессу, системная политика не меняется.

В другом окне с работающим `npm run dev`:

```powershell
Set-Location 'C:\dev\team-repository\frontend'
$env:SREZ_AUTH_E2E_LIVE = '1'
npm run test:e2e:auth
Remove-Item Env:SREZ_AUTH_E2E_LIVE
```

Флаг разрешает создание тестового аккаунта только в подготовленном тестовом окружении. При включённом флаге недоступный API — ошибка, а не скрытый skip. После тестов нажмите Enter в окне `auth-test-api.ps1`: остановятся только созданные им app/db. **Volume, тестовые данные, контейнеры и сеть сохраняются; автоматического удаления нет.** Затем обычный API можно вернуть командой `docker compose -f compose.yaml -f compose.frontend.yaml up -d app` из корня.

Проверено 16.11.2026:

- `npm install`: успешно, 0 vulnerabilities по результату npm audit при установке.
- 38 unit-тестов в 4 файлах: успешно. Покрыты concurrency, bounded 409, network recovery, истечение сессии, logout races, смена личности, whitelist PATCH, форматирование/пароль/URL, JSON/204/timeout/cancellation.
- 9 браузерных тестов: 8 controlled-error UI + 1 настоящий Go/PostgreSQL сценарий, все прошли. Реально проверены register, username/email login, неправильный пароль, изменение профиля, reload/rotation, одновременные вкладки, общий logout и повторный вход.
- Подтверждены HttpOnly/SameSite=Strict, отсутствие refresh в document.cookie, отсутствие токенов в Web Storage, неизменный абсолютный срок refresh при reload, отказ backend при неправильном Origin/CSRF и неверном Bearer.
- TypeScript/Vite build и форматирование проверены. `/brands` отдаёт SPA HTML, `/api/brands` — Go JSON; `/media/...` направляется в Go и пока отвечает 404.
- Визуально проверены скриншоты auth-страниц и мобильного профиля; на 390 px нет горизонтального переполнения. Trace/video с секретами отключены, отчёты/скриншоты игнорируются Git.

## Файлы этого этапа

Новые:

```text
src/lib/auth-types.ts
src/lib/http.ts
src/lib/http.test.ts
src/lib/session-recovery.test.ts
src/components/forms.tsx
src/components/ServerUnavailable.tsx
src/components/PageBoundary.tsx
src/pages/System.tsx
e2e/auth.spec.ts
scripts/auth-test-api.ps1
docs/AUTH.md
```

Обновлены:

```text
src/lib/client.ts
src/lib/client.test.ts
src/lib/types.ts
src/lib/format.ts
src/lib/format.test.ts
src/auth/Auth.tsx
src/pages/Auth.tsx
src/components/Layout.tsx
src/App.tsx
src/styles.css
vite.config.ts
package.json
README.md
docs/IMPLEMENTATION_PLAN.md
../docs/architecture.md
```

## Оставшиеся ограничения

- Полная межвкладочная очередь требует Web Locks. При его отсутствии остаются локальная очередь, single-flight и один повтор 409; повторный конфликт явно показывает recovery. Без BroadcastChannel другая вкладка узнает об изменении сессии при следующей проверке. Поддерживаемый Chrome проверен реально.
- Backend logout отзывает refresh, но уже выданный JWT вне приложения может оставаться действительным до exp. В приложении память очищается, другие вкладки получают событие. Этот серверный контракт не изменялся.
- Auth TTL проверен без ожидания 15 минут/7 дней: истечение и ошибки дополнительно моделируются в тестах, cookie expiry после реальной ротации сравнивается с исходной.
- Email verification/recovery, управление списком сессий, Media upload и новые buyer-функции не реализованы в локальном backend и не добавлялись здесь.
- Dirty-state продавца, каталог и изменение API ошибок/DTO остаются в плане следующих этапов.
