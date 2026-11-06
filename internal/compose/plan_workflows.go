package compose

import (
	"encoding/json"
	"fmt"
)

// workflowsPhase builds the Store workflows the task queues name, once per
// selector. The binds that need a build's digest follow in the queue bindings
// phase.
type workflowsPhase struct{}

func (workflowsPhase) name() string { return "workflows" }

func (workflowsPhase) plan(p *planner, f File, _ *liveState) error {
	for _, prefix := range sortedKeys(f.TaskQueues) {
		ref, declared, err := f.TaskQueues[prefix].WorkflowRef()
		if err != nil {
			return fmt.Errorf("task queue %s workflow: %w", prefix, err)
		}
		key := "workflow:" + ref.Selector()
		if !declared || ref.Store == "" || len(p.needs(key)) > 0 {
			continue
		}
		p.provide(key, p.add(Action{Resource: key, Kind: "build", Fail: "build workflow " + ref.Selector(),
			Fields: []Field{{Name: "source", To: ref.Selector()}},
			run:    p.r.buildWorkflow(ref)}))
	}
	return nil
}

// buildWorkflow publishes a Store workflow. The build is unchanged when the
// daemon already had that version with the same digest (created is false).
func (r *Runner) buildWorkflow(ref WorkflowRef) func([]json.RawMessage) (json.RawMessage, bool, error) {
	return func([]json.RawMessage) (json.RawMessage, bool, error) {
		r.logf("building workflow %s", ref.Selector())
		raw, err := r.call.Call("POST", "/api/workflow-images/build", map[string]any{"source": ref.Selector(), "wait": true, "initiator": "compose"})
		if err != nil {
			return nil, false, fmt.Errorf("build workflow %s: %w", ref.Selector(), err)
		}
		var built struct {
			Created bool `json:"created"`
		}
		if err := json.Unmarshal(raw, &built); err != nil {
			return nil, false, fmt.Errorf("build workflow %s: %w", ref.Selector(), err)
		}
		return raw, built.Created, nil
	}
}
