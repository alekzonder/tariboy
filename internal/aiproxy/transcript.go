package aiproxy

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

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

// AppendTranscript synchronously appends one entry (hot path, spec §9).
func AppendTranscript(agentsDir string, e TranscriptEntry) error {
	path := transcriptPath(agentsDir, e.Meta.Agent, e.Meta.Iteration)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// GzipTranscript compresses the JSONL at iteration close and removes the plain
// file. A missing transcript is not an error (no AI calls that iteration).
func GzipTranscript(agentsDir, agent, iteration string) error {
	path := transcriptPath(agentsDir, agent, iteration)
	in, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(path+".gz", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(out)
	if _, err := io.Copy(gz, in); err != nil {
		gz.Close()
		out.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		out.Close()
		return err
	}
	// fsync the .gz before removing the plain file: a crash after os.Remove but
	// before the .gz pages reach disk would otherwise lose the transcript.
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	in.Close()
	return os.Remove(path)
}
