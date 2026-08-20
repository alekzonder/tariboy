package toolscli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/version"
)

// whoami is the first command an agent runs when its tools behave strangely, so
// it names both sides of the conversation: the client build that is printing
// and the daemon build that answered (SUPER-224 §4).
func TestToolsWhoamiPrintsClientAndDaemonVersions(t *testing.T) {
	sock := startAgentAPI(t, []string{"whoami", "loop", "messages"}, "")
	for _, args := range [][]string{{"whoami"}, {"whoami", "--json"}} {
		var out, errOut bytes.Buffer
		if code := Run(sock, args, &out, &errOut); code != 0 {
			t.Fatalf("%v exit=%d err=%q", args, code, errOut.String())
		}
		got := out.String()
		for _, want := range []string{"client_version", "daemon_version", version.Version} {
			if !strings.Contains(got, want) {
				t.Fatalf("%v out=%q, missing %q", args, got, want)
			}
		}
	}
}
