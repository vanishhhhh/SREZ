# Media Storage — Phase 2

Ветка `feat/media-storage-seaweedfs` продолжает Phase 1 на базе `f4edad2`.
Миграции 001–009 неизменны. Новая миграция — **010_media_ownership.sql**.

## Архитектура

```mermaid
sequenceDiagram
 participant React
 participant HTTP as Gin / JWT
 participant Media as Media usecase
 participant SQL as PostgreSQL / existing ManagementTx
 participant S3 as Private SeaweedFS
 React->>HTTP: POST /media/uploads (purpose + target)
 HTTP->>Media: actor из проверенного JWT
 Media->>SQL: актуальные права + цель, processing asset + pending request
 SQL-->>React: upload ID, backend content_url
 React->>HTTP: PUT content_url (raw File/Blob)
 HTTP->>HTTP: Content-Length, MaxBytesReader, read deadline
 HTTP->>Media: ограниченное тело, reserved upload slot
 Media->>SQL: повторная авторизация, atomic claim
 Media->>Media: bounded read, sniff, dimensions, full decode, normalize, output cap
 Media->>S3: только проверенный normalized PUT
 Media->>SQL: повторная авторизация; привязка + ready одной транзакцией
 SQL-->>React: media_id, image_id для товара
 React->>HTTP: GET /media/assets/:id/content
 HTTP->>SQL: ready + текущая ссылка + текущая видимость
 HTTP->>S3: bounded GET
 HTTP-->>React: JPEG/PNG, nosniff, no-store
```

Handler отвечает за JWT, HTTP framing/deadlines, строгое JSON и статусы. Usecase
задаёт назначение, проверку содержимого и привязку. Storage реализует SQL внутри
существующих родительских транзакций. S3 adapter использует AWS SDK v2, path-style,
ограниченные retries и HTTP timeout 20 секунд; endpoint/bucket/credentials — из env.

## Данные и инварианты

| Данные                                    | Назначение                                                                                                                                |
| ----------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `media_assets`                            | Физический bucket/key, инициатор, immutable purpose/target, MIME/size нормализованного объекта, даты                                      |
| `media_upload_requests`                   | Техническая загрузка, ожидаемые MIME/размер, фактический source_size, pending/processing/complete/rejected, replacement image_id/position |
| `users.avatar_media_id`                   | Привязка собственного приватного аватара                                                                                                  |
| `brands.logo_media_id`, `banner_media_id` | Отдельные ссылки оформления бренда                                                                                                        |
| `product_color_images.media_id`           | Managed источник; прежние image_id/color_id/position сохранены                                                                            |
| `media_references`                        | View актуальных доменных ссылок, без постоянных HTTP URL                                                                                  |
| `media_uploads`                           | Неизменённая по смыслу Phase 1 история, выведенная из нового API                                                                          |

Asset: `processing → ready → deleting → deleted`; отказ/истечение processing
переходит в deleting через GC. Processing заранее связан с целью и имеет TTL
15 минут. Ready фиксируется только вместе с одной правильной доменной ссылкой.
Deferred constraint triggers запрещают ready-orphan, неверную цель/назначение,
повторное использование asset и ссылку на неготовый/удалённый asset. При detach
SQL trigger в той же транзакции переводит старый asset в deleting с grace 5 минут.
Успешный rollback восстанавливает и ссылку, и ready-состояние старого объекта.

Publication **не является флагом upload/asset**: она определяется текущим
состоянием связанной сущности при каждом чтении. Это не позволяет загрузке
самостоятельно опубликовать файл или сохранить public-доступ после hide/revoke.

Legacy HTTPS URL и managed media ID — разные источники. В БД хранится либо
настоящий старый HTTPS URL, либо ID; локальные/фиктивные URL в БД не записываются.
Domain сохраняет HTTPS validation. DTO формируют относительный `/media/assets/ID/content`
для managed источника. В будущем renderer можно заменить HTTPS/CDN-адресом без
переписывания доменных ссылок.

## Права и блокировки

