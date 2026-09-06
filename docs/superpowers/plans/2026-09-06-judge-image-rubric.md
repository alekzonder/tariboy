# Judge Image Rubric Ownership Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Оставить смысловую рубрику только в image `llm-as-judge`, сохранив воспроизводимость и историю оценок.

**Architecture:** Image prompt template является источником инструкций; daemon отвечает за protocol, evidence validation и aggregation. Run фиксирует image identity своих judges, а assignment не исполняется с несовпадающим образом. Не вводить новый формат agent image или специальный rubric loader.

**Tech Stack:** Go, SQLite, существующие image archives и prompt template SHA-256, YAML, Go tests.

**Spec:** Раздел «Требования» ниже заменяет только решение о daemon-owned rubric из `docs/superpowers/specs/2026-09-06-judge-reliability-design.md`.

## Global Constraints

- Выполнять после `2026-09-06-judge-iteration-ui.md` в той же ветке и PR #15.
- Не менять текст рубрики одновременно с переносом: этот этап проверяет ownership, не качество новой формулировки.
- Прочитать contributor guide, architecture/index, state-model, iteration-loop, shim и images-and-groups/index; отсутствующий `docs/docs/images.mdx` не заменять выдуманными правилами.
- Не менять другие images, формат моделей sol/terra и harness Codex. Не включать automation и не перезапускать live daemon.
- Существующие runs и сохранённые criteria остаются читаемыми. Не приписывать старым данным текущий image digest.
- По уточнению пользователя `make full-check` не запускать; использовать адресные проверки и отдельный production Desktop-сценарий.

## Требования

1. Единственный исходник rubric: `store/images/llm-as-judge/rubric.md`, подключённый локально в Tariboyfile.
2. У daemon нет знания имени `judge-rubric.md`/`rubric.md`, нет чтения bundled rubric и нет вставки её текста в `OriginalRequest` новых runs.
3. Manual и scheduled review используют один путь фиксации provenance. Записать judge agent, image ref, digest и prompt template SHA-256 при создании run.
4. Не ограничиваться декоративным hash: до выдачи assignment сверить текущий execution image с зафиксированным. При несовпадении не выполнять/не принимать новый результат под видом исходной версии; показать причину и предложить новый run. Не менять agent image автоматически и не блокировать уже завершённые результаты.
5. Исторические runs без provenance читаются как legacy; не backfill их текущим образом. Новый run без проверяемого image/template отклоняется до enqueue с понятной ошибкой.
6. Изменение mutable tag не меняет правила существующего run. Проверяется содержимое digest/template, а не только строка tag.

## Task 1: Фиксация и проверка image identity

**Files:** Modify `internal/judge/model.go`, `internal/judge/store.go`, `internal/judge/service.go`, `internal/judge/automation.go`; tests `internal/judge/store_test.go`, `internal/judge/service_test.go`, `internal/judge/automation_test.go`. Use existing migration registration in judge store; не угадывать номер общей migration.

**Interfaces:** Новый `JudgeImageIdentity` содержит JSON string fields `agent`, `image_ref`, `image_digest`, `prompt_template_sha256`; Run получает `judge_images: JudgeImageIdentity[]`. Сохранить одной JSON-колонкой рядом с конфигурацией run через существующий migration mechanism, без новой relational subsystem. Пустое значение означает legacy, не доказанный образ.

- [ ] Добавить тест: manual и automatic run фиксируют одинаковую identity при одинаковой конфигурации; смена tag после создания не меняет сохранённое значение. Отдельный fixture старой базы читается без identity и без data loss.
- [ ] Запустить `go test ./internal/judge -run 'ImageIdentity|Legacy' -count=1`; увидеть RED.
- [ ] Разрешить образ через существующий image store и validated `ReadTemplate(ref)`; фиксировать digest и template hash из одного согласованного snapshot. Не искать layer по имени rubric и не дублировать его текст в базе.
- [ ] В общем пути выдачи/приёма assignment сверить identity фактической worker iteration. Тест: записан digest A, worker использует B → assignment не получает валидный результат; A → нормальное выполнение. Закрыть смену образа между claim и submit повторной проверкой фактической iteration provenance.
- [ ] Запустить `go test ./internal/judge ./internal/image -count=1`; проверить concurrent claim и legacy compatibility; закоммитить provenance отдельно от переноса текста.

## Task 2: Удаление daemon-owned рубрики

**Files:** Move `store/prompts/judge-rubric.md` to `store/images/llm-as-judge/rubric.md`; modify `store/images/llm-as-judge/Tariboyfile.yaml`, `internal/judge/service.go`, `internal/judge/automation.go`, `internal/judge/image_contract_test.go`; remove `internal/judge/criteria.go` и заменить обязанности `internal/judge/criteria_test.go` контрактным тестом образа.

**Interfaces:** Tariboyfile содержит `file: ./rubric.md` в прежней позиции. `OriginalRequest` новых runs содержит только задачу/контекст пользователя, не повторяет image instructions. Protocol/schema/validators остаются Go-owned.

- [ ] Найти все literal callers `ReviewCriteria` и ссылки на `judge-rubric.md`; проверить manual и scheduled paths, Store assets и тесты. Исторические experiment logs не переписывать.
- [ ] Добавить failing contract test: built image содержит rubric layer ровно один раз; после изменения только локального rubric файла rebuild меняет image/template digest; создание run не требует bundled semantic prompt.
- [ ] Запустить `go test ./internal/judge -run 'ImageContract|Criteria|ImageIdentity' -count=1` и убедиться в RED.
- [ ] Перенести текст без смысловых правок, заменить Tariboyfile entry и удалить helper/его вызовы. Проверить `git diff --find-renames`: рубрика перемещена, не отредактирована. Не оставлять fallback на старый bundled файл.
- [ ] Запустить `go test ./internal/judge ./internal/image ./internal/commands -count=1`; проверить built image и оба способа создания run; закоммитить перенос.

## Task 3: Видимая provenance и regression gate

**Files:** Modify `ui/src/lib/judge.ts`, `ui/src/pages/JudgeRunDetailPage.tsx`, `ui/src/pages/JudgeRunDetailPage.test.tsx`; update `docs/docs/images-and-groups/index.mdx`, `docs/docs/architecture/state-model.mdx`; extend `ui/tests/desktop/judge-runs.pw.ts`.

**Interfaces:** UI использует `run.judge_images` из Task 1. Показывает image ref/digest и template hash в существующих деталях, legacy — `Image provenance unavailable`, а не текущее состояние агента.

- [ ] Добавить failing UI test с новым и legacy run: новый показывает сохранённый digest A при текущем B, legacy не показывает B как доказанную provenance.
- [ ] Запустить `cd ui && npm test -- src/pages/JudgeRunDetailPage.test.tsx`; увидеть RED, добавить поля в существующий блок деталей, повторить до GREEN.
- [ ] Выполнить изолированный regression scenario: run A → update tag → A остаётся неизменным, несовпадающий worker блокируется явно → новый run B использует B. Старый завершённый run открывается с прежними evidence/criteria.
- [ ] Документировать image-owned rubric и mismatch behavior; выполнить `make check` и отдельно production Playwright/tauri-driver сценарий для изменённого UI. `make full-check` не запускать по уточнению пользователя. Если shared UI затрагивает Store bundle, пересобрать его по AGENTS.md.
- [ ] `git diff --check`, полный review, commit/push в PR #15. Не заявлять улучшение accuracy на основании переноса; новые статистические эксперименты принадлежат следующему процессу. Далее `2026-09-06-judge-image-improvement-skill.md`.
