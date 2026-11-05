Implemented in #344

# Handoff: Home

2026-11-05 · TARI-cset, задача дизайна TARIDESIGN-38fp (второй проход Home после #342) · story `mockups-home--long-text-360`, `mockups-home--long-text-1024`, `mockups-home--width-360`, `mockups-home--width-1024`, `mockups-home--busy`, `pages-home-homepage--long-text`.

Правка закрывает дефект Floor «рабочая ширина телефона». На 360 px строка Running now и Recent не показывала имя агента, строка Needs you не показывала заголовок задачи. Теперь каждая строка называет агента или задачу.

Причина дефекта: метаданные строки имеют класс `shrink-0`. Длинные метаданные (`1 h 12 min · a-build-host-with-a-deliberately-long-label`) занимают всю строку, и `min-w-0` сжимает имя до нуля. В Needs you колонка ключа `w-24` и метаданные `shrink-0` оставляют заголовку ноль пикселей.

## What changed

- Ниже `sm` (640 px) метаданные строки Running now и Recent стоят на второй строке под именем. Имя и метаданные обрезаются через `truncate`. Точка статуса Running now стоит у первой строки. Story: `mockups-home--long-text-360`, `pages-home-homepage--long-text` (360 px).
- От `sm` и шире строка остаётся однострочной. Метаданные занимают не больше половины строки (`sm:max-w-1/2`) и обрезаются, остаток получает имя. Story: `mockups-home--long-text-1024`, `pages-home-homepage--long-text` (1024 px).
- Ниже `sm` строка Needs you — сетка из двух колонок: ключ (6rem) и заголовок в первой строке, метаданные под заголовком. От `sm` строка однострочная, метаданные не шире половины места после ключа. Story: `mockups-home--long-text-360`, `mockups-home--width-360`, `mockups-home--long-text-1024`.
- При коротких метаданных вид на 1024 px не изменился. Story: `mockups-home--width-1024`, `mockups-home--busy`, `pages-home-homepage--default`.

Изменения в репозитории:

- `ui/src/pages/home/HomePage.view.tsx`: новая локальная часть `AgentLinkText` (имя и метаданные) для строк Running now и Recent. Правка в `HomeView` готова, разработчику её менять не нужно.
- `ui/src/mockups/home/NeedsYouList.view.tsx`: новые классы строки (`ROW_CLASS`) и метаданных. Это локальная копия разметки контейнера. Разработчик переносит те же классы в `ui/src/pages/home/NeedsYouList.tsx`, строки 93–115:
  - `ROW_CLASS`: `grid w-full min-w-0 grid-cols-[6rem_minmax(0,1fr)] items-baseline gap-x-3 rounded-control px-2 py-1.5 text-left text-body-sm hover:bg-accent sm:flex`;
  - метаданные: `col-start-2 min-w-0 truncate text-caption text-muted-foreground sm:max-w-1/2 sm:shrink-0`;
  - ключ и заголовок без изменений.
- `ui/src/mockups/home/Home.stories.tsx`: данные `longText` и story `LongText360`, `LongText1024`. Данные: длинное имя агента из `agentLinks.longText` в Running now и Recent, длинные ключ, заголовок и метаданные задачи в Needs you.

## New props

Нет.

## Changed and removed props

Нет.

## States

| Состояние                                                                                      | Story                                                                                           |
| ---------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| Обычная страница: 2 агента работают, 2 записи в Needs you                                      | `mockups-home--width-1024`, `mockups-home--width-360`                                           |
| Много агентов: 50 в боковой панели, 17 работают, 6 записей в Needs you                         | `mockups-home--busy`                                                                            |
| Пусто: нет работающих и недавних агентов                                                       | `pages-home-homepage--empty`                                                                    |
| Ошибка итогов дня: `totals: null`, текст ошибки под плитками                                   | `pages-home-homepage--today-error`                                                              |
| Needs you: загрузка (`Loading…`) и ошибка (`Could not read what needs you: …`, кнопка `Retry`) | нет story: состояния живут в контейнере `NeedsYouList.tsx`                                      |
| Длинное имя агента и длинные метаданные: имя видно, обе части обрезаются через `truncate`      | `pages-home-homepage--long-text`, `mockups-home--long-text-360`, `mockups-home--long-text-1024` |
| Длинные ключ, заголовок и метаданные задачи в Needs you: заголовок виден                       | `mockups-home--long-text-360`, `mockups-home--long-text-1024`                                   |

Поправка к первому проходу (#342): строка «Длинное имя агента: имена обрезаются через `truncate`» была неверной. Story `pages-home-homepage--long-text` на 360 px показывала только метаданные, имя было скрыто полностью (замечание О1 проверки TARIDESIGN-vk4z).

## Behavior

Поведение не изменено:

- Строка в Running now и Recent открывает агента по `href`. Вся двухстрочная строка — одна ссылка.
- Строка задачи в Needs you открывает панель задачи; шеврон `Answer in place` раскрывает карточку ответа. Строка письма открывает письмо. Вся двухстрочная строка — одна кнопка.
- Hover подсвечивает всю строку (`hover:bg-accent`), фокус — как раньше.
- В макете кнопки ничего не делают.

## Texts

Новых текстов нет.

## Accessibility

- Порядок чтения не изменён: имя, затем метаданные, внутри одной ссылки или кнопки.
- Порядок фокуса не изменён: `New agent`, строки Needs you, строки Running now, `History ›`, строки Recent.
- Новых пар токенов нет, контраст не изменён.
- Обрезанный текст не имеет `title`. Полное имя видно на странице агента. Это поведение первого прохода.

## Layout at widths

| Ширина  | Раскладка                                                                                                      | Story                                                                            |
| ------- | -------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| 360 px  | Только остров. Строки Running now, Recent и Needs you в две строки: имя или заголовок сверху, метаданные снизу | `mockups-home--long-text-360`, `mockups-home--width-360`                         |
| 390 px  | Как 360 px                                                                                                     | `mockups-home--width-390`                                                        |
| 820 px  | Боковая панель и остров. Строки однострочные, метаданные не шире половины строки                               | `mockups-home--width-820`                                                        |
| 1024 px | Как 820 px                                                                                                     | `mockups-home--long-text-1024`, `mockups-home--width-1024`, `mockups-home--busy` |
| 1180 px | Как 1024 px                                                                                                    | `mockups-home--width-1180`                                                       |

Снимки (`npm run story:shot -- <story-id> <light|dark> <width>`), каталог `ui/.story-shots/`:

- `mockups-home--long-text-360.light.360.png`, `mockups-home--long-text-360.dark.360.png`
- `mockups-home--long-text-1024.light.1024.png`, `mockups-home--long-text-1024.dark.1024.png`
- `mockups-home--width-360.light.360.png`, `mockups-home--width-360.dark.360.png`
- `mockups-home--width-1024.light.1024.png`, `mockups-home--width-1024.dark.1024.png`
- `mockups-home--busy.light.1024.png`, `mockups-home--busy.dark.1024.png`, `mockups-home--busy.light.360.png`, `mockups-home--busy.dark.360.png`
- `pages-home-homepage--long-text.light.360.png`, `pages-home-homepage--long-text.dark.360.png`, `pages-home-homepage--long-text.light.1024.png`, `pages-home-homepage--long-text.dark.1024.png`
- `pages-home-homepage--default.light.360.png`, `pages-home-homepage--default.dark.360.png`, `pages-home-homepage--default.light.1024.png`, `pages-home-homepage--default.dark.1024.png`

Каталог `ui/.story-shots/` не входит в git. Снимки повторяют командой выше.

## Tokens and styles

Токены `ui/src/index.css` и варианты `cva()` не изменены. Правка применяет только утилиты Tailwind: `flex-col`, `sm:flex-row`, `grid-cols-[6rem_minmax(0,1fr)]`, `col-start-2`, `sm:max-w-1/2`, `self-start`.

Отступления от правил Hallmark (Reflex), причина одна: правка не меняет вид продукта, токены и формат продукта главнее.

- Строки метаданных через `·`: формат продукта.
- Точка статуса в Running now: продукт кодирует ею состояние.
- Стрелка `›` в `History ›`: текущий текст продукта.

## Acceptance criteria

1. На 360 px в `mockups-home--long-text-360` и `pages-home-homepage--long-text` видно начало имени агента в Running now и Recent, в темах `light` и `dark`.
2. На 360 px в `mockups-home--long-text-360` и `mockups-home--width-360` видно начало заголовка задачи и темы письма в Needs you.
3. На 1024 px в `mockups-home--long-text-1024` и `pages-home-homepage--long-text` строки однострочные, метаданные не шире половины строки, имя и заголовок видны.
4. На 1024 px `mockups-home--width-1024` и `pages-home-homepage--default` совпадают с видом до правки.
5. Контейнер `NeedsYouList.tsx` применяет классы строки и метаданных из `NeedsYouList.view.tsx` макета. Реальная страница Home на 360 px совпадает с `mockups-home--width-360`.
6. `make designer-check` на последнем коммите ветки: четыре шага `ok`.

## Open questions

1. На 360 px колонка ключа `w-24` оставляет заголовку около 135 px: видно 15–20 знаков заголовка. Я оставил колонку ключа: ключи выровнены и строки задач легко сравнить. Если 135 px мало, ниже `sm` ключ можно перенести на строку метаданных. Нужно ли это?

   Ответ разработчика: колонку `w-24` оставили. Это решение продукта. Вид соответствует макету.

2. Правка `HomeView` уже в этой ветке. Нужен ли тест `HomePage.view.test.tsx` на то, что имя агента остаётся в разметке рядом с длинными метаданными? Вид проверяют снимки; jsdom ширину не считает, поэтому тест проверит только разметку.

   Ответ разработчика: тест добавлен в `ui/src/pages/home/HomePage.view.test.tsx`. Он проверяет разметку: имя агента стоит перед метаданными в Running now и Recent и имеет класс `truncate`. Метаданные имеют классы `truncate`, `sm:max-w-1/2` и `sm:shrink-0`. Ширину jsdom не считает, вид проверяют снимки. Тест строки Needs you лежит в `ui/src/pages/home/NeedsYouList.test.tsx`.

3. Разделение `NeedsYouList.tsx` на контейнер и view (вопрос 1 первого прохода) всё ещё открыто. До него разработчик переносит классы вручную, и копия макета может разойтись с контейнером.

   Ответ разработчика: разделение не входит в этот PR. Это отдельная работа. Пока контейнер и макет разделены, копия макета может разойтись с контейнером. Тест `NeedsYouList.test.tsx` фиксирует классы контейнера, но не заменяет разделение.

Вопросы первого прохода (#342), исправленные по факту:

- Вопрос 3 (story длинного имени агента). Исправленный ответ: story `pages-home-homepage--long-text` добавлена в #342, но на 360 px она показывала дефект: имя агента скрыто полностью. Правка этой ветки исправляет дефект, story теперь показывает имя.
- Вопрос 5 (заголовок задачи в Needs you на 360 px). Ответ: заказчик решил править в TARI-cset. Правка в этой ветке, раздел «What changed».
