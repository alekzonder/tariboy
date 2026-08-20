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
	"strings"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/store"
)

const (
	maxEntryBytes = 256 << 20 // 256 MiB per-entry cap (zip-bomb guard)
	maxTotalBytes = 1 << 30   // 1 GiB total decompressed cap
)

type RestoreOptions struct {
	NewName string
	Force   bool
}

// Restore recreates an agent from an archive on any host (spec §12). It is
// path-traversal-safe (rejects escaping/symlink entries), refuses archives whose
// schema is newer than the daemon, refuses an existing target unless Force, and —
// under NewName — remaps the agent identity + id prefixes across DB rows AND the
// unpacked file paths. All DB inserts run in one transaction.
func Restore(r io.Reader, s *store.Store, agentsDir string, opts RestoreOptions) (Manifest, error) {
	meta, files, err := readArchive(r)
	if err != nil {
		return Manifest{}, err
	}
	sv, err := s.SchemaVersion()
	if err != nil {
		return Manifest{}, err
	}
	if meta.Manifest.SchemaVersion > sv {
		return Manifest{}, fmt.Errorf("archive schema version %d is newer than daemon %d (data newer than binary)",
			meta.Manifest.SchemaVersion, sv)
	}
	oldName := meta.Manifest.Agent
	newName := oldName
	if opts.NewName != "" {
		newName = opts.NewName
	}
	// The (possibly renamed) agent name must be valid so it can never lexically
	// escape agentsDir when joined into a path, and to reject a traversing --name.
	if !agent.ValidName(newName) {
		return Manifest{}, fmt.Errorf("restore: invalid agent name %q", newName)
	}

	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM agents WHERE name=?`, newName).Scan(&n); err != nil {
		return Manifest{}, err
	}
	exists := n > 0
	if exists && !opts.Force {
		return Manifest{}, fmt.Errorf("agent %q already exists (use --force to overwrite)", newName)
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return Manifest{}, err
	}
	if exists {
		// Delete children-first (reverse table order) so indirectly-scoped tables
		// (deliveries, via a subquery on subscriptions) are cleared while their
		// parent rows still exist. Generic + transactional so restore is idempotent.
		for i := len(backupTables) - 1; i >= 0; i-- {
			ts := backupTables[i]
			if _, err := tx.Exec(`DELETE FROM `+ts.name+` WHERE `+ts.where, newName); err != nil {
				tx.Rollback()
				return Manifest{}, err
			}
		}
	}
	idMaps, err := buildIDMaps(tx, meta.Tables, oldName, newName)
	if err != nil {
		tx.Rollback()
		return Manifest{}, err
	}
	for _, ts := range backupTables {
		for _, rec := range meta.Tables[ts.name] {
			row := remapRow(rec, ts, oldName, newName, idMaps)
			if ts.name == "script_runs" {
				remapScriptRunLogPath(row, agentsDir, newName)
			}
			if ts.name == "script_result_outbox" {
				remapScriptResultOutbox(row, rec, agentsDir, oldName, newName, idMaps)
			}
			if err := insertRow(tx, ts.name, row); err != nil {
				tx.Rollback()
				return Manifest{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return Manifest{}, err
	}

	// Unpack files under the (possibly renamed) agent root.
	root := agentdir.New(agentsDir, newName).Root
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Manifest{}, err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, rel := range names {
		outRel := remapPath(rel, oldName, newName)
		dest := filepath.Join(root, outRel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return Manifest{}, err
		}
		if err := os.WriteFile(dest, files[rel], 0o600); err != nil {
			return Manifest{}, err
		}
	}
	return meta.Manifest, nil
}

// readArchive parses the tar.gz, returning the meta and the files/ payloads
// keyed by their (validated, traversal-safe) relative path. Only regular files
// are accepted — symlink/hardlink/device/dir entries are rejected.
func readArchive(r io.Reader) (Meta, map[string][]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return Meta{}, nil, err
	}
	tr := tar.NewReader(gz)
	var meta Meta
	haveMeta := false
	files := map[string][]byte{}
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Meta{}, nil, err
		}
		if h.Typeflag != tar.TypeReg {
			return Meta{}, nil, fmt.Errorf("archive entry %q is not a regular file", h.Name)
		}
		if h.Size > maxEntryBytes {
			return Meta{}, nil, fmt.Errorf("archive entry %q exceeds size cap", h.Name)
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxEntryBytes+1))
		if err != nil {
			return Meta{}, nil, err
		}
		if int64(len(data)) > maxEntryBytes {
			return Meta{}, nil, fmt.Errorf("archive entry %q exceeds size cap", h.Name)
		}
		total += int64(len(data))
		if total > maxTotalBytes {
			return Meta{}, nil, fmt.Errorf("archive exceeds total size cap")
		}
		switch {
		case h.Name == "meta.json":
			if err := json.Unmarshal(data, &meta); err != nil {
				return Meta{}, nil, err
			}
			haveMeta = true
		case strings.HasPrefix(h.Name, "files/"):
			rel := strings.TrimPrefix(h.Name, "files/")
			if !safeRel(rel) {
				return Meta{}, nil, fmt.Errorf("unsafe archive path %q", h.Name)
			}
			files[rel] = data
		default:
			return Meta{}, nil, fmt.Errorf("unexpected archive entry %q", h.Name)
		}
	}
	if !haveMeta {
		return Meta{}, nil, fmt.Errorf("archive missing meta.json")
	}
	return meta, files, nil
}

// safeRel confirms a cleaned relative path stays within the destination: no
// absolute path and no ".." segment that could escape the agent root.
func safeRel(rel string) bool {
	if rel == "" || filepath.IsAbs(rel) {
		return false
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || strings.Contains(clean, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// remapRow returns rec with the agent-scoping column set to newName and only
// references to rows actually owned by this archive replaced from idMaps.
func remapRow(rec map[string]any, ts tableSpec, oldName, newName string, ids map[string]map[string]string) map[string]any {
	if oldName == newName {
		return rec
	}
	out := map[string]any{}
	for k, v := range rec {
		out[k] = v
	}
	if ts.agentCol != "" {
		if _, ok := out[ts.agentCol]; ok {
			out[ts.agentCol] = newName
		}
	}
	for _, ref := range remapRefs[ts.name] {
		if value, ok := out[ref.column].(string); ok && value != "" {
			if mapped, owned := ids[ref.owner][value]; owned {
				out[ref.column] = mapped
			}
		}
	}
	return out
}

type remapRef struct {
	column string
	owner  string
}

// remapRefs identifies ownership rather than merely columns that look like IDs.
// A delivery may reference a message produced by another agent, and optional
// iteration references may be empty; neither is owned by this archive and both
// must survive a rename byte-for-byte.
var remapRefs = map[string][]remapRef{
	"iterations":           {{"id", "iterations"}},
	"ai_requests":          {{"id", "ai_requests"}, {"iteration", "iterations"}},
	"subscriptions":        {{"id", "subscriptions"}},
	"schedules":            {{"id", "schedules"}},
	"scripts":              {{"id", "scripts"}},
	"script_runs":          {{"id", "script_runs"}, {"script_id", "scripts"}},
	"script_result_outbox": {{"script_id", "scripts"}, {"run_id", "script_runs"}},
	"messages":             {{"id", "messages"}, {"produced_in_iteration", "iterations"}},
	"deliveries":           {{"subscription_id", "subscriptions"}, {"message_id", "messages"}},
}

func buildIDMaps(tx *sql.Tx, tables map[string][]map[string]any, oldName, newName string) (map[string]map[string]string, error) {
	result := map[string]map[string]string{}
	if oldName == newName {
		return result, nil
	}
	for _, table := range []string{"iterations", "ai_requests", "subscriptions", "schedules", "scripts", "script_runs", "messages"} {
		mapped := map[string]string{}
		reserved := map[string]bool{}
		for _, row := range tables[table] {
			oldID, ok := row["id"].(string)
			if !ok || oldID == "" {
				continue
			}
			candidate := remapID(oldID, oldName, newName)
			base := candidate
			for suffix := 2; ; suffix++ {
				var count int
				if err := tx.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE id=?`, candidate).Scan(&count); err != nil {
					return nil, err
				}
				if count == 0 && !reserved[candidate] {
					break
				}
				candidate = fmt.Sprintf("%s-restored-%d", base, suffix)
			}
			mapped[oldID] = candidate
			reserved[candidate] = true
		}
		result[table] = mapped
	}
	return result, nil
}

