Implemented in #342

# Handoff: Home

2026-11-05 · задача дизайна TARIDESIGN-an52 (пилот этапа 6, TARI-v96u) · story `mockups-home--width-360`, `mockups-home--width-390`, `mockups-home--width-820`, `mockups-home--width-1024`, `mockups-home--width-1180`, `mockups-home--busy`.

Storybook строит id story из имени экспорта: `Width360` даёт `width-360`. Разделы 5 и 6 спецификации пишут `mockups-home--width1024`; верная форма — `mockups-home--width-1024`.

Макет повторяет текущую страницу Home. Редизайна нет. Макет проверяет контракт «дизайнер — разработчик» на одной странице: если разработчик переименует проп `HomeViewProps`, сборка `HomePage.stories.tsx` и `Home.stories.tsx` падает на шаге `tsc -b` команды `make designer-check`.

## What changed

Вид страницы не изменён. Изменения в репозитории:

- Новый макет `ui/src/mockups/home/Home.stories.tsx` (`Mockups/Home`): боковая панель на фоне `--background` и остров контента с `HomeView`. Все story `mockups-home--*`.
- Новая локальная часть вида `ui/src/mockups/home/NeedsYouList.view.tsx`. Она повторяет разметку строк контейнера `ui/src/pages/home/NeedsYouList.tsx`. Story: `mockups-home--width-1024`, `mockups-home--busy`.
- Новая фабрика `taskActivityTotals(over)` в `ui/src/fixtures/activity.ts`. Story `HomePage.stories.tsx` берёт итоги дня из неё, а не из константы `totals`. Вид story `pages-home-homepage--default` не изменён.

## New props

Новых пропов у `HomeView` нет.

Предложение для разделения контейнера `NeedsYouList` (вопрос 1):

| Component                       | Prop                 | Type                                                                            | Meaning                                     | Source of data                                                                                             |
| ------------------------------- | -------------------- | ------------------------------------------------------------------------------- | ------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `NeedsYouListView` (новый view) | `rows`               | `NeedsYouViewRow[]`: `id`, `kind: "task" \| "letter"`, `label`, `title`, `meta` | Строки списка, уже отформатированные        | `needsYouRows(needs.hosts)` в `HomeContainer`; `meta` собирает `taskState`, `hostLabel` и `formatTaskTime` |
| `NeedsYouListView`              | `expanded`           | `ReadonlySet<string>`                                                           | Строки задач с открытой карточкой ответа    | состояние контейнера                                                                                       |
| `NeedsYouListView`              | `onToggle`, `onOpen` | `(id: string) => void`                                                          | Открыть карточку; открыть задачу или письмо | обработчики контейнера                                                                                     |
| `NeedsYouListView`              | `status`             | `"loading" \| "error" \| "ready"` и `error: string`                             | Состояния загрузки и ошибки                 | `useNeedsYou()`: `loaded`, `hosts[].error`                                                                 |

Окончательный тип определяет разработчик.

## Changed and removed props

Нет.

## States

| Состояние                                                                                      | Story                                                                 |
| ---------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| Обычная страница: 2 агента работают, 2 записи в Needs you                                      | `mockups-home--width-1024`                                            |
| Много агентов: 50 в боковой панели, 17 работают, 6 записей в Needs you                         | `mockups-home--busy`                                                  |
| Пусто: нет работающих и недавних агентов                                                       | `pages-home-homepage--empty`                                          |
| Ошибка итогов дня: `totals: null`, текст ошибки под плитками                                   | `pages-home-homepage--today-error`                                    |
| Needs you: загрузка (`Loading…`) и ошибка (`Could not read what needs you: …`, кнопка `Retry`) | нет story: состояния живут в контейнере `NeedsYouList.tsx` (вопрос 1) |
| Длинное имя агента: имена обрезаются через `truncate`                                          | `pages-home-homepage--long-text`                                      |

## Behavior

Поведение не изменено:

- Строка в Running now и Recent открывает агента по `href`.
- `History ›` открывает страницу Activity (`activityHref`).
- `New agent` вызывает `onNewAgent`.
- Строка задачи в Needs you открывает панель задачи; шеврон `Answer in place` раскрывает карточку ответа на месте. Строка письма открывает письмо во входящих.
- В макете кнопки ничего не делают: у макета нет данных и маршрутов.
- Ниже 768 px (`DRAWER_QUERY`) боковая панель уходит в выдвижную панель. Макет на 360 и 390 px показывает только остров.

## Texts

Новых текстов нет. Текущие тексты, которых нет в story макета:

- `Loading…`: загрузка Needs you.
- `Could not read what needs you: <error>` и `Retry`: ошибка Needs you.
- `No agent is running.`: пустой Running now (story `pages-home-homepage--empty`).
- `Nothing needs you.`: пустой Needs you.
- `aria-label="Answer in place"`: шеврон строки задачи.
- `title="not interactive (no tty)"`: метка `non-tty` в строке агента.

## Accessibility

Одно изменение: ошибка итогов дня (`todayError`) получила `role="alert"`. Вид страницы не изменился. Текущие правила:

- Шеврон строки задачи имеет `aria-label="Answer in place"` и `aria-expanded`.
- Иконки имеют `aria-hidden="true"`.
- Ошибка Needs you имеет `role="alert"`.
- Ошибка итогов дня (`todayError`) имеет `role="alert"`. Тест: `HomeView › announces the today error as an alert`.
- Порядок фокуса: `New agent`, строки Needs you, строки Running now, `History ›`, строки Recent.
- Новых пар токенов нет, поэтому контраст не изменён.

