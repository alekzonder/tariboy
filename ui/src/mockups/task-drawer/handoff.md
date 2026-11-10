# Handoff: TaskDrawer

Реализовано в этой же ветке (`designer/task-drawer-ux`), pull request ещё не открыт. Отличия реализации от макета — в разделе «Implementation notes» в конце.

2026-11-10 · без задачи TARIDESIGN (запрос в чате) · `mockups-taskdrawer--agent-works`, `mockups-taskdrawer--blocks-closed`, `mockups-taskdrawer--blocks-open`, `mockups-taskdrawer--needs-you`, `mockups-taskdrawer--paused`, `mockups-taskdrawer--check-rejected`, `mockups-taskdrawer--step-open`, `mockups-taskdrawer--editing-description`, `mockups-taskdrawer--plain-task`, `mockups-taskdrawer--width-360`, `mockups-taskdrawer--width-390`, `mockups-taskdrawer--width-820`, `mockups-taskdrawer--width-1024`, `mockups-taskdrawer--width-1180`

## What changed

Сейчас параметры задачи лежат в свёрнутой секции Details, а workflow — в свёрнутой секции Workflow, обе под лентой комментариев. Макет раскладывает drawer по частоте использования: состояние и управление в закреплённой шапке, содержание в середине, журнал внизу.

- Блок **Details** переехал в шапку и сворачивается. Свёрнутый — одна строка чипов: статус, приоритет, `agent`, `budget`, `PR`; чип меняет значение на месте. Развёрнутый добавляет под чипами Author, Customer, Queue, Group, Parent, Block reason и Relations (`blocks-closed`, `blocks-open`).
- Workflow стал одной строкой шапки: шаги, чей сейчас ход, **Return to agent…** и меню **Workflow actions**. Строка есть в любом состоянии, а не только когда ход за customer (`agent-works`, `needs-you`).
- Блок решения остался под строкой workflow и появляется только когда задача ждёт customer. Текстовая строка `Status: … · Owner: … · Holder: …` убрана: её заменяют шаги (`needs-you`, `paused`).
- Первая кнопка исхода — основная, остальные `outline` (`needs-you`).
- Отказ или сбой проверки показан целиком первым блоком тела, без кнопки **View failure details** и прыжка вниз (`check-rejected`).
- Артефакты стали секцией тела сразу под шапкой. Артефакт текущего решения помечен `for this decision` и открыт (`needs-you`).
- Description показывается как текст. Редактор открывается по **Edit** (`agent-works`, `editing-description`).
- Заголовок задачи занимает до двух строк вместо одной обрезанной.
- Script runs, Status visits, History и Metrics собраны внизу в секцию **More**, каждая строка открывается отдельно.
- Блок **Workflow** в шапке тоже сворачивается. Свёрнутый — текущий шаг со счётчиком (`implement 3/5`), чей ход и действия. Развёрнутый показывает все шаги (`blocks-closed`, `blocks-open`).
- Открыт блок или закрыт, запоминается для всех задач сразу, а не для одной: закрыл Details в одной задаче — он закрыт в любой следующей.
- У задачи без workflow статус — чип с меню, строки workflow нет (`plain-task`).

## New props

Все компоненты ниже — локальные view-части макета. Реальных view-файлов у drawer пока нет (см. Open questions).

