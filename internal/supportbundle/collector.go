// Package supportbundle builds the daemon-owned, allowlisted half of a Desktop
// support archive. It never walks agent trees: every readable source is named
// explicitly below.
package supportbundle

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/store"
)

const (
	MaxIterations       = 10
	MaxAgentSourceBytes = 128 << 20
	MaxDaemonLogLines   = 200
	MaxDaemonLogBytes   = 64 << 10
	MaxDaemonLogRead    = 1 << 20
)

var ErrTooLarge = errors.New("support bundle agent data exceeds 128 MiB")

type StateSource interface {
	LiveState(name string) (string, error)
}

type Options struct {
	IncludeAgentData bool
	IterationLimit   int
}

type Archive interface {
	WriteZIP(io.Writer) error
}

type Collector struct {
	Store   *store.Store
	Control StateSource
	BaseDir string
	LogFile string
	Version string
	Now     func() time.Time
	Environ func() []string
}

type entry struct {
	Name string
	Body []byte
}

type preparedArchive struct {
	entries []entry
}

type issue struct {
	Path string `json:"path"`
	Code string `json:"code"`
}

type diagnostics struct {
	GeneratedAt      string  `json:"generated_at"`
	DaemonVersion    string  `json:"daemon_version"`
	Platform         string  `json:"platform"`
	Architecture     string  `json:"architecture"`
	AgentData        bool    `json:"agent_data_included"`
	IterationLimit   int     `json:"iteration_limit"`
	MissingOrUnread  []issue `json:"missing_or_unread,omitempty"`
	RedactionVersion int     `json:"redaction_policy_version"`
}

type agentDiagnostic struct {
	Name        string `json:"name"`
	Image       string `json:"image"`
	Harness     string `json:"harness"`
	State       string `json:"state"`
	Interactive bool   `json:"interactive"`
	LoopEnabled bool   `json:"loop_enabled"`
	Enabled     bool   `json:"enabled"`
}

type iterationDiagnostic struct {
	ID                  string  `json:"id"`
	Trigger             string  `json:"trigger"`
	Status              string  `json:"status"`
	StartedAt           string  `json:"started_at"`
	EndedAt             string  `json:"ended_at"`
	ExitCode            *int    `json:"exit_code,omitempty"`
	Done                bool    `json:"done"`
	Productive          bool    `json:"productive"`
	CPUMs               *int    `json:"cpu_ms,omitempty"`
	MemPeakKB           *int    `json:"mem_peak_kb,omitempty"`
	TimeoutPeriodS      *int    `json:"timeout_period_s,omitempty"`
	TimeoutDeadline     *string `json:"timeout_deadline,omitempty"`
	HardTimeoutDeadline *string `json:"hard_timeout_deadline,omitempty"`
	TimeoutExtensions   int     `json:"timeout_extensions"`
	TimeoutTriggeredAt  *string `json:"timeout_triggered_at,omitempty"`
}

func (c Collector) Prepare(ctx context.Context, opts Options) (Archive, error) {
	if c.Store == nil {
		return nil, errors.New("support bundle store is not configured")
	}
	if opts.IterationLimit == 0 {
		opts.IterationLimit = MaxIterations
	}
	if opts.IterationLimit < 1 || opts.IterationLimit > MaxIterations {
		return nil, fmt.Errorf("iteration limit must be between 1 and %d", MaxIterations)
	}
	now := c.Now
	if now == nil {
		now = time.Now
	}
	environ := c.Environ
	if environ == nil {
		environ = os.Environ
	}
	diag := diagnostics{
		GeneratedAt:      now().UTC().Format(time.RFC3339Nano),
		DaemonVersion:    c.Version,
		Platform:         runtime.GOOS,
		Architecture:     runtime.GOARCH,
		AgentData:        opts.IncludeAgentData,
		IterationLimit:   opts.IterationLimit,
		RedactionVersion: 1,
	}
	entries := make([]entry, 0, 4)
	daemonLog, logIssue := readDaemonLogTail(c.LogFile)
	if logIssue == "" {
		entries = append(entries, entry{Name: "logs/tariboyd.log", Body: safeDaemonLog(daemonLog)})
	} else {
		diag.MissingOrUnread = append(diag.MissingOrUnread, issue{
			Path: "logs/tariboyd.log", Code: logIssue,
		})
		entries = append(entries, entry{Name: "logs/tariboyd.log", Body: nil})
	}

	if opts.IncludeAgentData {
		agentEntries, issues, err := c.collectAgents(ctx, opts.IterationLimit, environ())
		if err != nil {
			return nil, err
		}
		entries = append(entries, agentEntries...)
		diag.MissingOrUnread = append(diag.MissingOrUnread, issues...)
	}
	body, err := json.MarshalIndent(diag, "", "  ")
	if err != nil {
		return nil, err
	}
	body = append(body, '\n')
	entries = append(entries, entry{Name: "diagnostics.json", Body: body})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return preparedArchive{entries: entries}, nil
}

