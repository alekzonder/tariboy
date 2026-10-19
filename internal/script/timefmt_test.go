package script

import (
	"testing"
	"time"

	basestore "github.com/alekzonder/tariboy/internal/store"
)

// A recurring script is due exactly at next_run_at, compared by time: 10ns is
// before an 11ns next run in the same second although RFC3339Nano writes 10ns
// as the longer-sorting ".00000001Z".
func TestDueDefinitionsComparesSameSecondByTime(t *testing.T) {
	base := time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)
	st := newContractStore(t, base)
	definition, run, err := st.CreateSchedule("alice", CreateSchedule{Name: "watch", Description: "watch", Command: "false", IntervalSeconds: 30})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := st.ClaimRun("alice", run.ID, basestore.FormatTime(base), "/tmp/watch.log"); err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	finished := base.Add(11 - 30*time.Second)
	if _, err := st.CompleteRun("alice", run.ID, Completion{Status: RunFailed, ExitCode: intPtr(QuietExit),
		FinishedAt: finished.Format(time.RFC3339Nano), LogPath: "/tmp/watch.log"}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetDefinition("alice", definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := basestore.FormatTime(base.Add(11)); got.NextRunAt != want {
		t.Fatalf("next_run_at = %q, want %q", got.NextRunAt, want)
	}
	due, err := st.DueDefinitions(base.Add(10).Format(time.RFC3339Nano))
	if err != nil || len(due) != 0 {
		t.Fatalf("due at 10ns = %v err=%v, want none", due, err)
	}
	due, err = st.DueDefinitions(base.Add(11).In(time.FixedZone("x", 3600)).Format(time.RFC3339Nano))
	if err != nil || len(due) != 1 {
		t.Fatalf("due at 11ns = %v err=%v, want one", due, err)
	}
}

// Every time the script store writes is in the stored format, including the
// values callers pass in with another width or offset.
func TestScriptStoreWritesCanonicalTimes(t *testing.T) {
	now := time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)
	st := newContractStore(t, now)
	_, run, err := st.CreateOnce("alice", CreateOnce{Name: "check", Description: "checks repo", Command: "true"})
	if err != nil {
		t.Fatal(err)
	}
	offset := time.FixedZone("x", 3600)
	if claimed, err := st.ClaimRun("alice", run.ID, now.In(offset).Format(time.RFC3339), "/tmp/check.log"); err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	completed, err := st.CompleteRun("alice", run.ID, Completion{Status: RunSucceeded, ExitCode: intPtr(0), LogPath: "/tmp/check.log"})
	if err != nil {
		t.Fatal(err)
	}
	if completed.FinishedAt != basestore.FormatTime(now) {
		t.Fatalf("finished_at = %q", completed.FinishedAt)
	}
	_, cancelled, err := st.CreateOnce("alice", CreateOnce{Name: "other", Description: "other", Command: "true"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CancelRun("alice", cancelled.ID, now.In(offset).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`SELECT created_at FROM scripts`,
		`SELECT created_at FROM script_runs`,
		`SELECT started_at FROM script_runs WHERE started_at IS NOT NULL AND started_at<>''`,
		`SELECT finished_at FROM script_runs WHERE finished_at IS NOT NULL AND finished_at<>''`,
		`SELECT next_attempt_at FROM script_result_outbox`,
	} {
		rows, err := st.db.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				t.Fatal(err)
			}
			if _, err := basestore.ParseTime(v); err != nil {
				t.Errorf("%s: %v", q, err)
			}
			n++
		}
		rows.Close()
		if n == 0 {
			t.Errorf("%s: no rows", q)
		}
	}
}
