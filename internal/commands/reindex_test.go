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

func TestReindexRestoresRecordedUsage(t *testing.T) {
	base := t.TempDir()
	st, err := store.Open(filepath.Join(base, "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	agentsDir := filepath.Join(base, "agents")
	if err := agentdir.New(agentsDir, "alice").EnsureIteration("alice-1"); err != nil {
		t.Fatal(err)
	}
	row := aiproxy.AIRequest{
		ID: "air-cursor", Agent: "alice", Iteration: "alice-1", Provider: "cursor",
		Model: "grok-4.7-high", InputTokens: 4841, OutputTokens: 31, CacheReadTokens: 5888, Status: "ok",
	}
	var ingested []aiproxy.AIRequest
	if err := aiproxy.RecordUsage(agentsDir, row, func(r aiproxy.AIRequest) { ingested = append(ingested, r) }); err != nil {
		t.Fatal(err)
	}
	if len(ingested) != 1 || ingested[0].ID != "air-cursor" {
		t.Fatalf("ingested = %+v", ingested)
	}

	if _, err := daemonReindex().Handler(&registry.Ctx{Store: st, BaseDir: base}, nil); err != nil {
		t.Fatal(err)
	}
	var provider string
	var in, out, cacheRead int64
	if err := st.DB.QueryRow(
		`SELECT provider, input_tokens, output_tokens, cache_read_tokens FROM ai_requests WHERE id='air-cursor'`,
	).Scan(&provider, &in, &out, &cacheRead); err != nil {
		t.Fatal(err)
	}
	if provider != "cursor" || in != 4841 || out != 31 || cacheRead != 5888 {
		t.Fatalf("reindexed row = %s %d %d %d", provider, in, out, cacheRead)
	}
}