| Component          | Prop                                                                | Type                                                                           | Meaning                                            | Source of data                                                                                      |
| ------------------ | ------------------------------------------------------------------- | ------------------------------------------------------------------------------ | -------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| `PropertyChips`    | `task`                                                              | `Task`                                                                         | Задача, чьи параметры показаны                     | `taskQueries.detail`                                                                                |
| `PropertyChips`    | `workflow`                                                          | `boolean`                                                                      | Статус ставит workflow, чип статуса не открывается | `isWorkflowTask(task)`                                                                              |
| `PropertyChips`    | `agents`                                                            | `readonly string[]`                                                            | Агенты для назначения                              | `taskQueries.principals`, `assigneeOptions`                                                         |
| `PropertyChips`    | `budget`                                                            | `Pick<TaskBudget, "spent_usd" \| "budget_usd" \| "exhausted">`                 | Потрачено и лимит                                  | `useTaskBudget(task)`                                                                               |
| `PropertyChips`    | `editing`, `onEdit`                                                 | `ChipField \| null`, `(field) => void`                                         | Чип, который сейчас поле ввода                     | состояние контейнера                                                                                |
| `PropertyChips`    | `onStatus`, `onPriority`, `onAssignee`, `onBudget`, `onPullRequest` | `(value: string) => void`                                                      | Запись в черновик задачи                           | `useTaskDetailDraft`                                                                                |
| `WorkflowStrip`    | `view`                                                              | `WorkflowView`                                                                 | Workflow задачи                                    | `taskQueries.workflow`                                                                              |
| `WorkflowStrip`    | `selected`, `onSelect`                                              | `string \| null`, `(id) => void`                                               | Шаг, чьё описание открыто                          | состояние контейнера (сейчас `chosen` в `WorkflowPanel`)                                            |
| `WorkflowStrip`    | `open`, `onOpenChange`                                              | `boolean`, `(open) => void`                                                    | Блок Workflow развёрнут: видны все шаги            | новый ключ `storageKeys.taskWorkflowOpen` в `localStorage`, один на все задачи; по умолчанию открыт |
| `TaskDrawerHeader` | `detailsOpen`, `onDetailsOpenChange`                                | `boolean`, `(open) => void`                                                    | Блок Details развёрнут                             | новый ключ `storageKeys.taskDetailsOpen` в `localStorage`, один на все задачи; по умолчанию закрыт  |
| `TaskDrawerHeader` | `details`                                                           | `{ relations: readonly Relation[]; onOpenTask: (key: string) => void }`        | Связи задачи в развёрнутом Details                 | `taskQueries.detail`                                                                                |
| `WorkflowStrip`    | `onAction`                                                          | `(action: "run_watch" \| "return" \| "upgrade" \| "move" \| "cancel") => void` | Действие workflow                                  | обработчики `WorkflowPanel`                                                                         |
| `TaskDrawerHeader` | `decision`                                                          | `DecisionProps`                                                                | Сообщение, исходы, пауза, переход к артефакту      | `useWorkflowAdvance`, `resumeTaskWorkflow`                                                          |
| `TaskDrawerBody`   | `openArtifacts`, `onToggleArtifact`                                 | `ReadonlySet<string>`, `(name) => void`                                        | Открытые артефакты                                 | состояние контейнера                                                                                |
| `TaskDrawerBody`   | `description`                                                       | `DescriptionProps`                                                             | Текст, режим чтения или редактор                   | `useTaskDetailDraft` и состояние контейнера                                                         |
| `TaskDrawerBody`   | `onProblemAction`                                                   | `(action: "retry" \| "return" \| "log") => void`                               | Действие блока проблемы                            | обработчики `WorkflowProblem`                                                                       |
| `TaskDrawerMore`   | `metrics`                                                           | `string`                                                                       | Сводка `$4.20 · 6 runs · 1 rework`                 | `useTaskMetrics`                                                                                    |

## Changed and removed props

| Component        | Prop                                    | Type                      | Meaning                                                   | Source of data       |
| ---------------- | --------------------------------------- | ------------------------- | --------------------------------------------------------- | -------------------- |
| `WorkflowPanel`  | `decisionsElsewhere`                    | удалён                    | Панель больше не рисует решения и не живёт в теле целиком | —                    |
| `TaskDetailBody` | `historyOpen`, `setHistoryOpen`         | удалены                   | History открывается как строка секции More                | —                    |
| `TaskDetailBody` | `descriptionMode`, `setDescriptionMode` | переходят в `description` | Нужны только пока открыт редактор                         | состояние контейнера |

## States

| State                                       | Story                                     |
| ------------------------------------------- | ----------------------------------------- |
| Агент работает, решения нет                 | `mockups-taskdrawer--agent-works`         |
| Оба блока свёрнуты                          | `mockups-taskdrawer--blocks-closed`       |
| Оба блока развёрнуты                        | `mockups-taskdrawer--blocks-open`         |
| Ход за customer                             | `mockups-taskdrawer--needs-you`           |
| Workflow на паузе                           | `mockups-taskdrawer--paused`              |
| Проверка отклонила запрос                   | `mockups-taskdrawer--check-rejected`      |
| Открыт шаг workflow                         | `mockups-taskdrawer--step-open`           |
| Description в редакторе, есть несохранённое | `mockups-taskdrawer--editing-description` |
| Задача без workflow                         | `mockups-taskdrawer--plain-task`          |

Не показаны и остаются как сейчас: загрузка workflow, ошибка загрузки с **Retry**, закрытая задача, доступ `respond` и `context`, открытый вопрос к customer, формы **Move to status** и **Return to agent**, длинный заголовок и workflow из восьми и более шагов.

## Behavior

