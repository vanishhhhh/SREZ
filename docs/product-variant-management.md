# Product Variant Management (PVM)

Текущий контракт после Media Storage: owner/editor управляют цветами, размерами, остатками и порядком фотографий. **Внешние URL доступны только для чтения старых данных; новые изображения проходят Go Media API.** Полная таблица всех13 PVM endpoints и HTTP-кодов — [единая карта API](api-reference.md#товары-и-pvm).

## Доступ и границы агрегата

Каждый private маршрут требует Bearer JWT, действующую серверную сессию и active membership owner/editor именно бренда товара. Invited/declined/revoked не дают прав; suspended бренд запрещает управление. Чужие brand/product/color/variant/image IDs дают404. Роли и parent IDs повторно проверяются внутри транзакции; owner_id/status/brand_id нельзя подменить через PATCH.

Публичный GET /products/:id не требует JWT и возвращает только published товар active бренда. Управляющие GET через PVM доступны владельцу/редактору, включая draft/hidden. При конфликте публикации операция откатывается целиком.

## Цвет

POST /products/:id/colors —201, PATCH /products/:id/colors/:color_id —200:

```json
{ "color": "Чёрный" }
```

ManagedColor:

```json
{ "id": 11, "product_id": 7, "color": "Чёрный", "images": [] }
```

GET коллекции возвращает массив. DELETE цвета —204 и атомарное удаление допустимых дочерних данных. Существующие связи shopping/orders и publication checks не обходятся; нарушение инварианта даёт409/rollback. Дубликат имени в том же товаре недопустим.

## Размер и остаток

POST /products/:id/colors/:color_id/variants —201; PATCH .../variants/:variant_id —200. PATCH частичный; пустой запрос запрещён.

```json
{ "size": "M", "stock": 5, "sku": "KONTUR-M-BLACK" }
```

ManagedVariant содержит id,product_color_id,size,stock,sku. Size до20 Unicode-символов, нормализуется по доменным правилам; дубликат в цвете запрещён. Stock целый неотрицательный с проверкой диапазона; SKU необязателен. Нулевой остаток разрешён, такой размер купить нельзя. GET возвращает массив, DELETE —204 с сохранением целостности ссылок/резервов.

Общий available_sizes товара объединяет все цвета и не доказывает наличие M у выбранного цвета. UI использует конкретный product_variant_id; корзина и заказ повторно проверяют вариант/остаток на сервере.

## Фотографии через backend

1. POST /media/uploads с purpose=product_image, product_id, color_id, content_type и size_bytes.
2. PUT content_url с raw File/Blob, Content-Type и Content-Length. Backend проверяет права, фактическое содержимое и атомарно связывает нормализованный asset.
3. Ответ/управляющий GET содержит стабильный image_id и media_id; URL формируется как /media/assets/:id/content. Для замены передайте image_id в запросе upload.

[Полные JSON и ограничения Media](media-storage.md). Нет прямого приватного S3 upload и нет POST external HTTPS URL.

POST /products/:id/colors/:color_id/images —**200**, подтверждение уже связанного managed asset:

```json
{ "media_id": 31, "position": 0 }
```

PATCH .../images/:image_id принимает media_id и/или position, возвращает200. Поле url отклоняется; для новой фотографии/замены используйте upload. Пример ManagedImage (ID условные, берите реальные из API):

```json
{
  "id": 41,
  "product_color_id": 11,
  "url": "/media/assets/31/content",
  "position": 0,
  "media_id": 31
}
```

GET .../images — массив по position, DELETE —204. Reorder POST .../images/reorder принимает все актуальные image_ids ровно по одному и возвращает200/полный массив. Позиции начинаются с0, без дыр; move сдвигает соседей. Первая фотография — обложка цвета. На цвет до10 фотографий; ID сохраняется при замене и перестановке. Старые HTTPS изображения продолжают отображаться.

## Публикация и workflow UI

Товар создаётся draft через POST /products. Для публикации нужны описание, положительная цена, категория/материалы, цвет с фото и допустимые варианты по текущим инвариантам, active бренд с полными данными. Публикация выполняется отдельным POST /products/:id/publish с {}.

Если изменение структуры нарушит published агрегат, UI предлагает явное hide → edit → publish. Автоматически скрывать/публиковать товар ради прохождения формы нельзя. Любая ошибка сохраняет прежнее состояние и timestamp; SQL rollback восстанавливает дочерние связи.

React ProductEditor сохраняет форму, права и дизайн. Размеры/остатки вынесены в features/product-editor/Variants.tsx, фотографии/замена/reorder — в Images.tsx. Общие UploadField, useFormDraft и unsaved-changes guard сохраняют draft при позднем GET, предупреждают о потере выбранного файла и не выдают сетевую неопределённость за успешное сохранение.

## Ошибки и проверка

400: ID/тип/JSON/лишние поля, пустой PATCH, stock вне диапазона, неверная позиция/reorder, external URL write. 401: недействительная сессия/JWT. 403: достаточной роли нет. 404: чужая/неизвестная вложенная сущность. 409: дубликат, лимит фото, конфликт состояния/публикации/резерва. Infrastructure500/503 не содержит SQL/секретов.

Go/PostgreSQL tests проверяют полный aggregate, parent scoping, concurrent изменения, FK/rollback и timestamp. Browser seller scenarios проходят create brand → managed logo → product → color/size/photos → publish → editor invite/accept/revoke, отдельно cover/reorder/replace/delete и формы с задержками. Команды — [README](../README.md); фактические результаты последнего этапа — [release report](release-report.md).
