package workflowfile

import "time"

const (
	DefaultCheckTimeout = 60 * time.Second
	MaxCheckTimeout     = 30 * time.Minute
	DefaultWatchTimeout = 60 * time.Second
	MaxWatchTimeout     = 30 * time.Minute
	MinWatchEvery       = time.Second
	MinSourceEvery      = 10 * time.Second
	DefaultWatchSpacing = 5 * time.Second
	MinWatchSpacing     = time.Second
	MaxWatchSpacing     = 10 * time.Minute
)

const (
	defaultIdleIterations   = 3
	defaultRejectedRequests = 5
	defaultScriptFailures   = 3
	defaultUnavailableGrace = 5 * time.Minute
)

// EffectiveLimits are the limits that apply to one status after defaults and
// overrides are resolved.
type EffectiveLimits struct {
	IdleIterations   int
	RejectedRequests int
	ScriptFailures   int
	UnavailableGrace time.Duration
}

// StatusLimits resolves workflow defaults, workflow overrides, and the
// status's own overrides, in that order. An unknown status ID yields the
// workflow-level limits. A value that does not parse is ignored; Validate
// reports it.
func (f *File) StatusLimits(statusID string) EffectiveLimits {
	out := EffectiveLimits{
		IdleIterations:   defaultIdleIterations,
		RejectedRequests: defaultRejectedRequests,
		ScriptFailures:   defaultScriptFailures,
		UnavailableGrace: defaultUnavailableGrace,
	}
	out.apply(f.Limits)
	for i := range f.Statuses {
		if f.Statuses[i].ID == statusID {
			out.apply(f.Statuses[i].Limits)
			break
		}
	}
	return out
}

func (e *EffectiveLimits) apply(l *Limits) {
	if l == nil {
		return
	}
	if l.IdleIterations != nil {
		e.IdleIterations = *l.IdleIterations
	}
	if l.RejectedRequests != nil {
		e.RejectedRequests = *l.RejectedRequests
	}
	if l.ScriptFailures != nil {
		e.ScriptFailures = *l.ScriptFailures
	}
	if l.UnavailableGrace != "" {
		if d, err := time.ParseDuration(l.UnavailableGrace); err == nil && d >= 0 {
			e.UnavailableGrace = d
		}
	}
}

// EffectiveWatchSpacing is the least time between the starts of two watch
// runs of this workflow: watch_spacing, or DefaultWatchSpacing when it is
// absent or does not parse. Validate reports a value that does not parse.
func (f *File) EffectiveWatchSpacing() time.Duration {
	if d, err := time.ParseDuration(f.WatchSpacing); err == nil && d > 0 {
		return d
	}
	return DefaultWatchSpacing
}
