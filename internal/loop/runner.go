package loop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/harness"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/shim"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// PromptParts are the ordered sections of an iteration prompt (spec §5.3).
type PromptParts struct {
	Agent           string
	Cwd             string
	ImagePrompt     string
	Context         string
	Messages        string
	AwaitingReplies string
	UserPrompt      string
	OneShot         string
	Goal            string
	Tail            string
}

func AssemblePrompt(p PromptParts) string {
	header := fmt.Sprintf("# You are agent %s\ncwd: %s", p.Agent, p.Cwd)
	work := FormatTaskProcessingOrder(RuntimePromptValues{OneShot: p.OneShot, Messages: p.Messages, AwaitingReplies: p.AwaitingReplies, Goal: p.Goal})
	sections := []string{header, p.ImagePrompt, p.Context, work, p.UserPrompt, p.Tail}
	var kept []string
	for _, s := range sections {
		if strings.TrimSpace(s) != "" {
			kept = append(kept, strings.TrimRight(s, "\n"))
		}
	}
	return strings.Join(kept, "\n\n") + "\n"
}

// mailInstruction is the standing instruction after the rendered letters
// (mailbox design §3). It names tmail, which is platform-provided, so an agent
// whose image still ships the old messages skill is told the right command.
const mailInstruction = `Requests: answer each one. A reply closes it:
    tmail reply <id> "<your answer>"
A task.question is answered on its task, not by mail; the letter closes
itself. A request you do not answer is shown again next iteration and is
dead-lettered after 5 attempts. A reply to a system:* sender records your
result; nobody reads it, so state what you did.
Notices: read them; they close when this iteration finishes.`

