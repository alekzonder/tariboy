package judge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/schedule"
	"github.com/alekzonder/tariboy/internal/tasks"
	"github.com/google/uuid"
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

type AutomationApplyResult struct {
	Revision AutomationRevision `json:"revision"`
	Schedule schedule.Schedule  `json:"schedule"`
}

type AutomationService struct {
	store     *Store
	schedules *schedule.Store
	validator AutomationValidator
	now       func() time.Time
	tasks     *tasks.Service
	enqueue   func(string)
	activate  func([]string) error
}

type AutomationCycle struct {
	ID             string `json:"id"`
	ConfigRevision int    `json:"config_revision"`
	DeliveryID     string `json:"delivery_id"`
	TaskKey        string `json:"task_key"`
	RunID          string `json:"run_id"`
	Status         string `json:"status"`
	LastError      string `json:"last_error"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func NewAutomationService(store *Store, schedules *schedule.Store, validator AutomationValidator, now func() time.Time) *AutomationService {
	if now == nil {
		now = time.Now
	}
	return &AutomationService{store: store, schedules: schedules, validator: validator, now: now}
}

func (s *AutomationService) Get(ctx context.Context) (AutomationRevision, error) {
	return s.store.ActiveAutomation(ctx)
}

func (s *AutomationService) Validate(ctx context.Context, raw []byte) AutomationValidation {
	parsed := ParseAutomation(raw)
	if len(parsed.Diagnostics) > 0 {
		return parsed
	}
	return s.validator.Validate(ctx, parsed.Config)
}

func (s *AutomationService) ConfigureExecution(taskService *tasks.Service, enqueue func(string)) {
	s.tasks, s.enqueue = taskService, enqueue
}

func (s *AutomationService) SetActivator(activate func([]string) error) { s.activate = activate }

type AutomationValidator struct {
	Customer        string
	AgentExists     func(context.Context, string) bool
	ImagePlugins    func(string) ([]string, error)
	ImageDigest     func(string) (string, error)
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AutomationRevision{}, err
	}
	defer tx.Rollback()
	revision, err := s.saveAutomationTx(ctx, tx, canonical, "")
	if err != nil {
		return AutomationRevision{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationRevision{}, err
	}
	return revision, nil
}

func (s *Store) ActiveAutomation(ctx context.Context) (AutomationRevision, error) {
	var out AutomationRevision
	err := s.db.QueryRowContext(ctx, `SELECT r.revision,r.config_hash,r.config_json,r.created_at FROM judge_automation_state s JOIN judge_automation_revisions r ON r.revision=s.active_revision WHERE s.singleton=1`).
		Scan(&out.Revision, &out.Hash, &out.CanonicalJSON, &out.CreatedAt)
	return out, err
}

func (s *Store) automationRevision(ctx context.Context, revision int) (AutomationRevision, error) {
	var out AutomationRevision
	err := s.db.QueryRowContext(ctx, `SELECT revision,config_hash,config_json,created_at FROM judge_automation_revisions WHERE revision=?`, revision).
		Scan(&out.Revision, &out.Hash, &out.CanonicalJSON, &out.CreatedAt)
	return out, err
}

func (s *Store) saveAutomationTx(ctx context.Context, tx *sql.Tx, canonical, scheduleID string) (AutomationRevision, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(canonical)))
	var out AutomationRevision
	err := tx.QueryRowContext(ctx, `SELECT revision,config_hash,config_json,created_at FROM judge_automation_revisions WHERE config_hash=?`, hash).
		Scan(&out.Revision, &out.Hash, &out.CanonicalJSON, &out.CreatedAt)
	if err == sql.ErrNoRows {
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision),0)+1 FROM judge_automation_revisions`).Scan(&out.Revision); err != nil {
			return AutomationRevision{}, err
		}
		out.Hash, out.CanonicalJSON, out.CreatedAt = hash, canonical, s.now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `INSERT INTO judge_automation_revisions(revision,config_json,config_hash,created_at) VALUES(?,?,?,?)`, out.Revision, canonical, hash, out.CreatedAt); err != nil {
			return AutomationRevision{}, err
		}
	} else if err != nil {
		return AutomationRevision{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO judge_automation_state(singleton,active_revision,schedule_id,updated_at) VALUES(1,?,?,?) ON CONFLICT(singleton) DO UPDATE SET active_revision=excluded.active_revision,schedule_id=excluded.schedule_id,updated_at=excluded.updated_at`, out.Revision, scheduleID, now); err != nil {
		return AutomationRevision{}, err
	}
	return out, nil
}

func (s *AutomationService) Apply(ctx context.Context, raw []byte) (AutomationApplyResult, error) {
	var result AutomationApplyResult
	var roles []string
	err := image.WithPublicationGate(func() error {
		validated := s.Validate(ctx, raw)
		if len(validated.Diagnostics) > 0 {
			return fmt.Errorf("judge automation: %s: %s", validated.Diagnostics[0].Path, validated.Diagnostics[0].Message)
		}
		parsed := AutomationValidation{Config: validated.Config}
		tx, err := s.store.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var oldSchedule string
		_ = tx.QueryRowContext(ctx, `SELECT schedule_id FROM judge_automation_state WHERE singleton=1`).Scan(&oldSchedule)
		if oldSchedule != "" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM schedules WHERE id=?`, oldSchedule); err != nil {
				return err
			}
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		for _, queue := range []struct{ prefix, name, responsible string }{
			{"JUDGE", "Judge", parsed.Config.Judge.Lead},
			{"IMPROVE", "Improve", ""},
		} {
			if _, err := tx.ExecContext(ctx, `INSERT INTO task_queues(prefix,name,description,responsible_agent,next_number,revision,created_at,updated_at) VALUES(?,?,?, ?,1,1,?,?) ON CONFLICT(prefix) DO UPDATE SET responsible_agent=excluded.responsible_agent,revision=task_queues.revision+1,updated_at=excluded.updated_at`, queue.prefix, queue.name, "", queue.responsible, now, now); err != nil {
				return err
			}
		}
		revision, err := s.store.saveAutomationTx(ctx, tx, validated.CanonicalJSON, "")
		if err != nil {
			return err
		}
		var sch schedule.Schedule
		if parsed.Config.Enabled {
			message, _ := json.Marshal(map[string]any{"type": "judge.review.requested", "data": map[string]any{"config_revision": revision.Revision}})
			sch, err = s.schedules.AddTx(tx, schedule.Schedule{Agent: parsed.Config.Judge.Lead, Kind: "cron", Spec: parsed.Config.Schedule.Spec, Channel: "agent:" + parsed.Config.Judge.Lead + ":inbox", MessageTemplate: string(message)})
			if err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE judge_automation_state SET schedule_id=? WHERE singleton=1`, sch.ID); err != nil {
			return err
		}
		roles = append([]string{parsed.Config.Judge.Lead}, parsed.Config.Judge.Workers...)
		imageDigest := ""
		if s.validator.ImageDigest != nil {
			imageDigest, err = s.validator.ImageDigest(parsed.Config.Judge.ImageRef)
			if err != nil {
				return err
			}
		}
		for _, name := range roles {
			if _, err := tx.ExecContext(ctx, `UPDATE agents SET enabled=1,loop_enabled=1,max_idle_iterations=0,
				pending_image_ref=CASE WHEN ?='' THEN pending_image_ref ELSE ? END,
				pending_image_digest=CASE WHEN ?='' THEN pending_image_digest ELSE ? END,
				pending_image_error=CASE WHEN ?='' THEN pending_image_error ELSE '' END WHERE name=?`, imageDigest, parsed.Config.Judge.ImageRef, imageDigest, imageDigest, imageDigest, name); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		result = AutomationApplyResult{Revision: revision, Schedule: sch}
		return nil
	})
	if err != nil {
		return AutomationApplyResult{}, err
	}
	if s.activate != nil {
		if err := s.activate(roles); err != nil {
			return AutomationApplyResult{}, fmt.Errorf("judge automation: activate configured agents: %w", err)
		}
	}
	return result, nil
}

func (s *AutomationService) RunOnce(ctx context.Context, limit int) (schedule.Schedule, error) {
	if limit <= 0 {
		return schedule.Schedule{}, fmt.Errorf("judge automation: limit must be positive")
	}
	revision, err := s.store.ActiveAutomation(ctx)
	if err != nil {
		return schedule.Schedule{}, err
	}
	parsed := ParseAutomation([]byte(revision.CanonicalJSON))
	validated := s.validator.Validate(ctx, parsed.Config)
	if len(parsed.Diagnostics) > 0 || len(validated.Diagnostics) > 0 || !parsed.Config.Enabled {
		return schedule.Schedule{}, fmt.Errorf("judge automation: active configuration is not runnable")
	}
	message, _ := json.Marshal(map[string]any{"type": "judge.review.requested", "data": map[string]any{"config_revision": revision.Revision, "limit": limit}})
	return s.schedules.Add(schedule.Schedule{
		Agent: parsed.Config.Judge.Lead, Kind: "oneshot", Spec: s.now().UTC().Format(time.RFC3339),
		Channel: "agent:" + parsed.Config.Judge.Lead + ":inbox", MessageTemplate: string(message),
	})
}

func (s *AutomationService) Begin(ctx context.Context, callerAgent, callerIteration string, revision int, deliveryID string, limit int) (AutomationCycle, error) {
	if s.tasks == nil {
		return AutomationCycle{}, fmt.Errorf("judge automation: tasks service is not configured")
	}
	if strings.TrimSpace(deliveryID) == "" || limit <= 0 {
		return AutomationCycle{}, fmt.Errorf("judge automation: delivery id and positive limit are required")
	}
	if cycle, err := s.store.automationCycleByDelivery(ctx, deliveryID); err == nil {
		return cycle, nil
	} else if err != sql.ErrNoRows {
		return AutomationCycle{}, err
	}
	active, err := s.store.ActiveAutomation(ctx)
	if err != nil {
		return AutomationCycle{}, err
	}
	parsed := ParseAutomation([]byte(active.CanonicalJSON))
	validated := s.validator.Validate(ctx, parsed.Config)
	if len(parsed.Diagnostics) > 0 || len(validated.Diagnostics) > 0 || !parsed.Config.Enabled || active.Revision != revision {
		return AutomationCycle{}, fmt.Errorf("judge automation: requested configuration is not active and runnable")
	}
	if callerAgent != parsed.Config.Judge.Lead {
		return AutomationCycle{}, ErrUnauthorized
	}
	actor := tasks.AgentActor(callerAgent)
	task, err := s.tasks.CreateTask(ctx, actor, tasks.CreateTaskInput{
		Queue: "JUDGE", Title: fmt.Sprintf("Judge automation cycle %s", deliveryID),
		Description: fmt.Sprintf("Automatic review using configuration revision %d.", revision),
		Assignee:    "agent:" + callerAgent, Priority: tasks.PriorityP2, IdempotencyKey: "judge-cycle:" + deliveryID,
	})
	if err != nil {
		return AutomationCycle{}, err
	}
	cycle := AutomationCycle{ID: uuid.NewString(), ConfigRevision: revision, DeliveryID: deliveryID, TaskKey: task.Key, Status: "starting"}
	now := s.now().UTC().Format(time.RFC3339Nano)
	cycle.CreatedAt, cycle.UpdatedAt = now, now
	var activeCycle string
	err = s.store.db.QueryRowContext(ctx, `SELECT id FROM judge_automation_cycles WHERE config_revision=? AND status IN ('starting','running','summarizing')`, revision).Scan(&activeCycle)
	if err != nil && err != sql.ErrNoRows {
		return AutomationCycle{}, err
	}
	if activeCycle != "" {
		cycle.Status = "skipped"
		cycle.LastError = "another cycle for this configuration revision is active"
		if err := s.store.insertAutomationCycle(ctx, cycle); err != nil {
			return AutomationCycle{}, err
		}
		if err := s.completeTask(ctx, cycle, cycle.LastError); err != nil {
			return AutomationCycle{}, err
		}
		return cycle, nil
	}
	if err := s.store.insertAutomationCycle(ctx, cycle); err != nil {
		if existing, getErr := s.store.automationCycleByDelivery(ctx, deliveryID); getErr == nil {
			return existing, nil
		}
		return AutomationCycle{}, err
	}
	var group string
	if err := s.store.db.QueryRowContext(ctx, `SELECT "group" FROM agents WHERE name=?`, callerAgent).Scan(&group); err != nil {
		return AutomationCycle{}, err
	}
	run, _, err := s.store.CreateRun(ctx, CreateRunRequest{
		OriginalRequest: fmt.Sprintf("Automatic Judge review for task %s, configuration revision %d.", task.Key, revision),
		Selector:        Selector{Agents: parsed.Config.Targets.Agents, ImageRefs: parsed.Config.Targets.ImageRefs, OnlyUnprocessed: parsed.Config.Targets.OnlyUnprocessed, Statuses: []string{"done", "no_i_am_done", "harness_error", "timeout", "killed"}, Order: "oldest", Limit: limit},
		JudgeGroup:      group, LeadAgent: callerAgent, SummaryAgent: callerAgent, CreatorIteration: callerIteration,
		JudgeAgents: parsed.Config.Judge.Workers, JudgesPerIteration: 1, MaxAttempts: 1,
	})
	if err != nil {
		status := "failed"
		if err == ErrEmptySelection {
			status = "completed"
		}
		_, _ = s.store.db.ExecContext(ctx, `UPDATE judge_automation_cycles SET status=?,last_error=?,updated_at=? WHERE id=?`, status, err.Error(), s.now().UTC().Format(time.RFC3339Nano), cycle.ID)
		if err == ErrEmptySelection {
			cycle.Status, cycle.LastError = status, err.Error()
			if taskErr := s.completeTask(ctx, cycle, "No eligible target iterations were found."); taskErr != nil {
				return AutomationCycle{}, taskErr
			}
			return cycle, nil
		}
		return AutomationCycle{}, err
	}
	cycle.RunID, cycle.Status, cycle.UpdatedAt = run.ID, "running", s.now().UTC().Format(time.RFC3339Nano)
	if _, err := s.store.db.ExecContext(ctx, `UPDATE judge_automation_cycles SET run_id=?,status=?,updated_at=? WHERE id=?`, cycle.RunID, cycle.Status, cycle.UpdatedAt, cycle.ID); err != nil {
		return AutomationCycle{}, err
	}
	if s.enqueue != nil {
		s.enqueue(run.ID)
	}
	return cycle, nil
}

func (s *AutomationService) Finish(ctx context.Context, runID, conclusion string) error {
	var cycle AutomationCycle
	if err := s.store.db.QueryRowContext(ctx, `SELECT id,config_revision,delivery_id,task_key,run_id,status,last_error,created_at,updated_at FROM judge_automation_cycles WHERE run_id=?`, runID).
		Scan(&cycle.ID, &cycle.ConfigRevision, &cycle.DeliveryID, &cycle.TaskKey, &cycle.RunID, &cycle.Status, &cycle.LastError, &cycle.CreatedAt, &cycle.UpdatedAt); err == sql.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	if err := s.completeTask(ctx, cycle, conclusion); err != nil {
		return err
	}
	_, err := s.store.db.ExecContext(ctx, `UPDATE judge_automation_cycles SET status='completed',last_error='',updated_at=? WHERE id=?`, s.now().UTC().Format(time.RFC3339Nano), cycle.ID)
	return err
}

func (s *AutomationService) Fail(ctx context.Context, runID string, failure error) error {
	var cycle AutomationCycle
	if err := s.store.db.QueryRowContext(ctx, `SELECT id,config_revision,delivery_id,task_key,run_id,status,last_error,created_at,updated_at FROM judge_automation_cycles WHERE run_id=?`, runID).
		Scan(&cycle.ID, &cycle.ConfigRevision, &cycle.DeliveryID, &cycle.TaskKey, &cycle.RunID, &cycle.Status, &cycle.LastError, &cycle.CreatedAt, &cycle.UpdatedAt); err == sql.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	message := "Judge automation failed."
	if failure != nil {
		message += " " + failure.Error()
	}
	if err := s.completeTask(ctx, cycle, message); err != nil {
		return err
	}
	_, err := s.store.db.ExecContext(ctx, `UPDATE judge_automation_cycles SET status='failed',last_error=?,updated_at=? WHERE id=?`, message, s.now().UTC().Format(time.RFC3339Nano), cycle.ID)
	return err
}

func (s *AutomationService) RecordProposal(ctx context.Context, runID, proposalID, revisionHash string) error {
	var cycle AutomationCycle
	if err := s.store.db.QueryRowContext(ctx, `SELECT id,config_revision,delivery_id,task_key,run_id,status,last_error,created_at,updated_at FROM judge_automation_cycles WHERE run_id=?`, runID).
		Scan(&cycle.ID, &cycle.ConfigRevision, &cycle.DeliveryID, &cycle.TaskKey, &cycle.RunID, &cycle.Status, &cycle.LastError, &cycle.CreatedAt, &cycle.UpdatedAt); err == sql.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	revision, err := s.store.automationRevision(ctx, cycle.ConfigRevision)
	if err != nil {
		return err
	}
	config := ParseAutomation([]byte(revision.CanonicalJSON)).Config
	_, err = s.tasks.AddComment(ctx, tasks.AgentActor(config.Judge.Lead), cycle.TaskKey, tasks.AddCommentInput{
		Body:           fmt.Sprintf("Improvement proposal `%s` is awaiting plan approval (revision `%s`).\n\n@user:%s", proposalID, revisionHash, strings.TrimPrefix(s.validator.Customer, "user:")),
		IdempotencyKey: "judge-proposal:" + proposalID + ":" + revisionHash,
	})
	return err
}

func (s *AutomationService) completeTask(ctx context.Context, cycle AutomationCycle, result string) error {
	revision, err := s.store.automationRevision(ctx, cycle.ConfigRevision)
	if err != nil {
		return err
	}
	config := ParseAutomation([]byte(revision.CanonicalJSON)).Config
	actor := tasks.AgentActor(config.Judge.Lead)
	if _, err := s.tasks.AddComment(ctx, actor, cycle.TaskKey, tasks.AddCommentInput{
		Body:           fmt.Sprintf("%s\n\n@user:%s", strings.TrimSpace(result), strings.TrimPrefix(s.validator.Customer, "user:")),
		IdempotencyKey: "judge-cycle-result:" + cycle.ID,
	}); err != nil {
		return err
	}
	detail, err := s.tasks.GetTask(ctx, actor, cycle.TaskKey)
	if err != nil {
		return err
	}
	_, err = s.tasks.CompleteTask(ctx, actor, cycle.TaskKey, tasks.CompleteInput{Revision: detail.Task.Revision, CompleteAnyway: true})
	return err
}

func (s *Store) automationCycleByDelivery(ctx context.Context, deliveryID string) (AutomationCycle, error) {
	var cycle AutomationCycle
	err := s.db.QueryRowContext(ctx, `SELECT id,config_revision,delivery_id,task_key,run_id,status,last_error,created_at,updated_at FROM judge_automation_cycles WHERE delivery_id=?`, deliveryID).
		Scan(&cycle.ID, &cycle.ConfigRevision, &cycle.DeliveryID, &cycle.TaskKey, &cycle.RunID, &cycle.Status, &cycle.LastError, &cycle.CreatedAt, &cycle.UpdatedAt)
	return cycle, err
}

func (s *Store) insertAutomationCycle(ctx context.Context, cycle AutomationCycle) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO judge_automation_cycles(id,config_revision,delivery_id,task_key,run_id,status,last_error,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, cycle.ID, cycle.ConfigRevision, cycle.DeliveryID, cycle.TaskKey, cycle.RunID, cycle.Status, cycle.LastError, cycle.CreatedAt, cycle.UpdatedAt)
	return err
}
