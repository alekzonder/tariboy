package commands

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

func imageSHA256(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func TestContextGetSet(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "smoke", OnTimeout: "restart", OnError: "restart"})

	// missing CONTEXT.md reads as empty, not an error
	res, err := h(t, "context.get")(c, registry.Params{"name": "smoke"})
	if err != nil || res.(map[string]any)["context"] != "" {
		t.Fatalf("context.get on missing file: %v err=%v", res, err)
	}

	if _, err := h(t, "context.set")(c, registry.Params{"name": "smoke", "text": "remember X"}); err != nil {
		t.Fatal(err)
	}
	res, err = h(t, "context.get")(c, registry.Params{"name": "smoke"})
	if err != nil || res.(map[string]any)["context"] != "remember X" {
		t.Fatalf("context.get after set: %v err=%v", res, err)
	}

	l := agentdir.New(agentsDir(c), "smoke")
	data, err := os.ReadFile(l.ContextPath())
	if err != nil || string(data) != "remember X" {
		t.Fatalf("CONTEXT.md on disk = %q err=%v", data, err)
	}
}

func TestContextGetUnknownAgent(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	if _, err := h(t, "context.get")(c, registry.Params{"name": "ghost"}); err == nil {
		t.Fatal("expected not_found error")
	}
}

func TestPromptGet(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "smoke", UserPrompt: "focus on X", OnTimeout: "restart", OnError: "restart"})

	l := agentdir.New(agentsDir(c), "smoke")
	if err := os.MkdirAll(l.ImageDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.ImageDir(), "PROMPT.md"), []byte("# image prompt"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema_version":1,"name":"basic","tag":"latest","layers":[{"name":"system","sha256":"abc"}]}`
	if err := os.WriteFile(filepath.Join(l.ImageDir(), "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := h(t, "context.set")(c, registry.Params{"name": "smoke", "text": "remember X"}); err != nil {
		t.Fatal(err)
	}

	res, err := h(t, "prompt.get")(c, registry.Params{"name": "smoke"})
	if err != nil {
		t.Fatal(err)
	}
	out := res.(map[string]any)
	prompt, _ := out["prompt"].(string)
	for _, want := range []string{"# You are agent smoke", "# image prompt", "remember X", "focus on X"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	layers, ok := out["layers"].([]image.Layer)
	if !ok || len(layers) != 1 {
		t.Fatalf("layers = %v", out["layers"])
	}
}

func TestPromptGetNotProvisioned(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "smoke", OnTimeout: "restart", OnError: "restart"})
	if _, err := h(t, "prompt.get")(c, registry.Params{"name": "smoke"}); err == nil {
		t.Fatal("expected error for unprovisioned agent")
	}
}

func TestPromptPreviewV2UsesTemplateWithoutDrainingRuntimeMessages(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "v2", UserPrompt: "USER", Enabled: true, LoopEnabled: true, OnTimeout: "restart", OnError: "restart"})
	taskService := tasks.NewService(c.Store.DB, "customer", time.Now)
	if _, err := taskService.CreateQueue(context.Background(), tasks.CustomerActor("customer"), tasks.CreateQueueInput{Prefix: "NOGL", Name: "No goal runtime"}); err != nil {
		t.Fatal(err)
	}
	if _, err := taskService.CreateTask(context.Background(), tasks.CustomerActor("customer"), tasks.CreateTaskInput{
		Queue: "NOGL", Title: "Must remain unselected", Assignee: "agent:v2",
	}); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(agentsDir(c), "v2")
	if err := os.MkdirAll(filepath.Join(l.ImageDir(), "prompt", "layers"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.ImageDir(), "manifest.json"), []byte(`{"schema_version":2,"name":"x","tag":"latest"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.ImageDir(), "prompt", "layers", "000.md"), []byte("STATIC"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries := []image.TemplateEntry{{Kind: "file", ArchivePath: "prompt/layers/000.md", Size: 6, SHA256: imageSHA256("STATIC")}, {Kind: "runtime", Runtime: "messages"}, {Kind: "runtime", Runtime: "user-prompt"}}
	templateSHA, err := image.PromptTemplateHash(entries)
	if err != nil {
		t.Fatal(err)
	}
	template, err := json.Marshal(image.PromptTemplate{SchemaVersion: 2, Entries: entries, SHA256: templateSHA})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.ImageDir(), "prompt", "template.json"), template, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := h(t, "prompt.get")(c, registry.Params{"name": "v2"})
	if err != nil {
		t.Fatal(err)
	}
	prompt := res.(map[string]any)["prompt"].(string)
	if !strings.Contains(prompt, "STATIC") || !strings.Contains(prompt, "[runtime: messages]") || strings.Contains(prompt, "[runtime: awaiting-replies]") || !strings.Contains(prompt, "USER") {
		t.Fatalf("prompt = %q", prompt)
	}
	if strings.Count(prompt, "Use the `messages` skill") != 1 || strings.Count(prompt, "# Messages") != 1 {
		t.Fatalf("messages runtime was not rendered as one group: %q", prompt)
	}
	if got, err := as.Get("v2"); err != nil || got.CurrentGoalTaskKey != "" {
		t.Fatalf("template without runtime: goal selected %q, err=%v", got.CurrentGoalTaskKey, err)
	}
}

func TestPromptPreviewV2RendersAuthoritativeGoal(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	if err := as.Create(agent.Agent{
		Name: "v2", Enabled: true, LoopEnabled: true,
		OnTimeout: "restart", OnError: "restart",
	}); err != nil {
		t.Fatal(err)
	}
	taskService := tasks.NewService(c.Store.DB, "customer", func() time.Time {
		return time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	})
	ctx := context.Background()
	if _, err := taskService.CreateQueue(ctx, tasks.CustomerActor("customer"), tasks.CreateQueueInput{Prefix: "GOAL", Name: "Goals"}); err != nil {
		t.Fatal(err)
	}
	if _, err := taskService.CreateTask(ctx, tasks.CustomerActor("customer"), tasks.CreateTaskInput{
		Queue: "GOAL", Title: "Render goal", Description: "line one\nline two",
		Assignee: "agent:v2", Priority: tasks.PriorityP1,
	}); err != nil {
		t.Fatal(err)
	}

	l := agentdir.New(agentsDir(c), "v2")
	if err := os.MkdirAll(filepath.Join(l.ImageDir(), "prompt"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.ImageDir(), "manifest.json"), []byte(`{"schema_version":2,"name":"x","tag":"latest"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	entries := []image.TemplateEntry{{Kind: "runtime", Runtime: "goal"}}
	templateSHA, err := image.PromptTemplateHash(entries)
	if err != nil {
		t.Fatal(err)
	}
	template, err := json.Marshal(image.PromptTemplate{SchemaVersion: 2, Entries: entries, SHA256: templateSHA})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.ImageDir(), "prompt", "template.json"), template, 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := h(t, "prompt.get")(c, registry.Params{"name": "v2"})
	if err != nil {
		t.Fatal(err)
	}
	want := "Use the `tasks` skill for this runtime data.\n\n# Agent Goal\n\nkey: GOAL-1\ntitle: Render goal\npriority: P1\nstatus: open\ndescription: line one\nline two\n"
	if got := res.(map[string]any)["prompt"].(string); got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}