func remapScriptRunLogPath(row map[string]any, agentsDir, agentName string) {
	logPath, _ := row["log_path"].(string)
	if logPath == "" {
		return
	}
	runID, _ := row["id"].(string)
	row["log_path"] = filepath.Join(agentdir.New(agentsDir, agentName).Root, "scripts", runID+filepath.Ext(logPath))
}

func remapScriptResultOutbox(row, original map[string]any, agentsDir, oldName, newName string, ids map[string]map[string]string) {
	if oldName != newName {
		for _, ref := range []remapRef{{"script_id", "scripts"}, {"run_id", "script_runs"}} {
			originalID, _ := original[ref.column].(string)
			if mapped, ok := ids[ref.owner][originalID]; ok {
				row[ref.column] = mapped
			} else if originalID != "" {
				// Outbox intent outlives removable script/run metadata. Remap its
				// semantic IDs even when the owning rows are no longer in the archive.
				row[ref.column] = remapID(originalID, oldName, newName)
			}
		}
	}
	runID, _ := row["run_id"].(string)
	row["idempotency_key"] = "script-result:" + runID
	payloadText, _ := row["payload"].(string)
	if payloadText == "" {
		return
	}
	payload := map[string]any{}
	if json.Unmarshal([]byte(payloadText), &payload) != nil {
		return
	}
	payload["script_id"] = row["script_id"]
	payload["run_id"] = runID
	if logPath, _ := payload["log_path"].(string); logPath != "" {
		payload["log_path"] = filepath.Join(agentdir.New(agentsDir, newName).Root, "scripts", runID+filepath.Ext(logPath))
	}
	if encoded, err := json.Marshal(payload); err == nil {
		row["payload"] = string(encoded)
	}
}

