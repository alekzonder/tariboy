package loop

import (
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/tasks"
)

func TestTaskProcessingOrderIndependentOfTemplate(t *testing.T) {
	for _, names := range [][]string{nil, {"goal", "awaiting-replies", "messages", "one-shot"}, {"messages"}} {
		template := image.PromptTemplate{SchemaVersion: 2}
		for _, name := range names {
			template.Entries = append(template.Entries, image.TemplateEntry{Kind: "runtime", Runtime: name})
		}
		template.SHA256 = promptTemplateSHA(t, template)
		msg := bus.Message{ID: "message-1", Type: "request", Source: "agent:sender", Text: "message body\n# literal message heading", Kind: "request", InReplyTo: "parent-1", CorrelationID: "thread-1", ReplyTo: "agent:sender:inbox", Deadline: "2026-09-11T12:00:00Z", Subject: map[string]any{"subject": "value"}, Data: map[string]any{"data": "value"}}
		got, err := RenderPromptTemplate(template, t.TempDir(), RuntimePromptValues{
			OneShot:         "one-shot body\n# literal one-shot heading",
			Messages:        FormatMessages([]bus.Message{msg}),
			AwaitingReplies: FormatAwaitingReplies([]bus.Message{{ID: "awaiting-1", Channel: "agent:other:inbox", TS: "2026-09-11T10:00:00Z", Deadline: "2026-09-11T12:00:00Z"}}, time.Date(2026, 9, 11, 10, 1, 0, 0, time.UTC)),
			Goal:            FormatRuntimeGoal(tasks.Task{Key: "TEST-1", Title: "goal title", Priority: tasks.PriorityP1, Status: tasks.StatusWaitCustomer, Description: "goal body\n# literal goal heading"}),
		})
		if err != nil {
			t.Fatal(err)
		}
		last := -1
		for _, want := range []string{"# Task Processing Order\n", "one-shot, then messages, then goal", "## One-shot\n", "one-shot body\n# literal one-shot heading", "## Messages\n", "message body\n# literal message heading", "### Awaiting replies\n", "## Goal\n", "goal body\n# literal goal heading"} {
			at := strings.Index(got, want)
			if at <= last || strings.Count(got, want) != 1 {
				t.Fatalf("missing, duplicated or misordered %q in %s", want, got)
			}
			last = at
		}
		for _, want := range []string{"id message-1", "kind: request", "in_reply_to: parent-1", "correlation_id: thread-1", "reply_to: agent:sender:inbox", "deadline: 2026-09-11T12:00:00Z", `subject: {"subject":"value"}`, `data: {"data":"value"}`, messageProcessedInstruction, "id awaiting-1  channel agent:other:inbox  age 1m0s  deadline 2026-09-11T12:00:00Z", "key: TEST-1\ntitle: goal title\npriority: P1\nstatus: wait_customer", "wait for the customer answer", "do not merge it yourself"} {
			if !strings.Contains(got, want) {
				t.Fatalf("lost %q in %s", want, got)
			}
		}
		for _, old := range []string{"# [runtime: goal]", "# [runtime: messages]", "# [runtime: awaiting-replies]", "# [runtime: one-shot]", "\n# Agent Goal\n", "\n# Messages\n", "\n# Awaiting replies\n"} {
			if strings.Contains(got, old) {
				t.Fatalf("old standalone section %q in %s", old, got)
			}
		}
	}
}

func TestTaskProcessingOrderEmptyInputs(t *testing.T) {
	for _, awaiting := range []string{"", "# Awaiting replies\nwaiting"} {
		got, err := RenderPromptTemplate(runtimeTemplate(t, "identity"), t.TempDir(), RuntimePromptValues{AwaitingReplies: awaiting})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"# Task Processing Order", "No one-shot instruction for this iteration.", "No incoming messages for this iteration.", "No goal selected for this iteration.", "do not run commands merely to look for that absent input"} {
			if !strings.Contains(got, want) {
				t.Fatalf("missing %q in %s", want, got)
			}
		}
		if awaiting != "" && !strings.Contains(got, "### Awaiting replies\nwaiting") {
			t.Fatal(got)
		}
	}
}

func TestLegacyTaskProcessingOrder(t *testing.T) {
	got := AssemblePrompt(PromptParts{OneShot: "run first", Messages: "message second", AwaitingReplies: "# Awaiting replies\nreply state", Goal: "# Agent Goal\n\nkey: LEGACY-1\ndescription: selected legacy task", Tail: "finish"})
	last := -1
	for _, want := range []string{"# Task Processing Order", "## One-shot", "run first", "## Messages", "message second", "### Awaiting replies", "## Goal", "key: LEGACY-1\ndescription: selected legacy task", "finish"} {
		at := strings.Index(got, want)
		if at <= last {
			t.Fatalf("missing or misordered %q in %s", want, got)
		}
		last = at
	}
}
