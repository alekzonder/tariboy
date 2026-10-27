package aiproxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/alekzonder/tariboy/internal/agentdir"
)

// TranscriptEntry is one line of proxy-transcript.jsonl: request-time metadata
// (including nullable group snapshots) plus full bodies (spec §9 — the primary
// transcript). Bodies may contain secrets, so the file lives under the agent
// dir (0700) with mode 0600.
//
// Request/Response are OPAQUE BYTES, not json.RawMessage: this transcript is a
// source of truth, so a non-JSON body (e.g. an HTML 502 page) or a truncated
// body must still round-trip. encoding/json marshals []byte as a base64 string,
// which round-trips ANY bytes safely and never fails Marshal on the whole entry.
type TranscriptEntry struct {
	Meta     AIRequest `json:"meta"`
	Request  []byte    `json:"request,omitempty"`
	Response []byte    `json:"response,omitempty"`
}

func transcriptPath(agentsDir, agent, iteration string) string {
	return filepath.Join(agentdir.New(agentsDir, agent).IterationDir(iteration), "proxy-transcript.jsonl")
}

// transcriptLocks serialize AppendTranscript and GzipTranscript per path, so
// an append never lands in a plain file that the close-time gzip just removed.
var transcriptLocks [64]sync.Mutex

func lockTranscript(path string) func() {
	h := fnv.New32a()
	h.Write([]byte(path))
	mu := &transcriptLocks[h.Sum32()%uint32(len(transcriptLocks))]
	mu.Lock()
	return mu.Unlock
}

// AppendTranscript synchronously appends one entry (hot path, spec §9). After
// the iteration closed (plain file gone, .gz present) the entry becomes a new
// gzip member of the archive: a request still open at session stop finishes
// after the close-time gzip, and readers treat the members as one stream.
func AppendTranscript(agentsDir string, e TranscriptEntry) error {
	path := transcriptPath(agentsDir, e.Meta.Agent, e.Meta.Iteration)
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	defer lockTranscript(path)()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if _, err := os.Stat(path + ".gz"); err == nil {
			return appendGzipMember(path+".gz", bytes.NewReader(line))
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(line); err != nil {
		return err
	}
	return f.Sync()
}

// RecordUsage persists a row the proxy did not observe itself (cursor tokens
// read from harness stdout): it appends a body-less transcript entry, so daemon
// reindex can rebuild the row, then ingests it. Like persist, the row is
// ingested even when the append fails; the append error is returned.
func RecordUsage(agentsDir string, row AIRequest, ingest func(AIRequest)) error {
	err := AppendTranscript(agentsDir, TranscriptEntry{Meta: row})
	ingest(row)
	return err
}

// GzipTranscript compresses the JSONL at iteration close and removes the plain
// file. A missing transcript is not an error (no AI calls that iteration). An
// existing .gz is extended, never truncated, so no earlier entry is lost.
func GzipTranscript(agentsDir, agent, iteration string) error {
	path := transcriptPath(agentsDir, agent, iteration)
	defer lockTranscript(path)()
	in, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer in.Close()
	if err := appendGzipMember(path+".gz", in); err != nil {
		return err
	}
	in.Close()
	return os.Remove(path)
}

// appendGzipMember writes src as one gzip member at the end of path.
func appendGzipMember(path string, src io.Reader) error {
	out, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(out)
	if _, err := io.Copy(gz, src); err != nil {
		gz.Close()
		out.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		out.Close()
		return err
	}
	// fsync the .gz before the caller removes the plain file: a crash after
	// os.Remove but before the .gz pages reach disk would lose the transcript.
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
