package judge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
	"github.com/google/uuid"
)

type Store struct {
	db  *sql.DB
	now func() time.Time
}

func NewStore(s *store.Store, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{db: s.DB, now: now}
}

func (s *Store) CreateRun(ctx context.Context, req CreateRunRequest) (Run, []Target, error) {
	if req.JudgesPerIteration == 0 {
		req.JudgesPerIteration = 1
	}
	if req.MaxAttempts == 0 {
		req.MaxAttempts = 1
	}
	if req.OriginalRequest == "" {
		return Run{}, nil, fmt.Errorf("judge: original request is required")
	}
	if req.JudgesPerIteration > len(req.JudgeAgents) {
		return Run{}, nil, ErrInsufficientJudges
	}
	if err := uniqueEligible(req.JudgeAgents); err != nil {
		return Run{}, nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, nil, err
	}
	defer tx.Rollback()
	if err := validateJudgeAgents(ctx, tx, req); err != nil {
		return Run{}, nil, err
	}
	selected, err := s.selectIterations(ctx, tx, req.Selector)
	if err != nil {
		return Run{}, nil, err
	}
	if len(selected) == 0 {
		return Run{}, nil, ErrEmptySelection
	}
	spec, err := json.Marshal(req.Selector)
	if err != nil {
		return Run{}, nil, err
	}
	judges, err := json.Marshal(req.JudgeAgents)
	if err != nil {
		return Run{}, nil, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	run := Run{ID: uuid.NewString(), CreatedAt: now, UpdatedAt: now, CreatorIteration: req.CreatorIteration, OriginalRequest: req.OriginalRequest, Spec: req.Selector, JudgeGroup: req.JudgeGroup, LeadAgent: req.LeadAgent, JudgeAgents: req.JudgeAgents, SummaryAgent: req.SummaryAgent, JudgesPerIteration: req.JudgesPerIteration, MaxAttempts: req.MaxAttempts, Status: RunSnapshotting, TargetsTotal: len(selected)}
	_, err = tx.ExecContext(ctx, `INSERT INTO judge_runs(id,created_at,updated_at,creator_iteration,original_request,spec_json,judge_group,lead_agent,judge_agents_json,summary_agent,judges_per_iteration,max_attempts,status,targets_total) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, run.ID, now, now, run.CreatorIteration, run.OriginalRequest, string(spec), run.JudgeGroup, run.LeadAgent, string(judges), run.SummaryAgent, run.JudgesPerIteration, run.MaxAttempts, run.Status, len(selected))
	if err != nil {
		return Run{}, nil, err
	}
	targets := make([]Target, 0, len(selected))
	for seq, row := range selected {
		t := Target{ID: uuid.NewString(), RunID: run.ID, Iteration: row.ID, Agent: row.Agent, Sequence: seq, SnapshotStatus: "pending", TargetState: "pending"}
		_, err = tx.ExecContext(ctx, `INSERT INTO judge_targets(id,run_id,target_iteration,target_agent,sequence) VALUES(?,?,?,?,?)`, t.ID, t.RunID, t.Iteration, t.Agent, t.Sequence)
		if err != nil {
			return Run{}, nil, err
		}
		targets = append(targets, t)
	}
	if err = tx.Commit(); err != nil {
		return Run{}, nil, err
	}
	return run, targets, nil
}

// validateJudgeAgents keeps worker authority explicit. Every named judge-side
// agent must be a current member of the selected group and have opted into the
// capability in its normal image plugin list. Targets deliberately need no
// membership relationship: they are historical iterations from any group.
func validateJudgeAgents(ctx context.Context, tx *sql.Tx, req CreateRunRequest) error {
	names := append([]string{}, req.JudgeAgents...)
	names = append(names, req.LeadAgent, req.SummaryAgent)
	seen := map[string]struct{}{}
	for _, name := range names {
		if name == "" {
			return fmt.Errorf("judge: judge group agents are required")
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		var group, plugins string
		err := tx.QueryRowContext(ctx, `SELECT "group", plugins FROM agents WHERE name=?`, name).Scan(&group, &plugins)
		if err == sql.ErrNoRows {
			return fmt.Errorf("%w: judge agent %s", ErrNotFound, name)
		}
		if err != nil {
			return err
		}
		if group != req.JudgeGroup {
			return fmt.Errorf("judge: agent %s is not in group %s", name, req.JudgeGroup)
		}
		var enabled []string
		if err := json.Unmarshal([]byte(plugins), &enabled); err != nil {
			return fmt.Errorf("judge: decode plugins for %s: %w", name, err)
		}
		ok := false
		for _, plugin := range enabled {
			if plugin == "llm-as-judge" {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("judge: agent %s is not eligible for llm-as-judge", name)
		}
	}
	return nil
}
func uniqueEligible(agents []string) error {
	seen := map[string]struct{}{}
	for _, a := range agents {
		if a == "" {
			return fmt.Errorf("judge: empty judge agent")
		}
		if _, ok := seen[a]; ok {
			return fmt.Errorf("judge: duplicate judge agent %q", a)
		}
		seen[a] = struct{}{}
	}
	return nil
}

func (s *Store) GetRun(id string) (Run, error) {
	rows, err := s.ListRuns(ListFilter{})
	if err != nil {
		return Run{}, err
	}
	for _, r := range rows {
		if r.ID == id {
			return r, nil
		}
	}
	return Run{}, ErrNotFound
}
func (s *Store) ListRuns(f ListFilter) ([]Run, error) {
	q := `SELECT id,created_at,updated_at,creator_iteration,original_request,spec_json,judge_group,lead_agent,judge_agents_json,summary_agent,judges_per_iteration,max_attempts,status,targets_total,targets_ready,assignments_total,assignments_completed,manifest_hash,current_summary_version,last_error FROM judge_runs`
	args := []any{}
	if len(f.Statuses) > 0 {
		q += " WHERE status IN (" + placeholders(len(f.Statuses)) + ")"
		for _, x := range f.Statuses {
			args = append(args, x)
		}
	}
	q += " ORDER BY created_at,id"
	if f.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, f.Limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var r Run
		var spec, judges string
		if err := rows.Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt, &r.CreatorIteration, &r.OriginalRequest, &spec, &r.JudgeGroup, &r.LeadAgent, &judges, &r.SummaryAgent, &r.JudgesPerIteration, &r.MaxAttempts, &r.Status, &r.TargetsTotal, &r.TargetsReady, &r.AssignmentsTotal, &r.AssignmentsCompleted, &r.ManifestHash, &r.CurrentSummaryVersion, &r.LastError); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(spec), &r.Spec); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(judges), &r.JudgeAgents); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Store) ListTargets(runID string) ([]Target, error) {
	rows, err := s.db.Query(`SELECT id,run_id,target_iteration,target_agent,sequence,bundle_path,bundle_hash,bundle_bytes,snapshot_status,target_state,consensus_verdict,consensus_score,assignments_completed,assignments_failed,assignments_pending FROM judge_targets WHERE run_id=? ORDER BY sequence`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Target{}
	for rows.Next() {
		var t Target
		if err := rows.Scan(&t.ID, &t.RunID, &t.Iteration, &t.Agent, &t.Sequence, &t.BundlePath, &t.BundleHash, &t.BundleBytes, &t.SnapshotStatus, &t.TargetState, &t.ConsensusVerdict, &t.ConsensusScore, &t.AssignmentsCompleted, &t.AssignmentsFailed, &t.AssignmentsPending); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		var n int
		err = s.db.QueryRow("SELECT COUNT(*) FROM judge_runs WHERE id=?", runID).Scan(&n)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, ErrNotFound
		}
	}
	return out, nil
}

// ListAnalyses and ListSummaries are intentionally operator read models. They
// decode normalized JSON so callers never need to parse database payloads.
func (s *Store) ListAnalyses(runID string) ([]Analysis, error) {
	rows, err := s.db.Query(`SELECT id,run_id,target_id,assignment_id,judge_agent,judge_iteration,schema_version,result_json,raw_submission,created_at FROM judge_analyses WHERE run_id=? ORDER BY created_at,id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Analysis{}
	for rows.Next() {
		var a Analysis
		var raw string
		if err := rows.Scan(&a.ID, &a.RunID, &a.TargetID, &a.AssignmentID, &a.JudgeAgent, &a.JudgeIteration, &a.SchemaVersion, &raw, &a.RawSubmission, &a.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &a.Result); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Store) ListSummaries(runID string) ([]Summary, error) {
	rows, err := s.db.Query(`SELECT id,run_id,summary_agent,summary_iteration,version,result_json,raw_submission,created_at FROM judge_summaries WHERE run_id=? ORDER BY version`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Summary{}
	for rows.Next() {
		var x Summary
		var raw string
		if err := rows.Scan(&x.ID, &x.RunID, &x.SummaryAgent, &x.SummaryIteration, &x.Version, &raw, &x.RawSubmission, &x.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &x.Result); err != nil {
			return nil, err
		}
		x.Coverage = x.Result.Coverage
		out = append(out, x)
	}
	return out, rows.Err()
}

// CreateAssignments materializes the fixed replica count only after immutable
// evidence is ready. INSERT OR IGNORE makes a daemon restart harmless.
func (s *Store) CreateAssignments(runID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var per int
	if err = tx.QueryRow(`SELECT judges_per_iteration FROM judge_runs WHERE id=?`, runID).Scan(&per); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	rows, err := tx.Query(`SELECT id FROM judge_targets WHERE run_id=? AND snapshot_status='ready' ORDER BY sequence`, runID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var target string
		if err := rows.Scan(&target); err != nil {
			return err
		}
		for i := 0; i < per; i++ {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO judge_assignments(id,run_id,target_id,replica_index) VALUES(?,?,?,?)`, uuid.NewString(), runID, target, i); err != nil {
				return err
			}
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err = tx.Exec(`UPDATE judge_runs SET assignments_total=(SELECT COUNT(*) FROM judge_assignments WHERE run_id=?),status='running',updated_at=? WHERE id=?`, runID, now, runID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Claim(req ClaimRequest) (Assignment, bool, error) {
	if req.RunID == "" || req.Agent == "" || req.Iteration == "" {
		return Assignment{}, false, ErrNoAssignment
	}
	if req.LeaseOwner == "" {
		req.LeaseOwner = req.Iteration
	}
	lease := time.Duration(req.LeaseDuration) * time.Second
	if lease <= 0 {
		lease = 15 * time.Minute
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Assignment{}, false, err
	}
	defer tx.Rollback()
	// A caller iteration cannot fan out work, even across runs.
	var active int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM judge_assignments WHERE judge_iteration=? AND state='claimed' AND lease_expires_at>?`, req.Iteration, s.now().UTC().Format(time.RFC3339Nano)).Scan(&active); err != nil {
		return Assignment{}, false, err
	}
	if active > 0 {
		return Assignment{}, false, nil
	}
	now := s.now().UTC()
	nowS := now.Format(time.RFC3339Nano)
	// Expired final attempts become terminal failures before selecting work.
	if _, err = tx.Exec(`UPDATE judge_assignments SET state='failed',lease_owner='',lease_expires_at='',last_error='lease expired: maximum attempts reached' WHERE run_id=? AND state='claimed' AND lease_expires_at<=? AND attempt_count >= (SELECT max_attempts FROM judge_runs WHERE id=judge_assignments.run_id)`, req.RunID, nowS); err != nil {
		return Assignment{}, false, err
	}
	row := tx.QueryRow(`SELECT a.id,a.run_id,a.target_id,a.replica_index,a.state,a.judge_agent,a.judge_iteration,a.lease_owner,a.lease_expires_at,a.attempt_count,a.last_error,a.analysis_id
		FROM judge_assignments a JOIN judge_targets t ON t.id=a.target_id JOIN judge_runs r ON r.id=a.run_id
		WHERE a.run_id=? AND r.status='running' AND t.snapshot_status='ready' AND (a.state='pending' OR (a.state='claimed' AND a.lease_expires_at<=?))
		AND a.attempt_count < r.max_attempts
		AND NOT EXISTS (SELECT 1 FROM judge_assignments prior WHERE prior.target_id=a.target_id AND prior.state='completed' AND prior.judge_agent=?)
		ORDER BY t.sequence,a.replica_index LIMIT 1`, req.RunID, nowS, req.Agent)
	var a Assignment
	err = row.Scan(&a.ID, &a.RunID, &a.TargetID, &a.ReplicaIndex, &a.State, &a.JudgeAgent, &a.JudgeIteration, &a.LeaseOwner, &a.LeaseExpiresAt, &a.AttemptCount, &a.LastError, &a.AnalysisID)
	if err == sql.ErrNoRows {
		if err = tx.Commit(); err != nil {
			return Assignment{}, false, err
		}
		return Assignment{}, false, nil
	}
	if err != nil {
		return Assignment{}, false, err
	}
	expires := now.Add(lease).Format(time.RFC3339Nano)
	res, err := tx.Exec(`UPDATE judge_assignments SET state='claimed',judge_agent=?,judge_iteration=?,lease_owner=?,lease_expires_at=?,attempt_count=attempt_count+1,last_error='' WHERE id=? AND (state='pending' OR lease_expires_at<=?)`, req.Agent, req.Iteration, req.LeaseOwner, expires, a.ID, nowS)
	if err != nil {
		return Assignment{}, false, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return Assignment{}, false, nil
	}
	a.State, a.JudgeAgent, a.JudgeIteration, a.LeaseOwner, a.LeaseExpiresAt, a.AttemptCount = "claimed", req.Agent, req.Iteration, req.LeaseOwner, expires, a.AttemptCount+1
	if err = tx.Commit(); err != nil {
		return Assignment{}, false, err
	}
	return a, true, nil
}

func (s *Store) SubmitAnalysis(req SubmitAnalysisRequest) (Analysis, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Analysis{}, err
	}
	defer tx.Rollback()
	var a Assignment
	var bundle string
	err = tx.QueryRow(`SELECT a.id,a.run_id,a.target_id,a.replica_index,a.state,a.judge_agent,a.judge_iteration,a.lease_owner,a.lease_expires_at,a.attempt_count,a.last_error,a.analysis_id,t.bundle_hash FROM judge_assignments a JOIN judge_targets t ON t.id=a.target_id WHERE a.id=?`, req.AssignmentID).Scan(&a.ID, &a.RunID, &a.TargetID, &a.ReplicaIndex, &a.State, &a.JudgeAgent, &a.JudgeIteration, &a.LeaseOwner, &a.LeaseExpiresAt, &a.AttemptCount, &a.LastError, &a.AnalysisID, &bundle)
	if err == sql.ErrNoRows {
		return Analysis{}, ErrNotFound
	}
	if err != nil {
		return Analysis{}, err
	}
	if a.State != "claimed" || a.JudgeAgent != req.Agent || a.JudgeIteration != req.Iteration || a.LeaseExpiresAt <= s.now().UTC().Format(time.RFC3339Nano) {
		return Analysis{}, ErrLeaseNotOwned
	}
	resolver := CitationResolverFunc(func(c Citation) error {
		if c.BundleHash != bundle {
			return ErrBadLocator
		}
		if req.Resolve == nil {
			return ErrBadLocator
		}
		return req.Resolve.ResolveCitation(c)
	})
	validErr := ValidateAnalysis(req.Result, resolver)
	now := s.now().UTC().Format(time.RFC3339Nano)
	if req.RawSubmission == "" {
		b, _ := json.Marshal(req.Result)
		req.RawSubmission = string(b)
	}
	// A claim attempt may include one or more schema repairs. Number every raw
	// submission independently so neither invalid output nor its replacement is
	// overwritten.
	var submissionNumber int
	if err = tx.QueryRow(`SELECT COALESCE(MAX(attempt_number),0)+1 FROM judge_submission_attempts WHERE assignment_id=?`, a.ID).Scan(&submissionNumber); err != nil {
		return Analysis{}, err
	}
	_, err = tx.Exec(`INSERT INTO judge_submission_attempts(id,assignment_id,attempt_number,raw_json,validation_error,created_at) VALUES(?,?,?,?,?,?)`, uuid.NewString(), a.ID, submissionNumber, req.RawSubmission, errString(validErr), now)
	if err != nil {
		return Analysis{}, err
	}
	if validErr != nil {
		_, err = tx.Exec(`UPDATE judge_assignments SET last_error=? WHERE id=?`, validErr.Error(), a.ID)
		if err != nil {
			return Analysis{}, err
		}
		if err = tx.Commit(); err != nil {
			return Analysis{}, err
		}
		return Analysis{}, fmt.Errorf("%w: %v", ErrInvalidSubmission, validErr)
	}
	result, err := json.Marshal(req.Result)
	if err != nil {
		return Analysis{}, err
	}
	out := Analysis{ID: uuid.NewString(), RunID: a.RunID, TargetID: a.TargetID, AssignmentID: a.ID, JudgeAgent: a.JudgeAgent, JudgeIteration: a.JudgeIteration, SchemaVersion: 1, Result: req.Result, RawSubmission: req.RawSubmission, CreatedAt: now}
	_, err = tx.Exec(`INSERT INTO judge_analyses(id,run_id,target_id,assignment_id,judge_agent,judge_iteration,schema_version,result_json,raw_submission,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, out.ID, out.RunID, out.TargetID, out.AssignmentID, out.JudgeAgent, out.JudgeIteration, out.SchemaVersion, string(result), out.RawSubmission, now)
	if err != nil {
		return Analysis{}, err
	}
	_, err = tx.Exec(`UPDATE judge_assignments SET state='completed',lease_owner='',lease_expires_at='',analysis_id=?,last_error='' WHERE id=?`, out.ID, a.ID)
	if err != nil {
		return Analysis{}, err
	}
	if err = s.recomputeTx(tx, a.RunID, a.TargetID); err != nil {
		return Analysis{}, err
	}
	if err = tx.Commit(); err != nil {
		return Analysis{}, err
	}
	return out, nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *Store) recomputeTx(tx *sql.Tx, runID, targetID string) error {
	rows, err := tx.Query(`SELECT result_json FROM judge_analyses WHERE target_id=? ORDER BY created_at,id`, targetID)
	if err != nil {
		return err
	}
	var results []AnalysisResult
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		var r AnalysisResult
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			rows.Close()
			return err
		}
		results = append(results, r)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	c := Consensus(results)
	_, err = tx.Exec(`UPDATE judge_targets SET assignments_completed=(SELECT COUNT(*) FROM judge_assignments WHERE target_id=? AND state='completed'),assignments_failed=(SELECT COUNT(*) FROM judge_assignments WHERE target_id=? AND state IN ('failed','cancelled')),assignments_pending=(SELECT COUNT(*) FROM judge_assignments WHERE target_id=? AND state IN ('pending','claimed')),consensus_verdict=?,consensus_score=?,target_state=CASE WHEN (SELECT COUNT(*) FROM judge_assignments WHERE target_id=? AND state IN ('pending','claimed'))=0 THEN 'terminal' ELSE 'running' END WHERE id=?`, targetID, targetID, targetID, c.Verdict, nullFloat(len(results) > 0, c.Score), targetID, targetID)
	if err != nil {
		return err
	}
	var total, completed, pending, analyses, snapshots int
	err = tx.QueryRow(`SELECT COUNT(*),SUM(state='completed'),SUM(state IN ('pending','claimed')), (SELECT COUNT(*) FROM judge_analyses WHERE run_id=?), (SELECT COUNT(*) FROM judge_targets WHERE run_id=? AND snapshot_status NOT IN ('ready','snapshot_failed')) FROM judge_assignments WHERE run_id=?`, runID, runID, runID).Scan(&total, &completed, &pending, &analyses, &snapshots)
	if err != nil {
		return err
	}
	status := "running"
	if pending == 0 && snapshots == 0 {
		if analyses > 0 {
			status = "summarizing"
		} else {
			status = "partial"
		}
	}
	_, err = tx.Exec(`UPDATE judge_runs SET assignments_total=?,assignments_completed=?,status=?,updated_at=? WHERE id=?`, total, completed, status, s.now().UTC().Format(time.RFC3339Nano), runID)
	return err
}
func nullFloat(ok bool, f float64) any {
	if !ok {
		return nil
	}
	return f
}

// ClaimSummary reserves the terminal run for its configured summary agent.
func (s *Store) ClaimSummary(runID, agent, iteration string) (Run, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()
	var configured, status string
	if err = tx.QueryRow(`SELECT summary_agent,status FROM judge_runs WHERE id=?`, runID).Scan(&configured, &status); err == sql.ErrNoRows {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, err
	}
	if configured != agent || status != "summarizing" {
		return Run{}, ErrLeaseNotOwned
	}
	// Summary claims use the run's error field as an intentionally small durable lease.
	_, err = tx.Exec(`UPDATE judge_runs SET last_error=?,updated_at=? WHERE id=? AND last_error=''`, "summary claimed by "+iteration, s.now().UTC().Format(time.RFC3339Nano), runID)
	if err != nil {
		return Run{}, err
	}
	if err = tx.Commit(); err != nil {
		return Run{}, err
	}
	return s.GetRun(runID)
}

func (s *Store) SubmitSummary(req SubmitSummaryRequest) (Summary, error) {
	if err := ValidateSummary(req.Result); err != nil {
		return Summary{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Summary{}, err
	}
	defer tx.Rollback()
	var configured, status, claimed string
	if err = tx.QueryRow(`SELECT summary_agent,status,last_error FROM judge_runs WHERE id=?`, req.RunID).Scan(&configured, &status, &claimed); err == sql.ErrNoRows {
		return Summary{}, ErrNotFound
	}
	if err != nil {
		return Summary{}, err
	}
	if configured != req.Agent || status != "summarizing" || claimed != "summary claimed by "+req.Iteration {
		return Summary{}, ErrLeaseNotOwned
	}
	var version int
	if err = tx.QueryRow(`SELECT COALESCE(MAX(version),0)+1 FROM judge_summaries WHERE run_id=?`, req.RunID).Scan(&version); err != nil {
		return Summary{}, err
	}
	if req.RawSubmission == "" {
		b, _ := json.Marshal(req.Result)
		req.RawSubmission = string(b)
	}
	raw, err := json.Marshal(req.Result)
	if err != nil {
		return Summary{}, err
	}
	coverage, _ := json.Marshal(req.Result.Coverage)
	now := s.now().UTC().Format(time.RFC3339Nano)
	out := Summary{ID: uuid.NewString(), RunID: req.RunID, Version: version, SummaryAgent: req.Agent, SummaryIteration: req.Iteration, Coverage: req.Result.Coverage, Result: req.Result, RawSubmission: req.RawSubmission, CreatedAt: now}
	_, err = tx.Exec(`INSERT INTO judge_summaries(id,run_id,version,summary_agent,summary_iteration,coverage_json,result_json,raw_submission,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, out.ID, out.RunID, out.Version, out.SummaryAgent, out.SummaryIteration, string(coverage), string(raw), out.RawSubmission, out.CreatedAt)
	if err != nil {
		return Summary{}, err
	}
	var incomplete int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM judge_assignments WHERE run_id=? AND state!='completed'`, req.RunID).Scan(&incomplete); err != nil {
		return Summary{}, err
	}
	status = "completed"
	if incomplete > 0 {
		status = "partial"
	}
	_, err = tx.Exec(`UPDATE judge_runs SET status=?,current_summary_version=?,last_error='',updated_at=? WHERE id=?`, status, version, now, req.RunID)
	if err != nil {
		return Summary{}, err
	}
	if err = tx.Commit(); err != nil {
		return Summary{}, err
	}
	return out, nil
}

// Retry returns terminal failed work to the queue without changing already
// immutable analyses or summaries.
func (s *Store) Retry(runID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Cancellation is an operator terminal decision. Retrying is deliberately
	// narrower: only failures re-enter the queue, so a cancelled run preserves
	// both its artifacts and its cancellation state until a new run is created.
	res, err := tx.Exec(`UPDATE judge_assignments SET state='pending',judge_agent='',judge_iteration='',lease_owner='',lease_expires_at='',last_error='' WHERE run_id=? AND state='failed'`, runID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNoAssignment
	}
	_, err = tx.Exec(`UPDATE judge_runs SET status='running',last_error='',updated_at=? WHERE id=?`, s.now().UTC().Format(time.RFC3339Nano), runID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// RecoverExpiredLeases releases expired work even when no worker asks for a
// claim. Recomputing affected targets also advances a fully exhausted run.
func (s *Store) RecoverExpiredLeases(runID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.now().UTC().Format(time.RFC3339Nano)
	rows, err := tx.Query(`SELECT DISTINCT target_id FROM judge_assignments WHERE run_id=? AND state='claimed' AND lease_expires_at<=?`, runID, now)
	if err != nil {
		return err
	}
	var targets []string
	for rows.Next() {
		var target string
		if err = rows.Scan(&target); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, target)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if len(targets) == 0 {
		return tx.Commit()
	}
	_, err = tx.Exec(`UPDATE judge_assignments SET state=CASE WHEN attempt_count >= (SELECT max_attempts FROM judge_runs WHERE id=judge_assignments.run_id) THEN 'failed' ELSE 'pending' END, judge_agent='',judge_iteration='',lease_owner='',lease_expires_at='',last_error=CASE WHEN attempt_count >= (SELECT max_attempts FROM judge_runs WHERE id=judge_assignments.run_id) THEN 'lease expired: maximum attempts reached' ELSE 'lease expired' END WHERE run_id=? AND state='claimed' AND lease_expires_at<=?`, runID, now)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err = s.recomputeTx(tx, runID, target); err != nil {
			return err
		}
	}
	return tx.Commit()
}

var _ = errors.Is
