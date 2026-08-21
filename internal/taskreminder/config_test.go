package taskreminder

import "testing"

func TestTaskReminderParsePolicyDefaultsWhenConfigIsAbsent(t *testing.T) {
	got, err := ParsePolicy("")
	if err != nil {
		t.Fatalf("ParsePolicy absent config: %v", err)
	}
	want := Policy{Enabled: false, IdleThresholdS: 300}
	if got != want {
		t.Fatalf("ParsePolicy absent config = %#v, want %#v", got, want)
	}
}

func TestTaskReminderParsePolicyAcceptsValidPolicy(t *testing.T) {
	got, err := ParsePolicy(`{"enabled":true,"idle_threshold_s":120}`)
	if err != nil {
		t.Fatalf("ParsePolicy valid config: %v", err)
	}
	want := Policy{Enabled: true, IdleThresholdS: 120}
	if got != want {
		t.Fatalf("ParsePolicy valid config = %#v, want %#v", got, want)
	}
}

func TestTaskReminderParsePolicyRejectsInvalidPolicy(t *testing.T) {
	for _, raw := range []string{
		`{`,
		`{"enabled":"true","idle_threshold_s":300}`,
		`{"enabled":true,"idle_threshold_s":1.5}`,
		`{"enabled":true,"idle_threshold_s":0}`,
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParsePolicy(raw); err == nil {
				t.Fatalf("ParsePolicy(%s) succeeded", raw)
			}
		})
	}
}
