package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/backup"
	"github.com/alekzonder/tariboy/internal/registry"
)

func sha256File(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func backupOne(c *registry.Ctx, name, out string, opts backup.Options) (map[string]any, error) {
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	man, err := backup.Build(f, c.Store, agentsDir(c), name, opts, time.Now)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	sum, size, err := sha256File(out)
	if err != nil {
		return nil, err
	}
	res := map[string]any{"agent": man.Agent, "path": out, "bytes": size, "sha256": sum,
		"schema_version": man.SchemaVersion}
	if opts.IncludeSecrets {
		res["warning"] = "secret values are included in this archive; store it securely"
	}
	return res, nil
}

func backupCommand() registry.Command {
	return registry.Command{
		Path:    "backup",
		Summary: "Back up an agent (or 'all') to a portable tar.gz",
		Args: []registry.Arg{
			{Name: "target", Type: registry.String, Required: true, Help: "agent name, or 'all'"},
			{Name: "output", Flag: "output", Short: "o", Type: registry.String, Help: "output file (or dir for 'all')"},
			{Name: "include-workdir", Flag: "include-workdir", Type: registry.Bool, Help: "include the agent workdir (default off)"},
			{Name: "include-secrets", Flag: "include-secrets", Type: registry.Bool, Help: "include secret values (default masked)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/backup"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			target := str(p, "target")
			if target == "" {
				return nil, api.UserError{Code: "missing_target", Msg: "target is required"}
			}
			opts := backup.Options{}
			opts.IncludeWorkdir, _ = p["include-workdir"].(bool)
			opts.IncludeSecrets, _ = p["include-secrets"].(bool)
			backupsDir := filepath.Join(c.BaseDir, "backups")
			ts := time.Now().UTC().Format("20060102T150405Z")
			if target == "all" {
				ags, err := agent.NewStore(c.Store).List()
				if err != nil {
					return nil, err
				}
				dir := str(p, "output")
				if dir == "" {
					dir = backupsDir
				}
				var archives []map[string]any
				for _, a := range ags {
					res, err := backupOne(c, a.Name, filepath.Join(dir, fmt.Sprintf("%s-%s.tar.gz", a.Name, ts)), opts)
					if err != nil {
						return nil, api.UserError{Code: "backup_failed", Msg: err.Error()}
					}
					archives = append(archives, res)
				}
				return map[string]any{"archives": archives, "count": len(archives)}, nil
			}
			out := str(p, "output")
			if out == "" {
				out = filepath.Join(backupsDir, fmt.Sprintf("%s-%s.tar.gz", target, ts))
			}
			res, err := backupOne(c, target, out, opts)
			if err != nil {
				return nil, api.UserError{Code: "backup_failed", Msg: err.Error()}
			}
			return res, nil
		},
	}
}

func restoreCommand() registry.Command {
	return registry.Command{
		Path:    "restore",
		Summary: "Restore an agent from a backup tar.gz (optionally under a new name)",
		Args: []registry.Arg{
			{Name: "file", Type: registry.String, Required: true, Help: "archive path (daemon-accessible)"},
			{Name: "name", Flag: "name", Type: registry.String, Help: "restore under a new agent name"},
			{Name: "force", Flag: "force", Short: "f", Type: registry.Bool, Help: "overwrite an existing agent"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/restore"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			file := str(p, "file")
			if file == "" {
				return nil, api.UserError{Code: "missing_file", Msg: "file is required"}
			}
			f, err := os.Open(file)
			if err != nil {
				return nil, api.UserError{Code: "open_failed", Msg: err.Error()}
			}
			defer f.Close()
			force, _ := p["force"].(bool)
			man, err := backup.Restore(f, c.Store, agentsDir(c), backup.RestoreOptions{NewName: str(p, "name"), Force: force})
			if err != nil {
				return nil, api.UserError{Code: "restore_failed", Msg: err.Error()}
			}
			name := man.Agent
			if nn := str(p, "name"); nn != "" {
				name = nn
			}
			return map[string]any{"agent": name, "source_agent": man.Agent, "schema_version": man.SchemaVersion}, nil
		},
	}
}
