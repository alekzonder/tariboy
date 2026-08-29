package judge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/schedule"
)

type AutomationConfig struct {
	SchemaVersion int                `json:"schema_version"`
	Enabled       bool               `json:"enabled"`
	Judge         AutomationJudge    `json:"judge"`
	Schedule      AutomationSchedule `json:"schedule"`
	Targets       AutomationTargets  `json:"targets"`
}

type AutomationJudge struct {
	Lead     string   `json:"lead"`
	Workers  []string `json:"workers"`
	ImageRef string   `json:"image_ref"`
}

type AutomationSchedule struct {
	Spec string `json:"spec"`
}

type AutomationTargets struct {
	Agents          []string `json:"agents"`
	ImageRefs       []string `json:"image_refs"`
	OnlyUnprocessed bool     `json:"only_unprocessed"`
}

type ValidationDiagnostic struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type AutomationValidation struct {
	Config        AutomationConfig       `json:"config"`
	CanonicalJSON string                 `json:"canonical_json,omitempty"`
	Diagnostics   []ValidationDiagnostic `json:"diagnostics"`
}

type AutomationRevision struct {
	Revision      int    `json:"revision"`
	Hash          string `json:"hash"`
	CanonicalJSON string `json:"canonical_json"`
	CreatedAt     string `json:"created_at"`
}

type AutomationValidator struct {
	Customer        string
	AgentExists     func(context.Context, string) bool
	ImagePlugins    func(string) ([]string, error)
	TargetImageUsed func(context.Context, []string, string) bool
}

func ParseAutomation(raw []byte) AutomationValidation {
	result := AutomationValidation{Diagnostics: []ValidationDiagnostic{}}
	if diagnostic := unknownAutomationField(raw); diagnostic != nil {
		result.Diagnostics = append(result.Diagnostics, *diagnostic)
		return result
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result.Config); err != nil {
		result.Diagnostics = append(result.Diagnostics, ValidationDiagnostic{Path: "/", Message: err.Error()})
		return result
	}
	if decoder.More() {
		result.Diagnostics = append(result.Diagnostics, ValidationDiagnostic{Path: "/", Message: "multiple JSON values are not allowed"})
	}
	return result
}

func unknownAutomationField(raw []byte) *ValidationDiagnostic {
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil {
		return nil
	}
	checks := []struct {
		path    string
		object  map[string]json.RawMessage
		allowed map[string]bool
	}{
		{"", root, set("schema_version", "enabled", "judge", "schedule", "targets")},
	}
	for _, nested := range []struct {
		name    string
		allowed map[string]bool
	}{
		{"judge", set("lead", "workers", "image_ref")},
		{"schedule", set("spec")},
		{"targets", set("agents", "image_refs", "only_unprocessed")},
	} {
		var object map[string]json.RawMessage
		if json.Unmarshal(root[nested.name], &object) == nil {
			checks = append(checks, struct {
				path    string
				object  map[string]json.RawMessage
				allowed map[string]bool
			}{"/" + nested.name, object, nested.allowed})
		}
	}
	for _, check := range checks {
		keys := make([]string, 0, len(check.object))
		for key := range check.object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if !check.allowed[key] {
				return &ValidationDiagnostic{Path: check.path + "/" + key, Message: "unknown field"}
			}
		}
	}
	return nil
}