// FormatMessages renders the open letters of an iteration into the prompt's
// Messages section (mailbox design §3): requests first, then notices, each
// with its id, sender, kind and attempts, followed by the standing
// instruction. The body is indented so its text cannot open a heading.
func FormatMessages(letters []mail.Letter) string {
	if len(letters) == 0 {
		return ""
	}
	var requests, notices []mail.Letter
	for _, l := range letters {
		if l.NeedsReply {
			requests = append(requests, l)
		} else {
			notices = append(notices, l)
		}
	}
	var b strings.Builder
	b.WriteString("# Messages\nOpen letters for this iteration, oldest first. Each is listed with its id.\n")
	for _, group := range []struct {
		title   string
		letters []mail.Letter
	}{{"Requests", requests}, {"Notices", notices}} {
		if len(group.letters) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n### %s\n", group.title)
		for _, l := range group.letters {
			formatLetter(&b, l)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(mailInstruction)
	return b.String()
}

func formatLetter(b *strings.Builder, l mail.Letter) {
	fmt.Fprintf(b, "\n- id %s  from %s  kind %s  attempts %d", l.ID, l.From, l.Kind, l.Attempts)
	fmt.Fprintf(b, "\n  subject: %s", mail.OneLine(l.Subject))
	if l.InReplyTo != "" {
		fmt.Fprintf(b, "\n  in_reply_to: %s", l.InReplyTo)
	}
	if l.Deadline != "" {
		fmt.Fprintf(b, "\n  deadline: %s", l.Deadline)
	}
	if len(l.Data) > 0 {
		if j, err := json.Marshal(l.Data); err == nil {
			fmt.Fprintf(b, "\n  data: %s", string(j))
		}
	}
	if key, _ := l.Data["task_key"].(string); l.Kind == mail.KindTaskQuestion && key != "" {
		fmt.Fprintf(b, "\n  answer: ttasks comment %s \"<answer>\"", key)
	}
	for _, line := range strings.Split(l.Body, "\n") {
		b.WriteString("\n  " + line)
	}
}

// FormatAwaitingReplies renders the requests the agent sent with a deadline
// that have no answer yet into the prompt's "# Awaiting replies" section.
// Empty renders "" so the section is skipped. now anchors each request's age.
func FormatAwaitingReplies(letters []mail.Letter, now time.Time) string {
	if len(letters) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Awaiting replies\nRequests you sent that have no reply yet:\n")
	for _, l := range letters {
		fmt.Fprintf(&b, "\n- id %s  to %s", l.ID, l.To)
		if age, ok := messageAge(l.CreatedAt, now); ok {
			fmt.Fprintf(&b, "  age %s", age)
		}
		if l.Deadline != "" {
			fmt.Fprintf(&b, "  deadline %s", l.Deadline)
		}
	}
	return b.String()
}

// messageAge reports how long ago the stored ts was relative to now, truncated
// to whole seconds. ok=false when ts is unparseable, so callers omit the age
// rather than render a bogus one.
func messageAge(ts string, now time.Time) (time.Duration, bool) {
	t, err := store.ParseTime(ts)
	if err != nil {
		return 0, false
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	return d.Truncate(time.Second), true
}

// BuildEnv merges base env with the per-iteration injections (spec §10). Agent
// env and secrets override base; TARIBOY_* and the agent bin PATH are set
// last. When proxyEnabled is set the harness is pointed at the local AI proxy,
// while provider credentials remain the harness' responsibility and are
// forwarded by the proxy as-is.
func BuildEnv(base []string, agentBin, agentName, iterationID, toolsSock string, proxyEnabled bool, proxyURL, proxyToken string, env, secrets map[string]string) []string {
	m := map[string]string{}
	for _, kv := range base {
		if i := strings.Index(kv, "="); i >= 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	for k, v := range env {
		m[k] = v
	}
	for k, v := range secrets {
		m[k] = v
	}
	m["TARIBOY_AGENT"] = agentName
	m["TARIBOY_ITERATION"] = iterationID
	m["TARIBOY_TOOLS_SOCKET"] = toolsSock
	m["SHELL"] = "/bin/sh"
	if proxyURL != "" && proxyToken != "" {
		// Route the harness through the AI proxy. Attribution lives in proxyURL;
		// provider auth env remains unchanged.
		m["ANTHROPIC_BASE_URL"] = proxyURL
		m["OPENAI_BASE_URL"] = proxyURL + "/v1"
	}
	basePath := m["PATH"]
	if basePath == "" {
		basePath = "/usr/bin:/bin"
	}
	if agentBin != "" {
		m["PATH"] = agentBin + ":" + basePath
	} else {
		m["PATH"] = basePath
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(m))
	for _, k := range keys {
		out = append(out, k+"="+m[k])
	}
	return out
}

// baseEnv returns a copy of env, or the process environment when env is nil.
func baseEnv(env []string) []string {
	if env == nil {
		return os.Environ()
	}
	return slices.Clone(env)
}

const agentShellPrelude = `set -e
readonly -a tariboy_harness_argv=("${@:3}")
for script in "$1" "$2"; do
  if [[ -f "$script" ]]; then
    set -e
    # Keep source outside a conditional so errexit also applies inside it.
    source "$script"
    status=$?
    if [[ "$status" != 0 ]]; then exit "$status"; fi
  fi
done
exec "${tariboy_harness_argv[@]}"`

func agentShellCommand(bash, baseDir string, layout agentdir.Layout, harnessArgv []string) []string {
	argv := []string{bash, "-c", agentShellPrelude, "tariboy-agent-shell", filepath.Join(baseDir, "global-agent-shell.sh"), layout.ShellScriptPath()}
	return append(argv, harnessArgv...)
}

func mergeSkillLaunchEnv(base, overlay []string) ([]string, error) {
	m := make(map[string]string, len(base)+len(overlay))
	for _, kv := range base {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	for _, kv := range overlay {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			return nil, fmt.Errorf("invalid image skill environment entry %q", kv)
		}
		key := kv[:i]
		switch key {
		case "HOME", "CODEX_HOME", "XDG_CONFIG_HOME":
			return nil, fmt.Errorf("image skill bridge may not override %s", key)
		}
		m[key] = kv[i+1:]
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+m[key])
	}
	return out, nil
}

// Classify maps the shim result to an iteration status (spec §5).
func Classify(exitCode int, done, softTimeout, hardTimeout bool) agent.IterationStatus {
	if softTimeout || hardTimeout {
		return agent.IterationTimeout
	}
	if done {
		return agent.IterationDone
	}
	if exitCode != 0 {
		return agent.IterationHarnessError
	}
	return agent.IterationNoIAmDone
}

// Spawner starts the shim process; the seam lets tests avoid real processes.
type Spawner interface {
	Start(argv, env []string, dir string) error
}

// ManagedSpawner additionally returns a handle on the started shim process.
type ManagedSpawner interface {
	StartManaged(argv, env []string, dir string) (ManagedShim, error)
}

// ManagedShim is the daemon's handle on one detached shim process.
type ManagedShim struct {
	// Terminate is used when Stop cancels an iteration while detached shim
	// launch is in progress. The production function first asks the shim
	// process to run its signal cleanup, then escalates against the shim
	// process group.
	Terminate func() error
	// Exited closes once the shim process has exited and been reaped. A shim
	// writes result.json before it exits, so an exit without a result means
	// the shim failed (for example, it could not bind its RPC socket). Nil
	// when the spawner cannot observe the process.
	Exited <-chan struct{}
}

// shimExitedReason is the harness_error reason for a shim process that
// exited without writing result.json.
const shimExitedReason = "shim exited before writing result (see shim.log)"

// ProxyBinder is the slice of the AI proxy the runner needs. Defined here (plain
// strings, no aiproxy import) so loop stays free of an import cycle; satisfied by
// *aiproxy.Proxy via thin wrapper methods (Task 12).
type ProxyBinder interface {
	ProxyBaseURL() string
	MintToken(agent, iteration, imageName, imageTag, imageDigest string) (string, error)
	RevokeToken(token string)
	RevokeIteration(iteration string)
	// UpdateTask stamps native task/root attribution onto the live token(s) for
	// key (a token string or iteration id); empty task/root clear it. Returns
	// the number of tokens updated. Iteration preparation uses it before launch.
	UpdateTask(key, taskID, epicID string) int
}

// ExecSpawner launches the shim detached with os/exec.
type ExecSpawner struct{}

func (ExecSpawner) Start(argv, env []string, dir string) error {
	_, err := (ExecSpawner{}).StartManaged(argv, env, dir)
	return err
}

func (ExecSpawner) StartManaged(argv, env []string, dir string) (ManagedShim, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = env
	cmd.Dir = dir
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return ManagedShim{}, err
	}
	// The shim is launched detached: the runner observes the iteration through
	// result.json and the shim RPC, never through this process handle. Nothing
	// else waits on the child, so without an explicit reaper every finished
	// iteration leaves a zombie behind for the daemon's whole uptime. Waiting in
	// a goroutine reaps the shim without blocking the launch path, and the shim
	// still outlives the daemon on shutdown (it has its own process group).
	pgid := cmd.Process.Pid
	reaped := make(chan struct{})
	go func() {
		defer close(reaped)
		_ = cmd.Wait() // reaper only: the shim reports its outcome through result.json, not its exit status
	}()
	// A process group outlives its leader — its pgid stays reserved until the last
	// member exits. Before this reaper the shim lingered as a zombie and pinned
	// the pgid forever; now the group usually empties when the shim is reaped, and
	// an empty pgid is free for the kernel to hand out again. Signalling then
	// could land on somebody else's group, so skip the signal once the reaper has
	// run and report the os.ErrProcessDone an ESRCH kill already produces.
	//
	// This narrows the window rather than closing it: the shim can still be reaped
	// between the check and the kill. It also gives up on a process that outlives
	// the shim inside its group — the harness is not one (the shim starts it with
	// Setsid) and terminateCanceledLaunch tries the shim RPC and tmux before ever
	// reaching here, so losing that rare straggler beats signalling a stranger.
	kill := func(sig syscall.Signal) error {
		select {
		case <-reaped:
			return os.ErrProcessDone
		default:
		}
		return syscall.Kill(-pgid, sig)
	}
	terminate := func() error {
		err := kill(syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone) {
			return os.ErrProcessDone
		}
		timer := time.NewTimer(250 * time.Millisecond)
		defer timer.Stop()
		<-timer.C
		killErr := kill(syscall.SIGKILL)
		if errors.Is(killErr, syscall.ESRCH) || errors.Is(killErr, os.ErrProcessDone) {
			return nil
		}
		return killErr
	}
	return ManagedShim{Terminate: terminate, Exited: reaped}, nil
}

// ErrSessionAlive is returned by Run when an interactive agent already has a live
// tmux session — launching a second iteration on top would fail with a duplicate
// session and silently produce no harness. The loop maps it to a failed manual
// iteration (and skips loop-triggered ticks) instead.
var ErrSessionAlive = errors.New("tmux_session_exists")

// ErrIterationDetached means the daemon-side observer stopped while the
// shim-owned harness continues. The durable iteration remains running for the
// next daemon to adopt; this is not a terminal harness error.
var ErrIterationDetached = errors.New("iteration_detached")

type RunnerConfig struct {
	AgentsDir  string
	RuntimeDir string
	// BaseEnv is the baseline environment of the harness. Nil uses the daemon
	// process environment.
	BaseEnv      []string
	ShimBin      string
	ImgStore     *image.Store
	Store        *agent.Store
	Spawner      Spawner
	Clock        func() time.Time
	PollInterval time.Duration
	DoneGrace    time.Duration
	Logger       *slog.Logger
	Mail         *mail.Mail
	Proxy        ProxyBinder
	CurrentGoal  func(string, time.Time) (tasks.Task, bool, error)
	// WorkflowGoals supplies the workflow data for the Goal block of a workflow
	// task; nil renders the flexible text with a notice.
	WorkflowGoals WorkflowGoalSource
	Tasks         nativeTaskReader
	// GoalRead, if set, hears the Goal key an iteration's prompt was assembled
	// with ("" for none) and when it was read; the manager reports it when the
	// iteration ends.
	GoalRead func(agent, iterationID, key string, at time.Time)
	// HasTmuxSession reports whether an interactive agent's tmux session is already
	// alive. Injectable so tests avoid a real tmux. Defaults to tmuxHasSession.
	HasTmuxSession func(session string) bool
	// KillTmuxSession tears down an orphaned tmux session (one no running
	// iteration owns). Injectable so tests avoid a real tmux. Defaults to
	// tmuxKillSession.
	KillTmuxSession func(session string) error
	// AuditFor returns the shared per-agent audit recorder (one *audit.Log
	// instance per agent, also used by the daemon and engine so the seq counter
	// stays consistent). Nil disables audit tailing.
	AuditFor func(agent string) Recorder
	// RecordCursorUsage persists token counts from a non-interactive cursor
	// stream-json result line. Nil leaves those counts unrecorded.
	RecordCursorUsage func(agent, iteration, imageName, imageTag, imageDigest string, usage CursorResultUsage)
	// CodexApproval returns the daemon's codex approval mode, read at every
	// iteration start (harness.Config.CodexApproval). Nil means the default.
	CodexApproval func() string
}

const defaultDoneGrace = 2 * time.Second

type doneGraceTracker struct {
	seen     bool
	seenAt   time.Time
	killSent bool
}

type cooperativeKill struct {
	sent bool
}

func (k *cooperativeKill) send(kill func() error) error {
	if k.sent {
		return nil
	}
	k.sent = true
	return kill()
}

func (d *doneGraceTracker) observe(done bool, now time.Time, grace time.Duration, kill func() error) {
	if !done || d.killSent {
		return
	}
	if !d.seen {
		d.seen = true
		d.seenAt = now
		return
	}
	if now.Before(d.seenAt.Add(grace)) {
		return
	}
	d.killSent = true
	_ = kill() // the watcher's kill callback already logs its failure
}

// tmuxHasSession is the production HasTmuxSession: `tmux has-session -t =<name>`
// exits 0 iff the session exists.
func tmuxHasSession(session string) bool {
	return shim.TmuxHasSession(session)
}

// tmuxKillSession is the production KillTmuxSession.
func tmuxKillSession(session string) error {
	return shim.KillTmuxSession(session)
}

// SessionBlocked reports whether launching an iteration for ag would collide with
// an already-live tmux session (interactive agents only).
func (r *ShimRunner) SessionBlocked(ag agent.Agent) bool {
	return ag.Interactive && r.cfg.HasTmuxSession != nil && r.cfg.HasTmuxSession(ag.Name)
}

// ReapOrphanBlock self-heals a launch blocked solely by an ORPHANED tmux session:
// one that is alive but owned by no running iteration (left behind by a crashed
// daemon or a failed adoption). It kills such a session and returns true so the
// engine can proceed with the launch instead of failing every future iteration.
// It returns false when the agent is not session-blocked, or when a genuinely
// running iteration still owns the session — a live interactive harness must
// never be killed from under the user. Mirrors Manager.reapOrphanSessions, but at
// launch time rather than only on daemon restart.
func (r *ShimRunner) ReapOrphanBlock(ag agent.Agent) bool {
	if !r.SessionBlocked(ag) {
		return false
	}
	kill := r.cfg.KillTmuxSession
	if kill == nil {
		kill = tmuxKillSession // engine may hold a runner built without the default
	}
	killed, err := killOrphanSession(r.cfg.Store, kill, ag.Name)
	if err != nil {
		r.cfg.Logger.Warn("reap orphan tmux session before launch", "agent", ag.Name, "err", err)
		return false
	}
	if killed {
		r.cfg.Logger.Info("reaped orphan tmux session before launch", "agent", ag.Name)
	}
	return killed
}

type ShimRunner struct{ cfg RunnerConfig }

func NewShimRunner(cfg RunnerConfig) *ShimRunner {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 200 * time.Millisecond
	}
	if cfg.DoneGrace <= 0 {
		cfg.DoneGrace = defaultDoneGrace
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.HasTmuxSession == nil {
		cfg.HasTmuxSession = tmuxHasSession
	}
	if cfg.KillTmuxSession == nil {
		cfg.KillTmuxSession = tmuxKillSession
	}
	return &ShimRunner{cfg: cfg}
}

func (r *ShimRunner) Run(ctx context.Context, ag agent.Agent, trigger, iterationID, oneShot string) (Outcome, error) {
	if err := ctx.Err(); err != nil {
		return Outcome{}, err
	}
	// Backstop guard: never spawn a shim on top of a live tmux session. The engine
	// normally intercepts this before creating the iteration (SessionBlocked), so
	// reaching here means a direct Run — fail loudly rather than silently no-op.
	if r.SessionBlocked(ag) {
		return Outcome{}, ErrSessionAlive
	}
	l := agentdir.New(r.cfg.AgentsDir, ag.Name).WithRuntime(r.cfg.RuntimeDir)
	if err := l.EnsureIteration(iterationID); err != nil {
		return Outcome{}, err
	}

	// Child spans nest under the iteration span carried by ctx (spec §14).
	// prepare -> shim.spawn -> harness are sequential phases, not nested, so each
	// ends before the next starts. OTel-off ⇒ no-op spans (free).
	tr := otel.Tracer("tariboy/loop")

	prep, err := r.prepare(ctx, tr, ag, l, iterationID, oneShot)
	if err != nil {
		return Outcome{}, err
	}
	// The proxy token (if any) was minted inside prepare and must outlive it.
	// A normal terminal exit revokes it; a daemon-side detach deliberately
	// leaves it active for the replacement daemon and adopted shim.
	detached := false
	if r.cfg.Proxy != nil && prep.proxyToken != "" {
		defer func() {
			if !detached {
				r.cfg.Proxy.RevokeToken(prep.proxyToken)
			}
		}()
	}

	var rec Recorder
	if r.cfg.AuditFor != nil {
		rec = r.cfg.AuditFor(ag.Name)
	}
	r.recordLaunch(ag, iterationID, prep, rec)
	managedShim, err := r.spawnShim(ctx, tr, ag, iterationID, &prep, rec)
	if err != nil {
		return Outcome{}, err
	}
	if ctx.Err() != nil {
		if it, loadErr := r.cfg.Store.GetIteration(ag.Name, iterationID); loadErr == nil && it.Status != agent.IterationRunning {
			r.terminateCanceledLaunch(ag, l, iterationID, managedShim.Terminate)
			return Outcome{}, ctx.Err()
		}
	}
	r.cfg.Logger.Info("harness spawned", "agent", ag.Name, "iteration", iterationID)
	if stop := r.startHarnessTailer(ag, l, iterationID, rec); stop != nil {
		defer stop()
	}

	hctx, harnessSpan := tr.Start(ctx, "harness")
	outcome, err := r.await(hctx, ag, l, iterationID, managedShim.Exited)
	harnessSpan.End()
	if errors.Is(err, ErrIterationDetached) {
		detached = true
	}
	if err == nil && outcome.Status == agent.IterationHarnessError && outcome.Error == "" {
		outcome.Error = harnessExitReason(outcome.ExitCode, l.HarnessStderr(iterationID), ag.Interactive)
	}
	// The notices rendered into a successful iteration are read; after an
	// error, a timeout or a kill they stay open and render again (mailbox
	// design §1). Requests close only by a reply.
	if err == nil && outcome.Status == agent.IterationDone {
		r.closeReadNotices(ag.Name, iterationID, prep.letterIDs)
	}
	return outcome, err
}

// closeReadNotices closes the notices an iteration rendered. A failure leaves
// them open, so they render again: the safe direction.
func (r *ShimRunner) closeReadNotices(agentName, iterationID string, ids []string) {
	if r.cfg.Mail == nil || len(ids) == 0 {
		return
	}
	if _, err := r.cfg.Mail.CloseRead(agentName, ids); err != nil {
		r.cfg.Logger.Warn("close read notices", "agent", agentName, "iteration", iterationID, "err", err)
	}
}

// recordLaunch logs the masked harness launch and records the
// launching_harness audit event.
func (r *ShimRunner) recordLaunch(ag agent.Agent, iterationID string, prep preparedRun, rec Recorder) {
	r.cfg.Logger.Info("launching harness",
		"agent", ag.Name, "iteration", iterationID, "harness", ag.HarnessType,
		"cwd", prep.cwd, "argv", prep.maskedArgv)
	if rec != nil {
		rec.Record("launching_harness", "system", iterationID,
			map[string]any{
				"harness":     ag.HarnessType,
				"interactive": ag.Interactive,
				"argv":        prep.maskedArgv,
				"env":         prep.maskedEnv,
			})
	}
}

// spawnShim runs the shim.spawn phase: it snapshots the iteration deadlines,
// starts the shim, and returns the managed shim handle when the spawner
// provides one (a zero handle otherwise).
func (r *ShimRunner) spawnShim(ctx context.Context, tr oteltrace.Tracer, ag agent.Agent, iterationID string, prep *preparedRun, rec Recorder) (ManagedShim, error) {
	_, spawn := tr.Start(ctx, "shim.spawn")
	if err := ctx.Err(); err != nil {
		spawn.End()
		return ManagedShim{}, err
	}
	if err := r.snapshotDeadlines(ag, iterationID, prep); err != nil {
		spawn.End()
		return ManagedShim{}, err
	}
	if err := ctx.Err(); err != nil {
		spawn.End()
		return ManagedShim{}, err
	}
	var handle ManagedShim
	var err error
	if managed, ok := r.cfg.Spawner.(ManagedSpawner); ok {
		handle, err = managed.StartManaged(prep.shimArgv, prep.env, prep.cwd)
	} else {
		err = r.cfg.Spawner.Start(prep.shimArgv, prep.env, prep.cwd)
	}
	spawn.End()
	if err != nil {
		r.cfg.Logger.Error("spawn shim failed", "agent", ag.Name, "iteration", iterationID, "err", err)
		if rec != nil {
			rec.Record("shim_error", "system", iterationID, map[string]any{"error": err.Error()})
		}
		return ManagedShim{}, fmt.Errorf("spawn shim: %w", err)
	}
	return handle, nil
}

// snapshotDeadlines persists both deadlines from one clock sample directly
// before the shim is launched, so preparation time does not consume the
// configured timeout, and passes the hard deadline to the shim.
func (r *ShimRunner) snapshotDeadlines(ag agent.Agent, iterationID string, prep *preparedRun) error {
	now := r.cfg.Clock()
	if err := r.cfg.Store.InitializeIterationTimeout(iterationID, ag.TimeoutS, hardTimeout(ag), now); err != nil {
		return fmt.Errorf("initialize iteration timeout: %w", err)
	}
	// The shim starts its watchdog before the runner's first polling
	// synchronization. Pass the exact persisted deadline at launch so that
	// initial enforcement is anchored to the pre-spawn snapshot, rather than to
	// the shim process's later wall-clock read.
	it, err := r.cfg.Store.GetIteration(ag.Name, iterationID)
	if err != nil {
		return fmt.Errorf("read initialized iteration timeout: %w", err)
	}
	if it.HardTimeoutDeadline != nil {
		separator := slices.Index(prep.shimArgv, "--")
		if separator < 0 {
			return fmt.Errorf("shim argv missing harness separator")
		}
		prep.shimArgv = slices.Insert(prep.shimArgv, separator, "--hard-deadline", *it.HardTimeoutDeadline)
		prep.maskedArgv = slices.Insert(prep.maskedArgv, separator, "--hard-deadline", *it.HardTimeoutDeadline)
	}
	return nil
}

// recordsProxyUsage reports whether the harness adapter asks the runner to
// record its usage from the stdout stream (harness.ProxyUsageRecorder).
func recordsProxyUsage(harnessType string) bool {
	a, err := harness.Get(harnessType)
	if err != nil {
		return false
	}
	_, ok := a.(harness.ProxyUsageRecorder)
	return ok
}

// startHarnessTailer records harness_spawned and tees logs into the audit log
// for the lifetime of the harness; the final drain on Stop captures trailing
// output. Interactive agents tee only shim.log (their harness output is a tmux
// TUI capture, not audit-worthy). It returns the tailer's stop function, or
// nil when nothing is tailed.
func (r *ShimRunner) startHarnessTailer(ag agent.Agent, l agentdir.Layout, iterationID string, rec Recorder) func() {
	var onStdout func(string)
	if recordsProxyUsage(ag.HarnessType) && !ag.Interactive && r.cfg.RecordCursorUsage != nil {
		onStdout = cursorUsageObserver(ag.Model, func(usage CursorResultUsage) {
			imageName, imageTag := imageNameTag(ag.ImageRef)
			r.cfg.RecordCursorUsage(ag.Name, iterationID, imageName, imageTag, ag.ImageDigest, usage)
		})
	}
	if rec != nil {
		rec.Record("harness_spawned", "system", iterationID, nil)
	}
	if rec == nil && onStdout == nil {
		return nil
	}
	tailer := startTailer(rec, iterationID, l.LogsDir(iterationID), r.cfg.PollInterval, ag.Interactive, onStdout)
	return tailer.Stop
}

// terminateCanceledLaunch first asks the actual shim to stop cooperatively, so
// its Kill implementation owns the harness process group or tmux session. The
// socket can appear just after detached process creation, so connection is
// retried for a bounded interval. Only when no shim can be reached do we fall
// back to an explicit tmux kill plus the managed outer process-group handle.
func (r *ShimRunner) terminateCanceledLaunch(
	ag agent.Agent,
	l agentdir.Layout,
	iterationID string,
	terminate func() error,
) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		dialTimeout := minDuration(remaining, 100*time.Millisecond)
		client, err := shim.ConnectTimeout(l.ShimSock(), dialTimeout)
		if err == nil {
			if err := client.Kill(); err == nil {
				break
			}
		}
		time.Sleep(minDuration(remaining, 20*time.Millisecond))
	}
	if ag.Interactive && r.cfg.KillTmuxSession != nil {
		if err := r.cfg.KillTmuxSession(ag.Name); err != nil {
			r.cfg.Logger.Warn("terminate canceled tmux launch",
				"agent", ag.Name, "iteration", iterationID, "err", err)
		}
	}
	if terminate != nil {
		if err := terminate(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			r.cfg.Logger.Warn("terminate canceled shim launch",
				"agent", ag.Name, "iteration", iterationID, "err", err)
		}
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// preparedRun carries everything the prepare phase computed for shim.spawn.
type preparedRun struct {
	shimArgv   []string
	env        []string
	cwd        string
	proxyToken string // non-empty only when a proxy token was minted
	// maskedArgv/maskedEnv mirror shimArgv/env with secret values redacted for
	// logs and the launching_harness audit event — never for the actual spawn.
	maskedArgv []string
	maskedEnv  []string
	// letterIDs are the letters rendered into the prompt; the notices among
	// them close when the iteration finishes successfully.
	letterIDs []string
}

type activatedImageContextKey struct{}

// prepare runs the prepare phase (prompt assembly, harness command build,
// proxy token mint) in its own scope so the "prepare" span is ended on every
// exit — success or any of its early-return errors — via defer, without
// staying open across the later shim.spawn/harness phases. Failed spans are
// marked with an error status so prepare-phase failures are traceable.
func (r *ShimRunner) prepare(ctx context.Context, tr oteltrace.Tracer, ag agent.Agent, l agentdir.Layout, iterationID, oneShot string) (preparedRun, error) {
	_, span := tr.Start(ctx, "prepare")
	defer span.End()

	prep, err := r.prepareSteps(ctx, ag, l, iterationID, oneShot)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return preparedRun{}, err
	}
	return prep, nil
}

// prepareInputs is the trusted iteration state the later prepare steps read.
type prepareInputs struct {
	iteration agent.Iteration
	// bare marks an image that gets an instructions-free session: no assembled
	// prompt, no message drain, no bin shims on PATH (spec
	// 2026-07-22-terminals-simple-ui §1.3).
	bare           bool
	schemaVersion  int
	currentGoal    tasks.Task
	hasCurrentGoal bool
	activated      activatedImage
}

// prepareSteps runs the prepare steps in order. Once a proxy token is minted
// prepare owns it until the successful return hands it to Run, so every later
// failure revokes it exactly once through the deferred revoke below.
func (r *ShimRunner) prepareSteps(ctx context.Context, ag agent.Agent, l agentdir.Layout, iterationID, oneShot string) (preparedRun, error) {
	in, err := r.readPrepareInputs(ag, iterationID)
	if err != nil {
		return preparedRun{}, err
	}
	in.activated, _ = ctx.Value(activatedImageContextKey{}).(activatedImage)
	letterIDs, err := r.writePrompt(ctx, ag, l, iterationID, oneShot, in)
	if err != nil {
		return preparedRun{}, err
	}
	adapter, cwd, secrets, err := r.resolveHarness(ag, l, in)
	if err != nil {
		return preparedRun{}, err
	}

	proxyURL, proxyToken, err := r.mintProxyToken(ctx, ag, iterationID, in)
	revoke := proxyToken != ""
	defer func() {
		if revoke {
			r.cfg.Proxy.RevokeToken(proxyToken)
		}
	}()
	if err != nil {
		return preparedRun{}, err
	}

	prep, err := r.buildLaunch(ag, l, iterationID, in, adapter, cwd, secrets, proxyURL, proxyToken)
	if err != nil {
		return preparedRun{}, err
	}
	// Success: the caller takes ownership of the token and defers its revoke
	// for the rest of the iteration instead.
	revoke = false
	prep.letterIDs = letterIDs
	return prep, nil
}

// readPrepareInputs verifies the trusted iteration image identity against the
// active agent and reads the agent's current Goal.
func (r *ShimRunner) readPrepareInputs(ag agent.Agent, iterationID string) (prepareInputs, error) {
	in := prepareInputs{bare: ag.ImageRef == image.BareRef.String(), schemaVersion: 1}
	iteration, err := r.cfg.Store.GetIteration(ag.Name, iterationID)
	if err != nil {
		return in, fmt.Errorf("read trusted iteration image identity: %w", err)
	}
	if iteration.ImageRef != "" && iteration.ImageRef != ag.ImageRef {
		return in, fmt.Errorf("iteration image ref does not match active agent image")
	}
	if iteration.ImageDigest != "" && iteration.ImageDigest != ag.ImageDigest {
		return in, fmt.Errorf("iteration image digest does not match active agent image")
	}
	in.iteration = iteration
	if iteration.PromptTemplateSHA256 != "" {
		in.schemaVersion = 2
	}
	if r.cfg.CurrentGoal != nil && !in.bare {
		goalAt := r.cfg.Clock().UTC()
		in.currentGoal, in.hasCurrentGoal, err = r.cfg.CurrentGoal(ag.Name, goalAt)
		if err != nil {
			return in, fmt.Errorf("read current agent goal: %w", err)
		}
		if r.cfg.GoalRead != nil {
			key := ""
			if in.hasCurrentGoal {
				key = in.currentGoal.Key
			}
			r.cfg.GoalRead(ag.Name, iterationID, key, goalAt)
		}
	}
	return in, nil
}

// writePrompt assembles and writes the prompt, records the iteration_prepared
// audit event, stores the prompt path on the iteration row, and returns the
// ids of the letters it rendered.
func (r *ShimRunner) writePrompt(ctx context.Context, ag agent.Agent, l agentdir.Layout, iterationID, oneShot string, in prepareInputs) ([]string, error) {
	prompt := ""
	var batch []mail.Letter
	if !in.bare {
		var err error
		prompt, batch, err = r.assemblePrompt(ctx, ag, l, iterationID, oneShot, in)
		if err != nil {
			return nil, err
		}
	}
	if in.activated.Skills.PromptPrefix != "" {
		prompt = in.activated.Skills.PromptPrefix + prompt
	}
	if err := os.WriteFile(l.PromptPath(iterationID), []byte(prompt), 0o600); err != nil {
		return nil, err
	}
	letterIDs := make([]string, 0, len(batch))
	for _, letter := range batch {
		letterIDs = append(letterIDs, letter.ID)
	}
	// Preserve stable evidence references without leaking prompt or letter
	// bodies into the audit timeline.
	if r.cfg.AuditFor != nil {
		if rec := r.cfg.AuditFor(ag.Name); rec != nil {
			sum := sha256.Sum256([]byte(prompt))
			rec.Record("iteration_prepared", "system", iterationID, map[string]any{
				"prompt_sha256": hex.EncodeToString(sum[:]),
				"letter_ids":    letterIDs,
			})
		}
	}
	// Record the prompt path on the iteration row. This is best-effort: a
	// failure here must not abort the run, but it should not be silently
	// swallowed either. prompt_path is owned exclusively by
	// SetIterationPromptPath so a later UpdateIteration call cannot clobber it.
	if err := r.cfg.Store.SetIterationPromptPath(iterationID, l.PromptPath(iterationID)); err != nil {
		r.cfg.Logger.Warn("record prompt path", "agent", ag.Name, "id", iterationID, "err", err)
	}
	return letterIDs, nil
}

// pendingMessages renders the agent's open letters (mailbox design §3), which
// counts one attempt on each, and reads its requests awaiting a reply.
func (r *ShimRunner) pendingMessages(ag agent.Agent) (batch, awaiting []mail.Letter) {
	if r.cfg.Mail == nil {
		return nil, nil
	}
	n := ag.MessagesBatch
	if n <= 0 {
		n = 10
	}
	if letters, err := r.cfg.Mail.RenderBatch(ag.Name, n); err != nil {
		r.cfg.Logger.Warn("render mail batch", "agent", ag.Name, "err", err)
	} else {
		batch = letters
	}
	if reqs, err := r.cfg.Mail.Awaiting(ag.Name); err != nil {
		r.cfg.Logger.Warn("awaiting replies", "agent", ag.Name, "err", err)
	} else {
		awaiting = reqs
	}
	return batch, awaiting
}

// assemblePrompt renders the prompt of a non-bare image from its schema-v2
// template or its schema-v1 PROMPT.md, and returns the rendered message batch.
func (r *ShimRunner) assemblePrompt(ctx context.Context, ag agent.Agent, l agentdir.Layout, iterationID, oneShot string, in prepareInputs) (string, []mail.Letter, error) {
	contextText := ""
	if data, err := os.ReadFile(l.ContextPath()); err == nil {
		contextText = string(data)
	}
	batch, awaiting := r.pendingMessages(ag)

	goal := ""
	if in.hasCurrentGoal {
		goal = RuntimeGoal(ctx, r.cfg.WorkflowGoals, in.currentGoal, ag.Name, r.cfg.Logger)
	}
	if in.schemaVersion == 2 {
		template, err := ReadPromptTemplate(l.ImageDir(), in.iteration.PromptTemplateSHA256)
		if err != nil {
			return "", nil, err
		}
		workdir, err := FormatRuntimeWorkdir(l.Workdir())
		if err != nil {
			return "", nil, err
		}
		prompt, err := RenderPromptTemplate(template, l.ImageDir(), RuntimePromptValues{
			Identity: FormatRuntimeIdentity(ag.Name, ag.ImageRef, in.iteration.ImageVersion, ag.ImageDigest, agentCwd(ag, l), iterationID),
			Goal:     goal,
			Workdir:  workdir,
			Context:  contextText, Messages: FormatMessages(batch), AwaitingReplies: FormatAwaitingReplies(awaiting, r.cfg.Clock()), UserPrompt: ag.UserPrompt, OneShot: oneShot,
		})
		if err != nil {
			return "", nil, err
		}
		return prompt, batch, nil
	}
	imagePrompt, err := os.ReadFile(filepath.Join(l.ImageDir(), "PROMPT.md"))
	if err != nil {
		return "", nil, fmt.Errorf("read image prompt: %w", err)
	}
	// PROMPT_TAIL.md is optional: an absent file means no tail, anything else is worth a warning.
	tail, err := os.ReadFile(filepath.Join(l.ImageDir(), "PROMPT_TAIL.md"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		r.cfg.Logger.Warn("read image prompt tail", "agent", ag.Name, "err", err)
	}
	prompt := AssemblePrompt(PromptParts{
		Agent: ag.Name, Cwd: agentCwd(ag, l), ImagePrompt: string(imagePrompt),
		Context: contextText, Messages: FormatMessages(batch),
		AwaitingReplies: FormatAwaitingReplies(awaiting, r.cfg.Clock()),
		UserPrompt:      ag.UserPrompt, OneShot: oneShot, Goal: goal, Tail: string(tail),
	})
	return prompt, batch, nil
}

// resolveHarness resolves the harness adapter and working directory,
// materializes legacy skills, and reads the injected secrets.
func (r *ShimRunner) resolveHarness(ag agent.Agent, l agentdir.Layout, in prepareInputs) (harness.Adapter, string, map[string]string, error) {
	adapter, err := harness.Get(ag.HarnessType)
	if err != nil {
		return nil, "", nil, err
	}
	cwd := agentCwd(ag, l)
	if ag.Cwd != "" {
		if st, err := os.Stat(cwd); err != nil || !st.IsDir() {
			return nil, "", nil, fmt.Errorf("agent cwd %s does not exist", cwd)
		}
	}

	// Schema-v1 images retain their legacy CWD-relative skill layout. Schema-v2
	// skills are attached through the activation-time harness bridge instead.
	if sub := harness.LegacySkillsSubdir(adapter.Type()); in.schemaVersion != 2 && sub != "" {
		if err := agentdir.MaterializeSkills(l.ImageDir(), cwd, sub); err != nil {
			return nil, "", nil, fmt.Errorf("materialize skills: %w", err)
		}
	}

	// Injected env (spec §10).
	secrets, err := r.cfg.Store.SecretMap(ag.Name)
	if err != nil {
		return nil, "", nil, err
	}
	return adapter, cwd, secrets, nil
}

// mintProxyToken mints a per-iteration proxy token (spec §9) and stamps the
// current Goal's attribution onto it. Once a token is minted it is returned
// even alongside an error, so the caller can revoke it; a mint failure
// returns no token.
//
// Fail-closed: when a proxy is configured the harness MUST route through it.
// If we cannot obtain a token or the proxy URL is unavailable, running the
// iteration would let the harness reach the real upstream API directly with
// the operator's key — so we abort instead. The abort surfaces as an error
// which the engine records as a failed (harness_error) iteration.
func (r *ShimRunner) mintProxyToken(ctx context.Context, ag agent.Agent, iterationID string, in prepareInputs) (proxyURL, token string, err error) {
	if r.cfg.Proxy == nil {
		return "", "", nil
	}
	taskID, epicID := "", ""
	if in.hasCurrentGoal {
		taskID, epicID = in.currentGoal.Key, in.currentGoal.Key
		if in.currentGoal.ParentKey != "" {
			taskID, epicID, err = resolveNativeTaskAttribution(ctx, r.cfg.Tasks, ag.Name, in.currentGoal.Key)
			if err != nil {
				return "", "", fmt.Errorf("resolve current agent goal attribution: %w", err)
			}
		}
	}
	name, tag := imageNameTag(ag.ImageRef)
	tok, mErr := r.cfg.Proxy.MintToken(ag.Name, iterationID, name, tag, ag.ImageDigest)
	if mErr != nil {
		// Nothing was minted, so there is no token to revoke.
		return "", "", fmt.Errorf("proxy token unavailable, refusing to run without proxy: %w", mErr)
	}
	// A token exists from here on, so every return hands it back.
	if taskID != "" {
		if r.cfg.Proxy.UpdateTask(tok, taskID, epicID) != 1 {
			return "", tok, fmt.Errorf("attribute proxy token to current agent goal")
		}
	}
	proxyURL = r.cfg.Proxy.ProxyBaseURL()
	if u, ok := r.cfg.Proxy.(interface{ ProxyBaseURLForToken(string) string }); ok {
		proxyURL = u.ProxyBaseURLForToken(tok)
	}
	if proxyURL == "" {
		return "", tok, fmt.Errorf("proxy base URL unavailable, refusing to run without proxy")
	}
	return proxyURL, tok, nil
}

// buildLaunch builds the harness argv and environment, preflights the
// executables the iteration needs, and composes the shim command with its
// masked audit copy.
func (r *ShimRunner) buildLaunch(ag agent.Agent, l agentdir.Layout, iterationID string, in prepareInputs, adapter harness.Adapter, cwd string, secrets map[string]string, proxyURL, proxyToken string) (preparedRun, error) {
	codexApproval := ""
	if r.cfg.CodexApproval != nil {
		codexApproval = r.cfg.CodexApproval()
	}
	hargv, henv, err := adapter.Command(cwd, l.PromptPath(iterationID),
		harness.Config{Model: ag.Model, Effort: ag.Effort, Interactive: ag.Interactive, SessionID: iterationID, ProxyURL: proxyURL, Bare: in.bare, CodexApproval: codexApproval})
	if err != nil {
		return preparedRun{}, err
	}
	hargv = append(hargv, in.activated.Skills.Args...)

	base := append(baseEnv(r.cfg.BaseEnv), henv...)
	binDir := l.BinDir()
	if in.bare {
		binDir = ""
	}
	env := BuildEnv(base, binDir, ag.Name, iterationID, l.Sock(), r.cfg.Proxy != nil, proxyURL, proxyToken, ag.Env, secrets)
	env, err = mergeSkillLaunchEnv(env, in.activated.Skills.Env)
	if err != nil {
		return preparedRun{}, err
	}
	env, bash, err := preflightExecutables(adapter, env, cwd, in.bare)
	if err != nil {
		return preparedRun{}, err
	}
	hargv = agentShellCommand(bash, filepath.Dir(r.cfg.AgentsDir), l, hargv)

	// Compose the shim command.
	tmuxSession := ""
	if ag.Interactive {
		tmuxSession = ag.Name
	}
	shimArgv := []string{
		r.cfg.ShimBin,
		"--iteration-dir", l.IterationDir(iterationID),
		"--agent", ag.Name,
		"--iteration-id", iterationID,
		"--hard-timeout-s", fmt.Sprintf("%d", hardTimeout(ag)),
		"--tmux-session", tmuxSession,
		"--shim-sock", l.ShimSock(),
		"--",
	}
	shimArgv = append(shimArgv, hargv...)

	// Redact secret material for the audit record only. secrets carries the
	// per-agent secret key/values; proxyToken has no env key of its own (it is
	// embedded in the *_BASE_URL values) so it is passed as an extra value.
	maskedArgv, maskedEnv := maskLaunch(shimArgv, env, secrets, proxyToken)

	return preparedRun{
		shimArgv:   shimArgv,
		env:        env,
		cwd:        cwd,
		proxyToken: proxyToken,
		maskedArgv: maskedArgv,
		maskedEnv:  maskedEnv,
	}, nil
}

// preflightExecutables checks that the harness, python3 (non-bare images
// only) and bash resolve against the iteration environment. It returns env
// with TARIBOY_PYTHON3 set and the resolved bash path.
func preflightExecutables(adapter harness.Adapter, env []string, cwd string, bare bool) ([]string, string, error) {
	if _, err := harness.FindExecutable(adapter.Executable(), env, cwd); err != nil {
		return nil, "", fmt.Errorf("harness executable %q not found", adapter.Type())
	}
	if !bare {
		python3, err := harness.FindExecutable("python3", env, cwd)
		if err != nil {
			return nil, "", errors.New("python3 is required in the iteration environment for agent tool scripts")
		}
		env, err = mergeSkillLaunchEnv(env, []string{"TARIBOY_PYTHON3=" + python3})
		if err != nil {
			return nil, "", err
		}
	}
	bash, err := harness.FindExecutable("bash", env, cwd)
	if err != nil {
		return nil, "", errors.New("bash is required in the iteration environment for agent shell scripts")
	}
	return env, bash, nil
}

// await polls result.json (and the shim RPC) until the iteration finishes,
// enforcing the soft timeout via the shim kill RPC. shimExited, when not nil,
// closes once the shim process is reaped; a shim gone without result.json ends
// the iteration as harness_error.
func (r *ShimRunner) await(ctx context.Context, ag agent.Agent, l agentdir.Layout, iterationID string, shimExited <-chan struct{}) (Outcome, error) {
	w := iterationWatcher{
		store: r.cfg.Store, clock: r.cfg.Clock, log: r.cfg.Logger, agent: ag.Name, id: iterationID,
		resultPath: l.ResultPath(iterationID), shimSock: l.ShimSock(), poll: r.cfg.PollInterval,
		doneGrace: r.cfg.DoneGrace, shimExited: shimExited,
	}
	res, err := w.watch(ctx)
	if err == nil && res.shimGone {
		r.cfg.Logger.Warn("shim exited without result", "agent", ag.Name, "iteration", iterationID)
		return Outcome{Status: agent.IterationHarnessError, ExitCode: -1, Error: shimExitedReason}, nil
	}
	if err != nil {
		var ok bool
		if res.result, ok = readResult(l.ResultPath(iterationID)); !ok {
			if it, err := r.cfg.Store.GetIteration(ag.Name, iterationID); err == nil && it.Status == agent.IterationRunning {
				return Outcome{}, ErrIterationDetached
			}
			return Outcome{}, ctx.Err()
		}
	}
	it, err := r.cfg.Store.GetIteration(ag.Name, iterationID)
	done, productive := r.doneAndProductive(ag.Name, iterationID)
	soft := res.softHit || (err == nil && it.TimeoutTriggeredAt != nil)
	return Outcome{
		Status:     Classify(res.result.ExitCode, done, soft, res.result.TerminationReason == "hard_timeout"),
		ExitCode:   res.result.ExitCode,
		DoneFlag:   done,
		Productive: productive,
		CPUMs:      res.result.CPUMs,
		MemPeakKB:  res.result.MemPeakKB,
	}, nil
}

// enforceSoftTimeout gives the durable marker ownership of the kill. This
// keeps concurrent observers (including adoption after a daemon restart) from
// sending duplicate termination requests while still honoring the latest
// persisted deadline written by an extension.
func enforceSoftTimeout(st *agent.Store, it agent.Iteration, now time.Time, kill func() error) (bool, error) {
	if it.TimeoutTriggeredAt != nil {
		return true, nil
	}
	if it.TimeoutDeadline == nil {
		return false, nil
	}
	deadline, err := store.ParseTime(*it.TimeoutDeadline)
	if err != nil {
		return false, err
	}
	if now.Before(deadline) {
		return false, nil
	}
	marked, err := st.MarkIterationTimeoutTriggered(it.Agent, it.ID, *it.TimeoutDeadline, store.FormatTime(now))
	if err != nil || !marked {
		return false, err
	}
	_ = kill() // the marker is committed and the kill is sent once; the watcher's callback logs a failure
	return true, nil
}

// doneAndProductive reads both flags in one shot for the final Outcome. On a read
// error productive defaults to true, matching the column's DEFAULT 1 and the rule
// that only an explicit `--idle` declaration makes an iteration non-productive.
func (r *ShimRunner) doneAndProductive(agentName, id string) (done, productive bool) {
	it, err := r.cfg.Store.GetIteration(agentName, id)
	if err != nil {
		return false, true
	}
	return it.DoneFlag, it.Productive
}

func readResult(path string) (shim.IterationResult, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return shim.IterationResult{}, false
	}
	var res shim.IterationResult
	if jsonUnmarshal(data, &res) != nil {
		return shim.IterationResult{}, false
	}
	return res, true
}

// harnessExitReason describes a non-zero harness exit for the audit log: the
// exit code plus the tail of harness stderr. Interactive agents get only the
// code, because their stderr file is a tmux TUI capture.
func harnessExitReason(exitCode int, stderrPath string, interactive bool) string {
	reason := fmt.Sprintf("harness exited with code %d", exitCode)
	if interactive {
		return reason
	}
	b, err := os.ReadFile(stderrPath)
	if err != nil {
		return reason
	}
	const maxTail = 2048
	if len(b) > maxTail {
		b = b[len(b)-maxTail:]
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	if tail := strings.TrimSpace(strings.Join(lines, "\n")); tail != "" {
		reason += ": " + tail
	}
	return reason
}

func agentCwd(ag agent.Agent, l agentdir.Layout) string {
	if ag.Cwd != "" {
		return ag.Cwd
	}
	return l.Workdir()
}

func hardTimeout(ag agent.Agent) int {
	if ag.HardTimeoutS > 0 {
		return ag.HardTimeoutS
	}
	if ag.TimeoutS > 0 {
		return ag.TimeoutS + 60
	}
	return 0 // no hard deadline
}

func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

// imageNameTag splits an image ref "name:tag" for attribution; a parse failure
// degrades to empty fields rather than blocking the iteration.
func imageNameTag(ref string) (string, string) {
	r, err := image.ParseRef(ref)
	if err != nil {
		return "", ""
	}
	return r.Name, r.Tag
}
