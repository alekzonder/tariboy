package assessment

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const recordColumns = `seq, id, created_at, assessor_agent, assessor_image_ref, assessor_image_version,
	assessor_image_digest, rubric_version, iteration_id, task_key, subject_agent, image_ref, image_version,
	image_digest, workflow_digest, workflow_version, summary`

type scanner interface{ Scan(dest ...any) error }

func scanRecord(row scanner, extra ...any) (int64, AssessmentRecord, error) {
	var seq int64
	var r AssessmentRecord
	dest := append([]any{&seq, &r.ID, &r.CreatedAt, &r.AssessorAgent, &r.AssessorImageRef, &r.AssessorImageVersion,
		&r.AssessorImageDigest, &r.RubricVersion, &r.IterationID, &r.TaskKey, &r.SubjectAgent, &r.ImageRef,
		&r.ImageVersion, &r.ImageDigest, &r.WorkflowDigest, &r.WorkflowVersion, &r.Summary}, extra...)
	err := row.Scan(dest...)
	return seq, r, err
}

// Get returns one whole assessment.
func (s *Service) Get(id string) (AssessmentShowResult, error) {
	seq, rec, err := scanRecord(s.db.QueryRow(`SELECT `+recordColumns+` FROM assessments WHERE id = ?`, strings.TrimSpace(id)))
	if errors.Is(err, sql.ErrNoRows) {
		return AssessmentShowResult{}, &Error{Status: http.StatusNotFound, Code: "not_found", Msg: fmt.Sprintf("assessment %q not found", id)}
	}
	if err != nil {
		return AssessmentShowResult{}, err
	}
	axes, err := s.axes([]int64{seq})
	if err != nil {
		return AssessmentShowResult{}, err
	}
	rec.Axes = axes[seq]
	violations, err := s.violations(seq)
	if err != nil {
		return AssessmentShowResult{}, err
	}
	return AssessmentShowResult{Assessment: Assessment{AssessmentRecord: rec, Violations: violations}}, nil
}

// List returns the assessments that match f, newest first.
func (s *Service) List(f AssessmentFilter) (AssessmentList, error) {
	limit := f.Limit
	switch {
	case limit < 0 || limit > maxLimit:
		return AssessmentList{}, errInvalid("limit must be from 1 to %d", maxLimit)
	case limit == 0:
		limit = defaultLimit
	}
	var where []string
	var args []any
	for _, c := range []struct{ column, value string }{
		{"iteration_id", f.Iteration}, {"task_key", f.Task}, {"subject_agent", f.Agent},
		{"assessor_agent", f.Assessor}, {"image_ref", f.Image}, {"image_version", f.ImageVersion},
	} {
		if v := strings.TrimSpace(c.value); v != "" {
			where = append(where, c.column+` = ?`)
			args = append(args, v)
		}
	}
	q := `SELECT ` + recordColumns + `, (SELECT COUNT(*) FROM assessment_violations v WHERE v.assessment_seq = a.seq)
		FROM assessments a`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	rows, err := s.db.Query(q+` ORDER BY created_at DESC, seq DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return AssessmentList{}, err
	}
	defer rows.Close()
	out := AssessmentList{Assessments: []AssessmentSummary{}}
	var seqs []int64
	for rows.Next() {
		var count int
		seq, rec, err := scanRecord(rows, &count)
		if err != nil {
			return AssessmentList{}, err
		}
		seqs = append(seqs, seq)
		out.Assessments = append(out.Assessments, AssessmentSummary{AssessmentRecord: rec, ViolationCount: count})
	}
	if err := rows.Err(); err != nil {
		return AssessmentList{}, err
	}
	axes, err := s.axes(seqs)
	if err != nil {
		return AssessmentList{}, err
	}
	for i, seq := range seqs {
		out.Assessments[i].Axes = axes[seq]
	}
	return out, nil
}

// axes returns the axes of each assessment seq in the order Axes lists them.
func (s *Service) axes(seqs []int64) (map[int64][]AssessmentAxis, error) {
	out := map[int64][]AssessmentAxis{}
	if len(seqs) == 0 {
		return out, nil
	}
	args := make([]any, len(seqs))
	for i, seq := range seqs {
		args[i] = seq
	}
	rows, err := s.db.Query(`SELECT assessment_seq, axis, verdict, score, note FROM assessment_axes
		WHERE assessment_seq IN (`+strings.TrimSuffix(strings.Repeat("?,", len(seqs)), ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var seq int64
		var a AssessmentAxis
		var score sql.NullInt64
		if err := rows.Scan(&seq, &a.Axis, &a.Verdict, &score, &a.Note); err != nil {
			return nil, err
		}
		if score.Valid {
			n := int(score.Int64)
			a.Score = &n
		}
		out[seq] = append(out[seq], a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for seq := range out {
		out[seq] = sortedAxes(out[seq])
	}
	return out, nil
}

func (s *Service) violations(seq int64) ([]AssessmentViolation, error) {
	rows, err := s.db.Query(`SELECT axis, description, quote, evidence_kind, evidence_iteration_id, evidence_locator,
		evidence_task_key, evidence_task_event_seq, evidence_script_run_id
		FROM assessment_violations WHERE assessment_seq = ? ORDER BY id`, seq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AssessmentViolation{}
	for rows.Next() {
		var v AssessmentViolation
		e := &v.Evidence
		if err := rows.Scan(&v.Axis, &v.Description, &v.Quote, &e.Kind, &e.IterationID, &e.Locator,
			&e.TaskKey, &e.Sequence, &e.RunID); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
