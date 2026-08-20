// Package agentdir owns the on-disk layout of an agent service and provisions
// it from an image (spec §12).
package agentdir

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/image"
)

// Layout is rooted at <agentsDir>/<name> for durable data. Sockets live in a
// separate short runtime dir (Runtime) so they stay under the OS sun_path limit
// no matter how long/deep Root is; when Runtime is empty they fall back to Root.
type Layout struct {
	Root    string
	Runtime string
	Name    string
}

func New(agentsDir, name string) Layout {
	return Layout{Root: filepath.Join(agentsDir, name), Name: name}
}

// WithRuntime returns a copy whose unix sockets live under runtimeDir (a short
// home-rooted dir) instead of under the durable Root.
func (l Layout) WithRuntime(runtimeDir string) Layout {
	l.Runtime = runtimeDir
	return l
}

func (l Layout) sockDir() string {
	if l.Runtime != "" {
		return l.Runtime
	}
	return l.Root
}

func (l Layout) Workdir() string         { return filepath.Join(l.Root, "workdir") }
func (l Layout) ImageDir() string        { return filepath.Join(l.Root, "image") }
func (l Layout) ImageBridgesDir() string { return filepath.Join(l.Root, "image-bridges") }
func (l Layout) BinDir() string          { return filepath.Join(l.Root, "bin") }
func (l Layout) Sock() string            { return filepath.Join(l.sockDir(), l.Name+".sock") }
func (l Layout) ContextPath() string     { return filepath.Join(l.Root, "CONTEXT.md") }
func (l Layout) IterationsDir() string   { return filepath.Join(l.Root, "iterations") }

// AuditLog is the per-agent append-only audit log (spans iterations). Kept at the
// agent root rather than per-iteration so `logs`/`logs -f` read one durable file.
func (l Layout) AuditLog() string { return filepath.Join(l.Root, "audit.jsonl") }

func (l Layout) IterationDir(id string) string { return filepath.Join(l.IterationsDir(), id) }
func (l Layout) PromptPath(id string) string   { return filepath.Join(l.IterationDir(id), "PROMPT.md") }
func (l Layout) ResultPath(id string) string   { return filepath.Join(l.IterationDir(id), "result.json") }
func (l Layout) LogsDir(id string) string      { return filepath.Join(l.IterationDir(id), "logs") }
func (l Layout) HarnessStdout(id string) string {
	return filepath.Join(l.LogsDir(id), "harness.stdout.log")
}
func (l Layout) HarnessStderr(id string) string {
	return filepath.Join(l.LogsDir(id), "harness.stderr.log")
}
func (l Layout) ShimLog(id string) string { return filepath.Join(l.LogsDir(id), "shim.log") }

// ShimSock is the per-agent shim RPC socket. Only one iteration runs per agent
// at a time, so the socket is keyed by agent, not iteration — this keeps the
// path short (it lives in the runtime dir) and stable across iterations.
func (l Layout) ShimSock() string { return filepath.Join(l.sockDir(), l.Name+".shim.sock") }

func (l Layout) EnsureIteration(id string) error {
	return os.MkdirAll(l.LogsDir(id), 0o700)
}

// Provision creates the tree, unpacks the image, and writes the bin shims
// (tools -> exec toolsBin; i-am-done -> exec toolsBin loop done). Agent config is
// NOT snapshotted to disk — the DB is the single source of truth.
func Provision(l Layout, a agent.Agent, imgStore *image.Store, ref image.Ref, toolsBin string) error {
	for _, d := range []string{l.Root, l.Workdir(), l.ImageDir(), l.BinDir(), l.IterationsDir()} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	if err := imgStore.Unpack(ref, l.ImageDir()); err != nil {
		return fmt.Errorf("unpack image %s: %w", ref.String(), err)
	}
	return WriteShims(l, a, toolsBin)
}

func hasCapability(capabilities []string, name string) bool {
	for _, capability := range capabilities {
		if capability == name {
			return true
		}
	}
	return false
}

// LiveIteration is one iteration dir that still owns a shim.sock.
type LiveIteration struct {
	Agent      string
	ID         string
	ShimSock   string
	ResultPath string
}

// ListLive finds, per agent, a live iteration to adopt on daemon restart
// (reattach, §4.2). The shim socket is now per-agent (in runtimeDir), so its
// presence means "this agent has a shim that may still be running"; the
// iteration it belongs to is the newest one, since only one runs at a time.
func ListLive(agentsDir, runtimeDir string) ([]LiveIteration, error) {
	agents, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []LiveIteration
	for _, ae := range agents {
		if !ae.IsDir() {
			continue
		}
		l := New(agentsDir, ae.Name()).WithRuntime(runtimeDir)
		sock := l.ShimSock()
		if _, err := os.Stat(sock); err != nil {
			continue // no live shim socket for this agent
		}
		id, err := l.newestIteration()
		if err != nil {
			return nil, err
		}
		if id == "" {
			continue // socket present but no iteration dir — nothing to adopt
		}
		out = append(out, LiveIteration{
			Agent: ae.Name(), ID: id, ShimSock: sock, ResultPath: l.ResultPath(id),
		})
	}
	return out, nil
}

// newestIteration returns the lexicographically-largest iteration id (ids are
// timestamp-prefixed, so lexical order == chronological), or "" if none.
func (l Layout) newestIteration() (string, error) {
	iters, err := os.ReadDir(l.IterationsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	newest := ""
	for _, ie := range iters {
		if ie.IsDir() && ie.Name() > newest {
			newest = ie.Name()
		}
	}
	return newest, nil
}
