package metrics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// BenchmarkReport10kAssessments reads a report over 1000 done tasks with
// 10000 assessments: per task two of each of four iterations and two of the
// task itself, the seed internal/assessment's BenchmarkList10k uses.
func BenchmarkReport10kAssessments(b *testing.B) {
	st := storetest.Open(b)
	tx, err := st.DB.Begin()
	if err != nil {
		b.Fatal(err)
	}
	ex := func(q string, args ...any) {
		if _, err := tx.Exec(q, args...); err != nil {
			b.Fatalf("%s: %v", q, err)
		}
	}
	ex(`INSERT INTO task_queues(prefix, name, created_at, updated_at) VALUES ('DEV', 'Dev', ?, ?)`, ts(1, 0, 0), ts(1, 0, 0))
	const tasks, n = 1000, 10000
	for id := 1; id <= tasks; id++ {
		ex(`INSERT INTO tasks(id, task_key, queue_prefix, title, status, author, customer, created_at, updated_at, started_at, completed_at)
			VALUES (?, ?, 'DEV', 't', 'done', 'user:customer', 'user:customer', ?, ?, ?, ?)`,
			id, fmt.Sprintf("DEV-%d", id), ts(1, 0, 0), ts(1, 0, 0), ts(1, 1, 0), ts(2, 0, 0))
	}
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for seq := 1; seq <= n; seq++ {
		task := (seq-1)/10 + 1
		iteration := ""
		if object := (seq - 1) % 5; object > 0 {
			iteration = fmt.Sprintf("it-%d-%d", task, object)
		}
		ex(`INSERT INTO assessments(seq, id, created_at, assessor_agent, rubric_version, iteration_id, task_key, subject_agent)
			VALUES (?, ?, ?, 'judge', 'iteration-review/1', ?, ?, 'dev')`, seq, fmt.Sprintf("as-%d", seq),
			store.FormatTime(start.Add(time.Duration(seq)*time.Second)), iteration, fmt.Sprintf("DEV-%d", task))
		for _, axis := range []string{"image_process", "role", "task_fit"} {
			ex(`INSERT INTO assessment_axes(assessment_seq, axis, verdict, score) VALUES (?, ?, 'scored', ?)`, seq, axis, seq%101)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	r := New(st.Reader(), func() time.Time { return fixtureNow })
	for b.Loop() {
		if _, err := r.Report(context.Background(), period()); err != nil {
			b.Fatal(err)
		}
	}
}