// remapPath re-prefixes each path segment beginning <old>- to <new>- (the
// iterations/<old>-…/ and archive/<old>-….tar.gz dir names).
func remapPath(rel, oldName, newName string) string {
	if oldName == newName {
		return rel
	}
	segs := strings.Split(rel, "/")
	for i, s := range segs {
		segs[i] = reprefix(s, oldName, newName)
	}
	return strings.Join(segs, "/")
}

func reprefix(s, oldName, newName string) string {
	if strings.HasPrefix(s, oldName+"-") {
		return newName + "-" + strings.TrimPrefix(s, oldName+"-")
	}
	return s
}

func remapID(s, oldName, newName string) string {
	for _, prefix := range []string{"scr-", "srun-"} {
		if strings.HasPrefix(s, prefix+oldName+"-") {
			return prefix + newName + "-" + strings.TrimPrefix(s, prefix+oldName+"-")
		}
	}
	if remapped := reprefix(s, oldName, newName); remapped != s {
		return remapped
	}
	// Current bus IDs are opaque and are not required to start with the agent
	// name (for example sub-<agent>-<timestamp> and channel-derived message
	// IDs). A rename restored into the source database must still allocate a
	// distinct key. Prefixing the entire opaque value is deterministic and the
	// same transformation is applied to deliveries' foreign keys.
	return newName + "-" + s
}

// insertRow builds a parameterized INSERT from the row's column map (column
// names captured from the dump, so it survives added columns — they take their
// SQL defaults). Columns are sorted for deterministic statements and quoted so
// a reserved word (e.g. agents."group") is never emitted bare.
func insertRow(tx *sql.Tx, table string, row map[string]any) error {
	cols := make([]string, 0, len(row))
	for c := range row {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	quoted := make([]string, len(cols))
	ph := make([]string, len(cols))
	args := make([]any, len(cols))
	for i, c := range cols {
		quoted[i] = `"` + c + `"`
		ph[i] = "?"
		args[i] = row[c]
	}
	q := "INSERT INTO " + table + " (" + strings.Join(quoted, ",") + ") VALUES (" + strings.Join(ph, ",") + ")"
	_, err := tx.Exec(q, args...)
	return err
}
