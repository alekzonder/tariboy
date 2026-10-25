package commands

import (
	"encoding/json"

	"github.com/alekzonder/tariboy/internal/maintenance"
	"github.com/alekzonder/tariboy/internal/retention"
	"github.com/invopop/jsonschema"
)

// Response types of the daemon, maintenance, retention, backup and restore
// commands. Each command declares its type with registry.WithResult, which
// derives the OpenAPI result schema from it.

// DaemonStatus is the running daemon's identity, uptime and missing host
// tools (daemon.status).
type DaemonStatus struct {
	State         string              `json:"state"`
	MissingTools  []DaemonMissingTool `json:"missing_tools"`
	Message       string              `json:"message"`
	Version       string              `json:"version"`
	Pid           int                 `json:"pid"`
	StartedAt     string              `json:"started_at"`
	UptimeSeconds int64               `json:"uptime_seconds"`
	BaseDir       string              `json:"base_dir"`
	HTTPAddr      string              `json:"http_addr"`
	SchemaVersion int                 `json:"schema_version"`
}

// DaemonMissingTool is one host command the daemon cannot find on its PATH.
type DaemonMissingTool struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
	Install string `json:"install"`
}

// DaemonConfigEntry is the key and value daemon.config.set stored.
type DaemonConfigEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// DaemonReindexResult counts the ai_requests rows daemon.reindex rebuilt.
type DaemonReindexResult struct {
	Reindexed int `json:"reindexed"`
}

// MaintenanceSettings are the nightly database maintenance settings.
type MaintenanceSettings maintenance.Settings

// MaintenanceRun describes one maintenance run (maintenance.run). Deleted
// counts the removed rows per table.
type MaintenanceRun struct {
	StartedAt  string           `json:"started_at"`
	FinishedAt string           `json:"finished_at"`
	Trigger    string           `json:"trigger"`
	Backup     string           `json:"backup"`
	Deleted    map[string]int64 `json:"deleted" jsonschema_extras:"nullable=true"`
	SizeBefore int64            `json:"size_before"`
	SizeAfter  int64            `json:"size_after"`
	Compacted  bool             `json:"compacted"`
	Error      string           `json:"error,omitempty"`
}

// MaintenanceLastRun is the last maintenance run, encoded as null before
// the first run.
type MaintenanceLastRun struct {
	MaintenanceRun
	ran bool
}

func newMaintenanceLastRun(r *maintenance.Result) MaintenanceLastRun {
	if r == nil {
		return MaintenanceLastRun{}
	}
	return MaintenanceLastRun{MaintenanceRun: MaintenanceRun(*r), ran: true}
}

func (r MaintenanceLastRun) MarshalJSON() ([]byte, error) {
	if !r.ran {
		return []byte("null"), nil
	}
	return json.Marshal(r.MaintenanceRun)
}

func (MaintenanceLastRun) JSONSchemaExtend(s *jsonschema.Schema) {
	if s.Extras == nil {
		s.Extras = map[string]any{}
	}
	s.Extras["nullable"] = true
}

// MaintenanceView is the maintenance settings and the last run
// (maintenance.get).
type MaintenanceView struct {
	Settings MaintenanceSettings `json:"settings"`
	LastRun  MaintenanceLastRun  `json:"last_run"`
}

// RetentionPolicy limits the iterations an agent keeps; 0 means no limit,
// or for an agent, the daemon default.
type RetentionPolicy struct {
	KeepIterations int   `json:"keep_iterations"`
	KeepDays       int   `json:"keep_days"`
	MaxBytes       int64 `json:"max_bytes"`
}

func newRetentionPolicy(p retention.Policy) RetentionPolicy {
	return RetentionPolicy{KeepIterations: p.KeepIterations, KeepDays: p.KeepDays, MaxBytes: p.MaxBytes}
}

// RetentionView is a retention policy (retention.get). For an agent it is
// the effective policy with the agent's own override and the default; for
// "default" it is the default alone.
type RetentionView struct {
	RetentionPolicy
	Override *RetentionPolicy `json:"override,omitempty"`
	Default  *RetentionPolicy `json:"default,omitempty"`
}

// RetentionSetResult is the policy retention.set stored for agent, which is
// an agent name or "default".
type RetentionSetResult struct {
	RetentionPolicy
	Agent string `json:"agent"`
}

// PruneReport is what pruning one agent removed, or would remove on a dry run.
type PruneReport struct {
	Agent      string   `json:"agent"`
	Pruned     []string `json:"pruned"`
	Archived   []string `json:"archived"`
	FreedBytes int64    `json:"freed_bytes"`
	DryRun     bool     `json:"dry_run"`
}

// PruneResult is the prune result: for one agent the report keys, for "all"
// the reports of every agent.
type PruneResult struct {
	Agent      string         `json:"agent,omitempty"`
	Pruned     []string       `json:"pruned,omitzero"`
	Archived   []string       `json:"archived,omitzero"`
	FreedBytes *int64         `json:"freed_bytes,omitempty"`
	Reports    *[]PruneReport `json:"reports,omitempty" jsonschema_extras:"nullable=true"`
	DryRun     bool           `json:"dry_run"`
}

// BackupArchive is one written backup archive. Warning appears when the
// archive holds secret values.
type BackupArchive struct {
	Agent         string `json:"agent"`
	Path          string `json:"path"`
	Bytes         int64  `json:"bytes"`
	SHA256        string `json:"sha256"`
	SchemaVersion int    `json:"schema_version"`
	Warning       string `json:"warning,omitempty"`
}

// BackupResult is the backup result: for one agent the archive keys, for
// "all" the archives of every agent. The archive keys are never empty for a
// written archive, so omitempty only drops them from the "all" result.
type BackupResult struct {
	Agent         string           `json:"agent,omitempty"`
	Path          string           `json:"path,omitempty"`
	Bytes         int64            `json:"bytes,omitempty"`
	SHA256        string           `json:"sha256,omitempty"`
	SchemaVersion int              `json:"schema_version,omitempty"`
	Warning       string           `json:"warning,omitempty"`
	Archives      *[]BackupArchive `json:"archives,omitempty" jsonschema_extras:"nullable=true"`
	Count         *int             `json:"count,omitempty"`
}

// RestoreResult names the restored agent and the agent its archive held.
type RestoreResult struct {
	Agent         string `json:"agent"`
	SourceAgent   string `json:"source_agent"`
	SchemaVersion int    `json:"schema_version"`
}
