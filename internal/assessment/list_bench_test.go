package assessment

import (
	"fmt"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// seedReassessed writes n assessments, n/10 tasks DEV-N with ten each: two
// of each of four iterations and two of the task itself, so half of them are
// replaced. internal/metrics has the same seed for its benchmark.
func seedReassessed(b testing.TB, st *store.Store, n int) {
	b.Helper()
	tx, err := st.DB.Begin()
	if err != nil {
		b.Fatal(err)
	}
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for seq := 1; seq <= n; seq++ {
		task := (seq-1)/10 + 1
		object := (seq - 1) % 5 // 0 is the task, 1-4 its iterations
		iteration := ""
		if object > 0 {
			iteration = fmt.Sprintf("it-%d-%d", task, object)
		}
		if _, err := tx.Exec(`INSERT INTO assessments(seq, id, created_at, assessor_agent, rubric_version, iteration_id, task_key, subject_agent)
			VALUES (?, ?, ?, 'judge', 'iteration-review/1', ?, ?, 'dev')`, seq, fmt.Sprintf("as-%d", seq),
			store.FormatTime(start.Add(time.Duration(seq)*time.Second)), iteration, fmt.Sprintf("DEV-%d", task)); err != nil {
			b.Fatal(err)
		}
		for _, axis := range Axes {
			if _, err := tx.Exec(`INSERT INTO assessment_axes(assessment_seq, axis, verdict, score) VALUES (?, ?, 'scored', ?)`,
				seq, axis, seq%101); err != nil {
				b.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
}

func BenchmarkList10k(b *testing.B) {
	st := storetest.Open(b)
	seedReassessed(b, st, 10000)
	svc := New(st.DB, time.Now)
	for b.Loop() {
		if _, err := svc.List(AssessmentFilter{}); err != nil {
			b.Fatal(err)
		}
	}
}
