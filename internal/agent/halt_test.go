package agent

import "testing"

// TestHaltReason pins the derived halt accessor. Each row is a t.Run subtest so
// one failure never masks the others (notably the not-a-halt regression pin,
// which protects agent-authored status lines written by Store.SetStatus).
func TestHaltReason(t *testing.T) {
	const idle = IdleStopPrefix + " (3 idle iterations)"
	cases := []struct {
		name       string
		errReason  string
		statusMsg  string
		wantKind   string
		wantReason string
	}{
		{"error-only", "halted: boom", "", "error", "halted: boom"},
		{"idle-only", "", idle, IdleStopPrefix, idle},
		{"error-wins-over-idle", "halted: boom", idle, "error", "halted: boom"},
		{"not-a-halt", "", "reviewing the failing test", "", ""},
		{"empty-agent", "", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &Agent{ErrorReason: tc.errReason, StatusMessage: tc.statusMsg}
			kind, reason := a.HaltReason()
			if kind != tc.wantKind || reason != tc.wantReason {
				t.Fatalf("HaltReason() = (%q, %q), want (%q, %q)", kind, reason, tc.wantKind, tc.wantReason)
			}
		})
	}
}

// TestIdleStopPrefixValue pins the constant's value: the reason strings it
// composes must stay byte-identical to the literals it replaced.
func TestIdleStopPrefixValue(t *testing.T) {
	if IdleStopPrefix != "idle_limit" {
		t.Fatalf("IdleStopPrefix = %q, want %q", IdleStopPrefix, "idle_limit")
	}
}
