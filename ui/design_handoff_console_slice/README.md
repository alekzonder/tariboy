# Handoff: Tariboy Console — вертикальный срез (shell + tasks)

## Overview
Первый заход нового стиля Tariboy («style layer») в продукт: одна страница целиком, сверху донизу — топбар, сайдбар агентов, плавающий остров контента, шапка агента, табы, фильтр-бар и таблица задач (два режима: задачи агента и All tasks). Цель среза — зафиксировать в коде тему, плотность и правила статусов, чтобы остальные страницы (Task detail, Tasks workspace, dark mode) шли дальше почти без дизайн-решений.

## About the Design Files
Файлы в `reference/` — **дизайн-референсы на HTML** (Design Components из дизайн-системы Tariboy UI). Это прототипы, показывающие вид и поведение, а не production-код для копирования. Задача — **воспроизвести эти макеты в существующем окружении приложения** (React + Tailwind + shadcn-компоненты Tariboy UI), используя уже принятые в репозитории паттерны. Логика в `<script type="text/x-dc">` — демо-данные и состояние прототипа, в продукте её заменяют реальные данные.

Референсы рассчитаны на превью внутри дизайн-системы (они грузят `_ds_bundle.js` через `ds-base.js` по пути `../..`). Из распакованного zip они откроются без стилей — смотрите их в дизайн-проекте, а не из архива.

## Fidelity
**High-fidelity.** Цвета, типографика, отступы, размеры и состояния финальные. Воспроизводить попиксельно, но **через существующие компоненты** (`Button`, `Badge`, `Tabs`, `Input`, `ScrollArea`, `Card` и т.д.), а не новыми одноразовыми стилями. Переструктуризация компонентов разрешена — там, где старая разметка мешает новой плотности, её можно переписать.

## Порядок работы (важно)
1. **Тема.** Перенести `reference/tariboy-theme.css` в theme-блок скомпилированного стиля приложения (`styles.css` / `globals.css`), заменив текущие shadcn-neutral значения. Это меняет вид всех ~70 компонентов без правки разметки. Отдельным коммитом — легко откатить.
2. **Оболочка.** Топбар + сайдбар + «остров» контента (`AgentWorkspace`, `AgentLayout`).
3. **Строки и статусы.** `AgentRow`, `TaskRow`, статус-пилюли — новые примитивы, описаны ниже.
4. **Таблица задач.** Два режима (agent / all) и фильтр-бар.
5. Только потом — Task detail, Tasks workspace целиком, dark mode.

## Screens / Views

### 1. Console shell (весь экран)
**Purpose:** оператор видит все агенты, выбирает один, работает с его задачами.

**Layout**
- Корень: `height:100vh; display:flex; flex-direction:column; background:var(--background); overflow:hidden`. Базовый шрифт `Geist Variable`, `13px`.
- **Топбар**: высота `42px`, `padding:0 12px`, `gap:10px`, **без нижнего бордера** (разделение делает фон, не линия).
- **Тело**: `flex:1; display:flex; padding:0 8px 8px; gap:8px`.
- **Сайдбар**: `width:268px`, фон = `--background` (без бордера, без своего фона), `padding:0 2px`.
- **Остров контента**: `flex:1; background:var(--card); border-radius:var(--panel-radius) (14px); box-shadow:var(--lift); overflow:hidden`.

**Компоненты топбара**
- Селектор пространства: кнопка `height:26px; padding:0 8px; border-radius:7px; background:transparent`, hover `background:var(--accent)`; текст `font-weight:500`; шеврон 10×10, `opacity:.45`.
- Сегмент-контрол `Agents | All tasks`: контейнер `padding:2px; background:var(--muted); border-radius:9px; gap:2px`; кнопка `height:24px; padding:0 10px; border-radius:7px; font-size:12px`. Активная: `background:var(--card); box-shadow:0 1px 2px oklch(24% .03 75/.10); font-weight:500; color:var(--foreground)`. Неактивная: `transparent; color:var(--muted-foreground)`.
- Справа иконка настроек: `26×26; border-radius:7px`, hover `background:var(--accent)`.

**Компоненты сайдбара**
- Поиск (визуальная заглушка, открывает Command palette): `height:30px; padding:0 9px; border-radius:8px; background:var(--muted); color:var(--muted-foreground)`, слева лупа 13px, справа `⌘K` (`11px; opacity:.6`).
- Пилюли-табы `Agents | Groups | Servers`: `height:24px; padding:0 9px; border-radius:7px; font-size:12px`; активная `background:var(--accent); font-weight:500; color:var(--foreground)`; неактивная `transparent; color:var(--muted-foreground)`. Справа `+` (24×24, hover `--accent`).
- Заголовки групп `Pinned` / `All agents`: `font-size:11px; font-weight:500; letter-spacing:.02em; color:var(--muted-foreground); padding:6px 8px 4px`.
- Подвал сайдбара: высота `44px`, 22px круглая иконка (`background:var(--accent)`), строка «All servers» `12px/500` + `N connected · M agents` (`11px; --muted-foreground`), справа точка 6px `--status-running`.

