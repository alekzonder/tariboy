# Store Skill Help and Prompt Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every executable Store skill launcher support top-level `--help` without an agent socket, and remove obsolete `prompt.md` duplicates without losing instructions.

**Architecture:** Keep each existing skill self-contained. Add only local help dispatch in each owning Python CLI, exercise every launcher through one table-driven subprocess test, merge unique prompt guidance into the corresponding `SKILL.md`, then delete the six prompt files.

**Tech Stack:** Python 3 standard library, shell launchers, Markdown Agent Skills, Python `unittest`.

**Spec:** Native Task `TARI-41`, approved design in comments 714-715.

## Global Constraints

- Preserve the existing direct skill-local launchers and Python 3 standard-library-only implementation.
- `--help` must exit `0`, print useful usage, and work without `TARIBOY_TOOLS_SOCKET` for every executable skill launcher.
- Preserve all unique operational guidance from each deleted `store/skills/*/prompt.md` in its owning `SKILL.md`.
- Do not restore a `tools` dispatcher or shared cross-skill client.
- Keep `i-am-done` and bare `tasks` compatibility behavior unchanged.
- Keep version `0.46.1` and do not merge this branch into `main`.

---

### Task 1: Self-contained Store skill help and prompt cleanup

**Files:**
- Modify: `store/skills/test_store_skills.py`
- Modify: `store/skills/{whoami,loop,context,status,current-task,messages,schedule,scripts,image-creator,llm-as-judge,tasks}/scripts/*.py`
- Modify: `store/skills/{image-creator,llm-as-judge,messages,schedule,scripts,tasks}/SKILL.md`
- Delete: `store/skills/{image-creator,llm-as-judge,messages,schedule,scripts,tasks}/prompt.md`

**Interfaces:**
- Consumes: the existing direct `.sh` launchers and each local Python CLI's command parser.
- Produces: launcher-local top-level `--help` behavior and one authoritative `SKILL.md` per Store skill.

- [ ] **Step 1: Write the failing launcher contract**

Add a table-driven `unittest` that runs every entry in `COMMANDS` with `--help`, with `TARIBOY_TOOLS_SOCKET` removed, and asserts exit `0`, non-empty stdout containing `usage:`, and empty stderr. Add a filesystem assertion that no `store/skills/*/prompt.md` remains only after the content has been consolidated.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `python3 -B -m unittest store.skills.test_store_skills.StoreSkillsTest.test_each_executable_skill_supports_top_level_help`

Expected: FAIL because current launchers treat `--help` as an unknown command or require the socket.

- [ ] **Step 3: Implement the minimum local help branches**

In each owning Python CLI, recognize top-level `-h` and `--help` before socket validation, print that CLI's actual command surface, and return `0`. Reuse existing usage text where present; do not add shared helpers or imports between skills.

- [ ] **Step 4: Consolidate and remove duplicate prompt files**

Compare each of the six prompt files with its owning `SKILL.md`, copy only unique current operational constraints or command semantics into the skill, replace obsolete `tools <group>` examples with the direct sibling launcher, then delete the prompt file. Do not edit historical plans/specs or synthetic generic `prompt.md` fixtures.

- [ ] **Step 5: Verify GREEN and validate every skill**

Run:

```bash
python3 -B -m unittest store.skills.test_store_skills
for skill in store/skills/*/SKILL.md; do python3 /home/agent/.codex/skills/.system/skill-creator/scripts/quick_validate.py "$(dirname "$skill")"; done
git diff --check
```

Expected: all commands exit `0`; every Store skill validates; diff check is clean.

- [ ] **Step 6: Run repository verification and commit**

Run `make check`, then `make full-check` because packaged image/agent E2E behavior is in scope. Inspect the complete diff, resolve Critical and Important review findings, and commit only the intended task changes on `tari-41-skill-tools` without merging `main`.
