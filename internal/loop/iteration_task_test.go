package loop

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agentapi"
	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/store/storetest"
	"github.com/alekzonder/tariboy/internal/tasks"
)

func TestChooseIterationTask(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		goal    string
		hasGoal bool
		keys    []string
		want    string
	}{
		{"goal with letters", "DEV-1", true, []string{"DEV-2", "DEV-1"}, "DEV-1"},
		{"goal without letters, letters of another task", "DEV-1", true, []string{"DEV-2", "DEV-3"}, "DEV-2"},
		{"goal without any letter", "DEV-1", true, nil, "DEV-1"},
		{"no goal, letters", "", false, []string{"DEV-2", ""}, "DEV-2"},
		{"no goal, letters about no task first", "", false, []string{"", "DEV-2"}, ""},
		{"no goal, no letters", "", false, nil, ""},
	} {
		if got := chooseIterationTask(tt.goal, tt.hasGoal, tt.keys); got != tt.want {
			t.Errorf("%s: chooseIterationTask = %q; want %q", tt.name, got, tt.want)
		}
	}
}

// taskLetterRunner is a proxy-test runner with a mailbox and the hooks of the
// iteration task: the Goal reads, the message wakes and the Goal candidates.
type taskLetterRunner struct {
	r     *ShimRunner
	mail  *mail.Mail
	proxy *fakeBinder
	goals []string
	wakes []string
	logs  *bytes.Buffer
}

