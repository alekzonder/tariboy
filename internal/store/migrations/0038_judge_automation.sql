CREATE TABLE judge_automation_revisions (
  revision INTEGER PRIMARY KEY,
  config_json TEXT NOT NULL,
  config_hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL
);

CREATE TABLE judge_automation_state (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  active_revision INTEGER NOT NULL REFERENCES judge_automation_revisions(revision) ON DELETE RESTRICT,
  schedule_id TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);

CREATE TABLE judge_automation_cycles (
  id TEXT PRIMARY KEY,
  config_revision INTEGER NOT NULL REFERENCES judge_automation_revisions(revision) ON DELETE RESTRICT,
  delivery_id TEXT NOT NULL UNIQUE,
  task_key TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX judge_automation_cycles_active_revision_idx
  ON judge_automation_cycles(config_revision)
  WHERE status IN ('starting','running','summarizing');

CREATE TABLE improvement_tasks (
  proposal_id TEXT NOT NULL REFERENCES improvement_proposals(id) ON DELETE RESTRICT,
  revision_hash TEXT NOT NULL,
  task_key TEXT NOT NULL UNIQUE REFERENCES tasks(task_key) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  PRIMARY KEY(proposal_id, revision_hash)
);
