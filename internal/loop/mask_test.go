package loop

import (
	"strings"
	"testing"
)

func TestMaskLaunch(t *testing.T) {
	secrets := map[string]string{
		"ANTHROPIC_API_KEY": "sk-secret-123",
		"EMPTY_SECRET":      "", // must not cause over-masking
	}
	proxyToken := "tok-abc-xyz"

	argv := []string{
		"/bin/shim",
		"--iteration-id", "iter-1",
		// A secret embedded in an argv entry must be redacted.
		"--url=https://proxy/tok-abc-xyz/v1",
	}
	env := []string{
		"ANTHROPIC_API_KEY=sk-secret-123",                  // rule (a): key match
		"ANTHROPIC_BASE_URL=https://api/sk-secret-123/end", // rule (b): value substring
		"OPENAI_BASE_URL=https://proxy/tok-abc-xyz/v1",     // rule (b): proxy token substring
		"EMPTY_SECRET=",      // empty secret: untouched
		"PATH=/usr/bin:/bin", // non-secret: unchanged
		"TARIBOY_AGENT=demo", // non-secret: unchanged
	}

	maskedArgv, maskedEnv := maskLaunch(argv, env, secrets, proxyToken)

	// No masked output may still contain the raw secrets.
	joined := strings.Join(append(append([]string{}, maskedArgv...), maskedEnv...), "\n")
	for _, raw := range []string{"sk-secret-123", "tok-abc-xyz"} {
		if strings.Contains(joined, raw) {
			t.Fatalf("raw secret %q leaked into masked output:\n%s", raw, joined)
		}
	}

	// argv: proxy token embedded in URL redacted, other entries untouched.
	if got, want := maskedArgv[3], "--url=https://proxy/"+secretPlaceholder+"/v1"; got != want {
		t.Errorf("argv[3] = %q, want %q", got, want)
	}
	if maskedArgv[0] != "/bin/shim" || maskedArgv[1] != "--iteration-id" || maskedArgv[2] != "iter-1" {
		t.Errorf("non-secret argv entries changed: %v", maskedArgv[:3])
	}

	want := map[int]string{
		0: "ANTHROPIC_API_KEY=" + secretPlaceholder,                       // rule (a)
		1: "ANTHROPIC_BASE_URL=https://api/" + secretPlaceholder + "/end", // rule (b) full env value context
		2: "OPENAI_BASE_URL=https://proxy/" + secretPlaceholder + "/v1",   // rule (b) proxy token
		3: "EMPTY_SECRET=",                                                // empty secret untouched
		4: "PATH=/usr/bin:/bin",                                           // non-secret unchanged
		5: "TARIBOY_AGENT=demo",                                           // non-secret unchanged
	}
	for i, w := range want {
		if maskedEnv[i] != w {
			t.Errorf("env[%d] = %q, want %q", i, maskedEnv[i], w)
		}
	}

	// KEY names must remain visible even when values are masked.
	if !strings.HasPrefix(maskedEnv[0], "ANTHROPIC_API_KEY=") {
		t.Errorf("masked env lost its key name: %q", maskedEnv[0])
	}
}

// Finding #1: a short/common secret value ('true') must not substring-mask the
// whole record. It is still masked where its KEY matches (rule (a)), but its raw
// text is left alone inside unrelated argv/env entries.
func TestMaskLaunchShortSecretNoOverMask(t *testing.T) {
	secrets := map[string]string{
		"FEATURE_FLAG": "true", // short common value: len < minSubstringSecretLen
	}
	argv := []string{"/bin/shim", "--truncate", "--input=trueish"}
	env := []string{
		"FEATURE_FLAG=true",      // rule (a): key match -> value masked
		"OTHER=truename",         // unrelated value containing "true": must survive
		"PATH=/usr/bin/truetool", // unrelated: must survive
	}

	maskedArgv, maskedEnv := maskLaunch(argv, env, secrets)

	// Rule (a) still masks the value under its own key.
	if got, want := maskedEnv[0], "FEATURE_FLAG="+secretPlaceholder; got != want {
		t.Errorf("env[0] = %q, want %q", got, want)
	}
	// Everything else must be untouched — no substring nuking on "true".
	for i, want := range map[int]string{1: "OTHER=truename", 2: "PATH=/usr/bin/truetool"} {
		if maskedEnv[i] != want {
			t.Errorf("env[%d] = %q, want %q (short secret over-masked)", i, maskedEnv[i], want)
		}
	}
	for i, want := range map[int]string{0: "/bin/shim", 1: "--truncate", 2: "--input=trueish"} {
		if maskedArgv[i] != want {
			t.Errorf("argv[%d] = %q, want %q (short secret over-masked)", i, maskedArgv[i], want)
		}
	}
}

// Finding #4: a bare env entry with no '=' must stay bare, not become "FOO=".
func TestMaskLaunchBareEnvEntryPreserved(t *testing.T) {
	secrets := map[string]string{"ANTHROPIC_API_KEY": "sk-secret-123"}
	env := []string{
		"BARE_ENTRY",                      // no '=': keep exactly as-is
		"ANOTHER",                         // no '=': keep exactly as-is
		"ANTHROPIC_API_KEY=sk-secret-123", // normal entry still masked
	}

	_, maskedEnv := maskLaunch(env[:0], env, secrets)

	if maskedEnv[0] != "BARE_ENTRY" {
		t.Errorf("bare env[0] = %q, want %q", maskedEnv[0], "BARE_ENTRY")
	}
	if maskedEnv[1] != "ANOTHER" {
		t.Errorf("bare env[1] = %q, want %q", maskedEnv[1], "ANOTHER")
	}
	if got, want := maskedEnv[2], "ANTHROPIC_API_KEY="+secretPlaceholder; got != want {
		t.Errorf("env[2] = %q, want %q", got, want)
	}
}

// The daemon now inherits the account's whole login-shell environment, so any
// token exported by an rc file reaches the launch record even though it is not a
// per-agent secret and has no provider key name. The child still receives every
// value; only the audit copy is redacted.
func TestMaskLaunchRedactsInheritedSecretShapedKeys(t *testing.T) {
	env := []string{
		"GITHUB_TOKEN=ghp_live_value",
		"AWS_SECRET_ACCESS_KEY=aws_live_value",
		"ARTIFACTORY_PASSWORD=hunter2hunter",
		"NPM_CONFIG_USERCONFIG=/Users/dev/.npmrc",
		"PATH=/usr/bin:/bin",
	}

	_, maskedEnv := maskLaunch(nil, env, nil)

	want := []string{
		"GITHUB_TOKEN=" + secretPlaceholder,
		"AWS_SECRET_ACCESS_KEY=" + secretPlaceholder,
		"ARTIFACTORY_PASSWORD=" + secretPlaceholder,
		"NPM_CONFIG_USERCONFIG=/Users/dev/.npmrc",
		"PATH=/usr/bin:/bin",
	}
	for i, got := range maskedEnv {
		if got != want[i] {
			t.Errorf("maskedEnv[%d] = %q, want %q", i, got, want[i])
		}
	}
}
