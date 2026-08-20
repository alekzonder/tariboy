package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/registry"
)

// haltKeys asserts the two halt keys on a reader's output map. want=="" means
// NEITHER key may be present — absence, not an empty value, because a reader
// that emitted them unconditionally would pass an equality-against-"" check.
func assertHalt(t *testing.T, out map[string]any, wantKind, wantReason string) {
	t.Helper()
	kind, kindOK := out["halt_kind"]
	reason, reasonOK := out["halt_reason"]
	if wantKind == "" {
		if kindOK || reasonOK {
			t.Fatalf("halt keys present with no halt: halt_kind=%v (%v) halt_reason=%v (%v)", kind, kindOK, reason, reasonOK)
		}
		return
	}
	if !kindOK || !reasonOK {
		t.Fatalf("halt keys missing: halt_kind present=%v halt_reason present=%v in %#v", kindOK, reasonOK, out)
	}
	if kind != wantKind || reason != wantReason {
		t.Fatalf("halt keys = (%v, %v), want (%q, %q)", kind, reason, wantKind, wantReason)
	}
}

func TestReadersExposeHaltReason(t *testing.T) {
	const idle = agent.IdleStopPrefix + " (3 idle iterations)"

	readers := []struct {
		name string
		read func(t *testing.T, c *registry.Ctx) map[string]any
	}{
		{"inspect", func(t *testing.T, c *registry.Ctx) map[string]any {
			res, err := h(t, "agent.inspect")(c, registry.Params{"name": "smoke"})
			if err != nil {
				t.Fatal(err)
			}
			return res.(map[string]any)
		}},
		{"ps", func(t *testing.T, c *registry.Ctx) map[string]any {
			res, err := h(t, "agent.ps")(c, registry.Params{})
			if err != nil {
				t.Fatal(err)
			}
			rows := res.(map[string]any)["agents"].([]map[string]any)
			if len(rows) != 1 {
				t.Fatalf("want 1 ps row, got %d", len(rows))
			}
			return rows[0]
		}},
		{"status", func(t *testing.T, c *registry.Ctx) map[string]any {
			res, err := h(t, "agent.status.show")(c, registry.Params{"name": "smoke"})
			if err != nil {
				t.Fatal(err)
			}
			return res.(map[string]any)
		}},
	}

	states := []struct {
		name       string
		errReason  string
		statusMsg  string
		wantKind   string
		wantReason string
	}{
		{"halted", "halted: boom", "", "error", "halted: boom"},
		{"idle-stopped", "", idle, agent.IdleStopPrefix, idle},
		{"no-halt", "", "reviewing the failing test", "", ""},
	}

	for _, r := range readers {
		for _, st := range states {
			t.Run(r.name+"/"+st.name, func(t *testing.T) {
				c, as, _ := ctxWithStore(t)
				a := agent.Agent{Name: "smoke", ImageRef: "basic:latest", HarnessType: "stub"}
				if err := as.Create(a); err != nil {
					t.Fatal(err)
				}
				if st.errReason != "" {
					if err := as.SetError("smoke", st.errReason); err != nil {
						t.Fatal(err)
					}
				}
				if st.statusMsg != "" {
					if err := as.SetStatus("smoke", st.statusMsg, "2026-08-05T05:00:00Z"); err != nil {
						t.Fatal(err)
					}
				}
				assertHalt(t, r.read(t, c), st.wantKind, st.wantReason)
			})
		}
	}
}