### 2. AgentRow (строка агента в сайдбаре) — `reference/AgentRow.dc.html`
`height:30px; padding:0 10px; margin:1px 0; border-radius:8px; gap:9px; cursor:pointer`.
- Невыбранная: фон прозрачный, hover `background:var(--sidebar-accent)`.
- Выбранная: `background:var(--card); box-shadow:0 1px 2px oklch(24% .03 75/.10)` — тот же «остров», что и панель контента.
- Точка статуса 7px: `running` → `--status-running` + halo `0 0 0 3px color-mix(in oklab,var(--status-running) 18%,transparent)`; `failed` → `--status-failed`; остальное → `--status-stopped` (без halo).
- Имя: `font-weight:500`, обрезка ellipsis.
- Индикатор обновления образа: стрелка 11px, `--muted-foreground`, `opacity:.8`, `title="<image> → new digest available, applied on next start"`.
- Непрочитанное: точка 5px `--primary`.
- Пилюля статуса: `height:17px; padding:0 6px; border-radius:5px; font-size:10.5px; letter-spacing:.01em`.
  - `no budget` (бюджет исчерпан) — `color-mix(in oklab,var(--status-failed) 12%,transparent)` / `--status-failed`, `font-weight:500`; приоритетнее статуса.
  - `running` — `color-mix(in oklab,var(--status-running) 13%,transparent)` / `--status-running`, `font-weight:500`.
  - `failed` — как выше, но failed-цвета.
  - Прочие статусы — `background:var(--muted); color:var(--muted-foreground)`, `font-weight:400`.

### 3. Шапка агента (в острове)
`padding:14px 16px 11px; gap:10px; align-items:flex-start`.
- Первая строка (`gap:9px`): точка статуса 8px (правила как в AgentRow) → имя `15px/600, letter-spacing:-.01em` → статус (`running` пилюлей, `failed` текстом `--status-failed` 500, остальные текстом `--muted-foreground`) → сервер `12.5px --muted-foreground` → образ: пилюля `height:20px; padding:0 7px; border-radius:6px; background:var(--muted); font-family:var(--font-mono); font-size:11.5px` с иконкой-контейнером → `Goal:` + ключ задачи моно-шрифтом с `border-bottom:1px dotted var(--border)` (hover — бордер `--foreground`) и «?»-иконка с подсказкой «The agent works toward this goal between tasks».
- Вторая строка `12px`: `cwd:` + путь моно `11.5px` с ellipsis; ссылка «Open in VS Code» (`font-weight:500`, hover underline).
- Справа: `Start` (primary), `Exec` (secondary), `Kill` (secondary, текст `--destructive`) — `size="sm"`, высота 28px; далее «⋯» 28×28, `border-radius:8px`, открывает меню.
- Меню: `top:32px; right:0; min-width:164px; padding:5px; border-radius:12px; background:var(--popover); box-shadow:var(--lift)`; пункты `height:28px; padding:0 9px; border-radius:7px; font-size:13px`, hover `--accent`; «Delete agent» — `--destructive`, hover `color-mix(in oklab,var(--destructive) 10%,transparent)`. Разделитель `height:1px; margin:4px 3px; background:var(--border)`.

### 4. Табы контента
`height:34px; padding:0 10px; gap:2px`, контейнер `padding:0 12px; border-bottom:1px solid var(--border)`.
- Активный: `border-bottom:2px solid var(--primary); color:var(--foreground); font-weight:500`.
- Неактивный: `border-bottom:2px solid transparent; color:var(--muted-foreground)`.
- Порядок: `Tasks`, `Chats`, `Session`, `Prompt`, затем один «контекстный» таб (`Scripts | Iterations | Audit | Secrets | Files`) с шевроном-кнопкой, которая открывает выпадающий список остальных. Подчёркивание распространяется и на кнопку-шеврон.
- Точка-индикатор в табе: 5px `--primary`, `margin-left:6px` — `Tasks` при открытом вопросе от агента, `Chats` при непрочитанном.
- Дропдаун контекстных табов: `top:38px; left:236px; min-width:168px`, оформление как меню выше; активный пункт `background:var(--accent)`.

