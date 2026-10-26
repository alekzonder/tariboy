package mail_test

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/schedule"
)

// scheduleDeadlines arms request deadlines as one-shot schedules keyed on the
// request letter id, the way the daemon wires them.
type scheduleDeadlines struct{ sched *schedule.Store }

func (d scheduleDeadlines) ArmTx(tx *sql.Tx, request mail.Letter) error {
	tmpl, err := json.Marshal(map[string]any{
		"kind": "deadline",
		"text": "request timed out with no reply",
		"data": map[string]any{"request_id": request.ID},
	})
	if err != nil {
		return err
	}
	_, err = d.sched.AddTx(tx, schedule.Schedule{
		Agent:           strings.TrimPrefix(request.From, "agent:"),
		Kind:            "oneshot",
		Spec:            request.Deadline,
		Channel:         request.From,
		MessageTemplate: string(tmpl),
		CorrelationID:   request.ID,
	})
	return err
}

func (d scheduleDeadlines) CancelTx(tx *sql.Tx, requestID string) error {
	return d.sched.CancelByCorrelationTx(tx, requestID)
}

func TestDeadlineSchedulesAndReplyCancels(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	sched := schedule.NewStore(f.st, func() time.Time { return *f.now })
	f.m.SetDeadlines(scheduleDeadlines{sched: sched})

	req := f.sendOne(t, mail.Letter{From: "agent:alice", To: "agent:bob", Body: "help", NeedsReply: true, Deadline: "5m"})
	list, err := sched.List("alice")
	if err != nil || len(list) != 1 {
		t.Fatalf("schedules = %d, %v; want one deadline", len(list), err)
	}
	if list[0].Channel != "agent:alice" || list[0].Kind != "oneshot" || !strings.Contains(list[0].MessageTemplate, req.ID) {
		t.Fatalf("deadline schedule = %+v", list[0])
	}
	var keyed int
	if err := f.st.DB.QueryRow(`SELECT COUNT(*) FROM schedules WHERE correlation_id = ?`, req.ID).Scan(&keyed); err != nil || keyed != 1 {
		t.Fatalf("schedules keyed on the letter id = %d, %v", keyed, err)
	}
	// A notice and a request without a deadline arm nothing.
	f.sendOne(t, mail.Letter{From: "agent:alice", To: "agent:bob", Body: "fyi"})
	f.sendOne(t, mail.Letter{From: "agent:alice", To: "agent:bob", Body: "?", NeedsReply: true})
	if list, _ := sched.List("alice"); len(list) != 1 {
		t.Fatalf("schedules = %d; want 1", len(list))
	}

	if _, _, err := f.m.Reply(mail.ReplyRequest{As: "agent:bob", ID: req.ID, Text: "done"}); err != nil {
		t.Fatal(err)
	}
	if list, _ := sched.List("alice"); len(list) != 0 {
		t.Fatalf("the reply left %d deadline schedules", len(list))
	}
}