func (c Collector) collectAgents(ctx context.Context, limit int, environ []string) ([]entry, []issue, error) {
	as := agent.NewStore(c.Store)
	agents, err := as.List()
	if err != nil {
		return nil, nil, err
	}
	entries := []entry{}
	issues := []issue{}
	index := make([]agentDiagnostic, 0, len(agents))
	segments := map[string]bool{}
	sourceBytes := int64(0)
	roots := sensitiveRoots(c.BaseDir)
	agentsRoot := paths.Paths{Base: c.BaseDir}.AgentsDir()
	for _, record := range agents {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		state := ""
		if c.Control != nil {
			state, _ = c.Control.LiveState(record.Name)
		}
		meta := agentDiagnostic{
			Name: record.Name, Image: record.ImageRef, Harness: record.HarnessType,
			State: state, Interactive: record.Interactive,
			LoopEnabled: record.LoopEnabled, Enabled: record.Enabled,
		}
		index = append(index, meta)
		segment := archiveSegment(record.Name, segments)
		segments[segment] = true
		agentBody, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			return nil, nil, err
		}
		entries = append(entries, entry{
			Name: "agents/" + segment + "/agent.json",
			Body: append(agentBody, '\n'),
		})
		iterations, err := as.ListIterations(record.Name)
		if err != nil {
			return nil, nil, err
		}
		if len(iterations) > limit {
			iterations = iterations[len(iterations)-limit:]
		}
		layout := agentdir.New(paths.Paths{Base: c.BaseDir}.AgentsDir(), record.Name)
		for index := len(iterations) - 1; index >= 0; index-- {
			iteration := iterations[index]
			iterSegment := archiveSegment(iteration.ID, map[string]bool{})
			base := "agents/" + segment + "/iterations/" + iterSegment
			meta := iterationDiagnostic{
				ID: iteration.ID, Trigger: iteration.Trigger, Status: iteration.Status,
				StartedAt: iteration.StartedAt, EndedAt: iteration.EndedAt,
				ExitCode: iteration.ExitCode, Done: iteration.DoneFlag,
				Productive: iteration.Productive, CPUMs: iteration.CPUMs,
				MemPeakKB: iteration.MemPeakKB, TimeoutPeriodS: iteration.TimeoutPeriodS,
				TimeoutDeadline:     iteration.TimeoutDeadline,
				HardTimeoutDeadline: iteration.HardTimeoutDeadline,
				TimeoutExtensions:   iteration.TimeoutExtensions,
				TimeoutTriggeredAt:  iteration.TimeoutTriggeredAt,
			}
			metaBody, err := json.MarshalIndent(meta, "", "  ")
			if err != nil {
				return nil, nil, err
			}
			entries = append(entries, entry{Name: base + "/iteration.json", Body: append(metaBody, '\n')})
			files := []struct {
				archive string
				source  string
			}{
				{"result.json", layout.ResultPath(iteration.ID)},
				{"logs/shim.log", layout.ShimLog(iteration.ID)},
				{"logs/harness.stdout.log", layout.HarnessStdout(iteration.ID)},
				{"logs/harness.stderr.log", layout.HarnessStderr(iteration.ID)},
			}
			for _, file := range files {
				archivePath := base + "/" + file.archive
				body, code, err := readAllowedSource(
					agentsRoot,
					file.source,
					MaxAgentSourceBytes-sourceBytes,
				)
				if err != nil {
					return nil, nil, err
				}
				if code != "" {
					issues = append(issues, issue{Path: archivePath, Code: code})
					continue
				}
				sourceBytes += int64(len(body))
				entries = append(entries, entry{
					Name: archivePath,
					Body: redactText(body, roots, environ),
				})
			}
		}
	}
	indexBody, err := json.MarshalIndent(map[string]any{"agents": index}, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	entries = append(entries, entry{Name: "agents/index.json", Body: append(indexBody, '\n')})
	return entries, issues, nil
}

func readAllowedSource(rootPath, path string, remaining int64) ([]byte, string, error) {
	return readAllowedSourceWithHook(rootPath, path, remaining, nil)
}

