package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	basestore "github.com/alekzonder/tariboy/internal/store"
)

// staleMail sends letters to agent:dev-1 at the given minutes after
// stallStart.
type staleMail struct {
	t   *testing.T
	m   *mail.Mail
	now time.Time
}

func newStaleMail(t *testing.T, svc *Service) *staleMail {
	f := &staleMail{t: t}
	f.m = mail.New(&basestore.Store{DB: svc.db}, func() time.Time { return f.now }, questionDirectory{})
	return f
}

func (f *staleMail) send(minutes int, kind, key string) mail.Letter {
	f.t.Helper()
	return f.sendTo(minutes, "agent:dev-1", kind, key)
}

func (f *staleMail) sendTo(minutes int, to, kind, key string) mail.Letter {
	f.t.Helper()
	f.now = at(minutes)
	out, err := f.m.Send(mail.Letter{From: "system:tasks", To: to, Kind: kind, Subject: kind + " " + key,
		Body: kind, NeedsReply: kind == mail.KindTaskQuestion || kind == "task.goal", Data: map[string]any{"task_key": key}})
	if err != nil || len(out) != 1 {
		f.t.Fatalf("send %s: %v, %v", kind, out, err)
	}
	return out[0]
}

func (f *staleMail) open(l mail.Letter) bool {
	f.t.Helper()
	got, _, err := f.m.Show(l.To, l.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	return got.AnsweredAt == "" && got.DeadAt == ""
}

func TestIterationEndClosesTheNoticesAboutItsGoal(t *testing.T) {
	for _, status := range []string{"", "approval"} {
		t.Run("status "+status, func(t *testing.T) {
			svc, _, task := requestFixture(t)
			if status != "" {
				task = enter(t, svc, task.Key, status, "ask")
			}
			f := newStaleMail(t, svc)
			assigned := f.send(1, "task.assigned", task.Key)
			goal := f.send(2, "task.goal", task.Key)
			answered := f.send(3, "task.answered", task.Key)
			question := f.send(3, mail.KindTaskQuestion, task.Key)
			other := f.send(3, "task.assigned", "DEV-999")
			later := f.send(6, "task.assigned", task.Key)

			recordEnd(t, svc, iterationBetween("dev-1", task.Key, 0, 5))

			for _, l := range []mail.Letter{assigned, goal, answered} {
				if f.open(l) {
					t.Fatalf("%s stays open", l.Kind)
				}
			}
			for _, l := range []mail.Letter{question, other, later} {
				if !f.open(l) {
					t.Fatalf("%s %s was closed", l.Kind, l.Subject)
				}
			}
			if n := countRows(t, svc, `SELECT COUNT(*) FROM task_notification_outbox
				WHERE channel = 'agent:dev-1' AND message_type = 'task.assigned' AND published_at = ''
				AND json_extract(data, '$.task_key') = ?`, task.Key); n != 0 {
				t.Fatalf("unpublished task.assigned rows = %d", n)
			}
		})
	}
}

func TestCloseStaleTaskLettersClosesNoticesForWorkThatIsNotTheAgents(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, svc *Service, task Task)
		stale bool
		// keepsAnswer: the answer goes to an agent that no longer works the task.
		keepsAnswer bool
	}{
		{"a pool status the agent holds", func(*testing.T, *Service, Task) {}, false, false},
		{"a customer status", func(t *testing.T, svc *Service, task Task) { enter(t, svc, task.Key, "approval", "ask") }, true, false},
		{"a script status", func(t *testing.T, svc *Service, task Task) {
			if _, err := svc.db.Exec(`UPDATE tasks SET workflow_owner_kind = 'script' WHERE id = ?`, task.ID); err != nil {
				t.Fatal(err)
			}
		}, true, false},
		{"a paused task", func(t *testing.T, svc *Service, task Task) { pause(t, svc, task.Key, PauseScriptFailures, "") }, true, false},
		{"another assignee", func(t *testing.T, svc *Service, task Task) {
			if _, err := svc.ReassignWorkflow(context.Background(), CustomerActor("customer"), task.Key, "dev-2"); err != nil {
				t.Fatal(err)
			}
		}, true, true},
		{"done", func(t *testing.T, svc *Service, task Task) { forceStatus(t, svc, task, StatusDone) }, true, false},
		{"cancelled", func(t *testing.T, svc *Service, task Task) { forceStatus(t, svc, task, StatusCancelled) }, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, task := requestFixture(t)
			f := newStaleMail(t, svc)
			notices := []mail.Letter{f.send(1, "task.assigned", task.Key), f.send(1, "task.goal", task.Key),
				f.send(1, "task.answered", task.Key)}
			question := f.send(1, mail.KindTaskQuestion, task.Key)
			unknown := f.send(1, "task.assigned", "DEV-999")
			customerAnswer := f.sendTo(1, "agent:boss", "task.answered", task.Key)
			tc.setup(t, svc, task)

			n, err := svc.CloseStaleTaskLetters(context.Background(), "dev-1")
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			for _, l := range notices {
				stale := tc.stale && !(tc.keepsAnswer && l.Kind == "task.answered")
				if stale {
					want++
				}
				if f.open(l) == stale {
					t.Fatalf("%s open = %v; stale %v", l.Kind, f.open(l), stale)
				}
			}
			if n != want {
				t.Fatalf("closed = %d; want %d", n, want)
			}
			if !f.open(question) || !f.open(unknown) {
				t.Fatal("a question or a letter about an unknown task was closed")
			}
			// The customer agent's answer notice stays open whatever the status.
			if !f.open(customerAnswer) {
				t.Fatal("the answer to an agent that does not work the task was closed")
			}
		})
	}
}

func forceStatus(t *testing.T, svc *Service, task Task, status string) {
	t.Helper()
	if _, err := svc.db.Exec(`UPDATE tasks SET status = ? WHERE id = ?`, status, task.ID); err != nil {
		t.Fatal(err)
	}
}
