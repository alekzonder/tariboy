package compose

import (
	"errors"
	"fmt"

	"github.com/alekzonder/tariboy/internal/client"
)

// Status reports desired-vs-actual drift from the same Plan `up` applies
// (spec §5): one line per group, agent, budget, task queue and pool, then the
// workflow of each existing queue. An agent's line lists every setting `up`
// would change (group, image, harness, cwd, timeout, loop and Goal settings);
// an image already pending reads as "image pending" until the agent's next
// iteration adopts it.
//
// Coverage is partial (model/env/plugins are not part of agent.ps's view), so
// the output always ends with a one-line disclaimer; a bare "drift: N" must
// never be read as a full convergence guarantee.
//
// This is read-only: it never issues a mutating call.
func (r *Runner) Status(f File) error {
	plan, err := r.Plan(f)
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}
	drift := 0
	for _, res := range plan.Resources {
		for _, line := range res.status {
			r.logf("%s", line)
		}
		drift += res.drift
		if res.queue == "" {
			continue
		}
		if err := r.statusQueueWorkflow(res.queue, f.TaskQueues[res.queue], &drift); err != nil {
			return err
		}
	}
	r.logf("drift: %d", drift)
	r.logf("note: model/env/plugins drift not checked (not exposed by agent.ps)")
	return nil
}

// statusQueueWorkflow reports the workflow of one queue. It never builds: a
// Store workflow without a tag is compared with the workflow_version of its
// Store source when the daemon can read that source; any other declared
// workflow, and a Store source that cannot be read, is compared through
// NAME:TAG (latest by default), so a Store workflow that is not built yet reads
// as pending. A daemon error on the queue's binding is reported for the queue.
func (r *Runner) statusQueueWorkflow(prefix string, want TaskQueueSpec, drift *int) error {
	ref, declared, err := want.WorkflowRef()
	if err != nil {
		return fmt.Errorf("task queue %s workflow: %w", prefix, err)
	}
	bound, isBound, err := r.queueWorkflow(prefix)
	if err != nil {
		code := err.Error()
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.Code != "" {
			code = apiErr.Code
		}
		r.logf("task queue %-10s workflow UNKNOWN (%s)", prefix, code)
		return nil
	}
	have := fmt.Sprintf("%s:%s %s", bound.Name, bound.Version, shortDigest(bound.Digest))
	switch {
	case !declared && isBound:
		r.logf("task queue %-10s workflow %s (not declared)", prefix, have)
		return nil
	case !declared:
		return nil
	}
	if ref.Store != "" && ref.Tag == "" {
		if version, ok := r.storeWorkflowVersion(ref); ok {
			want := ref.Name + ":" + version
			switch {
			case !isBound:
				r.logf("task queue %-10s workflow MISSING (want=%s, Store source)", prefix, want)
				*drift++
			case bound.Name != ref.Name || bound.Version != version:
				r.logf("task queue %-10s workflow drift: have=%s want=%s (Store source)", prefix, have, want)
				*drift++
			default:
				r.logf("task queue %-10s workflow %s ok", prefix, have)
			}
			return nil
		}
	}
	img, found, err := r.inspectWorkflowImage(ref.Ref())
	if err != nil {
		return fmt.Errorf("status task queue %s workflow %s: %w", prefix, ref.Ref(), err)
	}
	switch {
	case !found:
		r.logf("task queue %-10s workflow MISSING (want=%s, not built)", prefix, ref.Ref())
		*drift++
	case !isBound:
		r.logf("task queue %-10s workflow MISSING (want=%s)", prefix, ref.Ref())
		*drift++
	case bound.Digest != img.Digest:
		r.logf("task queue %-10s workflow drift: have=%s want=%s %s", prefix, have, ref.Ref(), shortDigest(img.Digest))
		*drift++
	default:
		r.logf("task queue %-10s workflow %s ok", prefix, have)
	}
	return nil
}