- `user_avatar`: target user берётся исключительно из JWT actor; строка users
  блокируется `FOR NO KEY UPDATE`. Это сериализует замену, не блокируя FK KeyShare.
- `brand_logo`, `brand_banner`: существующий `Management.brand(..., owner=true)`;
  нужны active owner и несуспендированный бренд. Editor не получает новых прав.
- `product_image`: существующий `Management.withProduct` проверяет
  active owner/editor → фактический brand → product → color. Brand не принимается
  из запроса товара. Проверки выполняются при создании, claim и финальной привязке.
- Сохраняется порядок brand → membership → product → children → upload → asset.
  Avatar имеет отдельную user-границу. Квота pending сериализуется advisory lock
  на инициаторе, без захвата чужой user-строки внутри brand-транзакции.
- Сетевая передача и декодирование не держат SQL-транзакцию. Если права отозваны
  во время S3 PUT, финальная транзакция отказывает; объект остаётся приватным
  техническим кандидатом до GC.

Для published product используется прежняя проверка полного post-write aggregate.
Последнее изображение нужного цвета удалить нельзя: rollback сохраняет фото и
отменяет постановку asset на удаление. Для active brand нельзя удалить необходимый
логотип до unpublish. Reorder не меняет image_id/media_id.

## API

Все изменяющие операции требуют `Authorization: Bearer ...`. Неизвестные JSON
поля отклоняются, включая user_id, bucket, object_key, status и внешние URL.
Нельзя выбирать чужого владельца или использовать purpose как разрешение.

### 1. Создать техническую загрузку

`POST /media/uploads`, Content-Type application/json. Обязательны purpose,
content_type (`image/jpeg`, `image/png`, `image/webp`) и size_bytes (1…5242880).

```json
{ "purpose": "user_avatar", "content_type": "image/jpeg", "size_bytes": 124567 }
```

```json
{
  "purpose": "brand_logo",
  "brand_id": 3,
  "content_type": "image/png",
  "size_bytes": 124567
}
```

Для banner purpose — `brand_banner`, те же brand-поля. Только owner.

```json
{
  "purpose": "product_image",
  "product_id": 7,
  "color_id": 11,
  "content_type": "image/jpeg",
  "size_bytes": 124567,
  "position": 0
}
```

Product upload добавляет изображение и возвращает стабильный image_id.
Для замены укажите `image_id` вместо position: ссылка меняется с сохранением
image_id/color_id/position. Перестановка — отдельная PVM-операция. Одновременные
image_id и position запрещены. ID от других целей/товаров/цветов отклоняются.

201: `{"upload":{"id":1,"media_id":1,"state":"pending","asset":{...}},"content_url":"/media/uploads/1/content"}`.
S3 keys, bucket, credentials и presigned URL отсутствуют. POST создаёт новую
техническую запись, не является идемпотентным; максимум 20 активных записей на actor.

### 2. Отправить файл через backend

```sh
curl -X PUT http://127.0.0.1:8080/media/uploads/1/content \
 -H "Authorization: Bearer $TOKEN" -H 'Content-Type: image/jpeg' \
 --data-binary @photo.jpg
```

Raw body, **не multipart**. Нужен известный Content-Length, совпадающий с size_bytes.
Для React передавайте File/Blob как `fetch(..., {method:'PUT',headers:{Authorization,
'Content-Type':file.type},body:file})`; браузер формирует длину. Не задавайте
Content-Length вручную из JS. Dev proxy должен сохранять известную длину.
Unknown-length/chunked upload получает 411; ложная длина — 400 либо framing error
самого HTTP server. Независимый фактический лимит действует и при ложном заголовке.
Превышение известной/прочитанной длины даёт 413 **до S3 PUT**.

200 — complete upload с ready asset, фактическими MIME/output size/source size в SQL
и image_id для product_image. Привязка уже выполнена; отдельный complete не нужен.
Повтор PUT завершённой загрузки повторно проверяет права и возвращает состояние,
не читает/не записывает новый объект и не восстанавливает заменённую старую ссылку.
После ошибки уже claimed загрузки нужен новый POST; неоднозначный successful commit
можно обнаружить повтором PUT. Одновременный проигравший claim получает 409.

