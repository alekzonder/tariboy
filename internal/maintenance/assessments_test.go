package maintenance

import (
	"database/sql"
	"testing"
)

// addAssessment inserts assessment id, created daysAgo, with three axes and
// one violation.
func addAssessment(t *testing.T, db *sql.DB, id string, daysAgo int) {
	t.Helper()
	res, err := db.Exec(`INSERT INTO assessments(id, created_at, assessor_agent, rubric_version, iteration_id, summary)
		VALUES (?, ?, 'judge', 'r', 'it-1', 's')`, id, ts(daysAgo))
	if err != nil {
		t.Fatal(err)
	}
	seq, _ := res.LastInsertId()
	exec(t, db, `INSERT INTO assessment_axes(assessment_seq, axis, verdict, score)
		VALUES (?, 'image_process', 'scored', 1), (?, 'role', 'not_applicable', NULL), (?, 'task_fit', 'scored', 2)`, seq, seq, seq)
	exec(t, db, `INSERT INTO assessment_violations(assessment_seq, axis, description, quote, evidence_kind, evidence_script_run_id)
		VALUES (?, 'role', 'd', 'q', 'script_run', 'srun-1')`, seq)
}

func TestCleanupDeletesOldAssessmentsWithTheirRows(t *testing.T) {
	svc, st, _ := open(t)
	addAssessment(t, st.DB, "as-old", 100)
	addAssessment(t, st.DB, "as-new", 1)
	res, err := svc.Run()
	if err != nil || res.Error != "" {
		t.Fatalf("run: %+v %v", res, err)
	}
	if res.Deleted["assessments"] != 1 {
		t.Errorf("deleted assessments = %d, want 1", res.Deleted["assessments"])
	}
	if n := count(t, st.DB, `SELECT COUNT(*) FROM assessments WHERE id = 'as-new'`); n != 1 || count(t, st.DB, `SELECT COUNT(*) FROM assessments`) != 1 {
		t.Errorf("assessments left: want only as-new")
	}
	if n := count(t, st.DB, `SELECT COUNT(*) FROM assessment_axes`); n != 3 {
		t.Errorf("axes = %d, want the 3 of as-new", n)
	}
	if n := count(t, st.DB, `SELECT COUNT(*) FROM assessment_violations`); n != 1 {
		t.Errorf("violations = %d, want the 1 of as-new", n)
	}
}

func TestRetentionZeroKeepsAssessments(t *testing.T) {
	svc, st, _ := open(t)
	if err := svc.SetSettings(Settings{Enabled: true, Time: "03:00", KeepBackups: 7, RetentionDays: 0}); err != nil {
		t.Fatal(err)
	}
	addAssessment(t, st.DB, "as-old", 1000)
	res, err := svc.Run()
	if err != nil {
		t.Fatal(err)
	}
	if n := count(t, st.DB, `SELECT COUNT(*) FROM assessments`); n != 1 || res.Deleted["assessments"] != 0 {
		t.Fatalf("assessments = %d, deleted %d; want everything kept", n, res.Deleted["assessments"])
	}
}
