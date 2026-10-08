package harness

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func modelIDs(catalog Catalog) []string {
	ids := make([]string, 0, len(catalog.Models))
	for _, model := range catalog.Models {
		ids = append(ids, model.ID)
	}
	return ids
}

func TestParseCodexModelsKeepsListedModelsWithTheirEfforts(t *testing.T) {
	// Captured from `codex debug models` (codex-cli 0.153.4), trimmed to the
	// fields the parser reads.
	raw, err := os.ReadFile(filepath.Join("testdata", "codex-debug-models.json"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := parseCodexModels(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := modelIDs(catalog), []string{"gpt-6-astra", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %v, want %v", got, want)
	}
	if catalog.Models[0].Label != "GPT-6-Astra" {
		t.Fatalf("label = %q", catalog.Models[0].Label)
	}
	if got, want := catalog.Models[3].Efforts, []string{"low", "medium", "high", "xhigh", "max"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("luna efforts = %v, want %v", got, want)
	}
	if got, want := catalog.Efforts, []string{"low", "medium", "high", "xhigh", "max", "ultra"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("harness efforts = %v, want %v", got, want)
	}
}

func TestParseCodexModelsRejectsGarbage(t *testing.T) {
	for _, raw := range []string{"", "not json", `{"models":[]}`} {
		if _, err := parseCodexModels([]byte(raw)); err == nil {
			t.Fatalf("parseCodexModels(%q) succeeded", raw)
		}
	}
}

func TestParseCursorModels(t *testing.T) {
	// Shape of `agent models` as documented by the Cursor CLI; not captured
	// from a real install because the CLI is absent on the development host.
	raw := "\x1b[1mAvailable models\x1b[0m\n\nauto - Auto\ngpt-5 - GPT-5  (current)\nsonnet-4.5-thinking - Claude 4.5 Sonnet (Thinking) (default)\n\nTip: use --model <id> to switch.\n"
	catalog, err := parseCursorModels([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	want := []ModelInfo{
		{ID: "auto", Label: "Auto"},
		{ID: "gpt-5", Label: "GPT-5"},
		{ID: "sonnet-4.5-thinking", Label: "Claude 4.5 Sonnet (Thinking)"},
	}
	if !reflect.DeepEqual(catalog.Models, want) {
		t.Fatalf("models = %#v, want %#v", catalog.Models, want)
	}
	if _, err := parseCursorModels([]byte("No models available\n")); err == nil {
		t.Fatal("output without models parsed")
	}
}

func TestParseOpenCodeModels(t *testing.T) {
	// One provider/model per line, as `opencode models` documents it.
	raw := "anthropic/claude-sonnet-4-5\n\nopenai/gpt-5\nnot a model line\n"
	catalog, err := parseOpenCodeModels([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := modelIDs(catalog), []string{"anthropic/claude-sonnet-4-5", "openai/gpt-5"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %v, want %v", got, want)
	}
	if _, err := parseOpenCodeModels([]byte("\n")); err == nil {
		t.Fatal("empty output parsed")
	}
}

func TestListModelsClaudeIsFixed(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	catalog, err := ListModels(context.Background(), "claude", true)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := modelIDs(catalog), []string{"fable", "opus", "opus[1m]", "sonnet", "haiku"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %v, want %v", got, want)
	}
	if got, want := catalog.Efforts, []string{"low", "medium", "high", "xhigh", "max"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("efforts = %v, want %v", got, want)
	}
	if catalog.Error != "" {
		t.Fatalf("error = %q", catalog.Error)
	}
}

func TestListModelsRejectsUnknownHarness(t *testing.T) {
	if _, err := ListModels(context.Background(), "nope", false); err == nil {
		t.Fatal("unknown harness accepted")
	}
	catalog, err := ListModels(context.Background(), "stub", false)
	if err != nil || len(catalog.Models) != 0 || catalog.Error != "" {
		t.Fatalf("stub catalog = %+v, %v", catalog, err)
	}
}

func fakeHarness(t *testing.T, name, script string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/usr/bin:/bin")
	return dir
}

func TestListModelsReportsMissingCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	catalog, err := ListModels(context.Background(), "opencode", true)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Harness != "opencode" || len(catalog.Models) != 0 || catalog.Error != "opencode not found on PATH" {
		t.Fatalf("catalog = %+v", catalog)
	}
}

func TestListModelsRunsTheCLIAndCachesSuccess(t *testing.T) {
	dir := fakeHarness(t, "opencode", `[ "$1" = models ] || exit 2
echo openai/gpt-5
`)
	catalog, err := ListModels(context.Background(), "opencode", true)
	if err != nil || catalog.Error != "" || !reflect.DeepEqual(modelIDs(catalog), []string{"openai/gpt-5"}) {
		t.Fatalf("catalog = %+v, %v", catalog, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte("#!/bin/sh\necho openai/gpt-6\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cached, _ := ListModels(context.Background(), "opencode", false)
	if !reflect.DeepEqual(modelIDs(cached), []string{"openai/gpt-5"}) {
		t.Fatalf("cached = %+v", cached)
	}
	fresh, _ := ListModels(context.Background(), "opencode", true)
	if !reflect.DeepEqual(modelIDs(fresh), []string{"openai/gpt-6"}) {
		t.Fatalf("refreshed = %+v", fresh)
	}
}

func TestListModelsReportsFailures(t *testing.T) {
	fakeHarness(t, "agent", "echo 'not logged in' >&2\nexit 3\n")
	catalog, err := ListModels(context.Background(), "cursor", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Models) != 0 || !strings.Contains(catalog.Error, "not logged in") {
		t.Fatalf("catalog = %+v", catalog)
	}

	// A failure is not cached: the next request asks the CLI again.
	fakeHarness(t, "agent", "echo 'auto - Auto'\n")
	catalog, _ = ListModels(context.Background(), "cursor", false)
	if catalog.Error != "" || !reflect.DeepEqual(modelIDs(catalog), []string{"auto"}) {
		t.Fatalf("after failure catalog = %+v", catalog)
	}
}

func TestListModelsTimesOut(t *testing.T) {
	old := modelListTimeout
	modelListTimeout = 200 * time.Millisecond
	t.Cleanup(func() { modelListTimeout = old })
	fakeHarness(t, "codex", "sleep 5\n")
	start := time.Now()
	catalog, _ := ListModels(context.Background(), "codex", true)
	if time.Since(start) > 3*time.Second || !strings.Contains(catalog.Error, "timed out") {
		t.Fatalf("catalog = %+v after %s", catalog, time.Since(start))
	}
}
