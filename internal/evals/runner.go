package evals

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/plugins"
)

// PluginEvaluator dispatches one eval to an eval-type plugin (satisfied by
// *plugins.Host). Kept as an interface so evals does not depend on the host's
// full surface and tests can fake it.
type PluginEvaluator interface {
	Evaluate(ctx context.Context, req plugins.EvalRequestDTO) (plugins.EvalVerdictDTO, error)
}

// ProxyMinter mints/revokes a per-eval proxy token so an llm-judge call is
// attributed + budgeted (satisfied by *aiproxy.Proxy). The real upstream key
// never reaches the plugin.
type ProxyMinter interface {
	ProxyBaseURL() string
	MintToken(agent, iteration, imageName, imageTag, imageDigest string) (string, error)
	RevokeToken(token string)
}

type RunnerConfig struct {
	Store      *Store
	ImgStore   *image.Store
	Plugins    PluginEvaluator
	Minter     ProxyMinter // nil ⇒ llm-judge gets no proxy token (plugin decides)
	AgentsDir  string
	JudgeModel string
	Timeout    time.Duration
	Log        *slog.Logger
	Clock      func() time.Time
	Rand       io.Reader
	Buffer     int
}

type evalJob struct {
	ag          agent.Agent
	iterationID string
	status      string
}

// Runner executes an image's declared evals after each iteration completes
// (spec §7.3/§8). It is an ingester-style worker: RunEvals never blocks the
// loop; Run drains its queue on ctx.Done before returning.
type Runner struct {
	cfg     RunnerConfig
	ch      chan evalJob
	dropped atomic.Int64
}

func NewRunner(cfg RunnerConfig) *Runner {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.JudgeModel == "" {
		cfg.JudgeModel = "claude-opus-4-8"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.Buffer <= 0 {
		cfg.Buffer = 256
	}
	return &Runner{cfg: cfg, ch: make(chan evalJob, cfg.Buffer)}
}

// RunEvals enqueues a finished iteration for evaluation. Non-blocking: if the
// queue is full or the worker has stopped, the job is dropped (best-effort;
// never blocks or deadlocks the loop). Implements loop.EvalRunner.
func (r *Runner) RunEvals(ag agent.Agent, iterationID, status string) {
	select {
	case r.ch <- evalJob{ag: ag, iterationID: iterationID, status: status}:
	default:
		r.dropped.Add(1)
		r.cfg.Log.Warn("eval job dropped (queue full or runner stopped)",
			"agent", ag.Name, "iteration", iterationID)
	}
}

// Dropped is the count of jobs shed under backpressure/shutdown (observability).
func (r *Runner) Dropped() int64 { return r.dropped.Load() }

// Run processes jobs until ctx is cancelled, then drains the queue best-effort
// (the store + plugin host are still up during cancel+wg.Wait) and returns.
func (r *Runner) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case job := <-r.ch:
					r.process(job)
				default:
					return
				}
			}
		case job := <-r.ch:
			r.process(job)
		}
	}
}

func (r *Runner) process(job evalJob) {
	ref, err := image.ParseRef(job.ag.ImageRef)
	if err != nil {
		r.cfg.Log.Warn("eval: parse image ref", "agent", job.ag.Name, "ref", job.ag.ImageRef, "err", err)
		return
	}
	man, err := r.cfg.ImgStore.Inspect(ref)
	if err != nil {
		r.cfg.Log.Warn("eval: inspect image", "agent", job.ag.Name, "ref", job.ag.ImageRef, "err", err)
		return
	}
	for _, ev := range man.Evals {
		if !agent.ValidName(ev.Name) {
			r.cfg.Log.Warn("eval: skipping invalid eval name", "agent", job.ag.Name, "eval", ev.Name)
			continue
		}
		r.runOne(job, man, ev)
	}
}

func (r *Runner) runOne(job evalJob, man image.Manifest, ev image.ManifestEval) {
	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.Timeout)
	defer cancel()
	req := plugins.EvalRequestDTO{
		Iteration: job.iterationID, Agent: job.ag.Name,
		ImageName: man.Name, ImageTag: man.Tag, ImageDigest: man.Digest,
		EvalName: ev.Name, EvalType: ev.Type, Prompt: ev.Prompt, Status: job.status,
		Workdir: agentdir.New(r.cfg.AgentsDir, job.ag.Name).Workdir(),
	}
	// llm-judge: give the plugin a scoped proxy token so its AI call is accounted
	// to this iteration and the real key stays server-side. Revoked after the eval.
	if ev.Type == "llm-judge" && r.cfg.Minter != nil {
		tok, err := r.cfg.Minter.MintToken(job.ag.Name, job.iterationID, man.Name, man.Tag, man.Digest)
		if err != nil {
			r.cfg.Log.Warn("eval: mint proxy token", "agent", job.ag.Name, "err", err)
		} else {
			// Attribution rides the tokenized proxy URL path (/_tariboy/<token>),
			// mirroring how the agent runner injects ANTHROPIC_BASE_URL. A bare base
			// URL would reach the proxy without a path token and be rejected (401).
			req.ProxyBaseURL = r.cfg.Minter.ProxyBaseURL()
			if u, ok := r.cfg.Minter.(interface{ ProxyBaseURLForToken(string) string }); ok {
				req.ProxyBaseURL = u.ProxyBaseURLForToken(tok)
			}
			req.ProxyToken = tok
			req.Model = r.cfg.JudgeModel
			defer r.cfg.Minter.RevokeToken(tok)
		}
	}
	res := Result{
		ID: newID(r.cfg.Rand), Iteration: job.iterationID, Agent: job.ag.Name,
		ImageName: man.Name, ImageTag: man.Tag, ImageDigest: man.Digest,
		EvalName: ev.Name, EvalType: ev.Type,
		CreatedAt: r.cfg.Clock().UTC().Format(time.RFC3339Nano),
	}
	verdict, err := r.cfg.Plugins.Evaluate(ctx, req)
	if err != nil {
		res.Verdict = "error"
		res.Detail = err.Error()
	} else {
		res.Verdict = normalizeVerdict(verdict.Verdict)
		res.Score = verdict.Score
		res.Detail = verdict.Detail
	}
	if err := r.cfg.Store.Insert(res); err != nil {
		r.cfg.Log.Warn("eval: store result", "agent", job.ag.Name, "eval", ev.Name, "err", err)
	}
}

func normalizeVerdict(v string) string {
	switch v {
	case "pass", "fail", "error":
		return v
	default:
		return "error"
	}
}
