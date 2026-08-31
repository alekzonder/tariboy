## Local scripts

The Python script lives inside this skill directory under `scripts/`.

Durable scripts continue after an iteration ends. Never hold an iteration open
waiting for one, and never queue the same one-shot twice. Load the packaged
`scripts` skill for run, schedule, result, and lifecycle usage.

Commands: `tools script run`, `tools script schedule`, `tools script ls`,
`tools script runs`, `tools script logs`, `tools script rerun`,
`tools script cancel`, and `tools script rm`.