### 3. PVM и удаление

- `GET /products/:id/colors/:color_id/images`: прежние ID/position/url плюс
  `media_id` для managed источника.
- `POST .../images {"media_id":N}` (200): идемпотентное подтверждение уже созданной upload
  привязки этого цвета; не дублирует изображение и не переносит asset. Опциональный
  position вызывает обычный reorder. Непривязанный/чужой ID не принимается.
- `PATCH .../images/:image_id {"position":1}`: прежний порядок. media_id допускается
  лишь как подтверждение текущего источника. Для замены используется новый upload
  с target image_id. Старые create/patch по `url` отвечают 400.
- `POST .../images/reorder {"image_ids":[2,1]}` и DELETE фото/цвета сохраняют PVM policy.
- `DELETE /profile/avatar` — убрать свой аватар, без тела.
- `DELETE /brands/:id/media/brand_logo` или `brand_banner` — owner, без тела.
- PATCH profile.avatar_url и brand.logo_url/banner_url больше не являются writer API.
  Исторические импортные методы/factories остаются для доверенного seed/fixtures;
  HTTP не вызывает их с клиентскими URL.

### 4. Выдача

`GET /media/assets/:id/content` читает только ready asset с действующей ссылкой:

| Источник                                   | Guest                            | Авторизованный доступ                                    |
| ------------------------------------------ | -------------------------------- | -------------------------------------------------------- |
| Avatar                                     | 404                              | Только сам пользователь                                  |
| Brand logo/banner                          | Только active brand              | Также active member draft/non-suspended brand            |
| Product image                              | Published product + active brand | Также active member соответствующего non-suspended brand |
| Processing/deleting/deleted/Phase 1 object | 404                              | 404                                                      |

JWT необязателен только для public GET; переданный неверный JWT даёт 401.
Приватные изображения React загружает через fetch с Bearer и показывает через
Blob object URL (revokeObjectURL после использования); public — обычный img src.
Для локального отдельного frontend настройте same-origin dev proxy `/media`.

Ответ: проверенный Content-Type, X-Content-Type-Options nosniff, Cache-Control no-store,
CSP sandbox/default-src none. Нет чтения по произвольному key. Относительные ссылки
в существующих URL-полях — единственное изменение публичной формы URL; JSON-структуры
каталога/профиля/брендов сохранены. GET ограничен реальным output size и двумя отдельными delivery slots.

Общие статусы: 400 — неверные поля/формат/декодирование; 401 — JWT; 403 — известный
editor без owner-права; 404 — недоступный ресурс; 409 — конфликт/доменная публикация;
411 — неизвестная длина; 413 — source/output limit; 429 — заняты slots/квота;
500 — generic storage error, без SQL, paths и credentials. Metadata/errors — no-store.

## Проверка содержимого и ресурсов

До первого S3 PUT: ранний Content-Length cap, MaxBytesReader 5 MiB, независимый
LimitReader(max+1), точное сравнение длины, sniff сигнатуры, DecodeConfig и Decode.
Максимум каждой стороны 8192, максимум 12 миллионов пикселей. Обрезанные/ошибочные
изображения отклоняются. SVG/GIF/другие MIME не принимаются.

Opaque input → JPEG quality 85; alpha → PNG. Оба encoder — pure Go из stdlib,
WebP decoder — golang.org/x/image/webp. Новое кодирование убирает исходные EXIF,
ICC/comments и иные метаданные. Выходной writer ограничен **8 MiB**, при превышении
нет S3-записи. JPEG lossy, PNG сохраняет прозрачность; EXIF orientation не применяется.
Это не утверждение абсолютной защиты от всех дефектов codec-библиотек.

