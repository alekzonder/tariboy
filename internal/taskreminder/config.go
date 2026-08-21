// Package taskreminder defines the daemon policy for idle task reminders.
package taskreminder

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Policy controls task reminders for agents with assigned open work.
type Policy struct {
	Enabled        bool `json:"enabled"`
	IdleThresholdS int  `json:"idle_threshold_s"`
}

// DefaultPolicy is used while no task_reminder daemon config has been saved.
var DefaultPolicy = Policy{Enabled: false, IdleThresholdS: 300}

// ParsePolicy parses a persisted task_reminder JSON value. An absent value is
// the disabled default; persisted values must contain exactly the policy's
// typed fields and a positive, integral threshold in seconds.
func ParsePolicy(raw string) (Policy, error) {
	if strings.TrimSpace(raw) == "" {
		return DefaultPolicy, nil
	}

	var parsed struct {
		Enabled        *bool `json:"enabled"`
		IdleThresholdS *int  `json:"idle_threshold_s"`
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return Policy{}, fmt.Errorf("parse task reminder policy: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return Policy{}, fmt.Errorf("parse task reminder policy: multiple JSON values")
		}
		return Policy{}, fmt.Errorf("parse task reminder policy: %w", err)
	}
	if parsed.Enabled == nil {
		return Policy{}, fmt.Errorf("task reminder policy requires enabled")
	}
	if parsed.IdleThresholdS == nil {
		return Policy{}, fmt.Errorf("task reminder policy requires idle_threshold_s")
	}
	if *parsed.IdleThresholdS < 1 {
		return Policy{}, fmt.Errorf("task reminder policy idle_threshold_s must be at least 1")
	}
	return Policy{Enabled: *parsed.Enabled, IdleThresholdS: *parsed.IdleThresholdS}, nil
}
