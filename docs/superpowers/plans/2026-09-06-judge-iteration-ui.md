# Iteration Judge Analysis and UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Запускать ручной Judge review из итерации и видеть её оценку, доказательства и историю без поиска по всем runs.

**Architecture:** Использовать существующие Judge service, assignments и consensus. Добавить серверную проекцию результатов на итерации и target-specific представление существующей страницы анализа; не создавать второй pipeline оценки.

**Tech Stack:** Go, SQLite, registry HTTP API, React/TypeScript, Vitest, Playwright, tauri-driver.

**Spec:** Раздел «Требования» этого документа фиксирует текущий запрос пользователя; исторический контекст — `docs/superpowers/specs/2026-09-06-judge-reliability-design.md`.

## Global Constraints

- Первый из трёх последовательных планов; рубрику и алгоритм consensus здесь не менять.
- Работать в `.worktrees/judge-reliability`, ветка `feat/judge-reliability`, существующий PR #15; версию не повышать.
- Прочитать README, contributor guide и документы architecture/index, state-model, iteration-loop, shim, web-ui согласно AGENTS.md перед реализацией.
- Тесты только с изолированными base/runtime/listener; не перезапускать live daemon, не включать циклы, не применять automation configuration.
- Не изменять другие agent images. Не добавлять зависимости и отдельный dashboard.
- По уточнению пользователя `make full-check` не запускать; использовать адресные проверки и отдельный production Desktop-сценарий.

## Требования

1. В выбранной завершённой итерации есть кнопка `Run Judge review`, вызывающая тот же API, что `judge review`, с ровно одним iteration ID.
2. Запрос, постановка в очередь и выполнение — разные состояния. Pending-запрос блокирует повторный клик; существующий активный анализ показывается ссылкой. Ошибка оставляет возможность повторить запрос.
3. Сохранить текущую семантику запуска команды: UI не включает отключённых агентов/циклы. Если workers не работают, показывать ожидание и причину, а не обещать выполняющийся анализ. Автоматический one-shot dispatch вне этого плана.
4. Score в списке и деталях — median consensus последнего полностью оценённого target, а не среднее между runs. Target не обязан ждать summary всего multi-target run. Новый незавершённый/ошибочный review не затирает предыдущий score.
5. Показывать число в шкале 0–1, verdict и дату; `0` допустим, отсутствие — `—`. Это оценка, не вероятность. `uncertain`/`disputed` не красить как уверенный pass/fail. Частичный результат показывать отдельно с количеством ответов.
6. Прямая ссылка выбирает конкретный target. Хлебные крошки дают ссылки и на исходную итерацию, и на Judge runs; переходы, API-запросы и browser back сохраняют сервер.
7. Полезные дополнения в этом срезе: история повторных reviews, ответы отдельных judges, расхождения, evidence gaps, переход к цитируемому доказательству. Без графиков, новых фильтров и изменения scoring.
8. По уточнению пользователя одного Judge достаточно: ручной API/CLI уже использует `judges_per_iteration=1` по умолчанию, UI сохраняет этот default. Второй ответ нужен только при явном запросе двух judges; количество настроенных workers само по себе не является требуемым числом ответов. Число analyses не выдавать за число оплаченных harness-запусков.

## Task 1: Серверная проекция и история итерации

**Files:** Modify `internal/judge/model.go`, `internal/judge/store.go`, `internal/judge/service.go`, `internal/registry/registry.go`, `internal/commands/iteration.go`, `internal/commands/daemon.go` (route registration); tests in `internal/judge/store_test.go`, `internal/judge/service_test.go`, `internal/commands/iteration_test.go`. Inspect `internal/commands/judge.go` for POST compatibility; no change is required there when the existing command is preserved.

**Interfaces:** Новый JSON тип `IterationJudgeReview` имеет `run_id`, `target_id`, `created_at`, `state`, `verdict` (string), `score` (number|null), `completed`, `failed`, `pending` (integer). Проекция `judge` содержит `latest_completed: IterationJudgeReview|null` и `active: IterationJudgeReview|null`. Добавить её к строкам iteration list/detail; `GET /api/agents/{name}/iterations/{id}/judges` возвращает `{reviews: IterationJudgeReview[]}` в порядке created_at DESC, run_id DESC. Existing `POST /api/judges/review` остаётся совместимым.

- [x] Добавить fixtures с одной итерацией и тремя reviews: завершённый score=0, более новый завершённый score=0.8, самый новый pending; ещё один target принадлежит другому агенту. Проверить результат буквально: `latest_completed.score == 0.8`, `active.pending == 1`, история содержит только три своих review. Отдельно проверить единственный score=0 и отсутствие review.
- [x] Запустить `go test ./internal/judge ./internal/commands -run 'IterationJudge|Iteration' -count=1`; убедиться, что новые assertions падают из-за отсутствующего контракта.
- [x] Реализовать batch-чтение по iteration IDs на сервере, без запроса на каждую строку и без загрузки всех runs в браузере. Считать target завершённым только при наличии всех требуемых валидных analyses; failed/partial не выдавать за полностью оценённый. Использовать существующие статусы assignments, не новую машину состояний.
- [x] Проверить принадлежность iteration агенту, terminal eligibility по существующим правилам selector, пустые результаты и порядок при одинаковой дате. Сохранить прежние поля API.
- [x] Запустить `go test ./internal/judge ./internal/commands ./internal/registry -count=1`, проверить diff и закоммитить серверный контракт.

Completed in `7332a03` and `81aaf43`; backend-check passed, review clean after the 40,000-ID SQLite-boundary fix.

## Task 2: Запуск и score в iterations

