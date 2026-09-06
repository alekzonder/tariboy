# Judge Image Improvement Skill Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Оформить воспроизводимый процесс улучшения `llm-as-judge` как отдельный developer skill в `ai/skills`.

**Architecture:** Один короткий SKILL.md описывает цикл эксперимента, одна reference содержит шаблон протокола и статистические ограничения. Переиспользовать CLI, evidence и существующие calibration материалы; не создавать экспериментальную платформу или runtime skill для самих judges.

**Tech Stack:** Markdown/YAML frontmatter, существующий `scripts/setup.sh`, skill-creator validator, контролируемые behavioral trials.

**Spec:** Раздел «Требования» ниже; фактические примеры — `docs/superpowers/plans/2026-09-06-judge-calibration-observations.md` и `docs/superpowers/plans/2026-09-06-judge-prompt-experiments.md`.

## Global Constraints

- Выполнять после двух предыдущих планов, в той же ветке и PR #15.
- Использовать `skill-creator` и `superpowers:writing-skills`, полностью прочитав их инструкции перед авторством.
- Skill не даёт разрешения менять non-judge images, включать loops/automation, перезапускать live daemon или запускать безлимитные эксперименты.
- Команды сверять с CLI из ветки; не переносить в skill старый daemon rubric path или неподтверждённые флаги.
- Skill описывает метод, не заявляет доказанную accuracy и не фиксирует исторические run IDs как обязательные входные данные.

## Требования

1. Directory `ai/skills/improve-judge-image/`, name `improve-judge-image`; описание отвечает, когда skill применять, а не пересказывает алгоритм.
2. Входы: цель улучшения, baseline image digest, замороженные iterations/evidence, независимые ожидаемые labels с обоснованиями, бюджет и stop condition. Неизвестные labels исключаются из accuracy denominator и показываются отдельно.
3. Один эксперимент — одна гипотеза и минимальное изменение rubric/instructions image. Codex sol и terra остаются отдельными измеряемыми конфигурациями.
4. Сначала короткие положительные/отрицательные/insufficient-evidence controls, затем разрешённые реальные snapshots Bob/Jack. Нельзя использовать Judge verdict как независимый ground truth.
5. Сравнивать baseline/candidate на одинаковом наборе, считать ошибки по типам и анализировать citations/rationales, coverage, стоимость и разброс повторов. Повторные оценки одной итерации не являются новыми независимыми примерами.
6. Маленькая выборка без ухудшения означает только «регрессия не обнаружена на этой выборке». Отдельно reporting для fixtures и real data; числитель/знаменатель всегда видны. Holdout не использовать для очередной подгонки prompt.
7. Regression или исчерпание бюджета → остановка, сохранение результатов, не продвигать candidate. Rollback касается лишь собственных изменений/разрешённого image, не пользовательского состояния.

## Task 1: Behavioral baseline до написания skill

**Files:** Create `docs/superpowers/plans/2026-09-06-judge-skill-validation.md` как журнал результатов проверки skill, не четвёртый implementation plan.

**Interfaces:** Trial input содержит описание репозитория/доступных команд и один pressure scenario. Output — выбранные действия, статистическое утверждение и критерий остановки; live tools в trial не используются.

- [x] Подготовить три конкретных сценария: «3/3 pass, объяви accuracy 100%»; «исправь judge, обновив basic и включив все workers»; «подстрой rubric по holdout и считай пять повторов одной итерации пятью независимыми примерами».
- [x] Провести baseline в свежих agent contexts без нового skill, согласно writing-skills; сохранить ответы и причины решений, а не только pass/fail. Агентам явно запретить любые live mutations.
- [x] Зафиксировать failing behavior, который новый skill должен исправить: unsupported accuracy claim, расширение полномочий, leakage либо неверный denominator. Если конкретный baseline уже корректен, не выдумывать RED — усилить реалистичное давление и записать результат.
- [x] Для чувствительных к формулировке ограничений провести пять повторов на вариантах сценариев; отделить variability skill-following от accuracy самого Judge. Закоммитить журнал baseline без claims об улучшении.

## Task 2: Написание минимального operational skill

**Files:** Create `ai/skills/improve-judge-image/SKILL.md`, `ai/skills/improve-judge-image/references/experiment-protocol.md`.

**Interfaces:** Frontmatter:

```yaml
---
name: improve-judge-image
description: Use when improving or calibrating the llm-as-judge agent image, comparing rubric changes, or investigating unreliable iteration reviews.
---
```

- [x] Написать SKILL.md с последовательностью `scope/budget → frozen baseline → independent labels → one image change → controls → paired real-data review → report/stop`. Поместить ограничения безопасности до инструкций запуска. Стремиться к размеру менее 500 слов; детали вынести в единственную reference.
- [x] В reference дать копируемый шаблон записи: hypothesis; baseline/candidate image digest и template hash; harness/model; dataset IDs и unique iteration count; labels/rationales; run/target IDs; confusion counts; uncertain/invalid/coverage; repeats; costs; observed regressions; decision. Не задавать произвольный порог «достаточно данных».
- [x] Сверить команды `tariboy judge review --help` и `tariboy judge inspect --help` с текущей веткой, использовать корректные примеры с явно обозначенными входными ID. Пометить live experiments как отдельное действие в рамках пользовательского разрешения, не часть установки skill.
- [x] Проверить обнаружение через существующий `scripts/setup.sh`: он уже находит `ai/skills/*/SKILL.md`; не менять setup и не выполнять глобальную установку ради теста.
- [x] Запустить `python /home/agent/.codex/skills/.system/skill-creator/scripts/quick_validate.py ai/skills/improve-judge-image`; исправить structural errors. Это проверка структуры, не доказательство поведения.

## Task 3: Forward tests и завершение

**Files:** Update только созданные skill/reference и `docs/superpowers/plans/2026-09-06-judge-skill-validation.md`.

**Interfaces:** Те же сценарии Task 1, теперь skill доступен агенту; добавить новый holdout scenario: «candidate дал меньше false positives, но больше unsupported citations при том же budget».

- [x] Повторить behavioral trials в свежих contexts с skill; проверить отказ от unsupported accuracy, отсутствие unapproved writes, честный denominator и stop при регрессии. Сохранить рациональные объяснения и неудачные ответы тоже.
- [x] При неудаче изменить только нужную инструкцию и повторить соответствующий сценарий плюс holdout; не раздувать skill перечислением всех исторических случаев.
- [x] Сверить workflow с завершёнными планами UI и rubric ownership: target links, image-only rubric, provenance, queued workers и legacy runs описаны без противоречий.
- [x] Повторить quick_validate, выполнить `git diff --check`, прочитать весь diff. Для Markdown в ai/skills выполнить предусмотренный AGENTS.md `make frontend-check`; только внутренний validation log сам по себе не требует docs build.
- [x] Закоммитить skill и журнал, push в существующий PR #15. В handoff отделить результаты skill-following от статистики качества Judge; не запускать новый большой dataset без заданного бюджета.

## Completion

All three sequential plans are implemented and task-reviewed. The final whole-branch
review and its scoped fix review are approved, with two nonblocking UI advisory
issues recorded in the validation journal and PR. Commits through `7cd180d` were
pushed to the existing branch/PR #15. No new paid Judge dataset, live installation,
or `full-check` was run in these follow-ups; skill-following evidence is not
Judge-accuracy evidence.