func newTaskLetterRunner(t *testing.T, goal tasks.Task, hasGoal bool) (*taskLetterRunner, func() string) {
	t.Helper()
	f := &taskLetterRunner{proxy: &fakeBinder{base: "http://127.0.0.1:5555"}, logs: &bytes.Buffer{}}
	r, ag, l, as := newRunnerForProxyTest(t, f.proxy)
	// A done iteration closes the notices it showed.
	if err := as.SetIterationDone("alice-1", true); err != nil {
		t.Fatal(err)
	}
	f.r, f.mail = r, newTestMail(storetest.Open(t), ag.Name)
	r.cfg.Mail = f.mail
	r.cfg.Logger = slog.New(slog.NewTextHandler(f.logs, nil))
	r.cfg.CurrentGoal = func(string, time.Time) (tasks.Task, bool, error) { return goal, hasGoal, nil }
	r.cfg.GoalCandidate = func(string, string, time.Time) (tasks.Task, bool, error) { return tasks.Task{}, false, nil }
	r.cfg.GoalRead = func(_, _, key string, _ time.Time) { f.goals = append(f.goals, key) }
	r.cfg.WakeMessages = func(agent string) { f.wakes = append(f.wakes, agent) }
	r.cfg.Tasks = &attributionTaskReader{tasks: map[string]tasks.Task{}}
	run := func() string {
		t.Helper()
		if _, err := r.Run(context.Background(), ag, "message", "alice-1", ""); err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(l.PromptPath("alice-1"))
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	return f, run
}

func (f *taskLetterRunner) send(t *testing.T, l mail.Letter) mail.Letter {
	t.Helper()
	l.From, l.To = "user:customer", "agent:alice"
	out, err := f.mail.Send(l)
	if err != nil {
		t.Fatal(err)
	}
	return out[0]
}

func (f *taskLetterRunner) letter(t *testing.T, id string) mail.Letter {
	t.Helper()
	letters, err := inboxOf(f.mail, "alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range letters {
		if l.ID == id {
			return l
		}
	}
	return mail.Letter{ID: id}
}

// TestRunServesOnlyTheGoalLettersAndWakesForTheRest: Goal DEV-5das has a
// letter, and a task.assigned letter about DEV-nvvs is in the inbox too. The
// iteration shows only the Goal letter and spends on DEV-5das; the DEV-nvvs
// letter stays untouched, and a message wake runs it next.
func TestRunServesOnlyTheGoalLettersAndWakesForTheRest(t *testing.T) {
	t.Parallel()
	f, run := newTaskLetterRunner(t, tasks.Task{Key: "DEV-5das", Title: "goal"}, true)
	f.send(t, mail.Letter{Kind: "task.goal", Body: "work the goal", Data: map[string]any{"task_key": "DEV-5das"}})
	other := f.send(t, mail.Letter{Kind: "task.assigned", Body: "another task", Data: map[string]any{"task_key": "DEV-nvvs"}})

	prompt := run()
	if !strings.Contains(prompt, "work the goal") || strings.Contains(prompt, "another task") {
		t.Fatalf("prompt letters:\n%s", prompt)
	}
	if got := f.letter(t, other.ID); !got.Open() || got.Attempts != 0 {
		t.Fatalf("the letter of another task was touched: %+v", got)
	}
	if len(f.proxy.updates) != 1 || f.proxy.updates[0].task != "DEV-5das" {
		t.Fatalf("proxy attribution = %+v; want DEV-5das", f.proxy.updates)
	}
	if strings.Join(f.goals, ",") != "DEV-5das" {
		t.Fatalf("goal reads = %v; want DEV-5das", f.goals)
	}
	if strings.Join(f.wakes, ",") != "alice" {
		t.Fatalf("wakes = %v; want one for alice", f.wakes)
	}
}

// TestRunAttributesALetterTaskThatIsNoGoal: no Goal, a task.question about
// DEV-p6z4 that the agent cannot take as a Goal (it is in a customer status).
// The iteration is a request iteration about DEV-p6z4, and the proxy spends on
// DEV-p6z4 and its root. A request still open after the iteration keeps the
// task first, so no wake follows for the letter of another task.
func TestRunAttributesALetterTaskThatIsNoGoal(t *testing.T) {
	t.Parallel()
	f, run := newTaskLetterRunner(t, tasks.Task{}, false)
	reader := &attributionTaskReader{tasks: map[string]tasks.Task{
		"DEV-root": {Key: "DEV-root"},
		"DEV-p6z4": {Key: "DEV-p6z4", ParentKey: "DEV-root"},
	}}
	f.r.cfg.Tasks = reader
	f.send(t, mail.Letter{Kind: "task.question", Body: "answer me", NeedsReply: true, Data: map[string]any{"task_key": "DEV-p6z4"}})
	other := f.send(t, mail.Letter{Kind: "task.assigned", Body: "another task", Data: map[string]any{"task_key": "DEV-2"}})

	prompt := run()
	if !strings.Contains(prompt, "answer me") || strings.Contains(prompt, "another task") {
		t.Fatalf("prompt letters:\n%s", prompt)
	}
	assertModeLine(t, prompt, IterationModeRequest)
	if !strings.Contains(prompt, "Letters of this iteration are about task `DEV-p6z4`.") {
		t.Fatalf("prompt does not name the letter task:\n%s", prompt)
	}
	if len(f.proxy.updates) != 1 || f.proxy.updates[0].task != "DEV-p6z4" || f.proxy.updates[0].epic != "DEV-root" {
		t.Fatalf("proxy attribution = %+v; want DEV-p6z4/DEV-root", f.proxy.updates)
	}
	if reader.reads[0] != "user:customer DEV-p6z4" {
		t.Fatalf("reads = %v; the letter task is read as the customer", reader.reads)
	}
	if strings.Join(f.goals, ",") != "DEV-p6z4" {
		t.Fatalf("goal reads = %v; want DEV-p6z4", f.goals)
	}
	if got := f.letter(t, other.ID); got.Attempts != 0 {
		t.Fatalf("the letter of another task was touched: %+v", got)
	}
	if len(f.wakes) != 0 {
		t.Fatalf("wakes = %v; the open request keeps its task first", f.wakes)
	}
}

// TestRunShowsAnEligibleLetterTaskAsTheGoal: the Goal has no letter, and the
// letter is about another active task assigned to the agent. That task is the
// Goal of the iteration; the sticky Goal is not changed.
func TestRunShowsAnEligibleLetterTaskAsTheGoal(t *testing.T) {
	t.Parallel()
	f, run := newTaskLetterRunner(t, tasks.Task{Key: "DEV-1", Title: "sticky"}, true)
	f.r.cfg.GoalCandidate = func(agent, key string, _ time.Time) (tasks.Task, bool, error) {
		if agent != "alice" || key != "DEV-2" {
			t.Fatalf("GoalCandidate(%q, %q)", agent, key)
		}
		return tasks.Task{Key: "DEV-2", Title: "letter task"}, true, nil
	}
	f.r.cfg.Tasks = &attributionTaskReader{tasks: map[string]tasks.Task{"DEV-2": {Key: "DEV-2"}}}
	f.send(t, mail.Letter{Kind: "task.assigned", Body: "take it", Data: map[string]any{"task_key": "DEV-2"}})

	prompt := run()
	if !strings.Contains(prompt, "key: DEV-2\ntitle: letter task") || strings.Contains(prompt, "key: DEV-1") {
		t.Fatalf("goal block:\n%s", prompt)
	}
	assertModeLine(t, prompt, IterationModeFlexible)
	if len(f.proxy.updates) != 1 || f.proxy.updates[0].task != "DEV-2" {
		t.Fatalf("proxy attribution = %+v; want DEV-2", f.proxy.updates)
	}
}

// TestRunWithoutAReadableLetterTaskRunsUnattributed: a letter task the daemon
// cannot read leaves the iteration without attribution and logs why.
func TestRunWithoutAReadableLetterTaskRunsUnattributed(t *testing.T) {
	t.Parallel()
	f, run := newTaskLetterRunner(t, tasks.Task{}, false)
	f.r.cfg.Tasks = &attributionTaskReader{err: map[string]error{"DEV-9": errors.New("storage down")}}
	f.send(t, mail.Letter{Kind: "task.assigned", Body: "gone", Data: map[string]any{"task_key": "DEV-9"}})

	if prompt := run(); !strings.Contains(prompt, "gone") {
		t.Fatalf("prompt letters:\n%s", prompt)
	}
	if len(f.proxy.updates) != 0 {
		t.Fatalf("proxy attribution = %+v; want none", f.proxy.updates)
	}
	if !strings.Contains(f.logs.String(), "storage down") || !strings.Contains(f.logs.String(), "DEV-9") {
		t.Fatalf("logs = %s", f.logs.String())
	}
}

// TestRunStopsOnAParentCycleOfALetterTask: a broken task tree stops the
// iteration, as it does for a Goal.
func TestRunStopsOnAParentCycleOfALetterTask(t *testing.T) {
	t.Parallel()
	f, _ := newTaskLetterRunner(t, tasks.Task{}, false)
	f.r.cfg.Tasks = &attributionTaskReader{tasks: map[string]tasks.Task{
		"DEV-1": {Key: "DEV-1", ParentKey: "DEV-2"},
		"DEV-2": {Key: "DEV-2", ParentKey: "DEV-1"},
	}}
	f.send(t, mail.Letter{Kind: "task.assigned", Body: "loop", Data: map[string]any{"task_key": "DEV-1"}})
	ag, err := f.r.cfg.Store.Get("alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.r.Run(context.Background(), ag, "message", "alice-1", ""); err == nil || !strings.Contains(err.Error(), "parent cycle") {
		t.Fatalf("Run error = %v; want the parent cycle", err)
	}
}

// TestToolsInstantHoldsALetterOfAnotherTask: the instant take of an iteration
// that works DEV-1 delivers the DEV-1 letter and holds the DEV-2 letter.
func TestToolsInstantHoldsALetterOfAnotherTask(t *testing.T) {
	m, as := instantManager(t)
	mb := mail.New(storetest.Open(t), nil, agentStoreDirectory{as: as})
	mb.SetInstantRuntime(m)
	m.cfg.Mail = mb
	m.noteIterationGoal("run", "run-1", "DEV-1", time.Now())
	own, err := mb.Send(mail.Letter{From: "user:customer", To: "agent:run", Body: "own", Instant: true, Data: map[string]any{"task_key": "DEV-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mb.Send(mail.Letter{From: "user:customer", To: "agent:run", Body: "other", Instant: true, Data: map[string]any{"task_key": "DEV-2"}}); err != nil {
		t.Fatal(err)
	}
	f := toolsFacade{agent: "run", cfg: &m.cfg, iterations: fixedIteration("run-1"), goals: m}
	var d agentapi.Deps
	f.mailDeps(&d)
	res, err := d.MailAction("instant", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.(mail.MailInstantResult); len(got.IDs) != 1 || got.IDs[0] != own[0].ID {
		t.Fatalf("take = %+v; want only the DEV-1 letter", got)
	}
}