### 5. TaskFilterBar — `reference/TaskFilterBar.dc.html`
`padding:11px 16px 9px; gap:6px; flex-wrap:wrap`.
- Поиск: `width:220px; height:28px; padding:0 9px; border-radius:8px; background:var(--muted)`; input без бордера, `font-size:12px`. Placeholder: `Search tasks` / `Search all tasks` (режим all).
- Сегмент `Active | Closed | All` — как топбарный сегмент.
- Только в режиме all: кнопка `Server: <name>` (`height:28px; border-radius:8px; background:var(--muted)`, hover `--accent`) и чип активного агента `color-mix(in oklab,var(--primary) 12%,transparent)` / `--primary`, `font-weight:500`, с кнопкой `×` 18×18.
- Справа: при выделении — `N selected` + `Retry`/`Pause` (secondary) + `Kill` (destructive), `size="xs"` (24px), затем вертикальный разделитель `1px×18px --border`; всегда — `+ New task` (`size="xs"`).

### 6. TaskRow и таблица задач — `reference/TaskRow.dc.html`
Строка: `min-height:30px; padding:0 16px; gap:10px`, hover `background:var(--muted)`. Заголовок таблицы — те же колонки, `height:30px; font-size:11.5px; color:var(--muted-foreground)`; в режиме all — `position:sticky; top:0; background:var(--card); z-index:2`.

Колонки, режим **agent**: `Key 62px` (моно 11.5px, tabular-nums, `--muted-foreground`) · `Task` (flex, ellipsis) · `Pri 30px` · `Status 110px` · `Duration 74px` (справа, моно, tabular) · `Updated 76px` (справа, tabular, 12px).
Колонки, режим **all**: чекбокс `20px` · `Key 62px` · `Task` · `Agent 104px` (пилюля `height:19px; padding:0 7px; border-radius:6px; background:var(--muted); font-size:11.5px`) · `Server 96px` (моно 11.5px) · `Status 110px` · `Updated 76px`.

- Вложенность: отступ `14px` на уровень (спейсеры перед шевроном), максимум 6 уровней.
- Шеврон: кнопка 16×16, `color:var(--muted-foreground); opacity:.65`; развёрнуто → `transform:rotate(90deg)`, `transition:transform .12s`; у листьев кнопка пустая (место сохраняется).
- Точка-вопрос после названия: 5px `--primary`, `title="The agent is waiting on an answer from the customer"`.
- Чекбокс: 13×13, `border-radius:4px`; выбран — `background:var(--primary)`, иначе `border:1px solid var(--input)`.
- Приоритет: `min-width:22px; height:17px; border-radius:5px; font-family:var(--font-mono); font-size:10.5px; font-weight:500`. `P1` — `color-mix(in oklab,var(--status-failed) 12%,transparent)` / `--status-failed`; `P2`/`P3` — `--muted`/`--muted-foreground`.
- Статус (`reference/StatusPill.dc.html`), `height:20px; font-size:11.5px`:
  - `in_progress` → пилюля `padding:0 8px; border-radius:6px; color-mix(in oklab,var(--status-running) 13%,transparent)` / `--status-running`, 500.
  - `wait_customer` → пилюля с `--primary` 12% / `--primary`, 500.
  - `cancelled` → просто текст `--muted-foreground; opacity:.7`.
  - `open` / `done` → просто текст `--muted-foreground`.
  - **Правило системы:** заливку получает только «живое» и «требует внимания». Остальное — тихий текст.

## Interactions & Behavior
- Топбар-сегмент переключает режимы `agents` / `all` — меняется вся начинка острова.
- Клик по AgentRow выбирает агента и включает фильтр «только этот агент» в режиме all (чип агента в фильтр-баре, `×` снимает фильтр).
- Табы контента: клик выбирает; шеврон открывает список контекстных табов, выбор пункта делает его видимым табом.
- Фильтр `Active | Closed | All`: closed = `done` + `cancelled`. Поиск фильтрует по названию и ключу; **родитель остаётся видимым, если под фильтр подходит любой потомок**.
- `+ New task` добавляет задачу в начало списка, сбрасывает поиск и, если стоял фильтр `Closed`, переключает на `Active`.
- Раскрытие/сворачивание поддерева — по ключу задачи, состояние переживает фильтрацию.
- Одно открытое всплывающее меню за раз: открытие меню агента закрывает дропдаун табов и наоборот.
- Переходы: только `transform .12s` у шеврона. Никаких анимаций появления строк.
- Фокус: `--ring` (стандартный shadcn focus-visible), ничего кастомного.

## State Management
`mode: 'agents'|'all'`, `leftTab`, `tab`, `ctx` (видимый контекстный таб), `ctxOpen`, `menuOpen`, `sel` (id агента), `agentFilter: boolean`, `q`, `taskFilter: 'active'|'closed'|'all'`, `open: Record<taskKey, boolean>`. В продукте `sel`/`tab` живут в URL (`/agent/:name/:tab`), `open`/`taskFilter`/`q` — локальные, список агентов и задач приходит из существующих хуков данных.