- Изменение: шеврон слева от строки чипов открывает и закрывает Details, шеврон слева от строки workflow — Workflow. Клик по текущему шагу в свёрнутом Workflow тоже открывает блок. Выбор пишется в хранилище сразу и действует на все задачи. В макете он живёт только до перезагрузки Storybook.
- Изменение: чипы приоритета и `agent` открывают меню с отмеченным текущим значением. Выбор пишет в черновик, в шапке появляется **Save task**, как сейчас после правки поля.
- Изменение: чип `budget` по клику становится полем ввода на своём месте, фокус в поле, потеря фокуса возвращает чип. Чип `PR` открывает меню **Open pull request** / **Change link…**; без ссылки показан чип **Pull request** с плюсом, он сразу открывает поле.
- Изменение: в развёрнутом Workflow клик по шагу открывает его описание под строкой workflow внутри шапки, повторный клик закрывает. Раньше это же делал степпер в секции Workflow.
- Изменение: **Run watch now**, **Move to status…**, **Upgrade to …** и **Cancel task** ушли в меню **Workflow actions**. **Return to agent…** остался кнопкой с текстом. Формы Move, Upgrade и Return открываются под строкой workflow.
- Изменение: кнопка артефакта в блоке решения открывает этот артефакт в теле и прокручивает к нему. Раньше были кнопки **View plan** и свёрнутые блоки под шапкой.
- Изменение: **Edit** у Description открывает редактор, **Cancel** возвращает сохранённый текст. Сохранение — общая **Save task**.
- Статус workflow-задачи по-прежнему не редактируется. Подтверждения Continue, Release holder, Cancel task, Move и Upgrade не меняются.

## Texts

- `aria-label`: `Task parameters`, `Change status`, `Change priority`, `Change agent`, `Change budget`, `Change PR`, `Budget, USD`, `Pull request`, `Workflow`, `Workflow steps`, `Show all workflow steps`, `Expand details` / `Collapse details`, `Expand workflow` / `Collapse workflow`, `Details`, `Workflow actions`, `Workflow decision`, `Outcome message`, `Comment`.
- Заметка рядом с шагами: `<agent> works`, `waits for an agent`, `a script works`, `your decision`, `paused`, `check rejected · 2 of 5 rejections`, `check failed · …`, `closed`.
- Плейсхолдеры: `No limit`, `Pull request link`, `Message for this decision (optional)`, `Write a comment`.
- Пустые состояния: `No description.`, `No relations.`, `No script ran yet.`, `no value yet`.
- Тексты ошибок API, тостов и диалогов подтверждения не меняются.

## Accessibility

- Порядок фокуса: родитель, заголовок, **Save task**, **Task actions**, **Close**, чипы, шаги, **Return to agent…**, **Workflow actions**, блок решения, тело.
- Чипы с меню — кнопки с `aria-haspopup` от `DropdownMenu`; после закрытия меню фокус возвращается на чип.
- Шаг — кнопка с `aria-expanded` и `aria-current="step"` у текущего. Шевроны блоков Details и Workflow — кнопки с `aria-expanded` и подписью действия.
- Блок проблемы и текст паузы имеют `role="alert"`.
- Чей ход, передан не только цветом точки: рядом стоит текст заметки.
- Новых пар токенов нет. `bg-primary/8` под блоком решения несёт текст `text-primary` и `text-foreground`; контраст нужно проверить в обеих темах при переносе.

## Layout at widths

| Width      | Layout                                                                                                                                                         | Story                                                              |
| ---------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| 360, 390   | Панель на всё окно. Чипы в две строки. Развёрнутые шаги переносятся на вторую строку, чей ход и действия — на третью. Поля развёрнутого Details в один столбец | `mockups-taskdrawer--width-360`, `mockups-taskdrawer--width-390`   |
| 820        | Панель на всё окно. Чипы, шаги и действия в одну строку каждые                                                                                                 | `mockups-taskdrawer--width-820`                                    |
| 1024, 1180 | Панель в половину окна. Чипы в одну строку. Развёрнутые шаги в одну строку; чей ход и действия вместе переносятся на вторую, когда не помещаются               | `mockups-taskdrawer--width-1024`, `mockups-taskdrawer--width-1180` |

Сворачивание блоков не зависит от ширины: его выбирает пользователь. Поля развёрнутого Details переходят в два столбца по ширине самой шапки (контейнерный запрос `@md`, 448 px), потому что ширину панели меняет пользователь.

## Screenshots

| Story id                                  | Theme       | Width |
| ----------------------------------------- | ----------- | ----- |
| `mockups-taskdrawer--agent-works`         | light       | 1180  |
| `mockups-taskdrawer--blocks-closed`       | light       | 1180  |
| `mockups-taskdrawer--blocks-open`         | light       | 1180  |
| `mockups-taskdrawer--needs-you`           | light, dark | 1180  |
| `mockups-taskdrawer--paused`              | light       | 1180  |
| `mockups-taskdrawer--check-rejected`      | light       | 1180  |
| `mockups-taskdrawer--step-open`           | light       | 1180  |
| `mockups-taskdrawer--editing-description` | light       | 1180  |
| `mockups-taskdrawer--plain-task`          | dark        | 1180  |
| `mockups-taskdrawer--width-360`           | light       | 360   |
| `mockups-taskdrawer--width-820`           | light       | 820   |

