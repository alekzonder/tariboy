// Package assessment stores immutable assessments of an iteration or a task:
// three scored axes and the violations found, written by an assessor agent
// through its tools socket. An assessment keeps snapshots of the subject's
// image and workflow, so it outlives iteration retention and agent removal.
// There is no update and no delete; a reassessment is a new record.
package assessment

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// Limits of List.
const (
	defaultLimit = 100
	maxLimit     = 1000
)

// Error is an assessment failure with a stable code the API layer maps to a
// status.
type Error struct {
	Status int
	Code   string
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

func errInvalid(format string, args ...any) error {
	return &Error{Status: http.StatusBadRequest, Code: "invalid_assessment", Msg: fmt.Sprintf(format, args...)}
}

// Service is the assessment store over the assessments tables.
type Service struct {
	db    *sql.DB
	clock func() time.Time
}

// New returns the service over db. clock defaults to time.Now.
func New(db *sql.DB, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{db: db, clock: clock}
}

// Add validates in and writes one assessment by assessor, whose current
// iteration is assessorIteration. The assessor's image comes from that
// iteration's row, the subject snapshots from the subject iteration and task.
// The record, its axes and its violations are written in one transaction.
func (s *Service) Add(assessor, assessorIteration string, in AssessmentInput) (AssessmentShowResult, error) {
	if assessorIteration == "" {
		return AssessmentShowResult{}, &Error{Status: http.StatusConflict, Code: "no_iteration",
			Msg: "no iteration is currently running"}
	}
	in = trimInput(in)
	if err := validate(in); err != nil {
		return AssessmentShowResult{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AssessmentShowResult{}, err
	}
	defer func() { _ = tx.Rollback() }() // a no-op after Commit
	rec, err := snapshot(tx, assessor, assessorIteration, in)
	if err != nil {
		return AssessmentShowResult{}, err
	}
	now := s.clock().UTC()
	rec.CreatedAt = store.FormatTime(now)
	a, err := insert(tx, Assessment{AssessmentRecord: rec, Violations: in.Violations}, now)
	if err != nil {
		return AssessmentShowResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AssessmentShowResult{}, err
	}
	return AssessmentShowResult{Assessment: a}, nil
}

func trimInput(in AssessmentInput) AssessmentInput {
	in.RubricVersion = strings.TrimSpace(in.RubricVersion)
	in.IterationID = strings.TrimSpace(in.IterationID)
	in.TaskKey = strings.TrimSpace(in.TaskKey)
	in.Axes = slices.Clone(in.Axes)
	in.Violations = slices.Clone(in.Violations)
	if in.Violations == nil {
		in.Violations = []AssessmentViolation{}
	}
	return in
}

// validate checks everything that needs no database read.
func validate(in AssessmentInput) error {
	if in.IterationID == "" && in.TaskKey == "" {
		return errInvalid("an assessment needs iteration_id, task_key or both")
	}
	if in.RubricVersion == "" {
		return errInvalid("rubric_version is required")
	}
	if strings.TrimSpace(in.Summary) == "" {
		return errInvalid("summary is required")
	}
	if err := validateAxes(in.Axes); err != nil {
		return err
	}
	for i, v := range in.Violations {
		if err := validateViolation(v); err != nil {
			return errInvalid("violation %d: %s", i+1, err.(*Error).Msg)
		}
	}
	return nil
}

func validateAxes(axes []AssessmentAxis) error {
	if len(axes) != len(Axes) {
		return errInvalid("an assessment needs exactly %d axes (%s), got %d", len(Axes), strings.Join(Axes, ", "), len(axes))
	}
	seen := map[string]bool{}
	for _, a := range axes {
		if !slices.Contains(Axes, a.Axis) {
			return errInvalid("unknown axis %q; use %s", a.Axis, strings.Join(Axes, ", "))
		}
		if seen[a.Axis] {
			return errInvalid("axis %s is given twice", a.Axis)
		}
		seen[a.Axis] = true
		switch a.Verdict {
		case VerdictScored:
			if a.Score == nil || *a.Score < 0 || *a.Score > 100 {
				return errInvalid("axis %s: a scored verdict needs a score from 0 to 100", a.Axis)
			}
		case VerdictNotApplicable:
			if a.Score != nil {
				return errInvalid("axis %s: a not_applicable verdict takes no score", a.Axis)
			}
		default:
			return errInvalid("axis %s: verdict must be %s or %s, got %q", a.Axis, VerdictScored, VerdictNotApplicable, a.Verdict)
		}
	}
	return nil
}

func validateViolation(v AssessmentViolation) error {
	if !slices.Contains(Axes, v.Axis) {
		return errInvalid("unknown axis %q", v.Axis)
	}
	if strings.TrimSpace(v.Description) == "" {
		return errInvalid("description is required")
	}
	if strings.TrimSpace(v.Quote) == "" {
		return errInvalid("quote is required")
	}
	e := v.Evidence
	switch e.Kind {
	case EvidenceTranscript:
		if strings.TrimSpace(e.IterationID) == "" || strings.TrimSpace(e.Locator) == "" {
			return errInvalid("transcript evidence needs iteration_id and locator")
		}
	case EvidenceTaskEvent:
		if strings.TrimSpace(e.TaskKey) == "" || e.Sequence <= 0 {
			return errInvalid("task_event evidence needs task_key and sequence")
		}
	case EvidenceScriptRun:
		if !strings.HasPrefix(strings.TrimSpace(e.RunID), "srun-") {
			return errInvalid("script_run evidence needs a run_id that starts with srun-")
		}
	default:
		return errInvalid("evidence kind must be %s, %s or %s, got %q", EvidenceTranscript, EvidenceTaskEvent, EvidenceScriptRun, e.Kind)
	}
	return nil
}

// snapshot reads the assessor's iteration, the subject iteration and the
// subject task, checks that the two subjects are linked, and returns the
// record without its id and time.
func snapshot(tx *sql.Tx, assessor, assessorIteration string, in AssessmentInput) (AssessmentRecord, error) {
	rec := AssessmentRecord{AssessorAgent: assessor, RubricVersion: in.RubricVersion, IterationID: in.IterationID,
		Summary: in.Summary, Axes: in.Axes}
	if err := tx.QueryRow(`SELECT image_ref, image_version, image_digest FROM iterations WHERE id = ?`, assessorIteration).
		Scan(&rec.AssessorImageRef, &rec.AssessorImageVersion, &rec.AssessorImageDigest); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return rec, err
	}
	if in.IterationID != "" {
		err := tx.QueryRow(`SELECT agent, image_ref, image_version, image_digest FROM iterations WHERE id = ?`, in.IterationID).
			Scan(&rec.SubjectAgent, &rec.ImageRef, &rec.ImageVersion, &rec.ImageDigest)
		if errors.Is(err, sql.ErrNoRows) {
			return rec, errInvalid("iteration %q not found", in.IterationID)
		}
		if err != nil {
			return rec, err
		}
	}
	if in.TaskKey == "" {
		return rec, nil
	}
	var taskID int64
	err := tx.QueryRow(`SELECT t.id, t.task_key, COALESCE(t.workflow_digest, ''), COALESCE(wi.version, '')
		FROM tasks t LEFT JOIN task_workflow_images wi ON wi.digest = t.workflow_digest
		WHERE t.task_key = ? OR t.id = (SELECT task_id FROM task_key_aliases WHERE old_key = ?)
		ORDER BY t.task_key = ? DESC LIMIT 1`, in.TaskKey, in.TaskKey, in.TaskKey).
		Scan(&taskID, &rec.TaskKey, &rec.WorkflowDigest, &rec.WorkflowVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return rec, errInvalid("task %q not found", in.TaskKey)
	}
	if err != nil {
		return rec, err
	}
	if in.IterationID == "" {
		return rec, nil
	}
	// The iteration and the task are linked when the iteration's agent holds
	// the task or wrote an event on it: a workflow task leaves the agent when
	// its status does.
	principal := "agent:" + rec.SubjectAgent
	var linked bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM tasks WHERE id = ? AND assignee = ?)
		OR EXISTS(SELECT 1 FROM task_events WHERE task_id = ? AND actor = ?)`, taskID, principal, taskID, principal).
		Scan(&linked); err != nil {
		return rec, err
	}
	if !linked {
		return rec, errInvalid("iteration %s of agent %s is not linked to task %s: the agent neither holds the task nor wrote an event on it",
			in.IterationID, rec.SubjectAgent, rec.TaskKey)
	}
	return rec, nil
}

// assessmentID is the public handle as-<UTC yyyymmddThhmmssZ>-<seq, 6 digits>.
func assessmentID(now time.Time, seq int64) string {
	return fmt.Sprintf("as-%s-%06d", now.UTC().Format("20060102T150405Z"), seq)
}

func insert(tx *sql.Tx, a Assessment, now time.Time) (Assessment, error) {
	var seq int64
	if err := tx.QueryRow(`SELECT COALESCE((SELECT seq FROM sqlite_sequence WHERE name = 'assessments'), 0) + 1`).Scan(&seq); err != nil {
		return a, err
	}
	a.ID = assessmentID(now, seq)
	r := a.AssessmentRecord
	if _, err := tx.Exec(`INSERT INTO assessments(seq, id, created_at, assessor_agent, assessor_image_ref,
		assessor_image_version, assessor_image_digest, rubric_version, iteration_id, task_key, subject_agent,
		image_ref, image_version, image_digest, workflow_digest, workflow_version, summary)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		seq, a.ID, r.CreatedAt, r.AssessorAgent, r.AssessorImageRef, r.AssessorImageVersion, r.AssessorImageDigest,
		r.RubricVersion, r.IterationID, r.TaskKey, r.SubjectAgent, r.ImageRef, r.ImageVersion, r.ImageDigest,
		r.WorkflowDigest, r.WorkflowVersion, r.Summary); err != nil {
		return a, err
	}
	for _, axis := range r.Axes {
		if _, err := tx.Exec(`INSERT INTO assessment_axes(assessment_seq, axis, verdict, score, note) VALUES (?,?,?,?,?)`,
			seq, axis.Axis, axis.Verdict, axis.Score, axis.Note); err != nil {
			return a, err
		}
	}
	for _, v := range a.Violations {
		e := v.Evidence
		if _, err := tx.Exec(`INSERT INTO assessment_violations(assessment_seq, axis, description, quote, evidence_kind,
			evidence_iteration_id, evidence_locator, evidence_task_key, evidence_task_event_seq, evidence_script_run_id)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			seq, v.Axis, v.Description, v.Quote, e.Kind, e.IterationID, e.Locator, e.TaskKey, e.Sequence, e.RunID); err != nil {
			return a, err
		}
	}
	a.Axes = sortedAxes(a.Axes)
	return a, nil
}

// sortedAxes orders axes as Axes lists them.
func sortedAxes(axes []AssessmentAxis) []AssessmentAxis {
	slices.SortStableFunc(axes, func(x, y AssessmentAxis) int {
		return slices.Index(Axes, x.Axis) - slices.Index(Axes, y.Axis)
	})
	return axes
}

// toolBody is the JSON body of an agent tool action. Identity never comes
// from the body: the assessor is the calling agent.
type toolBody struct {
	AssessmentInput
	ID           string `json:"id"`
	Iteration    string `json:"iteration"`
	Task         string `json:"task"`
	Agent        string `json:"agent"`
	Assessor     string `json:"assessor"`
	Image        string `json:"image"`
	ImageVersion string `json:"image_version"`
	Limit        int    `json:"limit"`
	History      bool   `json:"history"`
}

// AgentAction runs one /tools/assessment/<action> call of agent: add (as the
// assessor, in iterationID), ls and show.
func (s *Service) AgentAction(agent, iterationID, action string, body map[string]any) (any, error) {
	var in toolBody
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, errInvalid("assessment %s: %v", action, err)
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, errInvalid("assessment %s: %v", action, err)
	}
	switch action {
	case "add":
		return s.Add(agent, iterationID, in.AssessmentInput)
	case "ls":
		return s.List(AssessmentFilter{Iteration: in.Iteration, Task: in.Task, Agent: in.Agent, Assessor: in.Assessor,
			Image: in.Image, ImageVersion: in.ImageVersion, Limit: in.Limit, History: in.History})
	case "show":
		return s.Get(in.ID)
	}
	return nil, &Error{Status: http.StatusNotFound, Code: "not_found", Msg: fmt.Sprintf("unknown assessment action %q", action)}
}
