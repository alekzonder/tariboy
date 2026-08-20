package retention

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/audit"
	"github.com/alekzonder/tariboy/internal/store"
)

type Report struct {
	Agent      string   `json:"agent"`
	Pruned     []string `json:"pruned"`
	Archived   []string `json:"archived"`
	FreedBytes int64    `json:"freed_bytes"`
	DryRun     bool     `json:"dry_run"`
}

type Pruner struct {
	db        *sql.DB
	agents    *agent.Store
	pol       *Store
	agentsDir string
	clock     func() time.Time
	log       *slog.Logger
}

func NewPruner(s *store.Store, agents *agent.Store, pol *Store, agentsDir string, clock func() time.Time, log *slog.Logger) *Pruner {
	if clock == nil {
		clock = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &Pruner{db: s.DB, agents: agents, pol: pol, agentsDir: agentsDir, clock: clock, log: log}
}

func (p *Pruner) PruneAll(dryRun bool) ([]Report, error) {
	ags, err := p.agents.List()
	if err != nil {
		return nil, err
	}
	var out []Report
	for _, a := range ags {
		rep, err := p.PruneAgent(a.Name, dryRun)
		if err != nil {
			p.log.Warn("prune agent", "agent", a.Name, "err", err)
			continue
		}
		out = append(out, rep)
	}
	return out, nil
}

func (p *Pruner) PruneAgent(name string, dryRun bool) (Report, error) {
	rep := Report{Agent: name, Pruned: []string{}, Archived: []string{}, DryRun: dryRun}
	pol, err := p.pol.Effective(name)
	if err != nil {
		return rep, err
	}
	if pol.KeepIterations == 0 && pol.KeepDays == 0 && pol.MaxBytes == 0 {
		return rep, nil // unlimited: nothing to do
	}
	its, err := p.agents.ListIterations(name) // oldest -> newest
	if err != nil {
		return rep, err
	}
	victims := p.selectVictims(name, its, pol)
	l := agentdir.New(p.agentsDir, name)

	// Cap the per-agent audit.jsonl at MaxBytes (drops oldest lines, keeps newest).
	// Best-effort: a rotation failure must not abort iteration pruning.
	if !dryRun && pol.MaxBytes > 0 {
		if err := audit.Rotate(l.AuditLog(), pol.MaxBytes); err != nil {
			p.log.Warn("rotate audit log", "agent", name, "err", err)
		}
	}
	for _, id := range victims {
		dir := l.IterationDir(id)
		size := dirSize(dir)
		if dryRun {
			rep.Pruned = append(rep.Pruned, id)
			rep.FreedBytes += size
			continue
		}
		if pol.Archive {
			arch := filepath.Join(p.agentsDir, name, "archive", id+".tar.gz")
			if err := p.archiveDir(dir, arch); err != nil {
				p.log.Warn("archive iteration", "agent", name, "id", id, "err", err)
				continue // skip: leave dir + rows intact
			}
			rep.Archived = append(rep.Archived, id)
		}
		if err := p.deleteRows(id); err != nil {
			p.log.Warn("delete iteration rows", "agent", name, "id", id, "err", err)
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			p.log.Warn("remove iteration dir", "agent", name, "id", id, "err", err)
		}
		rep.Pruned = append(rep.Pruned, id)
		rep.FreedBytes += size
	}
	return rep, nil
}

// selectVictims returns the iteration ids to prune, protecting the newest and
// any running iteration. Ids that are not path-safe single elements are never
// selected (path-traversal guard): the pruner must never touch a path outside
// the agent's iterations dir.
func (p *Pruner) selectVictims(name string, its []agent.Iteration, pol Policy) []string {
	if len(its) <= 1 {
		return nil
	}
	// newest-first view; protect index 0 (newest) and any running.
	nf := make([]agent.Iteration, len(its))
	for i, it := range its {
		nf[len(its)-1-i] = it
	}
	protected := map[string]bool{nf[0].ID: true}
	// A snapshot pins an iteration only while it is being copied into immutable
	// judge evidence.  Do not race that copy with retention.
	if rows, err := p.db.Query(`SELECT DISTINCT iteration_id FROM judge_retention_pins`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				protected[id] = true
			}
		}
	}
	for _, it := range nf {
		if it.Status == "running" || !safeID(it.ID) {
			protected[it.ID] = true
		}
	}
	victim := map[string]bool{}

	if pol.KeepIterations > 0 {
		// Keep the newest KeepIterations iterations. Protected iterations
		// (newest + any running) are always kept AND count toward the budget,
		// so keep_iterations=1 retains exactly the newest run.
		kept := 0
		for _, it := range nf {
			if protected[it.ID] {
				kept++
				continue
			}
			if kept < pol.KeepIterations {
				kept++
				continue
			}
			victim[it.ID] = true
		}
	}
	if pol.KeepDays > 0 {
		cutoff := p.clock().AddDate(0, 0, -pol.KeepDays)
		for _, it := range nf {
			if protected[it.ID] {
				continue
			}
			if ts, err := time.Parse(time.RFC3339, it.StartedAt); err == nil && ts.Before(cutoff) {
				victim[it.ID] = true
			}
		}
	}
	if pol.MaxBytes > 0 {
		l := agentdir.New(p.agentsDir, name)
		var cum int64
		for _, it := range nf {
			if protected[it.ID] {
				continue
			}
			cum += dirSize(l.IterationDir(it.ID))
			if cum > pol.MaxBytes {
				victim[it.ID] = true
			}
		}
	}
	// Emit victims oldest-first (deterministic) for stable reports/archival.
	var out []string
	for _, it := range its {
		if victim[it.ID] {
			out = append(out, it.ID)
		}
	}
	return out
}

// safeID reports whether id is a single, contained path element usable to build
// an iteration dir path — no separators, no traversal, no absolute paths.
func safeID(id string) bool {
	if id == "" || id == "." || id == ".." {
		return false
	}
	if strings.ContainsRune(id, '/') || strings.ContainsRune(id, os.PathSeparator) || strings.ContainsRune(id, 0) {
		return false
	}
	return filepath.Base(id) == id && !filepath.IsAbs(id)
}

func (p *Pruner) deleteRows(id string) error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM ai_requests WHERE iteration=?`, id); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM iterations WHERE id=?`, id); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// archiveDir writes a deterministic tar.gz of dir to dest (0600).
func (p *Pruner) archiveDir(dir, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(files)
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	mod := p.clock().UTC()
	for _, path := range files {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hdr := &tar.Header{Name: filepath.ToSlash(rel), Mode: 0o600, Size: int64(len(data)), ModTime: mod, Format: tar.FormatPAX}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func dirSize(dir string) int64 {
	var n int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return n
}