## Tokens and styles

Токены в `ui/src/index.css` и варианты `cva()` не менялись.

## Acceptance criteria

1. У workflow-задачи шаги, чей ход и действия workflow видны без прокрутки и без раскрытия секций в каждом состоянии из раздела States.
2. Priority, Assignee, Budget и Pull request меняются из шапки при свёрнутом и при развёрнутом Details; секции Details под комментариями больше нет.
3. Когда ход за customer, под строкой workflow есть сообщение, кнопки исходов и ссылки на артефакты; в остальных состояниях этого блока нет.
4. На паузе на месте блока решения показаны причина и Continue, Release holder (только для пула), Cancel task.
5. Отказ или сбой проверки показан первым блоком тела с полным текстом, советом и действиями.
6. Description показывается отрендеренным Markdown; редактор открывается только по Edit.
7. Шапка остаётся закреплённой при прокрутке и не выше 70% высоты окна; при большей высоте прокручивается сама.
8. Details и Workflow сворачиваются шевроном в одну строку каждый. Состояние каждого хранится одно на все задачи и переживает закрытие drawer и перезапуск приложения. Блок решения, пауза и блок проблемы видны при любом состоянии блоков.
9. У задачи без workflow нет строки workflow, а статус меняется из чипа.
10. Все действия, которые были в секции Workflow, доступны: Run watch now, Return to agent, Upgrade, Move to status, Cancel task.

## Open questions

1. `TaskDetailHeader`, `TaskDetailBody`, `TaskDecisionPanel` и `WorkflowPanel` — контейнеры без view-файлов. Прошу разделить их на данные и разметку и перенести туда части макета.
2. Чипы пишут в общий черновик с **Save task**, а назначение агента у workflow-задачи (`WorkflowAssignee`) сохраняется сразу. Оставить два поведения или сохранять каждый чип сразу?
3. Ключи хранилища для двух блоков добавляет разработчик в `ui/src/lib/storage`, по образцу `taskMetricsOpen`. Подходят ли значения по умолчанию: Details закрыт, Workflow открыт?
4. В `components/ui` нет Popover. Описание шага и поле бюджета сейчас раскрываются на месте. Нужен ли примитив Popover, чтобы шаг не менял высоту шапки?
5. Первая кнопка исхода выделена как основная. Порядок `outcomes` в манифесте всегда начинается с исхода «вперёд»?
6. Где показывать открытый вопрос к customer (`QuestionBanner`): в месте блока решения в шапке или первым блоком тела? В макете его нет.
7. Блок отказа проверки для статуса пула предлагает **Return to agent…**. Нужна ли там эта кнопка, если агент и так видит отказ в своей цели?

## Implementation notes

Реализация в `ui/src/pages/tasks/` сохраняет существующие контейнеры и их логику, поэтому отличается от макета в четырёх местах.

1. Чипы статуса, приоритета и исполнителя — нативные `select`, размер которых равен значению (`ChipSelect`), а не выпадающие меню. Подписи для скринридера остались `Status`, `Priority`, `Assignee`.
2. Бюджет — всегда поле ввода внутри чипа, без перехода «чип → поле». Ссылка pull request в чипе только открывается; правится она в развёрнутом Details.
3. В развёрнутом Workflow действия остались подписанными кнопками (Run watch now, Return to agent…, Upgrade, Move to status…, Cancel task), меню `⋯` нет. В свёрнутом Workflow действий нет: показаны текущий шаг, счётчик и чей ход, клик по шагу разворачивает блок.
4. Секция More не появилась. Внизу остались Metrics, свёрнутый Workflow log (запуски скриптов и визиты) и History, каждый со своим раскрытием.

Ответы на открытые вопросы:

1. Разделение на view-файлы сделано частично: появились `SectionToggle.view.tsx` и `WorkflowSummary.view.tsx`. Шапка и тело остались контейнерами.
2. Оставлены оба поведения: чипы пишут в черновик с **Save task**, исполнитель workflow-задачи сохраняется сразу.
3. Ключи `tasks:details-open:v1` (по умолчанию `closed`) и `tasks:workflow-open:v1` (по умолчанию `open`) добавлены в `ui/src/lib/storage.ts`.
4. Popover не добавлялся: описание шага раскрывается на месте.
5. Первая кнопка исхода — основная. Предположение о порядке `outcomes` в манифесте не проверено.
6. Открытый вопрос к customer остался первым блоком тела (`QuestionBanner`).
7. Блок проблемы — существующий `WorkflowProblem` без изменений.
