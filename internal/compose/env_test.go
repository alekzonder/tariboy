package compose

import (
	"bytes"
	"io"
	"maps"
	"strings"
	"testing"
)

// playerYAML is the reported case: an agent whose env and harness mode the
// live agent does not match.
const playerYAML = `
version: 1
agents:
  player_1:
    image: player:latest
    env:
      BLACKJACK_PLAYER: player_1
    harness:
      type: claude
      interactive: false
`

func runningPlayer(env map[string]any, interactive bool) map[string]any {
	return map[string]any{"name": "player_1", "state": "running", "image": "player:latest", "harness": "claude",
		"enabled": true, "env": env, "interactive": interactive}
}

// An existing agent converges to the file's env and interactive mode, and a
// second up reports it ok with no mutating call.
func TestUpConvergesEnvAndInteractiveOfExistingAgent(t *testing.T) {
	fc := newFake()
	fc.agents["player_1"] = runningPlayer(map[string]any{}, true)
	var out strings.Builder
	r := NewRunner(fc, "", &out)
	f, err := Parse([]byte(playerYAML))
	if err != nil {
		t.Fatal(err)
	}
	upNoBuild(t, r, f)
	if n := countCalls(fc, "POST /api/agents/player_1/env"); n != 1 {
		t.Fatalf("env calls = %d: %v", n, fc.calls)
	}
	body := bodyFor(fc, "POST /api/agents/player_1/env").(map[string]any)
	if want := map[string]string{"BLACKJACK_PLAYER": "player_1"}; !maps.Equal(body["value"].(map[string]string), want) {
		t.Fatalf("env body = %#v, want %v", body, want)
	}
	if body := bodyFor(fc, "POST /api/agents/player_1/interactive"); body == nil || body.(map[string]any)["value"] != false {
		t.Fatalf("interactive body = %#v: %v", body, fc.calls)
	}
	if countCalls(fc, "POST /api/agents/player_1/stop") != 0 {
		t.Fatalf("env or interactive restarted the agent: %v", fc.calls)
	}
	if !strings.Contains(out.String(), "1 changed") {
		t.Fatalf("report:\n%s", out.String())
	}
	before, outBefore := len(fc.calls), out.Len()
	upNoBuild(t, r, f)
	for _, c := range fc.calls[before:] {
		if !strings.HasPrefix(c, "GET ") {
			t.Fatalf("converged up made a mutating call %s", c)
		}
	}
	if got := out.String()[outBefore:]; !strings.Contains(got, "1 ok, 0 changed") {
		t.Fatalf("second report:\n%s", got)
	}
}

// Live keys the file does not declare, such as image manifest env, are kept.
func TestUpEnvMergeKeepsUndeclaredLiveKeys(t *testing.T) {
	fc := newFake()
	fc.agents["player_1"] = runningPlayer(map[string]any{"FROM_IMAGE": "1", "BLACKJACK_PLAYER": "old"}, false)
	r := NewRunner(fc, "", io.Discard)
	f, _ := Parse([]byte(playerYAML))
	upNoBuild(t, r, f)
	body := bodyFor(fc, "POST /api/agents/player_1/env").(map[string]any)
	if want := map[string]string{"FROM_IMAGE": "1", "BLACKJACK_PLAYER": "player_1"}; !maps.Equal(body["value"].(map[string]string), want) {
		t.Fatalf("env body = %#v, want %v", body, want)
	}
	if n := countCalls(fc, "POST /api/agents/player_1/interactive"); n != 0 {
		t.Fatalf("converged interactive was set %d times", n)
	}
}

// A file without a harness block leaves the live interactive mode alone.
func TestUpWithoutHarnessBlockKeepsInteractive(t *testing.T) {
	fc := newFake()
	fc.agents["player_1"] = runningPlayer(map[string]any{}, true)
	r := NewRunner(fc, "", io.Discard)
	f, _ := Parse([]byte("version: 1\nagents:\n  player_1:\n    image: player:latest\n"))
	upNoBuild(t, r, f)
	if n := countCalls(fc, "POST /api/agents/player_1/interactive") + countCalls(fc, "POST /api/agents/player_1/env"); n != 0 {
		t.Fatalf("unexpected env or interactive call: %v", fc.calls)
	}
}

// A stopped agent sets its env after the re-provision.
func TestUpSetsEnvAfterReprovision(t *testing.T) {
	fc := newFake()
	fc.agents["player_1"] = runningPlayer(map[string]any{}, false)
	fc.agents["player_1"]["state"], fc.agents["player_1"]["enabled"] = "stopped", false
	r := NewRunner(fc, "", io.Discard)
	f, _ := Parse([]byte(playerYAML))
	upNoBuild(t, r, f)
	re, env := callIndex(fc.calls, "POST /api/agents/player_1/reprovision"), callIndex(fc.calls, "POST /api/agents/player_1/env")
	if re < 0 || env < re {
		t.Fatalf("env call %d not after reprovision %d: %v", env, re, fc.calls)
	}
}

// A new agent gets its env and mode in the create request and is then
// converged: a second up makes no env or interactive call.
func TestUpCreatesAgentWithEnvAndInteractive(t *testing.T) {
	fc := newFake()
	r := NewRunner(fc, "", io.Discard)
	f, _ := Parse([]byte(playerYAML))
	upNoBuild(t, r, f)
	body := bodyFor(fc, "POST /api/agents").(map[string]any)
	if body["env"] != "BLACKJACK_PLAYER=player_1" || body["interactive"] != false {
		t.Fatalf("create body = %#v", body)
	}
	before := len(fc.calls)
	upNoBuild(t, r, f)
	if n := countCallsSince(fc.calls, before, "POST /api/agents/player_1/env") + countCallsSince(fc.calls, before, "POST /api/agents/player_1/interactive"); n != 0 {
		t.Fatalf("created agent drifted: %v", fc.calls[before:])
	}
}

// compose status names env keys and the interactive mode, never env values.
func TestStatusReportsEnvAndInteractiveDrift(t *testing.T) {
	fc := newFake()
	fc.agents["player_1"] = runningPlayer(map[string]any{"BLACKJACK_PLAYER": "secret-old"}, true)
	var buf bytes.Buffer
	r := NewRunner(fc, "", &buf)
	f, _ := Parse([]byte(playerYAML))
	if err := r.Status(f); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"env drift: BLACKJACK_PLAYER", "interactive drift: have=true want=false", "drift: 1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "secret-old") || strings.Contains(out, "=player_1") {
		t.Fatalf("status printed an env value:\n%s", out)
	}
}
