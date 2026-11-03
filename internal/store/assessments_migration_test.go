package store

import "testing"

// TestAssessmentsCascadeOnlyWithinAnAssessment checks the assessment tables:
// deleting an assessment removes its axes and violations, and an assessment
// references no iteration, task or agent, so it outlives them.
func TestAssessmentsCascadeOnlyWithinAnAssessment(t *testing.T) {
	t.Parallel()
	s := open(t)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.DB.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO assessments(id, created_at, assessor_agent, rubric_version, iteration_id, task_key, summary)
		VALUES ('as-1', '2026-11-02T08:00:00.000000000Z', 'judge', 'v1', 'it-gone', 'DEV-gone', 's')`)
	exec(`INSERT INTO assessment_axes(assessment_seq, axis, verdict, score, note)
		VALUES (1, 'image_process', 'scored', 80, ''), (1, 'role', 'not_applicable', NULL, ''), (1, 'task_fit', 'scored', 0, '')`)
	exec(`INSERT INTO assessment_violations(assessment_seq, axis, description, quote, evidence_kind, evidence_iteration_id, evidence_locator)
		VALUES (1, 'role', 'd', 'q', 'transcript', 'it-gone', 'line 3')`)
	for _, bad := range []string{
		`INSERT INTO assessment_axes(assessment_seq, axis, verdict, score) VALUES (1, 'other', 'scored', 1)`,
		`INSERT INTO assessment_axes(assessment_seq, axis, verdict, score) VALUES (1, 'role', 'scored', 1)`,
		`INSERT INTO assessment_violations(assessment_seq, axis, description, quote, evidence_kind) VALUES (1, 'role', 'd', 'q', 'mail')`,
	} {
		if _, err := s.DB.Exec(bad); err == nil {
			t.Fatalf("schema accepted %s", bad)
		}
	}
	exec(`DELETE FROM assessments WHERE id = 'as-1'`)
	for _, table := range []string{"assessment_axes", "assessment_violations"} {
		var n int
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s kept %d rows after its assessment was deleted", table, n)
		}
	}
	for _, table := range []string{"assessments", "assessment_axes", "assessment_violations"} {
		rows, err := s.DB.Query(`SELECT "table" FROM pragma_foreign_key_list(?)`, table)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var parent string
			if err := rows.Scan(&parent); err != nil {
				t.Fatal(err)
			}
			if parent != "assessments" {
				t.Errorf("%s references %s; an assessment must outlive iterations, tasks and agents", table, parent)
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestAssessmentsLatestIndexes checks the indexes that the latest-record
// subquery (assessment.IsLatest) searches by range on created_at. They
// replace idx_assessments_iteration and idx_assessments_task, their prefixes.
func TestAssessmentsLatestIndexes(t *testing.T) {
	t.Parallel()
	s := open(t)
	for name, want := range map[string]string{
		"idx_assessments_iteration_created":      "iteration_id,created_at",
		"idx_assessments_task_iteration_created": "task_key,iteration_id,created_at",
		"idx_assessments_iteration":              "",
		"idx_assessments_task":                   "",
	} {
		var got string
		err := s.DB.QueryRow(`SELECT COALESCE(group_concat(name, ','), '') FROM
			(SELECT name FROM pragma_index_info(?) ORDER BY seqno)`, name).Scan(&got)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("index %s has columns %q, want %q", name, got, want)
		}
	}
}
