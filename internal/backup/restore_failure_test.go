package backup

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

func builtMeta(t *testing.T, src *store.Store, agentsDir string) (Meta, map[string][]byte) {
	t.Helper()
	var arc bytes.Buffer
	if _, err := Build(&arc, src, agentsDir, "bot", Options{}, clk); err != nil {
		t.Fatal(err)
	}
	meta, files, err := readArchive(bytes.NewReader(arc.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return meta, files
}

func archiveWithFiles(t *testing.T, meta Meta, files map[string][]byte) []byte {
	t.Helper()
	var extra []tarEntry
	for name, data := range files {
		extra = append(extra, tarEntry{name: "files/" + name, data: data, typ: tar.TypeReg})
	}
	return archiveWith(t, meta, extra)
}

func countRows(t *testing.T, s *store.Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.DB.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Catches a failed forced restore keeping its deletes: every row of the agent
// being overwritten must survive a failure part way through.
func TestRestoreForceFailureKeepsExistingRows(t *testing.T) {
	src, agentsDir := seed(t)
	meta, files := builtMeta(t, src, agentsDir)
	schedulesBefore := countRows(t, src, `SELECT COUNT(*) FROM schedules WHERE agent='bot'`)
	mailBefore := countRows(t, src, `SELECT COUNT(*) FROM mail`)
	// A second row with the same primary key fails the insert after the delete.
	meta.Tables["schedules"] = append(meta.Tables["schedules"], meta.Tables["schedules"][0])
	arc := archiveWithFiles(t, meta, files)

	if _, err := Restore(bytes.NewReader(arc), src, agentsDir, RestoreOptions{Force: true}); err == nil {
		t.Fatal("restore with a duplicate key must fail")
	}
	if got := countRows(t, src, `SELECT COUNT(*) FROM agents WHERE name='bot'`); got != 1 {
		t.Fatalf("agent rows = %d", got)
	}
	if got := countRows(t, src, `SELECT COUNT(*) FROM schedules WHERE agent='bot'`); got != schedulesBefore {
		t.Fatalf("schedules = %d, want %d", got, schedulesBefore)
	}
	if got := countRows(t, src, `SELECT COUNT(*) FROM mail`); got != mailBefore {
		t.Fatalf("mail = %d, want %d", got, mailBefore)
	}
}

// Catches a refused restore touching a target that exists without Force.
func TestRestoreRefusesExistingWithoutForce(t *testing.T) {
	src, agentsDir := seed(t)
	meta, files := builtMeta(t, src, agentsDir)
	_, err := Restore(bytes.NewReader(archiveWithFiles(t, meta, files)), src, agentsDir, RestoreOptions{})
	if err == nil || err.Error() != `agent "bot" already exists (use --force to overwrite)` {
		t.Fatalf("err = %v", err)
	}
}

// Catches files being unpacked before a failed transaction: a failed restore
// leaves no agent directory.
func TestRestoreFailureUnpacksNoFiles(t *testing.T) {
	src, agentsDir := seed(t)
	meta, files := builtMeta(t, src, agentsDir)
	meta.Tables["schedules"] = append(meta.Tables["schedules"], meta.Tables["schedules"][0])
	dst := storetest.Open(t)
	dstAgents := filepath.Join(t.TempDir(), "agents")
	if _, err := Restore(bytes.NewReader(archiveWithFiles(t, meta, files)), dst, dstAgents, RestoreOptions{}); err == nil {
		t.Fatal("restore with a duplicate key must fail")
	}
	if got := countRows(t, dst, `SELECT COUNT(*) FROM agents`); got != 0 {
		t.Fatalf("agent rows = %d", got)
	}
	if _, err := os.Stat(dstAgents); !os.IsNotExist(err) {
		t.Fatalf("agents dir stat = %v, want absent", err)
	}
}

// Catches the row check order: a letter that is already present is skipped
// before its columns are checked, because the row is not written.
func TestRestoreSkipsPresentLetterBeforeColumnCheck(t *testing.T) {
	src, agentsDir := seed(t)
	meta, files := builtMeta(t, src, agentsDir)
	marked := 0
	for _, row := range meta.Tables["mail"] {
		if row["sender"] == "agent:bot" {
			row["no_such_column"] = "v"
			marked++
		}
	}
	if marked != 1 {
		t.Fatalf("seed has %d letters sent by bot, want 1", marked)
	}
	if err := agent.NewStore(src).Delete("bot"); err != nil {
		t.Fatal(err)
	}
	if err := agent.NewStore(src).PurgeAgentData("bot"); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(bytes.NewReader(archiveWithFiles(t, meta, files)), src, agentsDir, RestoreOptions{}); err != nil {
		t.Fatalf("present letter must be skipped: %v", err)
	}
}

// Catches the commit and unpack order: files are unpacked after the commit,
// so an unpack failure leaves the committed rows in place.
func TestRestoreUnpackFailureKeepsCommittedRows(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, dstAgents string)
		files map[string][]byte
		want  string
	}{
		{
			name:  "agent root",
			setup: func(t *testing.T, dstAgents string) { mustWrite(t, dstAgents) },
			want:  "not a directory",
		},
		{
			// Sorted paths write the file "a" first; "a/b" then needs it as a directory.
			name:  "parent directory",
			setup: func(*testing.T, string) {},
			files: map[string][]byte{"a": []byte("1"), "a/b": []byte("2")},
			want:  "not a directory",
		},
		{
			name: "file write",
			setup: func(t *testing.T, dstAgents string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(agentdir.New(dstAgents, "bot").Root, "a"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
			files: map[string][]byte{"a": []byte("1")},
			want:  "is a directory",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, agentsDir := seed(t)
			meta, files := builtMeta(t, src, agentsDir)
			if tc.files != nil {
				files = tc.files
			}
			dst := storetest.Open(t)
			dstAgents := filepath.Join(t.TempDir(), "agents")
			tc.setup(t, dstAgents)
			_, err := Restore(bytes.NewReader(archiveWithFiles(t, meta, files)), dst, dstAgents, RestoreOptions{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			agents, err := agent.NewStore(dst).List()
			if err != nil {
				t.Fatal(err)
			}
			if len(agents) != 1 || agents[0].Name != "bot" {
				t.Fatalf("agents = %#v", agents)
			}
		})
	}
}

func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}