Один upload/decoder slot и два отдельных delivery slots на процесс, без очередей.
Два публичных GET не занимают резерв загрузки. Максимум одновременно работает
один codec вместо прежних двух, плюс два ограниченных response buffers; это
общий фиксированный бюджет, а не удвоение decoder concurrency. Source <=5 MiB,
output <=8 MiB, pixels <=12MP; Go ReadAll/encoder могут иметь дополнительную
ёмкость буферов и allocations, поэтому это не обещание точного RSS в байтах.
Delivery slot удерживается до окончания HTTP write; defer освобождает слоты
при ошибке, отмене, timeout и disconnect. Source read deadline 10s, upload context 45s, GET context/write
deadline 15s; server header/read/write/idle timeouts 5/15/60/60s, headers <=16 KiB.
Декодеры не имеют API принудительной отмены CPU: работа ограничена pixels/dimensions
и concurrency; context проверяется до PUT, сеть/SQL ограничены им. Для hard CPU
isolation понадобился бы отдельный process, что не входит в этот MVP.

## Старый presigned flow и SeaweedFS

`POST /media/uploads/:id/complete` возвращает 410 после JWT. Ни один HTTP writer
не выдаёт presigned URL. При старте backend обязательно добавляет в bucket policy
проверяет ACL и policy перед включением API. ACL допускает только canonical owner;
неподдерживаемое/неполное чтение ACL закрывает запуск. Policy без Allow или с
конкретными AWS IAM principals допустима; wildcard/NotPrincipal/неизвестные формы
Allow отклоняются консервативно, даже если conditions/Deny могли бы сузить доступ.
Небезопасную policy приложение не переписывает. После проверки добавляется
собственный explicit Deny `s3:PutObject` для `staging/*`, сохраняя прочие statements.
Это блокирует ещё действующие Phase 1 PUT-ссылки; ошибка policy setup закрывает
запуск media backend. Adapter Presign остаётся инфраструктурным методом, не API.

010 отменяет старые pending. Старые private ready без доменной ссылки сохраняются
7 дней от retired_at, затем переводятся в rejected и очищаются. Их автоматически
не привязывают к ProductImage: в Phase 1 не было такой связи. Файлы остаются
приватными в течение retention. Старый staging очищается повторно.

SeaweedFS: один официальный `chrislusf/seaweedfs:4.54`, OCI digest
`sha256:4b42c212543a0acd4c0e675215ecdcaf62a186ef98def76c7e12701871222d15`, arm64.
Mini, порт S3 8333 только 127.0.0.1; data и filer metadata в seaweedfs_data.
Credentials из .env, приватный mode-0600 config через deploy entrypoint без вывода
ключей. CORS только localhost/127.0.0.1:5173; он не предоставляет доступ к bucket.
Bucket srez-media приватный, без anonymous identity/public policy. Filer/master/admin
не публикуются наружу. Internal endpoint http://seaweedfs:8333; для host Go — loopback.
SREZ_S3_PUBLIC_ENDPOINT сохранён для адаптера, frontend его больше не использует.

Скопируйте .env.example, создайте независимые JWT/CSRF/S3 secrets, затем
`docker compose up -d --build --wait`. Остановка без потери данных:
`docker compose stop` либо `docker compose down` **без -v**. Существующий
postgres_data не заменяется. Не меняйте bucket/endpoint без переноса данных;
не переиспользуйте bucket с внешними anonymous/public настройками.

## Согласование, удаление, recovery

1. SQL journal/key и цель создаются до работы с объектом. Claim долговечен и одноразовый.
2. Только normalized bytes отправляются под случайный 256-bit key `assets/...`.
3. Ready + новая ссылка + detach старой ссылки коммитятся вместе. До commit старый
   файл продолжает работать; ошибка S3/SQL не убирает текущий аватар/логотип/фото.
4. S3 DELETE никогда не выполняется в пользовательской транзакции. GC выбирает
   максимум 100 expired processing/deleting и отдельно до 10 deleted для reconciliation,
   FOR UPDATE SKIP LOCKED,
   проверяет отсутствие ссылок и фиксирует terminal deleting до сетевого DELETE.
