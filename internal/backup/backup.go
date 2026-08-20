// Package backup produces and restores portable agent archives (spec §12): a
// deterministic tar.gz of an agent's DB rows (generic, schema-drift tolerant)
// plus its on-disk dir, with secret values masked and workdir excluded by
// default. restore.go re-inserts on any host, id-remapping on --name.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/store"
)

// FormatVersion is the backup-archive layout version.
const FormatVersion = 1

type Options struct {
	IncludeWorkdir bool
	IncludeSecrets bool
}

type Manifest struct {
	FormatVersion int    `json:"format_version"`
	SchemaVersion int    `json:"schema_version"`
	Agent         string `json:"agent"`
	CreatedAt     string `json:"created_at"`
}

type Meta struct {
	Manifest Manifest                    `json:"manifest"`
	Tables   map[string][]map[string]any `json:"tables"`
}

// tableSpec describes how to dump one table for a single agent's backup.
// where is a parameterized clause scoping rows to the agent (the agent name is
// bound via a single ? placeholder — no string interpolation, so no injection).
// orderBy is a stable key so two dumps of the same rows are byte-identical
// across hosts (SQLite has no guaranteed row order without ORDER BY).
type tableSpec struct {
	name     string
	where    string   // parameterized scoping clause; exactly one ? = agent name
	orderBy  string   // stable sort key for cross-host determinism
	agentCol string   // direct agent-scoping column ("" when scoped indirectly, e.g. deliveries)
	maskCols []string // blanked unless IncludeSecrets
}

// backupTables enumerates the agent-scoped tables. Every table has
// an explicit ORDER BY for determinism. messages are scoped to the ones this
// agent PRODUCED (its footprint) — channel history is cross-agent and not owned
// by any single agent, so it is intentionally not dumped. deliveries are scoped
// to the agent's own subscriptions.
var backupTables = []tableSpec{
	{name: "agents", where: "name = ?", orderBy: "name", agentCol: "name"},
	{name: "iterations", where: "agent = ?", orderBy: "id", agentCol: "agent"},
	{name: "ai_requests", where: "agent = ?", orderBy: "id", agentCol: "agent"},
	{name: "secrets", where: "agent = ?", orderBy: "key", agentCol: "agent", maskCols: []string{"value"}},
	{name: "subscriptions", where: "agent = ?", orderBy: "id", agentCol: "agent"},
	{name: "schedules", where: "agent = ?", orderBy: "id", agentCol: "agent"},
	{name: "scripts", where: "agent = ?", orderBy: "id", agentCol: "agent"},
	{name: "script_runs", where: "agent = ?", orderBy: "id", agentCol: "agent"},
	{name: "script_result_outbox", where: "agent = ?", orderBy: "idempotency_key", agentCol: "agent"},
	{name: "messages", where: "produced_by_agent = ?", orderBy: "id", agentCol: "produced_by_agent"},
	{name: "deliveries", where: "subscription_id IN (SELECT id FROM subscriptions WHERE agent = ?)", orderBy: "subscription_id, message_id"},
}

func Build(w io.Writer, s *store.Store, agentsDir, agentName string, opts Options, clock func() time.Time) (Manifest, error) {
	if !agent.ValidName(agentName) {
		return Manifest{}, fmt.Errorf("backup: invalid agent name %q", agentName)
	}
	if clock == nil {
		clock = time.Now
	}
	sv, err := s.SchemaVersion()
	if err != nil {
		return Manifest{}, err
	}
	man := Manifest{
		FormatVersion: FormatVersion, SchemaVersion: sv, Agent: agentName,
		CreatedAt: clock().UTC().Format(time.RFC3339),
	}
	tables, err := dumpTables(s.DB, agentName, opts)
	if err != nil {
		return Manifest{}, err
	}
	metaBytes, err := json.MarshalIndent(Meta{Manifest: man, Tables: tables}, "", "  ")
	if err != nil {
		return Manifest{}, err
	}

	// Collect agent dir files (deterministic order), excluding workdir + sock.
	root := agentdir.New(agentsDir, agentName).Root
	var files []string
	if _, err := os.Stat(root); err == nil {
		err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			// Skip anything that isn't a regular file: unix sockets (a running
			// agent's <name>.sock / <name>.shim.sock, agentdir.go), fifos,
			// devices, and symlinks. os.ReadFile on a socket fails with ENXIO,
			// which used to abort the whole backup whenever the agent had an
			// in-flight iteration. Skipping by mode (not by name) covers every
			// non-regular file and matches restore's rejection of symlinks.
			if !info.Mode().IsRegular() {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			if !opts.IncludeWorkdir && (rel == "workdir" || hasPrefixDir(rel, "workdir")) {
				return nil
			}
			files = append(files, rel)
			return nil
		})
		if err != nil {
			return Manifest{}, err
		}
	}
	sort.Strings(files)

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	mod := clock().UTC()
	writeEntry := func(name string, data []byte) error {
		hdr := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), ModTime: mod, Format: tar.FormatPAX}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err := tw.Write(data)
		return err
	}
	if err := writeEntry("meta.json", metaBytes); err != nil {
		return Manifest{}, err
	}
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return Manifest{}, err
		}
		if err := writeEntry("files/"+filepath.ToSlash(rel), data); err != nil {
			return Manifest{}, err
		}
	}
	if err := tw.Close(); err != nil {
		return Manifest{}, err
	}
	if err := gz.Close(); err != nil {
		return Manifest{}, err
	}
	return man, nil
}

// dumpTables generically dumps each backup table scoped to a single agent.
//
// Secret handling: secret VALUES live only in the secrets table and are masked
// unless opts.IncludeSecrets (see tableSpec.maskCols). The agents.env column and
// the on-disk config.json are dumped UNMASKED on purpose: in this system env is
// NON-SECRET by design — config.json and agents.env carry only plain env vars
// and the NAMES of required secrets (requires_secrets), never secret values.
// Secret values are injected at run time from the secrets table and never
// persisted into env or config. So their presence in a backup is correct, not a
// leak. If secret values could ever land in env/config, this masking model would
// be wrong and must be revisited.
func dumpTables(db *sql.DB, agent string, opts Options) (map[string][]map[string]any, error) {
	out := map[string][]map[string]any{}
	for _, ts := range backupTables {
		rows, err := db.Query(`SELECT * FROM `+ts.name+` WHERE `+ts.where+` ORDER BY `+ts.orderBy, agent)
		if err != nil {
			return nil, err
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return nil, err
		}
		var recs []map[string]any
		for rows.Next() {
			cells := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range cells {
				ptrs[i] = &cells[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return nil, err
			}
			rec := map[string]any{}
			for i, c := range cols {
				rec[c] = normalize(cells[i])
			}
			if !opts.IncludeSecrets {
				for _, mc := range ts.maskCols {
					if _, ok := rec[mc]; ok {
						rec[mc] = ""
					}
				}
			}
			recs = append(recs, rec)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		out[ts.name] = recs
	}
	return out, nil
}

func normalize(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	default:
		return x
	}
}

func hasPrefixDir(rel, dir string) bool {
	return len(rel) > len(dir) && rel[:len(dir)] == dir && (rel[len(dir)] == '/' || rel[len(dir)] == filepath.Separator)
}
