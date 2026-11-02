package loop

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/tasks"
)

func TestTaskProcessingOrderIndependentOfTemplate(t *testing.T) {
	t.Parallel()
	for _, names := range [][]string{nil, {"goal", "awaiting-replies", "messages", "one-shot"}, {"messages"}} {
		template := image.PromptTemplate{SchemaVersion: 2}
		for _, name := range names {
			template.Entries = append(template.Entries, image.TemplateEntry{Kind: "runtime", Runtime: name})
		}
		template.SHA256 = promptTemplateSHA(t, template)
		msg := mail.Letter{ID: "message-1", From: "agent:sender", Subject: "subject value", Body: "message body\n# literal message heading", Kind: "message", NeedsReply: true, InReplyTo: "parent-1", Deadline: "2026-09-11T12:00:00Z", Data: map[string]any{"data": "value"}, Attempts: 1}
		got, err := RenderPromptTemplate(template, t.TempDir(), RuntimePromptValues{
			OneShot:         "one-shot body\n# literal one-shot heading",
			Messages:        FormatMessages([]mail.Letter{msg}),
			AwaitingReplies: FormatAwaitingReplies([]mail.Letter{{ID: "awaiting-1", To: "agent:other", CreatedAt: "2026-09-11T10:00:00.000000000Z", Deadline: "2026-09-11T12:00:00Z"}}, time.Date(2026, 9, 11, 10, 1, 0, 0, time.UTC)),
			Goal:            FormatRuntimeGoal(tasks.Task{Key: "TEST-1", Title: "goal title", Priority: tasks.PriorityP1, Status: tasks.StatusWaitCustomer, Description: "goal body\n# literal goal heading"}),
			Mode:            IterationModeFlexible,
		})
		if err != nil {
			t.Fatal(err)
		}
		last := -1
		for _, want := range []string{"# Task Processing Order\n", "one-shot, then messages, then goal", "## One-shot\n", "one-shot body\n# literal one-shot heading", "## Messages\n", "  message body\n  # literal message heading", "### Awaiting replies\n", "## Goal\n", "goal body\n# literal goal heading"} {
			at := strings.Index(got, want)
			if at <= last || strings.Count(got, want) != 1 {
				t.Fatalf("missing, duplicated or misordered %q in %s", want, got)
			}
			last = at
		}
		for _, want := range []string{"id message-1  from agent:sender  kind message  attempts 1", "in_reply_to: parent-1", "deadline: 2026-09-11T12:00:00Z", "subject: subject value", `data: {"data":"value"}`, mailInstruction, "id awaiting-1  to agent:other  age 1m0s  deadline 2026-09-11T12:00:00Z", "key: TEST-1\ntitle: goal title\npriority: P1\nstatus: wait_customer", "wait for the customer answer", "do not merge it yourself"} {
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
	t.Parallel()
	for _, awaiting := range []string{"", "# Awaiting replies\nwaiting"} {
		got, err := RenderPromptTemplate(runtimeTemplate(t, "identity"), t.TempDir(), RuntimePromptValues{AwaitingReplies: awaiting, Mode: IterationModeRequest})
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

var iterationModeLines = map[IterationMode]string{
	IterationModeWorkflow: "Iteration mode: `workflow`. The goal task follows a workflow: the status instructions in the Goal section set the process.",
	IterationModeFlexible: "Iteration mode: `flexible`. The goal task has no workflow: your image instructions set the process.",
	IterationModeRequest:  "Iteration mode: `request`. No goal task is selected: work from the one-shot instruction or the messages.",
}

// assertModeLine checks the mode line is one paragraph after the opening
// lines, before the one-shot section, and appears once.
func assertModeLine(t *testing.T, got string, mode IterationMode) {
	t.Helper()
	line := iterationModeLines[mode]
	intro := "do not run commands merely to look for that absent input.\n\n"
	at := strings.Index(got, intro+line+"\n\n## One-shot\n")
	if at < 0 || strings.Count(got, "Iteration mode: ") != 1 || strings.Count(got, line) != 1 {
		t.Fatalf("mode %q line missing, misplaced or repeated in %s", mode, got)
	}
}

func TestTaskProcessingOrderModeLine(t *testing.T) {
	t.Parallel()
	for mode := range iterationModeLines {
		for _, names := range [][]string{{"identity"}, {"goal", "messages", "one-shot"}} {
			template := image.PromptTemplate{SchemaVersion: 2}
			for _, name := range names {
				template.Entries = append(template.Entries, image.TemplateEntry{Kind: "runtime", Runtime: name})
			}
			template.SHA256 = promptTemplateSHA(t, template)
			got, err := RenderPromptTemplate(template, t.TempDir(), RuntimePromptValues{Mode: mode, Goal: "# Agent Goal\n\nkey: MODE-1"})
			if err != nil {
				t.Fatal(err)
			}
			assertModeLine(t, got, mode)
		}
		got, err := AssemblePrompt(PromptParts{Agent: "a", Cwd: "/w", Mode: mode, ImagePrompt: "BODY"})
		if err != nil {
			t.Fatal(err)
		}
		assertModeLine(t, got, mode)
	}
}

func TestTaskProcessingOrderModeNotSet(t *testing.T) {
	t.Parallel()
	for _, mode := range []IterationMode{"", "bogus"} {
		got, err := RenderPromptTemplate(runtimeTemplate(t, "identity"), t.TempDir(), RuntimePromptValues{Mode: mode})
		if !errors.Is(err, ErrIterationModeNotSet) || got != "" {
			t.Fatalf("v2 mode %q: prompt %q, err %v", mode, got, err)
		}
		got, err = AssemblePrompt(PromptParts{Agent: "a", Mode: mode})
		if !errors.Is(err, ErrIterationModeNotSet) || got != "" {
			t.Fatalf("v1 mode %q: prompt %q, err %v", mode, got, err)
		}
		if err.Error() != "iteration mode is not set" {
			t.Fatalf("error text = %q", err)
		}
	}
}

func TestLegacyTaskProcessingOrder(t *testing.T) {
	t.Parallel()
	got, err := AssemblePrompt(PromptParts{OneShot: "run first", Messages: "message second", AwaitingReplies: "# Awaiting replies\nreply state", Goal: "# Agent Goal\n\nkey: LEGACY-1\ndescription: selected legacy task", Mode: IterationModeFlexible, Tail: "finish"})
	if err != nil {
		t.Fatal(err)
	}
	last := -1
	for _, want := range []string{"# Task Processing Order", "## One-shot", "run first", "## Messages", "message second", "### Awaiting replies", "## Goal", "key: LEGACY-1\ndescription: selected legacy task", "finish"} {
		at := strings.Index(got, want)
		if at <= last {
			t.Fatalf("missing or misordered %q in %s", want, got)
		}
		last = at
	}
}
