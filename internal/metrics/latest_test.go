package metrics

import (
	"context"
	"testing"

	"github.com/alekzonder/tariboy/internal/store"
)

type dev2Assessment struct {
	seq                   int
	iteration, rubric, at string
	score                 int
}

// fixtureAssessingDEV2 is the fixture with assessments of DEV-2, which has
// none there: of an iteration, or of the task when iteration is empty, with
// one score on every axis.
func fixtureAssessingDEV2(t *testing.T, as ...dev2Assessment) *Reader {
	return newFixture(t, func(st *store.Store) {
		for _, a := range as {
			exec(t, st, `INSERT INTO assessments(seq, id, created_at, task_key, iteration_id, rubric_version)
				VALUES (?, ?, ?, 'DEV-2', ?, ?)`, a.seq, "as-dev2-"+string(rune('a'+a.seq)), a.at, a.iteration, a.rubric)
			for _, axis := range []string{"image_process", "role", "task_fit"} {
				exec(t, st, `INSERT INTO assessment_axes(assessment_seq, axis, verdict, score) VALUES (?, ?, 'scored', ?)`,
					a.seq, axis, a.score)
			}
		}
	})
}

// wantDEV2Quality checks DEV-2's quality in its task metrics and in the
// report slice of tasks without a workflow, where DEV-2 is the only task.
func wantDEV2Quality(t *testing.T, r *Reader, want MetricsAxisScore) {
	t.Helper()
	m, err := r.Task(context.Background(), "DEV-2")
	if err != nil {
		t.Fatal(err)
	}
	q := m.Quality
	if q.ImageProcess != want || q.Role != want || q.TaskFit != want {
		t.Errorf("task quality = %+v; want %+v on every axis", q, want)
	}
	rep, err := r.Report(context.Background(), period())
	if err != nil {
		t.Fatal(err)
	}
	if got := findSlice(t, rep.Workflows, "none", "").Tasks.Quality; got.ImageProcess != want || got.TaskFit != want {
		t.Errorf("report quality = %+v; want %+v", got, want)
	}
}

func TestQualityTakesTheLatestAssessmentOfAnIteration(t *testing.T) {
	t.Parallel()
	r := fixtureAssessingDEV2(t,
		dev2Assessment{10, "it-2a", "iteration-review/1", ts(3, 0, 0), 10},
		dev2Assessment{11, "it-2a", "iteration-review/1", ts(3, 1, 0), 40})
	wantDEV2Quality(t, r, MetricsAxisScore{Avg: 40, Min: 40, Count: 1})
}

func TestQualityTakesTheLatestAssessmentOfATask(t *testing.T) {
	t.Parallel()
	r := fixtureAssessingDEV2(t,
		dev2Assessment{10, "", "task-review/1", ts(3, 0, 0), 10},
		dev2Assessment{11, "", "task-review/1", ts(3, 1, 0), 40})
	wantDEV2Quality(t, r, MetricsAxisScore{Avg: 40, Min: 40, Count: 1})
}

func TestQualityAtEqualTimeTakesTheLatestRubricVersion(t *testing.T) {
	t.Parallel()
	// The winner has the lower seq, so seq alone would pick the loser.
	r := fixtureAssessingDEV2(t,
		dev2Assessment{10, "it-2a", "iteration-review/10", ts(3, 0, 0), 30},
		dev2Assessment{11, "it-2a", "iteration-review/2", ts(3, 0, 0), 90})
	wantDEV2Quality(t, r, MetricsAxisScore{Avg: 30, Min: 30, Count: 1})
}
