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
	"reflect"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

// Catches an HTTP command whose result is not a declared component: every
// HTTP command must return a named struct (or a slice of one) declared with
// registry.WithResult, so its result schema is a component reference.
func TestHTTPCommandsDeclareResultSchema(t *testing.T) {
	t.Parallel()
	for _, c := range BuildRegistry().Commands() {
		if c.HTTP == nil {
			continue
		}
		schema := c.ResultSchema
		if schema["type"] == "array" {
			schema, _ = schema["items"].(map[string]any)
		}
		if ref, _ := schema["$ref"].(string); !strings.HasPrefix(ref, "#/components/schemas/") {
			t.Errorf("HTTP command %s result is %v, not a component reference: return a named struct declared with registry.WithResult", c.Path, c.ResultSchema)
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

// Catches two commands that declare one component name with different
// schemas: the served spec keeps only one of them, so the other command
// would publish a wrong result schema.
func TestCommandsDeclareConsistentComponents(t *testing.T) {
	t.Parallel()
	if err := componentConflicts(BuildRegistry().Commands()); err != "" {
		t.Error(err)
	}
}

func componentConflicts(commands []registry.Command) string {
	declared := map[string]registry.Command{}
	var problems []string
	for _, c := range commands {
		for name, schema := range c.Schemas {
			if first, ok := declared[name]; ok && !reflect.DeepEqual(first.Schemas[name], schema) {
				problems = append(problems, fmt.Sprintf("component %s: commands %s and %s declare different schemas; give one type a distinct name", name, first.Path, c.Path))
				continue
			}
			declared[name] = c
		}
	}
	return strings.Join(problems, "\n")
}

func TestComponentConflictsReportsDifferentSchemas(t *testing.T) {
	t.Parallel()
	a := registry.Command{Path: "a.get", Schemas: map[string]map[string]any{"Item": {"type": "object"}}}
	b := registry.Command{Path: "b.get", Schemas: map[string]map[string]any{"Item": {"type": "string"}}}
	same := registry.Command{Path: "c.get", Schemas: map[string]map[string]any{"Item": {"type": "object"}}}
	if got := componentConflicts([]registry.Command{a, same}); got != "" {
		t.Errorf("equal schemas reported: %s", got)
	}
	if got := componentConflicts([]registry.Command{a, b}); !strings.Contains(got, "component Item: commands a.get and b.get") {
		t.Errorf("conflict = %q", got)
	}
}
