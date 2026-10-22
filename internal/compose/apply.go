package compose

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Outcome is what Apply did to one resource.
type Outcome string

const (
	OutcomeOK      Outcome = "ok"      // already converged; nothing to do
	OutcomeChanged Outcome = "changed" // every action applied
	OutcomeFailed  Outcome = "failed"  // an action failed
	OutcomeSkipped Outcome = "skipped" // an action it depends on failed
)

// ReportEntry is the outcome of one resource and, when it failed, the first
// error.
type ReportEntry struct {
	Resource string
	Outcome  Outcome
	Err      string
}

// Report lists the outcome of every resource of a Plan in plan order.
type Report struct {
	Entries []ReportEntry
	errs    []error
}

// Err joins the errors of every failed action; nil when none failed.
func (rep Report) Err() error { return errors.Join(rep.errs...) }

// failedError is what Up returns after it printed the report: a short count
// of the failed resources, since the report already shows each error. It
// unwraps to the errors of the failed actions.
type failedError struct {
	failed int
	err    error
}

func (e failedError) Error() string {
	if e.failed == 1 {
		return "1 resource failed"
	}
	return fmt.Sprintf("%d resources failed", e.failed)
}

func (e failedError) Unwrap() error { return e.err }

// summaryErr returns a failedError for a report that has failures; nil when
// none failed.
func (rep Report) summaryErr() error {
	err := rep.Err()
	if err == nil {
		return nil
	}
	n := 0
	for _, e := range rep.Entries {
		if e.Outcome == OutcomeFailed {
			n++
		}
	}
	return failedError{failed: n, err: err}
}

// Apply runs the plan's actions in order. A failed action does not stop the
// rest: the actions that depend on it are skipped, and independent actions
// still run.
func (r *Runner) Apply(p Plan) Report {
	outcomes := make([]Outcome, len(p.Actions))
	results := make([]json.RawMessage, len(p.Actions))
	errs := make([]error, len(p.Actions))
	for i, a := range p.Actions {
		if blocked(a.DependsOn, outcomes) {
			outcomes[i] = OutcomeSkipped
			continue
		}
		changed, err := true, error(nil)
		if a.run != nil {
			changed, err = a.run(results)
		} else {
			if a.Note != "" && a.changedBy == nil {
				r.logf("%s", a.Note)
			}
			results[i], err = r.call.Call(a.Call.Method, a.Call.Route, a.Call.Body)
			switch {
			case err != nil:
				err = fmt.Errorf("%s: %w", a.Fail, err)
			case a.changedBy != nil:
				if changed = a.changedBy(results[i]); changed && a.Note != "" {
					r.logf("%s", a.Note)
				}
			}
		}
		switch {
		case err != nil:
			outcomes[i], errs[i] = OutcomeFailed, err
		case changed:
			outcomes[i] = OutcomeChanged
		default:
			outcomes[i] = OutcomeOK
		}
	}
	return report(p, outcomes, errs)
}

func blocked(deps []int, outcomes []Outcome) bool {
	for _, d := range deps {
		if outcomes[d] == OutcomeFailed || outcomes[d] == OutcomeSkipped {
			return true
		}
	}
	return false
}

// report folds action outcomes into one outcome per resource: failed wins
// over skipped, skipped over changed, changed over ok.
func report(p Plan, outcomes []Outcome, errs []error) Report {
	rank := map[Outcome]int{OutcomeOK: 0, OutcomeChanged: 1, OutcomeSkipped: 2, OutcomeFailed: 3}
	var rep Report
	index := map[string]int{}
	for _, res := range p.Resources {
		index[res.ID] = len(rep.Entries)
		rep.Entries = append(rep.Entries, ReportEntry{Resource: res.ID, Outcome: OutcomeOK})
	}
	for i, a := range p.Actions {
		e := &rep.Entries[index[a.Resource]]
		if rank[outcomes[i]] > rank[e.Outcome] {
			e.Outcome = outcomes[i]
		}
		if errs[i] != nil {
			rep.errs = append(rep.errs, errs[i])
			if e.Err == "" {
				e.Err = errs[i].Error()
			}
		}
	}
	for i := range rep.Entries {
		if e := &rep.Entries[i]; e.Outcome == OutcomeSkipped {
			e.Err = "an action it depends on failed"
		}
	}
	return rep
}

// printReport writes one line per resource, then a summary.
func (r *Runner) printReport(rep Report) {
	counts := map[Outcome]int{}
	for _, e := range rep.Entries {
		counts[e.Outcome]++
		if e.Err != "" {
			r.logf("%-8s %s: %s", e.Outcome, e.Resource, e.Err)
		} else {
			r.logf("%-8s %s", e.Outcome, e.Resource)
		}
	}
	r.logf("up: %d ok, %d changed, %d failed, %d skipped",
		counts[OutcomeOK], counts[OutcomeChanged], counts[OutcomeFailed], counts[OutcomeSkipped])
}