5. SQL update request-state выполняется после освобождения asset locks, чтобы не
   инвертировать request→asset порядок финализации. DELETE идемпотентен; DB/S3 ошибки
   повторяются. Даже ошибка одного объекта не блокирует остальные в batch.
6. Миграция 011 добавляет deleted_at/reconcile_until/resweep_after и индексы.
   Tombstones допускаются к повторной проверке не чаще часа в течение 24 часов
   после первого успешного DELETE; окно не продлевается. Worker работает каждую
   минуту. История сохраняется после окна без вечных S3 DELETE. Backlog может
   задержать sweep, это не SLA проверки каждого объекта раз в час.
   Выбранный resweep становится deleting: при ошибке его DELETE повторяется в
   обязательной очереди даже после окончания окна; существующее окно не сбрасывается.
   Исторические 009 cleaned rows также перестают пересматриваться после 24 часов
   от expiry; ready retirement заново устанавливает cleaned=false для удаления.
7. При неоднозначном SQL commit backend НЕ удаляет кандидат. Следующее чтение/retry
   различает committed complete и processing. GC консультируется с текущими SQL
   ссылками/состоянием; live ready не удаляется. Откат published aggregate отменяет
   также deletion trigger. Разделяемых assets нет: инвариант ровно одной ссылки.

Cleanup необходимо запускать и наблюдать media_cleanup_failed и media_gc:
pending (обязательные due), oldest_seconds (возраст oldest due, не сбрасывается
на retry), reconciliation (due в действующем окне). Для bucket mismatch пишется
media_bucket_configuration_mismatch без имён/keys/credentials. Пока приложение
остановлено, worker не работает. Retention audit/tombstone rows пока не сокращается.

## Тесты и ограничения

`scripts/test-media.sh` создаёт уникальный Compose project, новые отдельные volumes,
случайные credentials и dynamic loopback ports, запускает полный Go suite с реальными
PostgreSQL/SeaweedFS, focused HTTP flow и проверку после restart. Teardown — down
без -v; тестовые volumes сохраняются, существующие пользовательские не монтируются.
При restart host port считывается заново. Без TEST_DATABASE_URL/TEST_S3_ENDPOINT
соответствующие интеграционные тесты явно skipped; это не считается PASS.

[Реальные результаты и matrix](media-storage-validation.md).
[Read-only security review итогового diff](media-storage-security-review.md).

Остаются: отсутствие distributed rate limiting/дисковых квот и автоматического
масштабирования GC; нет CDN/деривативов/EXIF autorotate/анимации; JPEG lossy;
20 pending на actor, один upload и два delivery slots — локальные MVP ограничения. Нужны мониторинг,
backup SQL+filer/volume, политика хранения audit rows и нагрузочные бюджеты перед
публичным развёртыванием. Неавторизованные сырые файлы не публикуются.

## Перенос bucket

MVP обслуживает один bucket. Сохранённый bucket сверяется с активной настройкой
до Get/Put/Delete и до claim/finalize; legacy cleanup также проверяет bucket.
Несовпадение даёт явную внутреннюю ошибку конфигурации, не удаляет объект из
нового bucket по совпадающему key и не отмечает старый объект deleted.

Менять только env нельзя. Остановите media writers/GC, сделайте backup SQL и
SeaweedFS, создайте новый приватный bucket с проверенной policy/ACL. Скопируйте
все нужные объекты по сохранённым ключам, полностью сравните длину и SHA-256.
Согласованно обновите bucket в media_assets и legacy media_uploads и конфигурацию
(offline, без concurrent writers), затем проверьте ready references и startup.
Старый bucket сохраняйте для rollback до отдельного решения о retention;
при rollback верните и SQL location, и runtime config. Автоматического переноса нет.

24 часа reconciliation — инженерное окно для bounded backend/SDK операций,
а не гарантия против произвольного внешнего PUT через дни. После длительного
отключения worker/S3 или превышения backlog оператору нужна отдельная сверка
журнала и inventory. Не выдавайте runtime credentials внешним writers.

[Исправления пяти замечаний независимого аудита](media-storage-audit-fixes.md).