## Design Tokens
Полный источник — `reference/tariboy-theme.css` (light + `.dark`). Ключевое:
- Нейтрали тёплые, сдвиг оттенка ~hue 85, chroma 0.006–0.016. `--background: oklch(96.3% .016 88)`, `--card: oklch(100% .002 85)`, `--foreground: oklch(24% .012 75)`, `--muted: oklch(95.2% .014 88)`, `--muted-foreground: oklch(55% .014 80)`, `--accent: oklch(93% .020 88)`, `--border: oklch(90.5% .010 85)`, `--input: oklch(92% .010 85)`.
- Один акцент: `--primary: oklch(52% .085 248)` (steel), `--ring: oklch(62% .090 248)`.
- Статусы — одна светлота/хрома, работает только тон: `--status-running: oklch(56% .115 152)`, `--status-queued: oklch(58% .014 82)`, `--status-done: oklch(52% .014 82)`, `--status-failed: oklch(55% .155 27)`, `--status-stopped: oklch(64% .012 82)`.
- Новые токены поверх shadcn-набора: `--panel-radius: 14px`, `--lift: 0 1px 2px oklch(24% .03 75/.05), 0 10px 28px -14px oklch(24% .03 75/.22)`, `--font-mono`.
- Радиусы в интерфейсе: 5px (микро-пилюли) · 6px (статусы, чипы) · 7px (кнопки-сегменты, пункты меню) · 8px (строки, поля) · 9px (контейнер сегмента) · 12px (меню) · 14px (остров).
- Типошкала: 10.5 · 11 · 11.5 · 12 · 12.5 · 13 (база) · 15 (имя агента). Вес — 400/500, 600 только у имени агента.
- Плотность: строки 30px, кнопки 24/26/28px, пилюли 17/19/20px. Все идентификаторы, длительности и время — моно + `tabular-nums`.
- Приподнятость: два уровня. `0 1px 2px oklch(24% .03 75/.10)` (выбранная строка, активный сегмент) и `--lift` (остров, меню). Больше теней не вводить.

## Assets
Только инлайновые иконки (lucide-совместимые контуры, `stroke-width` 1.2–1.9) и шрифт `Geist Variable` — оба уже есть в приложении. Новых ассетов не требуется.

## Files
- `reference/TariboyConsole.dc.html` — эталон страницы целиком (топбар, сайдбар, остров, табы, оба режима таблицы).
- `reference/AgentRow.dc.html`, `reference/TaskRow.dc.html`, `reference/StatusPill.dc.html`, `reference/TaskFilterBar.dc.html` — примитивы среза с полным набором состояний.
- `reference/tariboy-theme.css` — слой темы, который уезжает в репозиторий первым.
- `PROMPT.md` — что вставить Claude Code, чтобы он начал работу.

## Куда это ложится в репозитории
По карте синка дизайн-системы:
- тема → компилируемый глобальный стиль (тот, из которого собран `styles.css` дизайн-системы);
- оболочка → `components/agents/AgentWorkspace/`, `components/general/AgentLayout/` (второй помечен deprecated — новую оболочку делать в `AgentWorkspace`);
- сайдбар агентов → `components/general/HostSwitcher/`, `HostStatus/`, `AgentColorSwatch/`;
- шапка агента → `components/general/AgentControls/`, `GoalHelp/`;
- таблица и фильтры → `components/tasks/TasksWorkspace/`;
- примитивы → `components/general/Button/`, `Badge/`, `Tabs/`, `Input/`, `ScrollArea/`.
### Заодно починить в стилях (валидатор дизайн-системы жалуется)
1. **215 CSS-переменных объявлены внутри селекторов компонентов** (`.flexlayout__layout`, `.terminal-workspace .flexlayout__layout`, `:where(.space-y-0>:not(:last-child))` и т.п.), поэтому не считаются токенами. Те из них, что по смыслу тематические (цвета, радиусы, отступы, тени), поднять в `:root` / `.dark` (или в `[data-*]`-скоуп темы) и оставить в компонентных селекторах только `var(--…)`. Локальные утилитарные переменные, если они не тема, оставить как есть — но тогда не давать им вид темы.
2. **Аннотации `/* @kind other */` на отдельной строке игнорируются.** Каждая должна стоять на той же строке, что и объявление токена, сразу после `;` (или прямо перед ним), с меткой `color|spacing|radius|shadow|font|other`. Пример: `--panel-radius:14px; /* @kind radius */`.

После правок в коде прогнать `/design-sync`, чтобы дизайн-система и карточки обновились и срез можно было сверить с эталоном.
