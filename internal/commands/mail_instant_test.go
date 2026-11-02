package commands

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestAgentMailInstantSendersSetReadReset(t *testing.T) {
	t.Parallel()
	c, agents := seedAgent(t)
	if err := agents.Create(agent.Agent{Name: "lead", OnTimeout: "restart", OnError: "restart"}); err != nil {
		t.Fatal(err)
	}
	run := h(t, "agent.messages.instant-senders")
	res, err := run(c, registry.Params{"name": "a1"})
	if err != nil || res.(AgentMailInstantSenders).MailInstantSenders != nil {
		t.Fatalf("read unset = %#v, %v; want nil", res, err)
	}
	res, err = run(c, registry.Params{"name": "a1", "senders": []any{"agent:lead"}})
	if err != nil || !reflect.DeepEqual(res.(AgentMailInstantSenders).MailInstantSenders, []string{"agent:lead"}) {
		t.Fatalf("set = %#v, %v", res, err)
	}
	if a, _ := agents.Get("a1"); !reflect.DeepEqual(a.MailInstantSenders, []string{"agent:lead"}) {
		t.Fatalf("stored = %v", a.MailInstantSenders)
	}
	res, err = run(c, registry.Params{"name": "a1", "senders": []any{}})
	if err != nil || res.(AgentMailInstantSenders).MailInstantSenders == nil || len(res.(AgentMailInstantSenders).MailInstantSenders) != 0 {
		t.Fatalf("set empty = %#v, %v; want []", res, err)
	}
	res, err = run(c, registry.Params{"name": "a1", "reset": true})
	if err != nil || res.(AgentMailInstantSenders).MailInstantSenders != nil {
		t.Fatalf("reset = %#v, %v", res, err)
	}
	for _, bad := range []string{"user:customer", "agent:ghost", "lead"} {
		_, err := run(c, registry.Params{"name": "a1", "senders": []any{bad}})
		if !isCode(err, "bad_mail_instant_senders") {
			t.Fatalf("sender %q: err %#v", bad, err)
		}
	}
	if _, err := run(c, registry.Params{"name": "a1", "senders": []any{"agent:lead"}}); err != nil {
		t.Fatal(err)
	}
	view, err := h(t, "agent.inspect")(c, registry.Params{"name": "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := view.(AgentView).MailInstantSenders; !reflect.DeepEqual(got, []string{"agent:lead"}) {
		t.Fatalf("view senders = %v", got)
	}
}

func TestMailInstantErrorsCarryCodeAndStatus(t *testing.T) {
	c, _ := mailFixture(t)
	for _, tc := range []struct {
		params registry.Params
		code   string
		status int
	}{
		{registry.Params{"to": "agent:worker", "text": "x", "instant": true, "as": "system:goal"}, "instant_not_allowed", http.StatusForbidden},
		{registry.Params{"to": "agent:worker", "text": "x", "instant": true, "as": "agent:other"}, "instant_not_allowed", http.StatusForbidden},
		{registry.Params{"to": "user:customer", "text": "x", "instant": true, "as": "agent:worker"}, "instant_recipient", http.StatusBadRequest},
		{registry.Params{"to": "agent:worker,user:customer", "text": "x", "instant": true}, "instant_recipient", http.StatusBadRequest},
	} {
		_, err := h(t, "mail.send")(c, tc.params)
		if mail.ErrorCode(err) != tc.code || mail.ErrorStatus(err) != tc.status {
			t.Errorf("%v: err %v; want %s %d", tc.params, err, tc.code, tc.status)
		}
	}
	res, err := h(t, "mail.send")(c, registry.Params{"to": "agent:worker", "text": "x", "instant": true})
	if err != nil || !res.(mail.MailSendResult).Letters[0].Instant {
		t.Fatalf("customer instant send = %#v, %v", res, err)
	}
}

func TestAgentRunTakesMailInstantSenders(t *testing.T) {
	t.Parallel()
	c, as, fc := ctxWithStore(t)
	if err := as.Create(agent.Agent{Name: "lead", OnTimeout: "restart", OnError: "restart"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h(t, "agent.run")(c, registry.Params{"image": "basic:latest", "name": "w1",
		"mail_instant_senders": []any{"agent:lead"}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fc.ran.MailInstantSenders, []string{"agent:lead"}) {
		t.Fatalf("spec senders = %v", fc.ran.MailInstantSenders)
	}
	if _, err := h(t, "agent.run")(c, registry.Params{"image": "basic:latest", "name": "w2"}); err != nil {
		t.Fatal(err)
	}
	if fc.ran.MailInstantSenders != nil {
		t.Fatalf("spec senders without the argument = %v; want nil", fc.ran.MailInstantSenders)
	}
	_, err := h(t, "agent.run")(c, registry.Params{"image": "basic:latest", "name": "w3", "mail_instant_senders": []any{"user:x"}})
	if !isCode(err, "bad_mail_instant_senders") {
		t.Fatalf("bad sender err = %#v", err)
	}
}
