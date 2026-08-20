package supportbundle

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/store"
)

type fixedState map[string]string

func (s fixedState) LiveState(name string) (string, error) {
	return s[name], nil
}

func collectorFixture(t *testing.T, iterations int) (Collector, string) {
	t.Helper()
	root := t.TempDir()
	base := filepath.Join(root, "base")
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(base, "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	as := agent.NewStore(db)
	for _, name := range []string{"alpha", "beta"} {
		if err := as.Create(agent.Agent{
			Name: name, ImageRef: "bare:latest", HarnessType: "codex",
			Cwd: "/private/customer", UserPrompt: "PROMPT_DB_SENTINEL",
			Env:         map[string]string{"API_TOKEN": "ENV_DB_SENTINEL"},
			Interactive: true, LoopEnabled: true, Enabled: true,
		}); err != nil {
			t.Fatal(err)
		}
		layout := agentdir.New(filepath.Join(base, "agents"), name)
		if err := os.MkdirAll(layout.Workdir(), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(layout.ImageDir(), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(layout.ContextPath(), []byte("CONTEXT_SENTINEL"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(layout.AuditLog(), []byte("AUDIT_SENTINEL"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(layout.Workdir(), "customer.txt"), []byte("WORKDIR_SENTINEL"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(layout.ImageDir(), "private.bin"), []byte("IMAGE_SENTINEL"), 0o600); err != nil {
			t.Fatal(err)
		}
		for index := 1; index <= iterations; index++ {
			id := strings.Join([]string{name, "iteration", twoDigits(index)}, "-")
			started := time.Date(2026, 7, index, 10, 0, 0, 0, time.UTC).Format(time.RFC3339)
			exit := index
			if err := as.CreateIteration(agent.Iteration{
				ID: id, Agent: name, Trigger: "manual", Status: "harness_error",
				StartedAt: started, EndedAt: started, ExitCode: &exit, Productive: true,
				PromptPath: layout.PromptPath(id),
			}); err != nil {
				t.Fatal(err)
			}
			if err := layout.EnsureIteration(id); err != nil {
				t.Fatal(err)
			}
			allowed := map[string]string{
				layout.ResultPath(id):    `{"exit_code":1}`,
				layout.ShimLog(id):       "ERROR tmux executable file not found",
				layout.HarnessStdout(id): "visible stdout " + id,
				layout.HarnessStderr(id): "visible stderr " + id,
				layout.PromptPath(id):    "PROMPT_SENTINEL",
				filepath.Join(layout.IterationDir(id), "proxy-transcript.jsonl"):    "TRANSCRIPT_SENTINEL",
				filepath.Join(layout.IterationDir(id), "proxy-transcript.jsonl.gz"): "GZIP_TRANSCRIPT_SENTINEL",
			}
			for path, body := range allowed {
				if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	logFile := filepath.Join(root, "tariboyd.log")
	if err := os.WriteFile(logFile, []byte(
		"2026-07-29T10:00:00Z daemon started\n"+
			"2026-07-29T10:00:01Z Authorization: Bearer DAEMON_SECRET\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	return Collector{
		Store: db, Control: fixedState{"alpha": "error", "beta": "idle"},
		BaseDir: base, LogFile: logFile, Version: "0.11.0",
		Now: func() time.Time { return time.Date(2026, 7, 29, 10, 2, 0, 0, time.UTC) },
		Environ: func() []string {
			return []string{"CUSTOM_SECRET=abcdefgh12345678"}
		},
	}, base
}

func twoDigits(value int) string {
	return fmt.Sprintf("%02d", value)
}

func archiveFiles(t *testing.T, archive Archive) map[string][]byte {
	t.Helper()
	var output bytes.Buffer
	if err := archive.WriteZIP(&output); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		body, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[file.Name] = data
	}
	return files
}

func TestPrepareDefaultExcludesEveryAgentSource(t *testing.T) {
	collector, _ := collectorFixture(t, 1)
	archive, err := collector.Prepare(context.Background(), Options{IterationLimit: 10})
	if err != nil {
		t.Fatal(err)
	}
	files := archiveFiles(t, archive)
	if len(files) != 2 || files["diagnostics.json"] == nil || files["logs/tariboyd.log"] == nil {
		t.Fatalf("default entries = %v, want diagnostics and daemon log only", mapKeys(files))
	}
	all := concatenate(files)
	for _, forbidden := range []string{
		"alpha", "beta", "PROMPT_SENTINEL", "TRANSCRIPT_SENTINEL",
		"AUDIT_SENTINEL", "CONTEXT_SENTINEL", "WORKDIR_SENTINEL",
		"IMAGE_SENTINEL", "GZIP_TRANSCRIPT_SENTINEL", "PROMPT_DB_SENTINEL",
		"ENV_DB_SENTINEL", "DAEMON_SECRET",
	} {
		if strings.Contains(all, forbidden) {
			t.Fatalf("default archive leaked %q", forbidden)
		}
	}
}

func TestPrepareIncludesOnlyNewestTenAllowedIterationFiles(t *testing.T) {
	collector, _ := collectorFixture(t, 12)
	archive, err := collector.Prepare(context.Background(), Options{
		IncludeAgentData: true,
		IterationLimit:   10,
	})
	if err != nil {
		t.Fatal(err)
	}
	files := archiveFiles(t, archive)
	all := concatenate(files)
	for _, agentName := range []string{"alpha", "beta"} {
		if !strings.Contains(all, agentName+"-iteration-12") ||
			!strings.Contains(all, agentName+"-iteration-03") {
			t.Fatalf("archive does not contain newest iterations for %s", agentName)
		}
		for _, old := range []string{"iteration-01", "iteration-02"} {
			if strings.Contains(all, agentName+"-"+old) {
				t.Fatalf("archive contains old iteration %s-%s", agentName, old)
			}
		}
	}
	for _, wanted := range []string{
		"ERROR tmux executable file not found",
		"visible stdout alpha-iteration-12",
		"visible stderr beta-iteration-03",
	} {
		if !strings.Contains(all, wanted) {
			t.Fatalf("archive missing %q", wanted)
		}
	}
	for _, forbidden := range []string{
		"PROMPT_SENTINEL", "TRANSCRIPT_SENTINEL", "AUDIT_SENTINEL",
		"GZIP_TRANSCRIPT_SENTINEL", "CONTEXT_SENTINEL", "WORKDIR_SENTINEL",
		"IMAGE_SENTINEL", "PROMPT_DB_SENTINEL", "ENV_DB_SENTINEL",
		"/private/customer", "prompt_path",
	} {
		if strings.Contains(all, forbidden) {
			t.Fatalf("agent archive leaked %q", forbidden)
		}
	}
}

func TestPreparedArchiveIsDeterministic(t *testing.T) {
	collector, _ := collectorFixture(t, 2)
	archive, err := collector.Prepare(context.Background(), Options{
		IncludeAgentData: true,
		IterationLimit:   10,
	})
	if err != nil {
		t.Fatal(err)
	}
	var first, second bytes.Buffer
	if err := archive.WriteZIP(&first); err != nil {
		t.Fatal(err)
	}
	if err := archive.WriteZIP(&second); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("same prepared archive produced different ZIP bytes")
	}
}

func TestPrepareRejectsOversizedAgentDataWithoutTruncating(t *testing.T) {
	collector, base := collectorFixture(t, 1)
	path := agentdir.New(filepath.Join(base, "agents"), "alpha").HarnessStdout("alpha-iteration-01")
	if err := os.Truncate(path, MaxAgentSourceBytes+1); err != nil {
		t.Fatal(err)
	}
	_, err := collector.Prepare(context.Background(), Options{
		IncludeAgentData: true,
		IterationLimit:   10,
	})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Prepare oversized = %v, want ErrTooLarge", err)
	}
}

func TestPrepareRejectsSymlinkAgentSource(t *testing.T) {
	collector, base := collectorFixture(t, 1)
	target := filepath.Join(t.TempDir(), "customer-secret.txt")
	if err := os.WriteFile(target, []byte("SYMLINK_USER_FILE_SENTINEL"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := agentdir.New(filepath.Join(base, "agents"), "alpha").HarnessStdout("alpha-iteration-01")
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, source); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	archive, err := collector.Prepare(context.Background(), Options{
		IncludeAgentData: true,
		IterationLimit:   10,
	})
	if err != nil {
		t.Fatal(err)
	}
	all := concatenate(archiveFiles(t, archive))
	if strings.Contains(all, "SYMLINK_USER_FILE_SENTINEL") {
		t.Fatal("support bundle followed an allowlisted path to a user file")
	}
	if !strings.Contains(all, `"code": "not_regular"`) {
		t.Fatalf("symlink rejection was not reported: %s", all)
	}
}

func TestPrepareRejectsSymlinkIterationDirectory(t *testing.T) {
	collector, base := collectorFixture(t, 1)
	layout := agentdir.New(filepath.Join(base, "agents"), "alpha")
	iterationDir := layout.IterationDir("alpha-iteration-01")
	outside := filepath.Join(t.TempDir(), "outside-iteration")
	if err := os.Rename(iterationDir, outside); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "logs", "harness.stdout.log"), []byte("PARENT_SYMLINK_USER_FILE_SENTINEL"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, iterationDir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	archive, err := collector.Prepare(context.Background(), Options{
		IncludeAgentData: true,
		IterationLimit:   10,
	})
	if err != nil {
		t.Fatal(err)
	}
	all := concatenate(archiveFiles(t, archive))
	if strings.Contains(all, "PARENT_SYMLINK_USER_FILE_SENTINEL") {
		t.Fatal("support bundle followed a symlinked iteration directory")
	}
	if !strings.Contains(all, `"code": "not_regular"`) {
		t.Fatalf("parent symlink rejection was not reported: %s", all)
	}
}

func TestReadAllowedSourceKeepsValidatedParentOpenDuringSwap(t *testing.T) {
	root := t.TempDir()
	iterationDir := filepath.Join(root, "alpha", "iterations", "it-1")
	logDir := filepath.Join(iterationDir, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(logDir, "harness.stdout.log")
	if err := os.WriteFile(source, []byte("SAFE_ORIGINAL_LOG"), 0o600); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(root, "alpha", "workdir", "replacement", "logs")
	if err := os.MkdirAll(replacement, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(replacement, "harness.stdout.log"), []byte("TOCTOU_USER_FILE_SENTINEL"), 0o600); err != nil {
		t.Fatal(err)
	}
	saved := filepath.Join(root, "saved-iteration")
	swapped := false
	afterParent := func(relative string) {
		if relative != filepath.Join("alpha", "iterations", "it-1") || swapped {
			return
		}
		swapped = true
		if err := os.Rename(iterationDir, saved); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("..", "workdir", "replacement"), iterationDir); err != nil {
			t.Fatal(err)
		}
	}

	body, code, err := readAllowedSourceWithHook(root, source, 1<<20, afterParent)
	if err != nil {
		t.Fatal(err)
	}
	if code != "" {
		t.Fatalf("readAllowedSourceWithHook code = %q", code)
	}
	if string(body) != "SAFE_ORIGINAL_LOG" {
		t.Fatalf("parent swap changed source to %q", body)
	}
	if !swapped {
		t.Fatal("test did not exercise parent swap barrier")
	}
}

func TestPrepareReadsOnlyBoundedDaemonLogTail(t *testing.T) {
	collector, _ := collectorFixture(t, 1)
	body := "2026-07-29T09:00:00Z daemon started code=old_prefix\n" +
		strings.Repeat("x", 2<<20) +
		"\n2026-07-29T10:00:00Z daemon failed code=tail_failure\n"
	if err := os.WriteFile(collector.LogFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	archive, err := collector.Prepare(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	log := string(archiveFiles(t, archive)["logs/tariboyd.log"])
	if strings.Contains(log, "old_prefix") {
		t.Fatalf("collector read beyond bounded daemon log tail: %q", log)
	}
	if !strings.Contains(log, "tail_failure") {
		t.Fatalf("collector lost newest lifecycle line: %q", log)
	}
}

func TestArchiveSegmentCannotCreateUnsafeOrDuplicatePaths(t *testing.T) {
	seen := map[string]bool{}
	for _, input := range []string{"../../escape", "a/b", "a\\b", "\x00\n", "тест", "١"} {
		segment := archiveSegment(input, seen)
		if segment == "" || strings.ContainsAny(segment, `/\`) || strings.Contains(segment, "..") {
			t.Fatalf("archiveSegment(%q) = %q", input, segment)
		}
		if strings.IndexFunc(segment, func(char rune) bool { return char > unicode.MaxASCII }) >= 0 {
			t.Fatalf("archiveSegment(%q) contains non-ASCII characters: %q", input, segment)
		}
		if seen[segment] {
			t.Fatalf("archiveSegment(%q) reused %q", input, segment)
		}
		seen[segment] = true
	}
}

func TestMissingAllowedFilesAreReportedWithoutAbsolutePaths(t *testing.T) {
	collector, base := collectorFixture(t, 1)
	missing := agentdir.New(filepath.Join(base, "agents"), "alpha").HarnessStderr("alpha-iteration-01")
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	archive, err := collector.Prepare(context.Background(), Options{
		IncludeAgentData: true,
		IterationLimit:   10,
	})
	if err != nil {
		t.Fatal(err)
	}
	files := archiveFiles(t, archive)
	var diagnostics map[string]any
	if err := json.Unmarshal(files["diagnostics.json"], &diagnostics); err != nil {
		t.Fatal(err)
	}
	body := string(files["diagnostics.json"])
	if !strings.Contains(body, `"code": "missing"`) ||
		!strings.Contains(body, "harness.stderr.log") {
		t.Fatalf("missing report = %s", body)
	}
	if strings.Contains(body, base) {
		t.Fatalf("diagnostics leaked base path: %s", body)
	}
}

func concatenate(files map[string][]byte) string {
	var output strings.Builder
	for name, body := range files {
		output.WriteString(name)
		output.WriteByte('\n')
		output.Write(body)
		output.WriteByte('\n')
	}
	return output.String()
}

func mapKeys(files map[string][]byte) []string {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	return keys
}