## Layout at widths

| Ширина  | Раскладка                                                                                                                                        | Story                                            |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------ |
| 360 px  | Только остров. Плитки Today в 2 колонки, Recent в 1 колонку. В строке задачи Needs you заголовок скрыт полностью, метаданные обрезаны (вопрос 5) | `mockups-home--width-360`                        |
| 390 px  | Как 360 px                                                                                                                                       | `mockups-home--width-390`                        |
| 820 px  | Боковая панель 268 px и остров. Плитки Today в 3 колонки, Recent в 2 колонки (`sm:`)                                                             | `mockups-home--width-820`                        |
| 1024 px | Как 820 px. Шесть плиток Today в один ряд                                                                                                        | `mockups-home--width-1024`, `mockups-home--busy` |
| 1180 px | Как 1024 px, остров шире                                                                                                                         | `mockups-home--width-1180`                       |

Снимки (`npm run story:shot -- <story-id> <light|dark> <width>`), каталог `ui/.story-shots/`:

- `mockups-home--width-360.light.360.png`, `mockups-home--width-360.dark.360.png`
- `mockups-home--width-390.light.390.png`, `mockups-home--width-390.dark.390.png`
- `mockups-home--width-820.light.820.png`, `mockups-home--width-820.dark.820.png`
- `mockups-home--width-1024.light.1024.png`, `mockups-home--width-1024.dark.1024.png`
- `mockups-home--width-1180.light.1180.png`, `mockups-home--width-1180.dark.1180.png`
- `mockups-home--busy.light.1024.png`, `mockups-home--busy.dark.1024.png`
- `pages-home-homepage--default`, `pages-home-homepage--empty`, `pages-home-homepage--today-error`: темы `light` и `dark`, ширина 360 и 1024 px, например `pages-home-homepage--default.dark.360.png`. Эти story изменены: итоги дня берут из фабрики

Каталог `ui/.story-shots/` не входит в git. Снимки повторяют командой выше.

## Tokens and styles

Токены `ui/src/index.css` и варианты `cva()` не изменены. Макет применяет только существующие классы: `bg-card`, `shadow-[var(--lift)]`, `rounded-[var(--panel-radius)]`, `bg-background`, `text-muted-foreground`.

Отступления от правил Hallmark (Reflex), причина одна: макет повторяет текущую страницу без редизайна, токены и вид продукта главнее.

- Строки метаданных через `·` (`12 min · build-01`): формат продукта.
- Точка статуса в Running now и `StatusDot` в строке агента: продукт кодирует ими состояние, это не украшение.
- Стрелка `›` в `History ›`: текущий текст продукта.

## Acceptance criteria

1. `Mockups/Home` содержит story `Width360`, `Width390`, `Width820`, `Width1024`, `Width1180` и `Busy`.
2. Макет импортирует только view-компоненты (`HomeView`, `AgentRow`), фикстуры, `@/types` и `@/lib/agentState` (только тип). Импортов `@/lib/api`, `@tanstack/react-query` и вызова `fetch` нет.
3. `make designer-check` на последнем коммите ветки: четыре шага `ok`.
4. Переименование пропа `todayError` в `HomeViewProps` ломает шаг `tsc -b` в `HomePage.stories.tsx` и в `Home.stories.tsx`.
5. `HomePage.stories.tsx` берёт итоги дня из `taskActivityTotals()`; вид `pages-home-homepage--default` не изменён.
6. Реальная страница Home на 360 и 1024 px совпадает со снимками макета, кроме частей из раздела «Open questions», вопрос 4.

## Open questions

1. Разделите `NeedsYouList.tsx` на контейнер и `NeedsYouList.view.tsx` по предложению из раздела «New props»? Тогда макет возьмёт настоящий view вместо локальной копии, а состояния загрузки и ошибки получат story.

   Ответ: нужно, но отдельной задачей. Контейнер держит панель задачи, карточку ответа и данные `useNeedsYou`. Это рефакторинг вне пилота. До него макет применяет локальную копию `ui/src/mockups/home/NeedsYouList.view.tsx`.

2. Добавить `role="alert"` к тексту `todayError`? Ошибка Needs you его уже имеет, ошибка итогов дня — нет.

   Ответ: сделано в этом PR, тест `HomeView › announces the today error as an alert`.

3. Нужна story длинного имени агента (`agentLinks.longText`) в `HomePage.stories.tsx`? Строки обрезаются через `truncate`, но story этого не показывает.

   Ответ: сделано в этом PR, story `pages-home-homepage--long-text`.

4. Боковая панель макета показывает только раздел Agents. Навигации (Home, Tasks, Inbox), кнопки `New agent`, подвала и заголовка окна в макете нет: это контейнеры без view-файлов. Нужен декоратор Storybook или view для каркаса окна, чтобы макеты страниц показывали полную оболочку?

   Ответ: нужен view каркаса, отдельной задачей. Эти части сейчас контейнеры без view-файлов. Декоратор без view повторил бы разметку и разошёлся бы с продуктом.

5. На 360 px строка задачи в Needs you не показывает заголовок: колонка ключа `w-24` и метаданные `shrink-0` занимают всю ширину (`mockups-home--width-360`). Это текущее поведение продукта и нарушение правила Floor «рабочая ширина телефона». Предложение: ниже `sm` перенести метаданные на вторую строку под заголовок. Решение о правке принимает заказчик отдельной задачей: эта задача без редизайна.

   Ответ: вне пилота, решение о правке у заказчика (TARI-v96u).
