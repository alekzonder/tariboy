package agent

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return NewStore(s)
}

func TestSetPendingImageIfEmptyPreservesExistingAssignment(t *testing.T) {
	st := openStore(t)
	if err := st.Create(sampleAgent()); err != nil {
		t.Fatal(err)
	}
	if won, err := st.SetPendingImageIfEmpty("smoke", "first:latest", "first-digest"); err != nil || !won {
		t.Fatalf("first pending assignment won=%t err=%v", won, err)
	}
	if won, err := st.SetPendingImageIfEmpty("smoke", "second:latest", "second-digest"); err != nil || won {
		t.Fatalf("second pending assignment won=%t err=%v", won, err)
	}
	pending, err := st.PendingImage("smoke")
	if err != nil || pending.Ref != "first:latest" || pending.Digest != "first-digest" {
		t.Fatalf("pending assignment=%+v err=%v", pending, err)
	}
}

func TestSetPendingImageErrorIfEmptyKeepsExplicitAssignment(t *testing.T) {
	st := openStore(t)
	if err := st.Create(sampleAgent()); err != nil {
		t.Fatal(err)
	}
	if recorded, err := st.SetPendingImageErrorIfEmpty("smoke", "image discovery failed"); err != nil || !recorded {
		t.Fatalf("empty pending error recorded=%t err=%v", recorded, err)
	}
	pending, err := st.PendingImage("smoke")
	if err != nil || pending.Ref != "" || pending.Digest != "" || pending.Error != "image discovery failed" {
		t.Fatalf("empty pending error=%+v err=%v", pending, err)
	}
	if cleared, err := st.ClearPendingImageErrorIfEmpty("smoke"); err != nil || !cleared {
		t.Fatalf("empty pending error cleared=%t err=%v", cleared, err)
	}
	pending, err = st.PendingImage("smoke")
	if err != nil || pending.Ref != "" || pending.Digest != "" || pending.Error != "" {
		t.Fatalf("cleared empty pending=%+v err=%v", pending, err)
	}
	if err := st.SetPendingImage("smoke", "explicit:latest", "explicit-digest"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetPendingImageError("smoke", "explicit failure"); err != nil {
		t.Fatal(err)
	}
	if cleared, err := st.ClearPendingImageErrorIfEmpty("smoke"); err != nil || cleared {
		t.Fatalf("explicit pending error cleared=%t err=%v", cleared, err)
	}
	pending, err = st.PendingImage("smoke")
	if err != nil || pending.Ref != "explicit:latest" || pending.Digest != "explicit-digest" || pending.Error != "explicit failure" {
		t.Fatalf("explicit pending=%+v err=%v", pending, err)
	}
}

func TestPurgeAgentDataRollsBackAllRowsWhenADeleteFails(t *testing.T) {
	st := openStore(t)
	const name = "purge-me"
	if _, err := st.db.Exec(`INSERT INTO scripts(id,agent,name,description,command,mode,state,created_at) VALUES(?,?,?,?,?,?,?,?)`,
		"scr-purge", name, "purge", "test", "true", "once", "completed", "2026-08-20T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`INSERT INTO script_result_outbox(idempotency_key,script_id,run_id,agent,payload,next_attempt_at) VALUES(?,?,?,?,?,?)`,
		"script-result:srun-purge", "scr-purge", "srun-purge", name, `{}`, "2026-08-20T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`CREATE TRIGGER reject_script_purge BEFORE DELETE ON scripts BEGIN SELECT RAISE(ABORT, 'reject purge'); END`); err != nil {
		t.Fatal(err)
	}

	if err := st.PurgeAgentData(name); err == nil {
		t.Fatal("PurgeAgentData succeeded despite rejecting script deletion")
	}
	for _, table := range []string{"scripts", "script_result_outbox"} {
		var count int
		if err := st.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE agent=?`, name).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s rows after rollback = %d, want 1", table, count)
		}
	}
}

func TestIterationTimeoutSnapshotAndExtensions(t *testing.T) {
	st := openStore(t)
	if err := st.Create(sampleAgent()); err != nil {
		t.Fatal(err)
	}
	// Rows from before the migration remain readable and are deliberately not
	// backfilled into an invented timeout configuration.
	if err := st.CreateIteration(Iteration{ID: "old", Agent: "smoke", Status: "done"}); err != nil {
		t.Fatal(err)
	}
	old, err := st.GetIteration("smoke", "old")
	if err != nil || old.TimeoutPeriodS != nil || old.TimeoutDeadline != nil || old.HardTimeoutDeadline != nil {
		t.Fatalf("old iteration timeout compatibility: %+v err=%v", old, err)
	}
	if _, err := st.ExtendIterationTimeout("smoke", "old", time.Now()); !errors.Is(err, ErrTimeoutNotExtendable) {
		t.Fatalf("extend old row = %v, want ErrTimeoutNotExtendable", err)
	}

	now := time.Date(2026, 7, 14, 10, 0, 0, 123456789, time.UTC)
	if err := st.CreateIteration(Iteration{ID: "live", Agent: "smoke", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := st.InitializeIterationTimeout("live", 30, 90, now); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetIteration("smoke", "live")
	if err != nil {
		t.Fatal(err)
	}
	if got.TimeoutPeriodS == nil || *got.TimeoutPeriodS != 30 || got.TimeoutDeadline == nil || got.HardTimeoutDeadline == nil ||
		*got.TimeoutDeadline != now.Add(30*time.Second).Format(time.RFC3339Nano) ||
		*got.HardTimeoutDeadline != now.Add(90*time.Second).Format(time.RFC3339Nano) {
		t.Fatalf("snapshot = %+v", got)
	}

	// Agent settings are intentionally irrelevant after initialization: every
	// extension adds the persisted 30-second period.
	for range 2 {
		got, err = st.ExtendIterationTimeout("smoke", "live", now.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}
	if got.TimeoutExtensions != 2 || *got.TimeoutDeadline != now.Add(90*time.Second).Format(time.RFC3339Nano) ||
		*got.HardTimeoutDeadline != now.Add(150*time.Second).Format(time.RFC3339Nano) {
		t.Fatalf("repeated extension = %+v", got)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := st.ExtendIterationTimeout("smoke", "live", now.Add(time.Second))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent extension: %v", err)
		}
	}
	got, err = st.GetIteration("smoke", "live")
	if err != nil {
		t.Fatal(err)
	}
	if got.TimeoutExtensions != 10 || *got.TimeoutDeadline != now.Add(330*time.Second).Format(time.RFC3339Nano) ||
		*got.HardTimeoutDeadline != now.Add(390*time.Second).Format(time.RFC3339Nano) {
		t.Fatalf("concurrent extensions lost an update: %+v", got)
	}
	if _, err := st.ExtendIterationTimeout("smoke", "live", now.Add(331*time.Second)); !errors.Is(err, ErrTimeoutNotExtendable) {
		t.Fatalf("expired extension = %v, want ErrTimeoutNotExtendable", err)
	}
	if ok, err := st.MarkIterationTimeoutTriggered("smoke", "live", *got.TimeoutDeadline, now.Add(330*time.Second).Format(time.RFC3339Nano)); !ok || err != nil {
		t.Fatalf("mark timeout = ok:%v err:%v", ok, err)
	}
	if _, err := st.ExtendIterationTimeout("smoke", "live", now.Add(time.Second)); !errors.Is(err, ErrTimeoutNotExtendable) {
		t.Fatalf("triggered extension = %v, want ErrTimeoutNotExtendable", err)
	}
	if err := st.UpdateIteration(Iteration{ID: "live", Status: "done"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ExtendIterationTimeout("smoke", "live", now.Add(time.Second)); !errors.Is(err, ErrTimeoutNotExtendable) {
		t.Fatalf("terminal extension = %v, want ErrTimeoutNotExtendable", err)
	}
}

func TestZeroIterationTimeoutCannotExtend(t *testing.T) {
	st := openStore(t)
	if err := st.Create(sampleAgent()); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateIteration(Iteration{ID: "zero", Agent: "smoke", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	if err := st.InitializeIterationTimeout("zero", 0, 120, now); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetIteration("smoke", "zero")
	if err != nil {
		t.Fatal(err)
	}
	if got.TimeoutPeriodS == nil || *got.TimeoutPeriodS != 0 || got.TimeoutDeadline != nil || got.HardTimeoutDeadline == nil {
		t.Fatalf("zero timeout snapshot = %+v", got)
	}
	if _, err := st.ExtendIterationTimeout("smoke", "zero", now); !errors.Is(err, ErrNoIterationTimeout) {
		t.Fatalf("zero timeout extension = %v, want ErrNoIterationTimeout", err)
	}
}

func TestTimeoutMarkerDoesNotBeatCommittedExtension(t *testing.T) {
	st := openStore(t)
	if err := st.Create(sampleAgent()); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	if err := st.CreateIteration(Iteration{ID: "live", Agent: "smoke", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := st.InitializeIterationTimeout("live", 30, 90, now); err != nil {
		t.Fatal(err)
	}
	// The timeout observer read the original 30-second deadline before the
	// extension committed, then reaches the durable marker afterward.
	snapshot, err := st.GetIteration("smoke", "live")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ExtendIterationTimeout("smoke", "live", now.Add(29*time.Second)); err != nil {
		t.Fatal(err)
	}
	marked, err := st.MarkIterationTimeoutTriggered("smoke", "live", *snapshot.TimeoutDeadline, now.Add(30*time.Second).Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	if marked {
		t.Fatal("stale timeout observation marked an iteration after its extension committed")
	}
	it, err := st.GetIteration("smoke", "live")
	if err != nil {
		t.Fatal(err)
	}
	if it.TimeoutTriggeredAt != nil {
		t.Fatalf("stale observation persisted timeout marker: %+v", it)
	}
}

func sampleAgent() Agent {
	return Agent{
		Name: "smoke", ImageRef: "basic:latest", ImageDigest: "deadbeef",
		Cwd: "/w", HarnessType: "stub", Model: "sonnet", Effort: "medium",
		Interactive: false, LoopEnabled: true, IntervalS: 60, TimeoutS: 30,
		HardTimeoutS: 0, OnTimeout: "restart", OnError: "restart", UserPrompt: "be good",
		Env: map[string]string{"APP_ENV": "prod"}, Plugins: []string{"whoami", "loop", "messages", "context"},
		MessagesBatch: 10, MessagesMaxQueue: 1000,
	}
}

func TestGoalDefaultsAndDisableClearsSelection(t *testing.T) {
	st := openStore(t)
	if err := st.Create(Agent{Name: "worker"}); err != nil {
		t.Fatal(err)
	}
	ag, err := st.Get("worker")
	if err != nil {
		t.Fatal(err)
	}
	if !ag.GoalEnabled || ag.GoalWaitCustomerTimeoutS != 300 || ag.CurrentGoalTaskKey != "" {
		t.Fatalf("goal defaults: %#v", ag)
	}
	if err := st.SetCurrentGoal("worker", "TARI-43"); err != nil {
		t.Fatal(err)
	}
	ag.GoalEnabled = false
	if err := st.Update(ag); err != nil {
		t.Fatal(err)
	}
	ag, err = st.Get("worker")
	if err != nil {
		t.Fatal(err)
	}
	if ag.CurrentGoalTaskKey != "" {
		t.Fatalf("stale goal %q", ag.CurrentGoalTaskKey)
	}
}

func TestGoalTimeoutValidationPreservesStoredValue(t *testing.T) {
	st := openStore(t)
	if err := st.Create(Agent{Name: "worker"}); err != nil {
		t.Fatal(err)
	}
	for _, timeout := range []int{0, -1} {
		ag, err := st.Get("worker")
		if err != nil {
			t.Fatal(err)
		}
		ag.GoalWaitCustomerTimeoutS = timeout
		if err := st.Update(ag); err == nil || err.Error() != "invalid_goal_wait_customer_timeout" {
			t.Fatalf("Update timeout %d error = %v", timeout, err)
		}
		ag, err = st.Get("worker")
		if err != nil {
			t.Fatal(err)
		}
		if ag.GoalWaitCustomerTimeoutS != 300 {
			t.Fatalf("timeout after rejected %d = %d, want 300", timeout, ag.GoalWaitCustomerTimeoutS)
		}
	}
}

// Catches the single-row create insert omitting the idle-stop threshold while
// later update/read paths continue to support it.
func TestCreatePersistsMaximumIdleIterations(t *testing.T) {
	st := openStore(t)
	agent := sampleAgent()
	agent.MaxIdleIterations = 9
	if err := st.Create(agent); err != nil {
		t.Fatal(err)
	}
	got, err := st.Get(agent.Name)
	if err != nil {
		t.Fatal(err)
	}
	if got.MaxIdleIterations != 9 {
		t.Fatalf("MaxIdleIterations = %d, want 9", got.MaxIdleIterations)
	}
}

func TestMigrationAddsEnabledDefaultingToLoopEnabled(t *testing.T) {
	st := openStore(t)
	// One agent with loop on, one with loop off.
	mustCreate := func(name string, loop bool) {
		if err := st.Create(Agent{Name: name, ImageRef: "x:1", HarnessType: "stub", LoopEnabled: loop, Enabled: loop}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	mustCreate("on", true)
	mustCreate("off", false)
	// Column exists and round-trips.
	on, _ := st.Get("on")
	off, _ := st.Get("off")
	if !on.Enabled {
		t.Fatalf("on.Enabled = false, want true")
	}
	if off.Enabled {
		t.Fatalf("off.Enabled = true, want false")
	}
}

func TestCreateRejectsInvalidName(t *testing.T) {
	st := openStore(t)
	a := sampleAgent()
	a.Name = "../../evil"
	if err := st.Create(a); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Create(%q) = %v, want ErrInvalidName", a.Name, err)
	}
	// Nothing was inserted.
	if list, err := st.List(); err != nil || len(list) != 0 {
		t.Fatalf("after rejected Create: list=%v err=%v, want empty", list, err)
	}
}

func TestAgentCRUD(t *testing.T) {
	st := openStore(t)
	if _, err := st.Get("smoke"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get on empty = %v, want ErrNotFound", err)
	}
	if err := st.Create(sampleAgent()); err != nil {
		t.Fatal(err)
	}
	if err := st.Create(sampleAgent()); err == nil {
		t.Fatal("duplicate agent name accepted")
	}
	got, err := st.Get("smoke")
	if err != nil {
		t.Fatal(err)
	}
	if got.ImageRef != "basic:latest" || got.Env["APP_ENV"] != "prod" ||
		len(got.Plugins) != 4 || got.LoopEnabled != true || got.IntervalS != 60 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	// message batching defaults are applied on create.
	if got.MessagesBatch != 10 || got.MessagesMaxQueue != 1000 {
		t.Fatalf("message defaults not applied: batch=%d queue=%d", got.MessagesBatch, got.MessagesMaxQueue)
	}
	got.UserPrompt = "changed"
	got.IntervalS = 120
	got.LoopEnabled = false
	if err := st.Update(got); err != nil {
		t.Fatal(err)
	}
	re, _ := st.Get("smoke")
	if re.UserPrompt != "changed" || re.IntervalS != 120 || re.LoopEnabled {
		t.Fatalf("update not persisted: %+v", re)
	}
	if err := st.SetError("smoke", "boom"); err != nil {
		t.Fatal(err)
	}
	if re, _ = st.Get("smoke"); re.ErrorReason != "boom" {
		t.Fatalf("error_reason = %q", re.ErrorReason)
	}
	list, err := st.List()
	if err != nil || len(list) != 1 || list[0].Name != "smoke" {
		t.Fatalf("list = %+v err=%v", list, err)
	}
	if err := st.Delete("smoke"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := st.Exists("smoke"); ok {
		t.Fatal("agent still present after delete")
	}
}

func TestIterations(t *testing.T) {
	st := openStore(t)
	if err := st.Create(sampleAgent()); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	id, err := st.NextIterationID("smoke", now)
	if err != nil {
		t.Fatal(err)
	}
	if id != "smoke-20260706100000-1" {
		t.Fatalf("id = %q", id)
	}
	it := Iteration{ID: id, Agent: "smoke", Trigger: "interval", Status: "running",
		StartedAt: now.Format(time.RFC3339), PromptPath: "/p/PROMPT.md"}
	if err := st.CreateIteration(it); err != nil {
		t.Fatal(err)
	}
	// second id in the same second increments the seq
	id2, _ := st.NextIterationID("smoke", now)
	if id2 != "smoke-20260706100000-2" {
		t.Fatalf("id2 = %q", id2)
	}
	if err := st.SetIterationDone(id, true); err != nil {
		t.Fatal(err)
	}
	ec := 0
	cpu := 1234
	mem := 5678
	it.Status = "done"
	it.EndedAt = now.Add(time.Minute).Format(time.RFC3339)
	it.ExitCode, it.CPUMs, it.MemPeakKB = &ec, &cpu, &mem
	if err := st.UpdateIteration(it); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetIteration("smoke", id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "done" || !got.DoneFlag || got.ExitCode == nil || *got.ExitCode != 0 ||
		got.CPUMs == nil || *got.CPUMs != 1234 || got.MemPeakKB == nil || *got.MemPeakKB != 5678 {
		t.Fatalf("iteration round trip: %+v", got)
	}
	if got.PromptPath != "/p/PROMPT.md" {
		t.Fatalf("prompt path round trip: %+v", got)
	}
	// prompt_path is owned exclusively by SetIterationPromptPath.
	if err := st.SetIterationPromptPath(id, "/p/other-PROMPT.md"); err != nil {
		t.Fatal(err)
	}
	got2, err := st.GetIteration("smoke", id)
	if err != nil {
		t.Fatal(err)
	}
	if got2.PromptPath != "/p/other-PROMPT.md" {
		t.Fatalf("prompt path not updated: %+v", got2)
	}
	// Regression: a later UpdateIteration call with an empty PromptPath must
	// not clobber the value set via SetIterationPromptPath (this is exactly
	// what the loop engine does at iteration end).
	it.PromptPath = ""
	it.Status = "harness_error"
	if err := st.UpdateIteration(it); err != nil {
		t.Fatal(err)
	}
	got3, err := st.GetIteration("smoke", id)
	if err != nil {
		t.Fatal(err)
	}
	if got3.PromptPath != "/p/other-PROMPT.md" {
		t.Fatalf("prompt path clobbered by UpdateIteration: %+v", got3)
	}
	if got3.Status != "harness_error" {
		t.Fatalf("status not updated: %+v", got3)
	}
	all, err := st.ListIterations("smoke")
	if err != nil || len(all) != 1 {
		t.Fatalf("list iterations = %+v err=%v", all, err)
	}
	if _, err := st.GetIteration("smoke", "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing iteration = %v", err)
	}
}

func TestSecrets(t *testing.T) {
	st := openStore(t)
	if err := st.Create(sampleAgent()); err != nil {
		t.Fatal(err)
	}
	if err := st.SecretSet("smoke", "JIRA_TOKEN", "s3cr3t"); err != nil {
		t.Fatal(err)
	}
	if err := st.SecretSet("smoke", "JIRA_TOKEN", "rotated"); err != nil { // upsert
		t.Fatal(err)
	}
	keys, err := st.SecretKeys("smoke")
	if err != nil || len(keys) != 1 || keys[0] != "JIRA_TOKEN" {
		t.Fatalf("keys = %v err=%v", keys, err)
	}
	m, _ := st.SecretMap("smoke")
	if m["JIRA_TOKEN"] != "rotated" {
		t.Fatalf("value not rotated: %v", m)
	}
	if err := st.SecretRemove("smoke", "JIRA_TOKEN"); err != nil {
		t.Fatal(err)
	}
	if keys, _ = st.SecretKeys("smoke"); len(keys) != 0 {
		t.Fatalf("secret not removed: %v", keys)
	}
}

func TestSetClearError(t *testing.T) {
	st := openStore(t)
	a := sampleAgent()
	a.Name = "err-agent"
	if err := st.Create(a); err != nil {
		t.Fatal(err)
	}

	if err := st.SetError("err-agent", "boom"); err != nil {
		t.Fatal(err)
	}
	got, err := st.Get("err-agent")
	if err != nil {
		t.Fatal(err)
	}
	if got.ErrorReason != "boom" || got.LoopEnabled {
		t.Fatalf("after SetError: reason=%q loop=%v, want boom/false", got.ErrorReason, got.LoopEnabled)
	}

	if err := st.ClearError("err-agent"); err != nil {
		t.Fatal(err)
	}
	got, err = st.Get("err-agent")
	if err != nil {
		t.Fatal(err)
	}
	if got.ErrorReason != "" {
		t.Fatalf("after ClearError: reason=%q, want empty", got.ErrorReason)
	}
}

func TestSetStatus(t *testing.T) {
	st := openStore(t)
	a := sampleAgent()
	a.Name = "status-agent"
	if err := st.Create(a); err != nil {
		t.Fatal(err)
	}
	// A fresh agent has an empty status message.
	if got, _ := st.Get("status-agent"); got.StatusMessage != "" || got.StatusUpdated != "" {
		t.Fatalf("fresh agent status = %q/%q, want empty", got.StatusMessage, got.StatusUpdated)
	}

	if err := st.SetStatus("status-agent", "reviewing PR diff", "2026-07-09T10:00:00Z"); err != nil {
		t.Fatal(err)
	}
	got, err := st.Get("status-agent")
	if err != nil {
		t.Fatal(err)
	}
	if got.StatusMessage != "reviewing PR diff" || got.StatusUpdated != "2026-07-09T10:00:00Z" {
		t.Fatalf("after SetStatus: %q/%q", got.StatusMessage, got.StatusUpdated)
	}

	// The status message is daemon-owned: an unrelated config Update must not
	// clobber it (mirrors error_reason).
	got.Effort = "high"
	got.StatusMessage = "STALE FROM UPDATE SNAPSHOT"
	if err := st.Update(got); err != nil {
		t.Fatal(err)
	}
	after, err := st.Get("status-agent")
	if err != nil {
		t.Fatal(err)
	}
	if after.StatusMessage != "reviewing PR diff" {
		t.Fatalf("Update clobbered status message: %q", after.StatusMessage)
	}
	if after.Effort != "high" {
		t.Fatalf("Update did not persist unrelated field: effort=%q", after.Effort)
	}
}

// TestIterationProductive exercises the idle-autostop self-declaration column:
// a plain done (i-am-done) writes productive=1, an idle done (i-am-done --idle)
// writes productive=0, and a no_i_am_done iteration (SetIterationDone never
// called) keeps the DEFAULT 1. It also confirms max_idle_iterations defaults to 0.
func TestIterationProductive(t *testing.T) {
	st := openStore(t)
	if err := st.Create(sampleAgent()); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)

	mkIter := func(seq string) string {
		id, err := st.NextIterationID("smoke", now)
		if err != nil {
			t.Fatal(err)
		}
		it := Iteration{ID: id, Agent: "smoke", Trigger: "interval", Status: "running",
			StartedAt: now.Format(time.RFC3339)}
		if err := st.CreateIteration(it); err != nil {
			t.Fatal(err)
		}
		return id
	}

	// plain done => productive true
	plain := mkIter("1")
	if err := st.SetIterationDone(plain, true); err != nil {
		t.Fatal(err)
	}
	if got, err := st.GetIteration("smoke", plain); err != nil {
		t.Fatal(err)
	} else if !got.DoneFlag || !got.Productive {
		t.Fatalf("plain done: DoneFlag=%v Productive=%v, want true/true", got.DoneFlag, got.Productive)
	}

	// idle done => productive false
	idle := mkIter("2")
	if err := st.SetIterationDone(idle, false); err != nil {
		t.Fatal(err)
	}
	if got, err := st.GetIteration("smoke", idle); err != nil {
		t.Fatal(err)
	} else if !got.DoneFlag || got.Productive {
		t.Fatalf("idle done: DoneFlag=%v Productive=%v, want true/false", got.DoneFlag, got.Productive)
	}

	// no_i_am_done (SetIterationDone never called) => default productive true
	stuck := mkIter("3")
	if got, err := st.GetIteration("smoke", stuck); err != nil {
		t.Fatal(err)
	} else if got.DoneFlag || !got.Productive {
		t.Fatalf("no_i_am_done: DoneFlag=%v Productive=%v, want false/true", got.DoneFlag, got.Productive)
	}

	// max_idle_iterations defaults to 0 on a fresh agent.
	if a, err := st.Get("smoke"); err != nil {
		t.Fatal(err)
	} else if a.MaxIdleIterations != 0 {
		t.Fatalf("MaxIdleIterations = %d, want 0", a.MaxIdleIterations)
	}
}

// TestIdleStreak verifies the consecutive-idle count is measured newest-first,
// stops at the first productive iteration, and treats abnormal outcomes
// (which default productive=1) as streak breakers, not idle.
func TestIdleStreak(t *testing.T) {
	st := openStore(t)
	if err := st.Create(sampleAgent()); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)

	// kind: "idle" => done productive=0, "prod" => done productive=1,
	// "timeout"/"stuck" => no SetIterationDone (default productive=1).
	add := func(kind string) {
		id, err := st.NextIterationID("smoke", now)
		if err != nil {
			t.Fatal(err)
		}
		status := "done"
		if kind == "timeout" {
			status = "timeout"
		} else if kind == "stuck" {
			status = "no_i_am_done"
		}
		if err := st.CreateIteration(Iteration{ID: id, Agent: "smoke", Trigger: "interval",
			Status: status, StartedAt: now.Format(time.RFC3339)}); err != nil {
			t.Fatal(err)
		}
		switch kind {
		case "idle":
			if err := st.SetIterationDone(id, false); err != nil {
				t.Fatal(err)
			}
		case "prod":
			if err := st.SetIterationDone(id, true); err != nil {
				t.Fatal(err)
			}
		}
	}

	streak := func() int {
		n, err := st.IdleStreak("smoke")
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	if got := streak(); got != 0 {
		t.Fatalf("no iterations: streak=%d, want 0", got)
	}
	add("idle")
	add("idle")
	if got := streak(); got != 2 {
		t.Fatalf("two idle: streak=%d, want 2", got)
	}
	// A productive iteration resets the streak.
	add("prod")
	if got := streak(); got != 0 {
		t.Fatalf("after productive: streak=%d, want 0", got)
	}
	// A timeout iteration (productive defaults 1) breaks the streak: the two
	// idle iterations that follow it count, but the older ones do not.
	add("idle")
	add("timeout")
	add("idle")
	add("idle")
	if got := streak(); got != 2 {
		t.Fatalf("idle after timeout break: streak=%d, want 2", got)
	}
	// A no_i_am_done iteration (also productive=1) likewise breaks the streak.
	add("stuck")
	if got := streak(); got != 0 {
		t.Fatalf("no_i_am_done breaks streak: streak=%d, want 0", got)
	}
}

// TestStartResetIdle confirms a Start/restart grants a fresh idle budget: after
// an idle streak, StartResetIdle establishes a boundary so IdleStreak counts only
// iterations recorded after it, and it clears a stale idle_limit status line
// while preserving a real agent status.
func TestStartResetIdle(t *testing.T) {
	st := openStore(t)
	if err := st.Create(sampleAgent()); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	addIdle := func() {
		id, err := st.NextIterationID("smoke", now)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.CreateIteration(Iteration{ID: id, Agent: "smoke", Trigger: "interval",
			Status: "done", StartedAt: now.Format(time.RFC3339)}); err != nil {
			t.Fatal(err)
		}
		if err := st.SetIterationDone(id, false); err != nil {
			t.Fatal(err)
		}
	}
	streak := func() int {
		n, err := st.IdleStreak("smoke")
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Build an idle streak of 3, then idle-stop (mirrors the engine at threshold).
	addIdle()
	addIdle()
	addIdle()
	if got := streak(); got != 3 {
		t.Fatalf("pre-restart streak=%d, want 3", got)
	}
	if err := st.SetIdleStopped("smoke", "idle_limit (3 idle iterations)", now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}

	// Start/restart: fresh budget + stale idle status cleared.
	if err := st.StartResetIdle("smoke"); err != nil {
		t.Fatal(err)
	}
	if got := streak(); got != 0 {
		t.Fatalf("after StartResetIdle the historical streak must not count: streak=%d, want 0", got)
	}
	a, err := st.Get("smoke")
	if err != nil {
		t.Fatal(err)
	}
	if a.StatusMessage != "" {
		t.Fatalf("StartResetIdle must clear the stale idle_limit status, got %q", a.StatusMessage)
	}

	// Idle iterations after the boundary count fresh, from zero.
	addIdle()
	addIdle()
	if got := streak(); got != 2 {
		t.Fatalf("post-restart streak=%d, want 2 (fresh budget)", got)
	}
}

// TestStartResetIdlePreservesRealStatus confirms StartResetIdle only wipes the
// idle_limit residue, never a genuine agent-authored status line.
func TestStartResetIdlePreservesRealStatus(t *testing.T) {
	st := openStore(t)
	if err := st.Create(sampleAgent()); err != nil {
		t.Fatal(err)
	}
	if err := st.SetStatus("smoke", "reviewing the failing test", "2026-07-11T12:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := st.StartResetIdle("smoke"); err != nil {
		t.Fatal(err)
	}
	a, err := st.Get("smoke")
	if err != nil {
		t.Fatal(err)
	}
	if a.StatusMessage != "reviewing the failing test" {
		t.Fatalf("StartResetIdle clobbered a real status: got %q", a.StatusMessage)
	}
}

// TestSetIdleStopped confirms the clean idle-halt path clears loop_enabled and
// records the reason in status_message WITHOUT setting error_reason, so the
// agent stays a non-error (stopped) state.
func TestSetIdleStopped(t *testing.T) {
	st := openStore(t)
	if err := st.Create(sampleAgent()); err != nil {
		t.Fatal(err)
	}
	reason := "idle_limit (3 idle iterations)"
	if err := st.SetIdleStopped("smoke", reason, "2026-07-11T12:00:00Z"); err != nil {
		t.Fatal(err)
	}
	a, err := st.Get("smoke")
	if err != nil {
		t.Fatal(err)
	}
	if a.LoopEnabled {
		t.Fatal("SetIdleStopped must clear loop_enabled")
	}
	if a.ErrorReason != "" {
		t.Fatalf("SetIdleStopped must not set error_reason, got %q", a.ErrorReason)
	}
	if a.StatusMessage != reason {
		t.Fatalf("StatusMessage = %q, want %q", a.StatusMessage, reason)
	}
}

func TestPendingImagePromotionPreservesRuntimeConfiguration(t *testing.T) {
	s := openStore(t)
	want := Agent{Name: "switcher", ImageRef: "a:v1", ImageDigest: "old", HarnessType: "codex", Model: "m", Effort: "high", Env: map[string]string{"A": "B"}, Plugins: []string{"x"}, OnTimeout: "restart", OnError: "restart"}
	if err := s.Create(want); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPendingImage(want.Name, "b:v2", "new"); err != nil {
		t.Fatal(err)
	}
	pending, err := s.PendingImage(want.Name)
	if err != nil || pending.Ref != "b:v2" || pending.Digest != "new" {
		t.Fatalf("pending=%#v err=%v", pending, err)
	}
	if err := s.SetPendingImageError(want.Name, "blocked"); err != nil {
		t.Fatal(err)
	}
	pending, _ = s.PendingImage(want.Name)
	if pending.Error != "blocked" {
		t.Fatalf("pending=%#v", pending)
	}
	if err := s.PromotePendingImage(want.Name); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(want.Name)
	if err != nil {
		t.Fatal(err)
	}
	if got.ImageRef != "b:v2" || got.ImageDigest != "new" || got.HarnessType != want.HarnessType || got.Model != want.Model || got.Effort != want.Effort || got.Env["A"] != "B" {
		t.Fatalf("agent=%#v", got)
	}
	pending, _ = s.PendingImage(want.Name)
	if pending != (ImageAssignment{}) {
		t.Fatalf("pending not cleared: %#v", pending)
	}
}

func TestPendingImagePromotionDoesNotCommitAReplacedAssignment(t *testing.T) {
	s := openStore(t)
	a := sampleAgent()
	a.ImageRef = "active:v1"
	a.ImageDigest = "active-digest"
	if err := s.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPendingImage(a.Name, "candidate-a:v1", "digest-a"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPendingImage(a.Name, "candidate-b:v1", "digest-b"); err != nil {
		t.Fatal(err)
	}
	if err := s.PromotePendingImageWithPlugins(a.Name, "candidate-a:v1", "digest-a", []string{"a"}); err == nil {
		t.Fatal("stale pending assignment promotion unexpectedly succeeded")
	}
	got, err := s.Get(a.Name)
	if err != nil {
		t.Fatal(err)
	}
	if got.ImageRef != a.ImageRef || got.ImageDigest != a.ImageDigest {
		t.Fatalf("active image changed to %#v", got)
	}
	pending, err := s.PendingImage(a.Name)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Ref != "candidate-b:v1" || pending.Digest != "digest-b" {
		t.Fatalf("replacement pending assignment changed: %#v", pending)
	}
}

func TestBroadUpdateCannotClobberPromotedImageIdentity(t *testing.T) {
	s := openStore(t)
	a := sampleAgent()
	if err := s.Create(a); err != nil {
		t.Fatal(err)
	}
	stale, err := s.Get(a.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPendingImage(a.Name, "next:v2", "next-digest"); err != nil {
		t.Fatal(err)
	}
	if err := s.PromotePendingImage(a.Name); err != nil {
		t.Fatal(err)
	}
	stale.Model = "updated-runtime-model"
	if err := s.Update(stale); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(a.Name)
	if err != nil {
		t.Fatal(err)
	}
	if got.ImageRef != "next:v2" || got.ImageDigest != "next-digest" || got.Model != stale.Model {
		t.Fatalf("agent after stale broad update = %#v", got)
	}
}

func TestIterationImageSnapshotIsImmutableAcrossAgentPromotion(t *testing.T) {
	s := openStore(t)
	a := sampleAgent()
	a.ImageRef = "a:v1"
	a.ImageDigest = "old"
	if err := s.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateIteration(Iteration{
		ID: "one", Agent: a.Name, Trigger: "manual", Status: "running",
		ImageRef: a.ImageRef, ImageDigest: a.ImageDigest, PromptTemplateSHA256: "template-a",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPendingImage(a.Name, "b:v2", "new"); err != nil {
		t.Fatal(err)
	}
	if err := s.PromotePendingImage(a.Name); err != nil {
		t.Fatal(err)
	}
	it, err := s.GetIteration(a.Name, "one")
	if err != nil {
		t.Fatal(err)
	}
	if it.ImageRef != "a:v1" || it.ImageDigest != "old" || it.PromptTemplateSHA256 != "template-a" {
		t.Fatalf("snapshot=%#v", it)
	}
}
