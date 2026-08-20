package commands

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store"
)

func TestReindexPreservesGroupSnapshotAndLeavesLegacyRowsUngrouped(t *testing.T) {
	base := t.TempDir()
	st, err := store.Open(filepath.Join(base, "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.DB.Exec(`INSERT INTO agents(name, image_ref, "group") VALUES ('alice', 'basic:latest', 'beta')`); err != nil {
		t.Fatal(err)
	}

	agentsDir := filepath.Join(base, "agents")
	layout := agentdir.New(agentsDir, "alice")
	if err := layout.EnsureIteration("alice-1"); err != nil {
		t.Fatal(err)
	}
	grouped := aiproxy.TranscriptEntry{Meta: aiproxy.AIRequest{
		ID: "air-grouped", Agent: "alice", Iteration: "alice-1",
		GroupID: "alpha", GroupName: "alpha",
	}}
	legacy := aiproxy.TranscriptEntry{Meta: aiproxy.AIRequest{
		ID: "air-legacy", Agent: "alice", Iteration: "alice-1",
	}}
	if err := aiproxy.AppendTranscript(agentsDir, grouped); err != nil {
		t.Fatal(err)
	}
	if err := aiproxy.AppendTranscript(agentsDir, legacy); err != nil {
		t.Fatal(err)
	}

	if _, err := daemonReindex().Handler(&registry.Ctx{Store: st, BaseDir: base}, nil); err != nil {
		t.Fatal(err)
	}

	read := func(id string) (groupID, groupName sql.NullString) {
		t.Helper()
		if err := st.DB.QueryRow(
			`SELECT group_id, group_name FROM ai_requests WHERE id=?`, id,
		).Scan(&groupID, &groupName); err != nil {
			t.Fatal(err)
		}
		return
	}
	groupID, groupName := read("air-grouped")
	if !groupID.Valid || groupID.String != "alpha" || !groupName.Valid || groupName.String != "alpha" {
		t.Fatalf("reindexed snapshot = group_id=%v group_name=%v", groupID, groupName)
	}
	groupID, groupName = read("air-legacy")
	if groupID.Valid || groupName.Valid {
		t.Fatalf("legacy row used current membership: group_id=%v group_name=%v", groupID, groupName)
	}
}
