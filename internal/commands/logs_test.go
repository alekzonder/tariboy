package commands

import (
	"encoding/json"
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
)

// An agent with zero recorded events must serialize as an empty JSON array, not
// null. A nil slice marshals to `null`, which crashes the web UI audit-log page
// (events.map on null). Regression test for that.
func TestLogsEmptyEventsSerializesAsArray(t *testing.T) {
	c, _, _ := ctxWithStore(t)

	got, err := h(t, "logs")(c, registry.Params{"name": "nobody"})
	if err != nil {
		t.Fatal(err)
	}

	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Events []map[string]any `json:"events"`
		Count  int              `json:"count"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Events == nil {
		t.Fatalf("events serialized as null, want []: %s", b)
	}
	if resp.Count != 0 {
		t.Fatalf("count = %d, want 0", resp.Count)
	}
}
