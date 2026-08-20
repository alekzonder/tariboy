package supportbundle

import (
	"strings"
	"testing"
)

func TestRedactTextRemovesSecretsAndPathsButPreservesDiagnostic(t *testing.T) {
	input := strings.Join([]string{
		`Authorization: Bearer top-secret-token`,
		`{"Authorization":"Bearer json-secret-token"}`,
		`{"OPENAI_API_KEY":"json-api-secret"}`,
		`api_key=sk-live-secret password: hunter2`,
		`OPENAI_API_KEY=openai-secret AWS_SECRET_ACCESS_KEY=aws-secret`,
		`https://user:pass@example.invalid/path?token=query-secret&safe=1`,
		`CUSTOM_SECRET=abcdefgh12345678`,
		`/Users/alice/.tariboy/agents/a/workdir/customer.txt`,
		`/home/alice/.tariboyd/tariboyd.log`,
		`ERROR tmux new-session session=test: exec: "tmux": executable file not found in $PATH`,
	}, "\n")

	output := string(redactText(
		[]byte(input),
		[]string{
			"/Users/alice/.tariboy",
			"/home/alice/.tariboyd",
		},
		[]string{"CUSTOM_SECRET=abcdefgh12345678"},
	))

	for _, forbidden := range []string{
		"top-secret-token", "json-secret-token", "json-api-secret",
		"sk-live-secret", "hunter2", "openai-secret", "aws-secret",
		"query-secret", "abcdefgh12345678", "user:pass", "/Users/alice",
		"/home/alice",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("redactor leaked %q in %q", forbidden, output)
		}
	}
	const diagnostic = `ERROR tmux new-session session=test: exec: "tmux": executable file not found in $PATH`
	if !strings.Contains(output, diagnostic) {
		t.Fatalf("redactor destroyed diagnostic: %q", output)
	}
}

func TestSafeDaemonLogKeepsLifecycleAndDropsArbitraryText(t *testing.T) {
	input := []byte(strings.Join([]string{
		"2026-07-29T10:00:00Z daemon started version=0.11.0",
		"2026-07-29T10:00:01Z daemon failed code=bind_failed customer=secret",
		"2026-07-29T10:00:02Z prompt private instructions",
		"2026-07-29T10:00:03Z Authorization: Bearer top-secret",
	}, "\n"))
	output := string(safeDaemonLog(input))
	if !strings.Contains(output, "daemon started") ||
		!strings.Contains(output, "daemon failed code=bind_failed") {
		t.Fatalf("safe daemon log lost lifecycle data: %q", output)
	}
	for _, forbidden := range []string{"customer=secret", "private instructions", "top-secret"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("safe daemon log leaked %q: %q", forbidden, output)
		}
	}
}

func TestSafeDaemonLogRejectsMalformedTimestampToken(t *testing.T) {
	output := string(safeDaemonLog([]byte(
		"ABCD-EF-GHTIJ:KL:MNO daemon started\n",
	)))
	if output != "" {
		t.Fatalf("malformed timestamp entered lifecycle log: %q", output)
	}
}
