# SREZ — редизайн витрины и локальный showcase

18 ноября 2026 · `feat/fashion-ui-redesign` · база `81801a12`

Витрина работает на [localhost:5174](http://localhost:5174/). Ветка находится в отдельном Git worktree. Незавершённая работа по macOS осталась в исходном checkout. Изменения готовы к локальному ревью.

## Финальная полировка

На главной теперь fashion-кадр, новые поступления и карточки брендов. Самостоятельный блок худи удалён, переход к категории остался. Витрина использует footer с copyright и ссылкой «Наверх», личные страницы — только copyright. Мобильный каталог переводит фокус в первый фильтр и возвращает его после Escape или применения. На пустой странице вишлистов осталось одно действие «Создать подборку». Сердце в избранном и закладка вишлиста по-прежнему открывают разные сценарии.

Карточка бренда берёт `banner_url`, при отсутствии — `logo_url`. Публичная страница без баннера сохраняет нейтральную шапку и отдельный логотип. Логотип, баннер и фото товара загружаются с разными media purpose. Любую демозамену выполняет владелец через «Загрузить баннер» в настройках бренда.

Для отдельных широких editorial-баннеров можно использовать следующие направления. Это предложения для съёмки.

| Бренд | Кадр |
| --- | --- |
| КОНТУР | Архитектурный бетон и молочная ткань, с ясным силуэтом в центре. |
| ПОЛЮС | Холодный рассеянный свет и многослойный силуэт на открытом пространстве. |
| СМЕНА | Чистая городская геометрия и движение, с одним кобальтовым акцентом. |
| ОСНОВА | Материалы крупным планом в тёплом свете мастерской, с запасом по краям. |
| РАЙОН | Уличная среда и съёмка аксессуаров в использовании, с читаемой центральной группой. |

Дальше в отчёте зафиксирован исходный этап редизайна до этой полировки.

## Визуальное направление

Широкая редакционная фотография задаёт характер первой страницы. Тёплая светлая подложка, графитовая одежда, Manrope и компактная шапка связывают её с каталогом. Две ссылки под кадром ведут в каталог и к брендам. На телефоне под фотографией видны ссылки, начало новых поступлений и категории.

У LEMAIRE изучен ритм между типографически тихой навигацией и фотоматериалом. У The Row — монохромный портрет как главный элемент первого экрана и компактная служебная навигация. У Our Legacy — прямые входы в коллекции и простые подписи. У Jil Sander изучены разделение кампаний и товарных разделов по официальной странице и её структуре. Прямой браузерный запрос к Jil Sander вернул 403, а первый кадр Our Legacy в Chromium показал только основные элементы шапки. [LEMAIRE](https://www.lemaire.fr/), [The Row](https://www.therow.com/), [Our Legacy](https://www.ourlegacy.com/), [Jil Sander](https://www.jilsander.com/en-jp/)

### Два варианта первого экрана

| Вариант | Desktop | Mobile | Решение |
| --- | --- | --- | --- |
| **Fashion editorial** | Модель в графитовом пальто на бетонной террасе. Один широкий кадр внутри полей страницы. Фигура помещается целиком, ссылки лежат на своей светлой строке под кадром. | Отдельный кроп сохраняет голову, пальто и шаг модели. Кадр 350×398 px при viewport 390 px. | Рабочий вариант. Городская среда и силуэт сразу объясняют характер площадки. |
| **Пейзаж** | Вулканическая глыба на мелководье. Кадр слева, ссылки справа. Меньшая высота позволяет раньше увидеть новые поступления. | Форма камня читается, ссылки переходят под фото, сетка начинается раньше. | Вариант для редакционного выпуска. Для постоянной главной кадр медленнее объясняет, что SREZ связан с одеждой. |

Оба варианта просмотрены на настоящем SREZ с реальным API. Пейзаж проверен локальной подменой изображения и композиции в Chromium. Оба исходных изображения созданы встроенным ImageGen для этого проекта. Коммерческие изображения сайтов-ориентиров в интерфейсе не использовались. Рабочие WebP находятся в `frontend/src/features/storefront/editorial`. Исходники остались в папке результатов ImageGen. Полные промпты приведены в конце.

## Изменения интерфейса

- Убраны чёрная верхняя полоса, старый hero с иллюстрацией худи и лозунгами, полоса под hero и рекламный блок студии. В шапке и footer сохранены прямые входы в студию. На главной остались новые товары, категории, подборка худи и бренды с логотипами. [HomePage.tsx](../frontend/src/features/storefront/HomePage.tsx), [home.css](../frontend/src/features/storefront/home.css), [Layout.tsx](../frontend/src/components/Layout.tsx)
- Шапка стала ниже. Сохранены логотип, каталог, бренды, вишлисты, заказы, студия, корзина, избранное и аккаунт. При раскрытии мобильной навигации фокус переходит к первому пункту, Escape закрывает меню и возвращает фокус. Footer содержит логотип, три раздела и copyright. [Layout.tsx](../frontend/src/components/Layout.tsx), [styles.css](../frontend/src/styles.css)
- В каталоге оставлены прежние фильтры, сортировка, пагинация и сетка. У товарной карточки выше читаемость цены и метаданных, размеры и цвета могут переноситься, состояние отсутствия товара не накладывается на controls. Убрана третья иконка поверх фото. Карточка по-прежнему ссылка, избранное и вишлист остаются отдельными действиями. [CatalogPage.tsx](../frontend/src/features/storefront/CatalogPage.tsx), [ui.tsx](../frontend/src/components/ui.tsx)
- Из списка и публичного профиля бренда, карточки товара, авторизации и студии убраны шаблонные фразы. В публичном профиле сохранены баннер, логотип, реальное описание, контакты и ассортимент. Вход и регистрация получили прямые названия. Глобальные кнопки, формы и состояния используют существующие компоненты. [BrandPages.tsx](../frontend/src/features/storefront/BrandPages.tsx), [ProductPage.tsx](../frontend/src/features/storefront/ProductPage.tsx), [Auth.tsx](../frontend/src/pages/Auth.tsx), [AuthFrame.tsx](../frontend/src/components/AuthFrame.tsx), [dashboard/Layout.tsx](../frontend/src/features/dashboard/Layout.tsx)

## Браузерная проверка

Скриншоты сделаны в Chromium на macOS ARM64 с одним и тем же `srez-integration` и каталогом. Для состояния «до» использована отдельная временная сборка источника `81801a12`, подключённая к тому же Go API. Тестовый frontend «до» остановлен после съёмки.

| Проверка | Наблюдение |
| --- | --- |
| 1440×900 и 1920×1080 | На первом экране вся фигура попадает в кадр, под ней видны «Каталог» и «Бренды». Ширина изображения 1344 и 1440 px соответственно. |
| 834×1112 | Под кадром видны категории и начало товарной сетки. Сохранена полная верхняя навигация. |
| 390×844 и 360×800 | Выбран мобильный WebP. На просмотренных страницах главной, каталога, бренда и товара горизонтального переполнения нет. При 390 px проверены клавиатурные вход и выход из меню. |
| Каталог, бренд, товар | На desktop и mobile отображаются реальные фото. На товарных карточках фото свободно от декоративной стрелки. На странице товара галерея, цена и интерфейс выбора варианта сохранены. |
| Footer | Ссылки работают с desktop/mobile. На узкой ширине логотип, ссылки и copyright занимают отдельные строки. |

Комплект скриншотов хранится вне Git. Имена имеют формат `before-<страница>-<устройство>.png` и `after-<страница>-<устройство>.png`. Для desktop и mobile сохранены `home`, `catalog`, `brand`, `product` и `footer`. Также есть `alternative-landscape-desktop.png`, `alternative-landscape-mobile.png` и финальные снимки главной для tablet, 360 и 1920 px.

### Производительность и доступность

- Desktop WebP 1536×1024 весит **96 352 байта**, mobile WebP 768×764 — **40 566 байт**. Chromium запросил только подходящий вариант на каждой проверенной ширине. `picture` задаёт источник, `fetchPriority="high"` приоритет, CSS резервирует размер контейнера. Vite добавляет хеш к URL, nginx выдаёт этот файл по существующей политике immutable для `/assets/`.
- На просмотренных размерах начальный CLS главной был менее **0,002** по браузерному PerformanceObserver. Это локальный замер до съёмки. Текст и ссылки находятся на светлом фоне рядом с изображением. Hero статичный. Существующий CSS для `prefers-reduced-motion` действует на остальную страницу.
- Каталожные изображения отрисовываются существующим `Media` с lazy-атрибутом на `img`. Для managed-медиа сетевой запрос Blob начинается через прежнюю очередь при mount, поэтому этот атрибут сам по себе не откладывает скачивание. Очередь сохраняет прежние ограничения API. [useImage.ts](../frontend/src/features/media/useImage.ts)
- Проверка проведена в Chromium. Отдельный просмотр Safari и screen reader в этот прогон не входили.

## Демоданные в srez-integration

Проверены Compose labels для app, db и SeaweedFS, имена volumes и read-only счётчики. Перед seed новые локальные таблицы users, brands, products и media_assets были пусты. Dry-run штатного скрипта проверил manifest и 21 уникальный PNG. Демоаккаунт подтвердил email в браузере по письму из локального Mailpit. Повторный запуск штатного `--apply` опубликовал ассортимент через действующий Go API. В контрольном повторе количества и байты совпали. [seed-showcase.mjs](../scripts/seed-showcase.mjs), [catalog.json](../demo/catalog/catalog.json)

| Что проверено | Количество |
| --- | ---: |
| Активные вымышленные бренды | 5 |
| Опубликованные товары | 21 |
| Категории | 7 |
| Managed-фотографии товаров | 21 |
| Managed-логотипы | 5 |
| Managed-баннеры | 5 |
| Ready media records, уникальные ключи объектов и действующие доменные ссылки | 31 каждого вида |
| Публичные Go media endpoints с HTTP 200 и изображением | 31 из 31 |

После пересоздания контейнера SeaweedFS на том же volume все 31 объекта повторно прочитаны через Go и nginx. SHA-256 каждого HTTP-тела совпал с замером до перезапуска. SQL-запросы выполнялись только для чтения. Публичный просмотр идёт через Go media endpoints, а seed загружает файлы штатным managed upload. QA использована отдельно для playwright fixtures. [Модель storage и доменные ссылки](../migrations/010_media_ownership.sql), [Go media handler](../internal/handler/media.go)

Существующие 21 предметный кадр подходят для сетки. Выдержаны общий свет, дистанция и спокойный фон, в цветных вещах сохранён реальный цвет товара. Слабое место — баннеры, сделанные seed из первых кадров товаров. При следующей работе с контентом сначала заменить широкими исходниками баннер КОНТУРА, где при кропе видна в основном ткань футболки, и РАЙОНА, где первый кадр футболки не передаёт форму ассортимента бренда. Товарные кадры оставить. Новый hero главной — отдельный редакционный материал.

## Тесты

- `npm run typecheck`, `npm run lint`, `npm run format:check`, `npm test` и `npm run build` прошли. Vitest — **96 тестов**.
- Полный Playwright suite выполнен на `srez-mvp-qa`, с Chromium как browser channel во временной копии Playwright-конфига. Финальный результат — **36 из 36**, включая 9 публичных сценариев. Первый прогон дал 35/36: тест входа ещё искал удалённый декоративный заголовок. Селектор заменён проверкой фактического заголовка, логотипа и переполнения панели.
- Go API, названия Compose-проектов, SQL и Media Storage код остались прежними. Реальные auth, buyer, seller, резервы и managed media проверяются E2E. Локальная отдельная проверка media описана выше.

## Изменённые файлы

- Главная и изображения — `frontend/src/features/storefront/HomePage.tsx`, `home.css`, `editorial/fashion-cover.webp`, `editorial/fashion-cover-mobile.webp`. Удалён старый `frontend/public/editorial.svg`.
- Общий интерфейс — `frontend/src/App.tsx`, `frontend/src/styles.css`, `frontend/src/components/Layout.tsx`, `ui.tsx`, `AuthFrame.tsx`.
- Остальные страницы — `frontend/src/features/storefront/CatalogPage.tsx`, `BrandPages.tsx`, `ProductPage.tsx`, `frontend/src/features/dashboard/Layout.tsx`, `frontend/src/pages/Auth.tsx`.
- Сценарии проверки — `frontend/e2e/auth.spec.ts`, `frontend/e2e/public.spec.ts`.
- Отчёт — `docs/fashion-showcase-review.md`.

## Будущий Brand Page Builder

Оставить платформенную главную в `HomePage` с собственной композицией. Для страниц брендов выделить общий renderer типизированных блоков, пригодный для публичного просмотра и приватного preview. Привязку палитры и токенов бренда ограничить контейнером витрины. В качестве первой версии выбрать баннер, фото с текстом, историю, каталог и контакты. Потом добавлять lookbook, галереи и подборки.

Для черновика и публикации предусмотреть явные версии и раздельное чтение, чтобы правки не меняли уже опубликованную страницу. Порядок блоков хранить отдельно от их содержимого. Изображения оставлять managed assets с явной привязкой и правилами crop. Сохранить базовую страницу для существующих брендов. В этой задаче конструктор не реализован.

## Промпты встроенного ImageGen

**Выбранный Fashion Editorial**

```text
Use case: photorealistic-natural. Original fashion editorial website hero, horizontal landscape composition approximately 3:2. A single young adult androgynous fashion model in a charcoal-black oversized technical cotton coat with a sculptural high collar, wide dark trousers, and unbranded shoes, walking slowly left to right across a pale chalk-grey concrete brutalist rooftop. The dark figure in three-quarter profile sits in the middle 35% of the frame, entire head and silhouette visible, moderate distance, top of head at about 22% frame height and feet at about 78%, enough safe margin either side for a centered mobile crop. Architectural steps and long horizontal parapet, pale overcast sky. Use restrained monochrome warm greys and near black. Camera at waist height, moderate-long 50mm look, dry matte clothing folds, subtle film grain and credible diffuse daylight, contemporary independent art-fashion magazine photograph, unposed, natural anatomy. Moody without black crush. Full bleed single photo, no text, logo, typography, brand cue, border, collage, watermark, commercial likeness, or typography overlay. Coherent understated editorial realism, no glossy CG or conspicuous effects.
```

**Nature / Landscape**

```text
Use case: photorealistic-natural. Original horizontal fine-art landscape photography for a minimalist streetwear marketplace editorial cover, approximately 3:2. A solitary massive wedge of weathered dark volcanic stone breaking the surface of a pale grey tidal flat on an overcast morning, seen low and slightly oblique. Main wedge in middle 40 percent with headroom and negative space at both edges to preserve the form in a center mobile crop. Shallow water naturally reflects sky, one thin misty horizon at far distance, rough basalt pores and fine mineral texture. Quiet abstract sculptural weight with an off-center hard diagonal edge, monochrome off-white, greige, slate and near black, beautiful subtle tonal separation, medium format documentary photograph, subdued real light and delicate 35mm grain. A single frame and a believable physical landscape, no people, text, logo, buildings, decorative borders, dramatic cinematic effects, recognizable art or landmark, watermark, or artificial polish.
```
