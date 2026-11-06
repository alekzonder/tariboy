package compose

import (
	"encoding/json"
	"fmt"

	"github.com/alekzonder/tariboy/internal/workflowfile"
)

// workflowsPhase builds workflow images. In version 2 it builds each entry of
// the workflows section, after the Store it comes from; in version 1 it
// builds the Store workflows the task queues name, once per selector. The
// binds that need a build follow in the queue bindings phase.
type workflowsPhase struct{}

func (workflowsPhase) name() string { return "workflows" }

func (workflowsPhase) plan(p *planner, f File, live *liveState) error {
	if p.r.archive != nil {
		p.planArchiveWorkflows(f)
		return nil
	}
	if f.Version == 2 {
		for _, name := range sortedKeys(f.Workflows) {
			p.planWorkflowEntry(name, f.Workflows[name].SourceSpec, live.sources)
		}
		return nil
	}
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

// planWorkflowEntry builds one workflows entry; the action is named
// "workflow:NAME" for the queue bindings.
func (p *planner) planWorkflowEntry(name string, s SourceSpec, live liveSources) {
	id := "workflow:" + name
	p.sourceStatus("workflow", name, s, live)
	p.provide(id, p.add(Action{Resource: id, Kind: "build", Fields: p.r.sourceFields(name, s),
		run: p.r.buildWorkflowEntry(name, s)}, p.storeNeeds(s)...))
}

// storeNeeds returns the actions of the Store a source comes from.
func (p *planner) storeNeeds(s SourceSpec) []int {
	if s.Store == "" {
		return nil
	}
	return p.needs("store:" + s.Store)
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
		result, built, err := workflowBuildResult(raw)
		if err != nil {
			return nil, false, fmt.Errorf("build workflow %s: %w", ref.Selector(), err)
		}
		return result, built.Created, nil
	}
}

// builtWorkflow is the result of a workflow build.
type builtWorkflow struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
	Created bool   `json:"created"`
}

// workflowBuildResult reads a waited workflow build: the finished operation
// holds the build result in workflow_result. result is that object, which
// the queue binding reads.
func workflowBuildResult(raw json.RawMessage) (json.RawMessage, builtWorkflow, error) {
	var op struct {
		Operation struct {
			WorkflowResult json.RawMessage `json:"workflow_result"`
		} `json:"operation"`
	}
	if err := json.Unmarshal(raw, &op); err != nil {
		return nil, builtWorkflow{}, err
	}
	result := op.Operation.WorkflowResult
	if len(result) == 0 || string(result) == "null" {
		return nil, builtWorkflow{}, fmt.Errorf("the build returned no workflow result")
	}
	var built builtWorkflow
	if err := json.Unmarshal(result, &built); err != nil {
		return nil, builtWorkflow{}, err
	}
	return result, built, nil
}

// buildWorkflowEntry checks the version and the name the source gives, builds
// the workflow and checks that the entry's tag exists. A conflict found
// before the build builds nothing, so no tag moves. The build is unchanged
// when the daemon already had that version with the same digest.
func (r *Runner) buildWorkflowEntry(name string, s SourceSpec) func([]json.RawMessage) (json.RawMessage, bool, error) {
	return func([]json.RawMessage) (json.RawMessage, bool, error) {
		body := map[string]any{"wait": true, "initiator": "compose"}
		source, err := r.readWorkflowSource(name, s, body)
		if err != nil {
			return nil, false, fmt.Errorf("workflow %s: %w", name, err)
		}
		if err := workflowNameConflict(name, source.Name); err != nil {
			return nil, false, err
		}
		if err := sourceVersionConflict("workflow", name, s.workflowVersion(), source.Version); err != nil {
			return nil, false, err
		}
		r.logf("building workflow %s", name)
		raw, err := r.call.Call("POST", "/api/workflow-images/build", body)
		if err != nil {
			return nil, false, fmt.Errorf("build workflow %s: %w", name, err)
		}
		result, built, err := workflowBuildResult(raw)
		if err != nil {
			return nil, false, fmt.Errorf("build workflow %s: %w", name, err)
		}
		if err := workflowNameConflict(name, built.Name); err != nil {
			return nil, false, err
		}
		if err := sourceVersionConflict("workflow", name, s.workflowVersion(), built.Version); err != nil {
			return nil, false, err
		}
		ref := name + ":" + s.tagOrLatest()
		if _, found, err := r.inspectWorkflowImage(ref); err != nil {
			return nil, false, fmt.Errorf("workflow %s: read %s: %w", name, ref, err)
		} else if !found {
			return nil, false, fmt.Errorf("workflow %s: %s does not exist after the build", name, ref)
		}
		return result, built.Created, nil
	}
}

// readWorkflowSource reads the name and the version a workflow source gives
// before the build, and sets the source of the build in body. A Store source
// is read through the daemon's validate route, which parses the Workflowfile
// the build would use; a path source is parsed here.
func (r *Runner) readWorkflowSource(name string, s SourceSpec, body map[string]any) (builtWorkflow, error) {
	if s.Store == "" {
		path := r.absPath(s.Path)
		body["path"] = path
		wf, err := workflowfile.Parse(path)
		if err != nil {
			return builtWorkflow{}, err
		}
		return builtWorkflow{Name: wf.Name, Version: wf.WorkflowVersion}, nil
	}
	selector := s.Store + "/" + s.sourceName(name)
	body["source"] = selector
	raw, err := r.call.Call("POST", "/api/workflow-images/validate", map[string]any{"source": selector})
	if err != nil {
		return builtWorkflow{}, fmt.Errorf("read %s: %w", selector, err)
	}
	var source builtWorkflow
	if err := json.Unmarshal(raw, &source); err != nil {
		return builtWorkflow{}, fmt.Errorf("read %s: %w", selector, err)
	}
	return source, nil
}

func workflowNameConflict(entry, built string) error {
	if entry == built {
		return nil
	}
	return fmt.Errorf("workflow %s: the source builds workflow %s; the entry key must equal the workflow name", entry, built)
}

// sourceVersionConflict fails when the file sets a version and the source
// gives another one.
func sourceVersionConflict(kind, name, want, got string) error {
	switch {
	case want == "" || want == got:
		return nil
	case got == "":
		return fmt.Errorf("%s %s: expected version %s, the source gives no version; the tag is not moved", kind, name, want)
	}
	return fmt.Errorf("%s %s: expected version %s, the source gives %s; the tag is not moved", kind, name, want, got)
}

// storeEntryVersion reads the version a Store gives for an image or workflow
// source, at apply time, after the stores phase refreshed it.
func (r *Runner) storeEntryVersion(kind, store, source string) (string, error) {
	var d storeDetail
	if err := r.get("/api/stores/"+store, &d); err != nil {
		return "", fmt.Errorf("read store %s: %w", store, err)
	}
	return d.entryVersion(kind, source)
}

// sourceFields are the fields dry-run prints for the build of an entry. The
// version a Store gives is known only after the refresh.
func (r *Runner) sourceFields(name string, s SourceSpec) []Field {
	var out []Field
	if s.Store != "" {
		out = append(out, Field{Name: "source", To: s.Store + "/" + s.sourceName(name)})
	} else {
		out = append(out, Field{Name: "path", To: s.Path})
	}
	out = append(out, Field{Name: "tag", To: s.tagOrLatest()})
	switch {
	case s.Version != "" && s.Store != "":
		out = append(out, Field{Name: "version", To: s.Version + " (checked at apply time)"})
	case s.Version != "":
		out = append(out, Field{Name: "version", To: s.Version})
	}
	return out
}
