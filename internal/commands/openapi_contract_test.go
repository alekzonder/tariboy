package commands

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

// commandsWithoutResultSchema lists the HTTP commands that still publish a
// free-form result. The list only shrinks: a command that gains a result
// schema leaves it in the same change, and review refuses new entries.
var commandsWithoutResultSchema = []string{
	"agent.alias.get",
	"agent.alias.set",
	"agent.color.get",
	"agent.color.set",
	"agent.cwd",
	"agent.effort",
	"agent.exec",
	"agent.file.create",
	"agent.file.delete",
	"agent.file.list",
	"agent.file.read",
	"agent.file.rename",
	"agent.file.write",
	"agent.harness",
	"agent.inbox.clear",
	"agent.inbox.ls",
	"agent.inbox.processed",
	"agent.inbox.reply",
	"agent.inbox.requeue",
	"agent.interactive",
	"agent.messages.batch",
	"agent.messages.max-queue",
	"agent.model",
	"agent.notes.get",
	"agent.notes.set",
	"agent.pull",
	"agent.screen",
	"agent.send-keys",
	"agent.shell-script.get",
	"agent.shell-script.set",
	"agent.status.history",
	"agent.subscribe",
	"agent.subscriptions",
	"agent.unsubscribe",
	"agent.usage",
	"backup",
	"budget.ls",
	"budget.set",
	"budget.status",
	"channel.inspect",
	"channel.ls",
	"channel.tail",
	"channel.watches",
	"chat.create",
	"chat.join",
	"chat.leave",
	"chat.ls",
	"chat.messages",
	"chat.participants",
	"chat.read",
	"chat.unanswered",
	"context.get",
	"context.set",
	"daemon.agent-shell-script.get",
	"daemon.agent-shell-script.set",
	"daemon.config.get",
	"daemon.config.set",
	"daemon.reindex",
	"daemon.status",
	"files.upload",
	"fs.list",
	"group.assign",
	"group.create",
	"group.inspect",
	"group.lead.set",
	"group.ls",
	"group.member.rm",
	"group.rename",
	"group.rm",
	"harness.codex-approval.get",
	"harness.codex-approval.set",
	"harness.models",
	"image.build",
	"image.file",
	"image.files",
	"image.inspect",
	"image.ls",
	"image.prompt",
	"image.provenance",
	"image.rm",
	"image.source.build",
	"image.source.create",
	"image.source.file.get",
	"image.source.file.put",
	"image.source.files",
	"image.source.inspect",
	"image.source.ls",
	"image.source.rm",
	"image.source.validate",
	"image.template",
	"image.validate",
	"iteration.extend-timeout",
	"iteration.inspect",
	"iteration.logs",
	"iteration.ls",
	"iteration.tag.add",
	"iteration.tag.rm",
	"iteration.tag.set",
	"logs",
	"loop.ai-stall-timeout",
	"loop.hard-timeout",
	"loop.interval",
	"loop.max-idle",
	"loop.on-error",
	"loop.on-timeout",
	"loop.schedule",
	"loop.timeout",
	"maintenance.get",
	"maintenance.run",
	"maintenance.set",
	"message.send",
	"plugin.action",
	"plugin.contributions",
	"plugin.inspect",
	"plugin.install",
	"plugin.logs",
	"plugin.ls",
	"plugin.restart",
	"plugin.rm",
	"plugin.routes",
	"prompt.get",
	"prune",
	"restore",
	"retention.get",
	"retention.set",
	"schedule.ls",
	"script.cancel",
	"script.logs",
	"script.ls",
	"script.rerun",
	"script.rm",
	"script.run",
	"script.run-get",
	"script.runs",
	"script.schedule",
	"secret.ls",
	"secret.rm",
	"secret.store",
	"store.add",
	"store.auto",
	"store.list",
	"store.refresh",
	"store.remove",
	"store.show",
	"tasks.activity",
	"team.compose",
	"team.import.archive.apply",
	"team.import.archive.status",
	"team.import.yaml",
	"transcript",
	"usage",
	"user-prompt.get",
	"user-prompt.set",
	"workflow.build",
	"workflow.inspect",
	"workflow.ls",
	"workflow.rm",
	"workflow.validate",
}

// Catches an HTTP command without a result schema outside the allowlist, and
// an allowlist entry that no longer names a schemaless HTTP command.
func TestHTTPCommandsDeclareResultSchema(t *testing.T) {
	t.Parallel()
	if !slices.IsSorted(commandsWithoutResultSchema) {
		t.Error("commandsWithoutResultSchema is not sorted")
	}
	allowed := map[string]bool{}
	for _, path := range commandsWithoutResultSchema {
		allowed[path] = true
	}
	schemaless := map[string]bool{}
	for _, c := range BuildRegistry().Commands() {
		if c.HTTP == nil || c.ResultSchema != nil {
			continue
		}
		schemaless[c.Path] = true
		if !allowed[c.Path] {
			t.Errorf("HTTP command %s declares no ResultSchema: wrap it in registry.WithResult", c.Path)
		}
	}
	for _, path := range commandsWithoutResultSchema {
		if !schemaless[path] {
			t.Errorf("allowlist entry %s is stale: the command has a result schema, no HTTP route, or no longer exists; remove the entry", path)
		}
	}
}

var updateOpenAPI = flag.Bool("update-openapi", false, "rewrite testdata/openapi.json")

// openAPISnapshot is the committed copy of GET /api/openapi.json and the
// input of generated client types, so a spec change is a deliberate commit
// that rewrites it.
const openAPISnapshot = "testdata/openapi.json"

// openAPISnapshotVersion replaces info.version, so a release leaves the
// snapshot unchanged.
const openAPISnapshotVersion = "snapshot"

const updateOpenAPIHint = "go test ./internal/commands -run TestOpenAPISnapshot -update-openapi"

// servedOpenAPI returns the result of GET /api/openapi.json, indented as the
// snapshot stores it.
func servedOpenAPI(t *testing.T) []byte {
	t.Helper()
	server := api.NewServer(BuildRegistry(), &registry.Ctx{Version: openAPISnapshotVersion, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/openapi.json", nil))
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	return indentOpenAPI(t, envelope.Result)
}

func indentOpenAPI(t *testing.T, spec []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := json.Indent(&out, spec, "", "  "); err != nil {
		t.Fatal(err)
	}
	out.WriteByte('\n')
	return out.Bytes()
}

// Catches a spec change committed without the updated snapshot.
func TestOpenAPISnapshot(t *testing.T) {
	got := servedOpenAPI(t)
	if *updateOpenAPI {
		if err := os.WriteFile(openAPISnapshot, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(openAPISnapshot)
	if err != nil {
		t.Fatalf("%v; create it with: %s", err, updateOpenAPIHint)
	}
	if diff := firstLineDiff(want, got); diff != "" {
		t.Fatalf("the served OpenAPI spec differs from %s at %s\nWhen the change is intended, update the snapshot in the same change:\n  %s", openAPISnapshot, diff, updateOpenAPIHint)
	}
}

// firstLineDiff describes the first line where want and got differ, or
// returns "" when they are equal.
func firstLineDiff(want, got []byte) string {
	if bytes.Equal(want, got) {
		return ""
	}
	wantLines, gotLines := strings.Split(string(want), "\n"), strings.Split(string(got), "\n")
	for i := 0; ; i++ {
		var w, g string
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g || i >= len(wantLines) || i >= len(gotLines) {
			return fmt.Sprintf("line %d:\n snapshot: %s\n   served: %s", i+1, w, g)
		}
	}
}
