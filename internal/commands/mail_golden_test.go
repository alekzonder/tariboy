package commands

import (
	"flag"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/mail/storedir"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

var updateMailGolden = flag.Bool("update-mail-golden", false, "rewrite testdata/mail_responses.golden.json")

// mailFixtureIDs names the letters the mail fixture sends.
type mailFixtureIDs struct {
	// request is open in the customer's inbox, notice is a notice there,
	// question is a task question, dead is dead-lettered in worker's inbox and
	// toWorker is open in worker's inbox.
	request, notice, question, dead, toWorker string
}

// mailFixture stores agents worker and other and the customer's mail: an open
// request with a deadline and data, a notice, a task question, a letter to
// worker and a dead letter in worker's inbox.
func mailFixture(t *testing.T) (*registry.Ctx, mailFixtureIDs) {
	t.Helper()
	s := storetest.Open(t)
	m := mail.New(s, steppingClock(), storedir.New(s, "customer"))
	c := &registry.Ctx{Store: s, Mail: m, Operator: "customer"}
	as := agent.NewStore(s)
	for _, name := range []string{"worker", "other"} {
		if err := as.Create(agent.Agent{Name: name, ImageRef: "basic:latest"}); err != nil {
			t.Fatal(err)
		}
	}
	send := func(l mail.Letter) string {
		t.Helper()
		out, err := m.Send(l)
		if err != nil {
			t.Fatal(err)
		}
		return out[0].ID
	}
	var ids mailFixtureIDs
	ids.request = send(mail.Letter{From: "agent:worker", To: "user:customer", Subject: "Deploy?", Body: "May I deploy?",
		NeedsReply: true, Deadline: "2026-07-15T00:00:00Z", Data: map[string]any{"env": "prod"}})
	ids.notice = send(mail.Letter{From: "agent:other", To: "user:customer", Subject: "Done", Body: "Deployed."})
	ids.question = send(mail.Letter{From: "agent:worker", To: "user:customer", Kind: mail.KindTaskQuestion, Subject: "Question on DEV-1",
		Body: "Which database?", NeedsReply: true, Data: map[string]any{"task_key": "DEV-1"}})
	ids.toWorker = send(mail.Letter{From: "user:customer", To: "agent:worker", Subject: "Status", Body: "How is it going?", NeedsReply: true})
	ids.dead = send(mail.Letter{From: "agent:other", To: "agent:worker", Subject: "Old", Body: "stale", NeedsReply: true})
	if _, err := s.DB.Exec(`UPDATE mail SET dead_at = created_at, answered_at = created_at, dead_reason = 'attempts', attempts = 5 WHERE id = ?`, ids.dead); err != nil {
		t.Fatal(err)
	}
	return c, ids
}

type mailGoldenCall struct {
	label, path string
	params      registry.Params
}

func mailGoldenCalls(ids mailFixtureIDs) []mailGoldenCall {
	return []mailGoldenCall{
		{"mail.ls inbox", "mail.ls", registry.Params{}},
		{"mail.ls sent as worker", "mail.ls", registry.Params{"box": "sent", "as": "agent:worker"}},
		{"mail.ls archive as worker", "mail.ls", registry.Params{"box": "archive", "as": "agent:worker"}},
		{"mail.unread", "mail.unread", registry.Params{}},
		{"mail.show", "mail.show", registry.Params{"id": ids.request}},
		{"mail.send", "mail.send", registry.Params{"to": "agent:worker,agent:other", "subject": "Hi", "text": "hello",
			"kind": "message", "data": map[string]any{"k": "v"}, "notice": true}},
		{"mail.reply", "mail.reply", registry.Params{"id": ids.request, "text": "Yes"}},
		{"mail.show answered", "mail.show", registry.Params{"id": ids.request}},
		{"mail.reply on behalf", "mail.reply", registry.Params{"id": ids.toWorker, "text": "Fine", "ask": true, "as": "agent:worker"}},
		{"mail.read", "mail.read", registry.Params{"ids": []any{ids.notice, ids.question}}},
		{"mail.close", "mail.close", registry.Params{"id": ids.question, "note": "handled elsewhere"}},
		{"mail.requeue", "mail.requeue", registry.Params{"id": ids.dead, "as": "agent:worker"}},
		{"agent.messages.batch", "agent.messages.batch", registry.Params{"name": "worker", "value": float64(7)}},
		{"agent.messages.max-queue", "agent.messages.max-queue", registry.Params{"name": "worker", "value": float64(70)}},
		{"agent.mail.clear", "agent.mail.clear", registry.Params{"name": "worker"}},
	}
}

// Catches a typed mail response changing the JSON a client sees: the set of
// keys, their JSON types, and their values must match the recorded responses.
func TestMailResponsesMatchGolden(t *testing.T) {
	c, ids := mailFixture(t)
	recorder := newGoldenRecorder(t, map[string]bool{})
	for _, call := range mailGoldenCalls(ids) {
		value, err := h(t, call.path)(c, call.params)
		if err != nil {
			t.Fatalf("%s: %v", call.label, err)
		}
		recorder.record(call.label, value)
	}
	recorder.check("mail_responses.golden.json", *updateMailGolden)
}

func TestMailErrorsCarryTheirCodes(t *testing.T) {
	c, ids := mailFixture(t)
	for _, tc := range []struct {
		path   string
		params registry.Params
		code   string
	}{
		{"mail.send", registry.Params{"to": "agent:ghost", "text": "x"}, "unknown_recipient"},
		{"mail.reply", registry.Params{"id": ids.question, "text": "x"}, "answer_on_task"},
		{"mail.reply", registry.Params{"id": ids.request, "text": "x", "as": "agent:other"}, "forbidden"},
		{"mail.show", registry.Params{"id": "m-none"}, "not_found"},
	} {
		_, err := h(t, tc.path)(c, tc.params)
		if got := mail.ErrorCode(err); got != tc.code {
			t.Errorf("%s %v: error %v (code %q); want %q", tc.path, tc.params, err, got, tc.code)
		}
	}
}

func init() { typedCallSets["mail"] = mailTypedCalls }

// mailTypedCalls drives every typed mail operation over HTTP against the mail
// fixture.
func mailTypedCalls(t *testing.T) (*registry.Ctx, []typedCall) {
	c, ids := mailFixture(t)
	return c, []typedCall{
		{"GET", "/api/mail", "/api/mail", nil},
		{"GET", "/api/mail", "/api/mail?box=sent&as=agent:worker&limit=1", nil},
		{"GET", "/api/mail/unread", "/api/mail/unread", nil},
		{"GET", "/api/mail/{id}", "/api/mail/" + ids.request, nil},
		{"POST", "/api/mail", "/api/mail", map[string]any{"to": "agent:worker", "subject": "Hi", "text": "hello",
			"data": map[string]any{"k": "v"}, "deadline": "5m"}},
		{"POST", "/api/mail/{id}/reply", "/api/mail/" + ids.request + "/reply", map[string]any{"text": "Yes"}},
		{"GET", "/api/mail/{id}", "/api/mail/" + ids.request, nil},
		{"POST", "/api/mail/read", "/api/mail/read", map[string]any{"ids": []string{ids.notice}}},
		{"POST", "/api/mail/{id}/close", "/api/mail/" + ids.question + "/close", map[string]any{"note": "n"}},
		{"POST", "/api/mail/{id}/requeue", "/api/mail/" + ids.dead + "/requeue", map[string]any{"as": "agent:worker"}},
		{"POST", "/api/agents/{name}/messages/batch", "/api/agents/worker/messages/batch", map[string]any{"value": 7}},
		{"POST", "/api/agents/{name}/messages/max-queue", "/api/agents/worker/messages/max-queue", map[string]any{"value": 70}},
		{"POST", "/api/agents/{name}/mail/clear", "/api/agents/worker/mail/clear", nil},
	}
}