func readAllowedSourceWithHook(
	rootPath, path string,
	remaining int64,
	afterParent func(string),
) ([]byte, string, error) {
	relative, err := filepath.Rel(rootPath, path)
	if err != nil || filepath.IsAbs(relative) || relative == "." ||
		relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, "not_regular", nil
	}
	rootInfo, err := os.Lstat(rootPath)
	if err != nil {
		return nil, fileIssueCode(err), nil
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, "not_regular", nil
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, "read_failed", nil
	}
	defer func() { _ = root.Close() }()
	openedRootInfo, err := root.Stat(".")
	if err != nil || !os.SameFile(rootInfo, openedRootInfo) {
		return nil, "not_regular", nil
	}
	parts := strings.Split(relative, string(filepath.Separator))
	current := ""
	for _, part := range parts[:len(parts)-1] {
		parentInfo, err := root.Lstat(part)
		if err != nil {
			return nil, fileIssueCode(err), nil
		}
		if !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
			return nil, "not_regular", nil
		}
		child, err := root.OpenRoot(part)
		if err != nil {
			return nil, "read_failed", nil
		}
		openedParentInfo, err := child.Stat(".")
		if err != nil || !os.SameFile(parentInfo, openedParentInfo) {
			_ = child.Close()
			return nil, "not_regular", nil
		}
		_ = root.Close()
		root = child
		current = filepath.Join(current, part)
		if afterParent != nil {
			afterParent(current)
		}
	}
	leaf := parts[len(parts)-1]
	info, err := root.Lstat(leaf)
	if err != nil {
		return nil, fileIssueCode(err), nil
	}
	if !info.Mode().IsRegular() {
		return nil, "not_regular", nil
	}
	file, err := root.Open(leaf)
	if err != nil {
		return nil, "read_failed", nil
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, "read_failed", nil
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return nil, "not_regular", nil
	}
	if remaining < 0 || openedInfo.Size() > remaining {
		return nil, "", ErrTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(file, remaining+1))
	if err != nil {
		return nil, "read_failed", nil
	}
	if int64(len(body)) > remaining {
		return nil, "", ErrTooLarge
	}
	return body, "", nil
}

func readDaemonLogTail(path string) ([]byte, string) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fileIssueCode(err)
	}
	if !info.Mode().IsRegular() {
		return nil, "not_regular"
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "read_failed"
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return nil, "not_regular"
	}
	start := openedInfo.Size() - MaxDaemonLogRead
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil, "read_failed"
	}
	body, err := io.ReadAll(io.LimitReader(file, MaxDaemonLogRead))
	if err != nil {
		return nil, "read_failed"
	}
	if start > 0 {
		newline := bytes.IndexByte(body, '\n')
		if newline < 0 {
			return nil, ""
		}
		body = body[newline+1:]
	}
	return body, ""
}

func (a preparedArchive) WriteZIP(output io.Writer) error {
	writer := zip.NewWriter(output)
	for _, item := range a.entries {
		header := &zip.FileHeader{Name: item.Name, Method: zip.Deflate}
		header.SetMode(0o600)
		header.Modified = time.Unix(0, 0).UTC()
		file, err := writer.CreateHeader(header)
		if err != nil {
			_ = writer.Close()
			return err
		}
		if _, err := file.Write(item.Body); err != nil {
			_ = writer.Close()
			return err
		}
	}
	return writer.Close()
}

func archiveSegment(value string, seen map[string]bool) string {
	var output strings.Builder
	separator := false
	for _, char := range strings.ToLower(value) {
		if char <= unicode.MaxASCII && (unicode.IsLetter(char) || unicode.IsDigit(char)) {
			output.WriteRune(char)
			separator = false
		} else if output.Len() > 0 && !separator {
			output.WriteByte('-')
			separator = true
		}
		if output.Len() >= 48 {
			break
		}
	}
	segment := strings.Trim(output.String(), "-")
	if segment == "" || segment == "." || segment == ".." || seen[segment] {
		sum := sha256.Sum256([]byte(value))
		hash := hex.EncodeToString(sum[:])[:12]
		if segment == "" || segment == "." || segment == ".." {
			segment = "item"
		}
		segment = strings.TrimRight(segment, "-")
		if len(segment) > 35 {
			segment = segment[:35]
		}
		segment += "-" + hash
	}
	return segment
}

func sensitiveRoots(base string) []string {
	p := paths.New(base)
	return []string{
		base,
		p.RuntimeDir(),
		p.AgentsDir(),
	}
}

func fileIssueCode(err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return "missing"
	}
	return "read_failed"
}
