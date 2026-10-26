package mail_test

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/mail"
)

func TestAgentActionSendsAsTheCallerAndScopesReads(t *testing.T) {
	f := newFixture(t)
	res, err := f.m.AgentAction("alice", "alice-7", "send", map[string]any{
		"to": "agent:bob", "subject": "hi", "text": "ping", "from": "agent:carol", "notice": true})
	if err != nil {
		t.Fatal(err)
	}
	sent := res.(mail.MailSendResult).Letters[0]
	if sent.From != "agent:alice" || sent.NeedsReply || sent.ProducedInIteration != "alice-7" {
		t.Fatalf("sent = %+v; want from agent:alice, a notice, iteration alice-7", sent)
	}
	inbox, err := f.m.AgentAction("bob", "", "inbox", nil)
	if err != nil {
		t.Fatal(err)
	}
	if list := inbox.(mail.MailList); list.Count != 1 || list.As != "agent:bob" || list.Letters[0].ID != sent.ID {
		t.Fatalf("bob inbox = %+v", list)
	}
	if _, err := f.m.AgentAction("carol", "", "show", map[string]any{"id": sent.ID}); mail.ErrorCode(err) != "forbidden" {
		t.Fatalf("carol show = %v; want forbidden", err)
	}
	if _, err := f.m.AgentAction("alice", "", "close", map[string]any{"id": sent.ID}); mail.ErrorCode(err) != "not_found" {
		t.Fatalf("close = %v; want not_found: agents answer, they do not archive", err)
	}
	reply, err := f.m.AgentAction("bob", "bob-1", "reply", map[string]any{"id": sent.ID, "text": "pong"})
	if err != nil {
		t.Fatal(err)
	}
	if r := reply.(mail.MailReplyResult); !r.Closed || r.Letter.To != "agent:alice" {
		t.Fatalf("reply = %+v", r)
	}
	all, err := f.m.AgentAction("bob", "", "inbox", map[string]any{"all": true})
	if err != nil {
		t.Fatal(err)
	}
	if list := all.(mail.MailList); list.Count != 1 {
		t.Fatalf("inbox --all = %+v; want the closed letter", list)
	}
}