func set(values ...string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func (v AutomationValidator) Validate(ctx context.Context, config AutomationConfig) AutomationValidation {
	result := AutomationValidation{Config: config, Diagnostics: []ValidationDiagnostic{}}
	add := func(path, message string) {
		result.Diagnostics = append(result.Diagnostics, ValidationDiagnostic{Path: path, Message: message})
	}
	if config.SchemaVersion != 1 {
		add("/schema_version", "must be 1")
	}
	if strings.TrimSpace(v.Customer) == "" {
		add("/customer", "USER is empty")
	}
	if strings.TrimSpace(config.Judge.Lead) == "" {
		add("/judge/lead", "is required")
	}
	if len(config.Judge.Workers) != 2 {
		add("/judge/workers", "must contain exactly two agents")
	}
	if strings.TrimSpace(config.Judge.ImageRef) == "" {
		add("/judge/image_ref", "is required")
	}
	if _, err := schedule.Parse(config.Schedule.Spec); err != nil {
		add("/schedule/spec", err.Error())
	}
	validateUnique := func(path string, values []string) {
		if len(values) == 0 {
			add(path, "must not be empty")
			return
		}
		seen := map[string]bool{}
		for i, value := range values {
			if strings.TrimSpace(value) == "" {
				add(fmt.Sprintf("%s/%d", path, i), "must not be empty")
			} else if seen[value] {
				add(fmt.Sprintf("%s/%d", path, i), "duplicate value")
			}
			seen[value] = true
		}
	}
	validateUnique("/judge/workers", config.Judge.Workers)
	validateUnique("/targets/agents", config.Targets.Agents)
	validateUnique("/targets/image_refs", config.Targets.ImageRefs)

	roles := append([]string{config.Judge.Lead}, config.Judge.Workers...)
	targets := set(config.Targets.Agents...)
	for i, name := range roles {
		path := "/judge/lead"
		if i > 0 {
			path = fmt.Sprintf("/judge/workers/%d", i-1)
		}
		if targets[name] {
			add(path, "must not also be a target agent")
		}
		if name != "" && v.AgentExists != nil && !v.AgentExists(ctx, name) {
			add(path, "agent does not exist")
		}
	}
	for i, name := range config.Targets.Agents {
		if v.AgentExists != nil && !v.AgentExists(ctx, name) {
			add(fmt.Sprintf("/targets/agents/%d", i), "agent does not exist")
		}
	}
	if v.ImagePlugins != nil && config.Judge.ImageRef != "" {
		plugins, err := v.ImagePlugins(config.Judge.ImageRef)
		if err != nil {
			add("/judge/image_ref", "image does not exist")
		} else {
			have := set(plugins...)
			for _, required := range []string{"llm-as-judge", "schedule", "tasks", "current-task", "messages", "loop"} {
				if !have[required] {
					add("/judge/image_ref", "image lacks "+required+" capability")
					break
				}
			}
		}
	}
	for i, ref := range config.Targets.ImageRefs {
		if v.ImagePlugins != nil {
			if _, err := v.ImagePlugins(ref); err != nil {
				add(fmt.Sprintf("/targets/image_refs/%d", i), "image does not exist")
				continue
			}
		}
		if v.TargetImageUsed != nil && !v.TargetImageUsed(ctx, config.Targets.Agents, ref) {
			add(fmt.Sprintf("/targets/image_refs/%d", i), "image was not used by a configured target")
		}
	}
	if len(result.Diagnostics) == 0 {
		canonical, _ := json.MarshalIndent(config, "", "  ")
		result.CanonicalJSON = string(canonical) + "\n"
	}
	return result
}

func (s *Store) SaveAutomation(ctx context.Context, canonical string) (AutomationRevision, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(canonical)))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AutomationRevision{}, err
	}
	defer tx.Rollback()
	var existing AutomationRevision
	err = tx.QueryRowContext(ctx, `SELECT revision,config_hash,config_json,created_at FROM judge_automation_revisions WHERE config_hash=?`, hash).
		Scan(&existing.Revision, &existing.Hash, &existing.CanonicalJSON, &existing.CreatedAt)
	if err == nil {
		return existing, tx.Commit()
	}
	var revision int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision),0)+1 FROM judge_automation_revisions`).Scan(&revision); err != nil {
		return AutomationRevision{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO judge_automation_revisions(revision,config_json,config_hash,created_at) VALUES(?,?,?,?)`, revision, canonical, hash, now); err != nil {
		return AutomationRevision{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO judge_automation_state(singleton,active_revision,updated_at) VALUES(1,?,?) ON CONFLICT(singleton) DO UPDATE SET active_revision=excluded.active_revision,updated_at=excluded.updated_at`, revision, now); err != nil {
		return AutomationRevision{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationRevision{}, err
	}
	return AutomationRevision{Revision: revision, Hash: hash, CanonicalJSON: canonical, CreatedAt: now}, nil
}

func (s *Store) ActiveAutomation(ctx context.Context) (AutomationRevision, error) {
	var out AutomationRevision
	err := s.db.QueryRowContext(ctx, `SELECT r.revision,r.config_hash,r.config_json,r.created_at FROM judge_automation_state s JOIN judge_automation_revisions r ON r.revision=s.active_revision WHERE s.singleton=1`).
		Scan(&out.Revision, &out.Hash, &out.CanonicalJSON, &out.CreatedAt)
	return out, err
}