**Files:** Modify `ui/src/lib/judge.ts`, `ui/src/lib/types.ts`, `ui/src/pages/AuditLogPage.tsx` и существующий `ui/src/pages/AuditLogPage.test.tsx`; create `ui/src/components/IterationJudgePanel.tsx`, `ui/src/components/IterationJudgePanel.test.tsx`. Inspect `ui/src/components/IterationAuditLog.tsx`; compose the Judge panel in the selected iteration detail of AuditLogPage without requiring changes to embedded overview logs.

**Interfaces:** Панель получает `agentName: string`, `iterationId: string`, `terminal: boolean` и серверную проекцию Task 1. API helper отправляет `{iteration: [iterationId]}` через существующий explicit-host транспорт, возвращает существующие `{id, status, targets}`. После ответа перечитать историю, чтобы получить target ID, не угадывать его по ID run.

- [x] Написать failing UI tests: score=0 отображается как число; pending сохраняет старые 0.8; двойной клик даёт один POST; ошибка POST видна и позволяет retry; незавершённая итерация не запускает review.
- [x] Запустить `cd ui && npm test -- src/components/IterationJudgePanel.test.tsx src/pages/AuditLogPage.test.tsx` и зафиксировать RED.
- [x] Реализовать панель на существующих компонентах. Показывать `Queued`, пока нет подтверждения выполнения; disabled workers объяснять через доступные данные конфигурации, не менять их. Активный review предлагает перейти к нему, а не создать новый. Это UI-защита от повторного клика, не глобальная дедупликация CLI-экспериментов.
- [x] Отображать score/verdict в строках AuditLogPage. Синхронизировать выбранную итерацию с `?iteration=` при клике, внешней навигации и back/forward, сохраняя остальные параметры. Для неизвестного ID показать явное отсутствие, не чужую итерацию.
- [x] Повторить focused tests до GREEN; проверить keyboard access и доступное имя кнопки; закоммитить UI iterations.

## Task 3: Target-specific анализ и обратная навигация

**Files:** Modify `ui/src/pages/JudgeRunDetailPage.tsx`, `ui/src/pages/JudgeRunDetailPage.test.tsx`, `ui/src/lib/judge.ts`; make the run-detail links in `ui/src/pages/JudgeRunsPage.tsx` explicit-host and update `ui/src/pages/JudgeRunsPage.test.tsx`. Reuse the existing route in `ui/src/App.tsx` without requiring a route change.

**Interfaces:** Существующий URL run дополняется `?target=<target_id>`. Page фильтрует analyses по `target_id`, берёт agent/iteration из target, а не из непроверенного return URL. Использует explicit-host API и существующий host-aware построитель ссылок.

- [x] Добавить failing test с двумя targets: URL выбирает второй, его score и analyses видны, первый не представлен как анализ выбранной итерации; breadcrumbs ведут к её `?iteration=` и к Judge runs того же сервера. Неизвестный target даёт not-found, не fallback.
- [x] Запустить `cd ui && npm test -- src/pages/JudgeRunDetailPage.test.tsx` и убедиться в RED.
- [x] Добавить target mode без дублирования всей страницы run. Оставить общий run view доступным. Вывести consensus, счётчик ответов, individual verdict/score, violations с citations и evidence gaps; не смешивать доказательства разных targets.
- [x] Привязать ссылки из панели/истории Task 2 к этому target mode; проверить прямое открытие URL и browser back после обновления страницы.
- [x] Повторить focused tests до GREEN и закоммитить.

## Task 4: Production-проверки и handoff

**Files:** Extend `ui/tests/desktop/judge-runs.pw.ts`; update `docs/docs/architecture/web-ui.mdx` и `docs/docs/architecture/state-model.mdx` для нового API-представления.

**Interfaces:** Сценарий `iteration → review → target analysis → iteration / Judge runs` использует контракты Tasks 1–3 и production Desktop.

- [x] Добавить сценарий со score=0, pending поверх прошлого результата, завершением review, выбором одного из двух targets и обоими breadcrumbs. Проверить host isolation: запросы и ссылки не уходят на default server.
- [x] Запустить production Desktop проверку через Playwright и tauri-driver согласно contributor guide; mock-list alone не доказывает работоспособность POST/dispatch, поэтому проверить их отдельно с изолированным daemon и контролируемым worker.
- [x] Обновить product docs, включая честное различие queued/running и ручной режим workers. При затронутом shared Store UI пересобрать committed `internal/storeui/dist` через `make store-ui` в изоляции; desktop build outputs не stage.
- [x] На интеграционной границе выполнить `make check`, отдельно собрать production Desktop и запустить затронутые сценарии через Playwright/tauri-driver. `make full-check` не запускать по уточнению пользователя. После сборки проверить обе формы `./bin/tariboy version` и `./bin/tariboy --version`; записать точные результаты выполненных проверок, не заявлять прохождение пропущенного полного набора.
- [x] Выполнить `git diff --check`, просмотреть весь diff, устранить Critical/Important замечания, закоммитить и обновить PR #15. Затем переходить к плану `2026-09-06-judge-image-rubric.md`.

Verification checkpoint: `make check` passed (backend 112 s, frontend 142 s).
Production Desktop Playwright/tauri-driver: 2/2 passed in 24.0 s on `ce8c74a`,
including real default-one POST/claim/submit and target-specific immutable evidence.
Both CLI version forms: `0.48.0`. Final product docs doctor/build passed.
`full-check` was not run. Controlled stub verdicts establish integration behavior,
not accuracy or paid-model cost statistics.

Whole-branch review `19c57fc..fe74a47`: no Critical/Important findings; PR #15
updated as draft. Two nonblocking advisory-display issues remain recorded:
successful polling can clear an action error, and a worker waiting reason can
remain stale while the pending count is unchanged. Subsequent phases are not
included in this completion checkpoint.
