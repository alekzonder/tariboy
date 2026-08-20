package commands

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/registry"
)

// confine resolves rel under root, rejecting escapes. It guards against both
// lexical traversal (../, absolute paths) AND symlink escapes: a symlink placed
// inside the workdir could otherwise redirect a read (pull) or write (push) to
// an arbitrary path outside it. After the lexical check we resolve symlinks on
// the real filesystem and re-verify that the resolved target still lives under
// the resolved root. The target file may not exist yet (push), so we resolve the
// deepest existing ancestor and re-append the not-yet-existing tail.
func confine(root, rel string) (string, error) {
	root = filepath.Clean(root)
	target := filepath.Join(root, filepath.Clean("/"+rel))
	if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		return "", api.UserError{Code: "bad_path", Msg: "path escapes the agent working directory"}
	}
	realRoot, err := resolveExisting(root)
	if err != nil {
		return "", api.UserError{Code: "bad_path", Msg: "cannot resolve agent working directory"}
	}
	realTarget, err := resolveExisting(target)
	if err != nil {
		return "", api.UserError{Code: "bad_path", Msg: "cannot resolve path"}
	}
	if realTarget != realRoot && !strings.HasPrefix(realTarget, realRoot+string(os.PathSeparator)) {
		return "", api.UserError{Code: "bad_path", Msg: "path escapes the agent working directory (symlink)"}
	}
	return target, nil
}

// resolveExisting evaluates symlinks on the deepest existing ancestor of p and
// re-appends the trailing components that do not exist yet, so it also works for
// a file about to be created. This means any symlink anywhere along the existing
// portion of the path (including p itself) is followed to its real location.
func resolveExisting(p string) (string, error) {
	p = filepath.Clean(p)
	rest := ""
	for {
		resolved, err := filepath.EvalSymlinks(p)
		if err == nil {
			if rest == "" {
				return resolved, nil
			}
			return filepath.Join(resolved, rest), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(p)
		if parent == p { // reached filesystem root without an existing ancestor
			return "", err
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}

func agentCwdFor(c *registry.Ctx, name string) (string, error) {
	a, err := getAgent(c, name)
	if err != nil {
		return "", err
	}
	if a.Cwd != "" {
		return a.Cwd, nil
	}
	return agentdir.New(agentsDir(c), name).Workdir(), nil
}

func agentPush() registry.Command {
	return registry.Command{
		Path:    "agent.push",
		Summary: "Write a base64 file into an agent's cwd (used by 'cp')",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "path", Type: registry.String, Required: true, Help: "destination path (relative to cwd)"},
			{Name: "content", Type: registry.String, Required: true, Help: "base64 file content"},
		},
		HTTP: &registry.HTTPRoute{Method: "PUT", Path: "/api/agents/{name}/files"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			root, err := agentCwdFor(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			target, err := confine(root, str(p, "path"))
			if err != nil {
				return nil, err
			}
			raw, err := base64.StdEncoding.DecodeString(str(p, "content"))
			if err != nil {
				return nil, api.UserError{Code: "bad_content", Msg: "content is not valid base64"}
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return nil, err
			}
			if err := os.WriteFile(target, raw, 0o600); err != nil {
				return nil, err
			}
			return map[string]any{"path": str(p, "path"), "abs": target, "bytes": len(raw)}, nil
		},
	}
}

func agentPull() registry.Command {
	return registry.Command{
		Path:    "agent.pull",
		Summary: "Read a base64 file from an agent's cwd (used by 'cp')",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "path", Type: registry.String, Required: true, Help: "source path (relative to cwd)"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/files"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			root, err := agentCwdFor(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			target, err := confine(root, str(p, "path"))
			if err != nil {
				return nil, err
			}
			raw, err := os.ReadFile(target)
			if err != nil {
				return nil, api.UserError{Code: "not_found", Msg: "file not found"}
			}
			return map[string]any{"path": str(p, "path"), "content": base64.StdEncoding.EncodeToString(raw)}, nil
		},
	}
}

// cp is CLI-local: it reads/writes a local file and calls push/pull on the daemon.
func cpCommand() registry.Command {
	return registry.Command{
		Path:    "cp",
		Summary: "Copy files to/from an agent cwd: cp SRC AGENT:DST | AGENT:SRC DST",
		Args: []registry.Arg{
			{Name: "src", Type: registry.String, Required: true, Help: "source (local path or AGENT:path)"},
			{Name: "dst", Type: registry.String, Required: true, Help: "destination (local path or AGENT:path)"},
		},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			name, remote, local, upload, err := parseCp(str(p, "src"), str(p, "dst"))
			if err != nil {
				return nil, api.UserError{Code: "bad_args", Msg: err.Error()}
			}
			cl := client.New(c.Socket)
			if upload {
				data, err := os.ReadFile(local)
				if err != nil {
					return nil, api.UserError{Code: "read_failed", Msg: err.Error()}
				}
				_, err = cl.Call("PUT", "/api/agents/"+name+"/files", map[string]string{
					"name": name, "path": remote, "content": base64.StdEncoding.EncodeToString(data),
				})
				if err != nil {
					return nil, err
				}
				return map[string]any{"copied": fmt.Sprintf("%s -> %s:%s", local, name, remote)}, nil
			}
			raw, err := cl.Call("GET", "/api/agents/"+name+"/files", map[string]string{"name": name, "path": remote})
			if err != nil {
				return nil, err
			}
			var res struct {
				Content string `json:"content"`
			}
			if err := jsonUnmarshalCmd(raw, &res); err != nil {
				return nil, err
			}
			data, err := base64.StdEncoding.DecodeString(res.Content)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(local, data, 0o600); err != nil {
				return nil, err
			}
			return map[string]any{"copied": fmt.Sprintf("%s:%s -> %s", name, remote, local)}, nil
		},
	}
}

// parseCp splits the two cp operands; exactly one must carry an AGENT: prefix.
func parseCp(src, dst string) (name, remote, local string, upload bool, err error) {
	sn, sp, sIsRemote := splitAgentPath(src)
	dn, dp, dIsRemote := splitAgentPath(dst)
	switch {
	case sIsRemote && !dIsRemote:
		return sn, sp, dst, false, nil // download AGENT:SRC -> DST
	case !sIsRemote && dIsRemote:
		return dn, dp, src, true, nil // upload SRC -> AGENT:DST
	default:
		return "", "", "", false, fmt.Errorf("exactly one of SRC/DST must be AGENT:path")
	}
}

func splitAgentPath(s string) (name, path string, isRemote bool) {
	i := strings.Index(s, ":")
	// A leading "./" or absolute path has no early colon; a Windows drive is out of scope.
	if i <= 0 || strings.Contains(s[:i], "/") {
		return "", s, false
	}
	return s[:i], s[i+1:], true
}

func jsonUnmarshalCmd(data []byte, v any) error { return json.Unmarshal(data, v) }
