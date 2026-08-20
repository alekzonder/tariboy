package image

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestManifestRoundTrip(t *testing.T) {
	m := Manifest{
		SchemaVersion: 1,
		Name:          "app",
		Tag:           "latest",
		Digest:        "deadbeef",
		BuiltAt:       "2026-07-05T00:00:00Z",
		Parents:       []string{"base:latest"},
		Plugins:       []ManifestPlugin{{Name: "whoami"}, {Name: "status", Version: ">=0.3"}},
		Skills: []ManifestSkill{{
			Name: "code-review", Description: "Review changes safely.", Source: "./skills/code-review", Category: "source",
			ArchiveRoot: "skills/code-review", FileCount: 3, Size: 2048, TreeSHA256: "abc123",
		}},
		RequiresSecrets: []string{"JIRA_TOKEN"},
		Harness:         ManifestHarness{Type: "claude", Model: "sonnet", Effort: "medium", Interactive: true},
		Env:             map[string]string{"APP_ENV": "prod"},
		Policy:          ManifestPolicy{ToolsAllow: []string{"context.*"}, ToolsDeny: []string{"scripts.*"}},
		Evals:           []ManifestEval{{Name: "t", Type: "llm-judge", Prompt: "/abs/eval.md"}},
		Layers:          []Layer{{Name: "system", SHA256: "aa"}, {Name: "task.md", SHA256: "bb"}, {Name: "tail", SHA256: "cc"}},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var got Manifest
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, got) {
		t.Fatalf("round trip mismatch:\n%+v\n%+v", m, got)
	}
}
