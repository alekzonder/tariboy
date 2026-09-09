#!/usr/bin/env bash

make_test_image_fixture() {
  local target="$1"
  rm -rf -- "$target"
  mkdir -p "$target/skills/loop/scripts" "$target/skills/tasks/scripts"
  local skill
  for skill in whoami loop messages context status schedule image-creator tasks; do
    mkdir -p "$target/skills/$skill"
    printf '%s\n' '---' "name: $skill" "description: Test $skill capability." '---' >"$target/skills/$skill/SKILL.md"
  done
  printf '%s\n' '#!/bin/sh' 'exit 0' >"$target/skills/loop/scripts/loop.sh"
  printf '%s\n' '#!/bin/sh' 'exec ttasks "$@"' >"$target/skills/tasks/scripts/tasks.sh"
  chmod 0700 "$target/skills/loop/scripts/loop.sh" "$target/skills/tasks/scripts/tasks.sh"
  printf '%s\n' 'Finish the iteration by calling i-am-done.' >"$target/skills/loop/finish.md"
  cat >"$target/Tariboyfile.yaml" <<'YAML'
schema_version: 2
plugins:
  - name: whoami
  - name: loop
  - name: messages
  - name: context
  - name: status
  - name: schedule
  - name: image-creator
  - name: tasks
skills:
  - dir: ./skills/whoami
  - dir: ./skills/loop
  - dir: ./skills/messages
  - dir: ./skills/context
  - dir: ./skills/status
  - dir: ./skills/schedule
  - dir: ./skills/image-creator
  - dir: ./skills/tasks
prompts:
  - runtime: identity
  - runtime: user-prompt
  - file: ./skills/loop/finish.md
YAML
}
