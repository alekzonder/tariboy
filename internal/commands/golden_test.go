package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// goldenRecorder collects handler responses for a golden comparison: each
// value is encoded as JSON, the replacements are applied, and the volatile
// keys are masked, so the recorded responses are stable across runs.
type goldenRecorder struct {
	t *testing.T
	// volatile names keys whose values depend on the wall clock or randomness.
	volatile map[string]bool
	// replacements holds old, new pairs applied to the encoded JSON.
	replacements []string
	got          map[string]any
}

func newGoldenRecorder(t *testing.T, volatile map[string]bool, replacements ...string) *goldenRecorder {
	t.Helper()
	return &goldenRecorder{t: t, volatile: volatile, replacements: replacements, got: map[string]any{}}
}

func (r *goldenRecorder) record(label string, value any) {
	r.t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		r.t.Fatalf("%s: %v", label, err)
	}
	encoded := string(data)
	for i := 0; i+1 < len(r.replacements); i += 2 {
		if r.replacements[i] != "" {
			encoded = strings.ReplaceAll(encoded, r.replacements[i], r.replacements[i+1])
		}
	}
	var decoded any
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		r.t.Fatal(err)
	}
	r.got[label] = r.mask(decoded)
}

func (r *goldenRecorder) mask(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if r.volatile[key] && item != "" {
				v[key] = "$VOLATILE"
			} else {
				v[key] = r.mask(item)
			}
		}
	case []any:
		for i, item := range v {
			v[i] = r.mask(item)
		}
	}
	return value
}

// check compares the recorded responses with testdata/name; update rewrites
// the file first.
func (r *goldenRecorder) check(name string, update bool) {
	r.t.Helper()
	golden := filepath.Join("testdata", name)
	if update {
		data, err := json.MarshalIndent(r.got, "", "  ")
		if err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(golden, append(data, '\n'), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
	data, err := os.ReadFile(golden)
	if err != nil {
		r.t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal(data, &want); err != nil {
		r.t.Fatal(err)
	}
	for label := range want {
		if !reflect.DeepEqual(r.got[label], want[label]) {
			gotJSON, _ := json.MarshalIndent(r.got[label], "", "  ")
			wantJSON, _ := json.MarshalIndent(want[label], "", "  ")
			r.t.Errorf("%s response changed:\n got: %s\nwant: %s", label, gotJSON, wantJSON)
		}
	}
	if len(r.got) != len(want) {
		r.t.Errorf("recorded %d responses, golden has %d", len(r.got), len(want))
	}
}
