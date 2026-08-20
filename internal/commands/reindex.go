package commands

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/registry"
)

func daemonReindex() registry.Command {
	return registry.Command{
		Path:    "daemon.reindex",
		Summary: "Rebuild ai_requests metadata from proxy-transcript.jsonl files",
		HTTP:    &registry.HTTPRoute{Method: "POST", Path: "/api/daemon/reindex"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			agentsDir := filepath.Join(c.BaseDir, "agents")
			rows, err := scanTranscripts(agentsDir)
			if err != nil {
				return nil, err
			}
			s := aiproxy.NewStore(c.Store, nil)
			if err := s.DeleteAll(); err != nil {
				return nil, err
			}
			if err := s.InsertBatch(rows); err != nil {
				return nil, err
			}
			return map[string]any{"reindexed": len(rows)}, nil
		},
	}
}

// scanTranscripts walks agents/<name>/iterations/<id>/proxy-transcript.jsonl[.gz]
// and collects the metadata rows.
func scanTranscripts(agentsDir string) ([]aiproxy.AIRequest, error) {
	agents, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []aiproxy.AIRequest
	for _, ae := range agents {
		if !ae.IsDir() {
			continue
		}
		itersDir := filepath.Join(agentsDir, ae.Name(), "iterations")
		iters, err := os.ReadDir(itersDir)
		if err != nil {
			continue
		}
		for _, ie := range iters {
			if !ie.IsDir() {
				continue
			}
			for _, name := range []string{"proxy-transcript.jsonl", "proxy-transcript.jsonl.gz"} {
				rows, err := readTranscript(filepath.Join(itersDir, ie.Name(), name))
				if err != nil {
					continue
				}
				out = append(out, rows...)
			}
		}
	}
	return out, nil
}

func readTranscript(path string) ([]aiproxy.AIRequest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var r io.Reader = f
	if filepath.Ext(path) == ".gz" {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	}
	var out []aiproxy.AIRequest
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for sc.Scan() {
		var e aiproxy.TranscriptEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Meta.ID != "" {
			// Preserve the transcript's historical metadata verbatim. Legacy
			// entries have empty group fields and must not consult live membership.
			out = append(out, e.Meta)
		}
	}
	return out, sc.Err()
}
