package userpath

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestResolve(t *testing.T) {
	const (
		validPath       = "/opt/user tools/bin:/usr/bin"
		candidateSecret = "/private/candidate-secret/bin"
	)

	tests := []struct {
		name          string
		shell         func(t *testing.T) string
		want          string
		wantErr       string
		forbiddenText []string
		maxDuration   time.Duration
	}{
		{
			name: "extracts a delimited path from noisy startup output",
			shell: func(t *testing.T) string {
				return writeShell(t, 0o700, `
if [ "$#" -ne 2 ] || [ "$1" != "-ilc" ]; then
	printf '%s\n' 'argument-secret' >&2
	exit 91
fi
printf '%s\n' 'startup-output-secret'
printf '%s\n' 'stderr-startup-secret' >&2
PATH='/usr/local/bin:/usr/bin' /bin/sh -c "$2"
printf '%s\n' 'trailing-output-secret'
`)
			},
			want: "/usr/local/bin:/usr/bin",
		},
		{
			name: "preserves spaces in the resolved path",
			shell: func(t *testing.T) string {
				return writeShell(t, 0o700, `
PATH='/opt/user tools/bin:/usr/bin' /bin/sh -c "$2"
`)
			},
			want: validPath,
		},
		{
			name: "rejects an empty path",
			shell: func(t *testing.T) string {
				return writeShell(t, 0o700, `
printf '%s\n' 'empty-result-secret'
PATH='' /bin/sh -c "$2"
`)
			},
			wantErr:       "empty result",
			forbiddenText: []string{"empty-result-secret"},
		},
		{
			name: "rejects output without the delimiter",
			shell: func(t *testing.T) string {
				return writeShell(t, 0o700, `
printf '%s\n' 'missing-delimiter-secret'
`)
			},
			wantErr:       "delimiter missing",
			forbiddenText: []string{"missing-delimiter-secret"},
		},
		{
			name: "rejects a result containing a NUL",
			shell: func(t *testing.T) string {
				return writeShell(t, 0o700, `
printf '\000TARIBOY_PATH\000%s\000%s\000TARIBOY_PATH\000' '/private/candidate-secret/bin' 'embedded-secret'
`)
			},
			wantErr:       "invalid result",
			forbiddenText: []string{candidateSecret, "embedded-secret"},
		},
		{
			name: "rejects a relative shell path",
			shell: func(t *testing.T) string {
				return "relative-shell-secret"
			},
			wantErr:       "invalid shell",
			forbiddenText: []string{"relative-shell-secret"},
		},
		{
			name: "rejects a missing shell",
			shell: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing-shell-secret")
			},
			wantErr:       "invalid shell",
			forbiddenText: []string{"missing-shell-secret"},
		},
		{
			name: "rejects a directory as the shell",
			shell: func(t *testing.T) string {
				return t.TempDir()
			},
			wantErr: "invalid shell",
		},
		{
			name: "rejects a non-executable shell",
			shell: func(t *testing.T) string {
				return writeShell(t, 0o600, "exit 0\n")
			},
			wantErr: "invalid shell",
		},
		{
			name: "rejects output larger than the limit",
			shell: func(t *testing.T) string {
				return writeShell(t, 0o700, `
i=0
while [ "$i" -lt 70000 ]; do
	printf x
	i=$((i + 1))
done
`)
			},
			wantErr:       "output too large",
			forbiddenText: []string{strings.Repeat("x", 32)},
		},
		{
			name: "does not expose output from a failed probe",
			shell: func(t *testing.T) string {
				return writeShell(t, 0o700, `
printf '%s\n' 'failed-probe-secret'
exit 23
`)
			},
			wantErr:       "probe failed",
			forbiddenText: []string{"failed-probe-secret"},
		},
		{
			name: "times out a slow shell",
			shell: func(t *testing.T) string {
				return writeShell(t, 0o700, `
exec /bin/sleep 10
`)
			},
			wantErr:     "probe timed out",
			maxDuration: 8 * time.Second,
		},
		{
			name: "bounds pipes held open by a shell descendant",
			shell: func(t *testing.T) string {
				return writeShell(t, 0o700, `
/bin/sleep 1 &
exit 0
`)
			},
			wantErr:     "probe timed out",
			maxDuration: 500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			started := time.Now()
			got, err := Resolve(context.Background(), tt.shell(t))
			elapsed := time.Since(started)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Resolve() error = %v", err)
				}
				if got != tt.want {
					t.Fatalf("Resolve() = %q, want %q", got, tt.want)
				}
				return
			}

			if err == nil {
				t.Fatalf("Resolve() error = nil, want category %q", tt.wantErr)
			}
			wantError := "resolve user path: " + tt.wantErr
			if err.Error() != wantError {
				t.Fatalf("Resolve() error = %q, want %q", err, wantError)
			}
			for _, forbidden := range tt.forbiddenText {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatalf("Resolve() error exposes forbidden text %q: %q", forbidden, err)
				}
			}
			if tt.maxDuration > 0 && elapsed > tt.maxDuration {
				t.Fatalf("Resolve() took %s, want at most %s", elapsed, tt.maxDuration)
			}
		})
	}
}

func TestResolveKillsLongLivedDescendant(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "descendant.pid")
	t.Setenv("TARIBOY_TEST_DESCENDANT_PID", pidPath)
	t.Cleanup(func() {
		data, err := os.ReadFile(pidPath)
		if err != nil {
			return
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil && processExists(pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	shell := writeShell(t, 0o700, `
/bin/sleep 30 </dev/null >/dev/null 2>&1 &
printf '%s\n' "$!" > "$TARIBOY_TEST_DESCENDANT_PID"
PATH='/usr/local/bin:/usr/bin' /bin/sh -c "$2"
`)

	got, err := Resolve(context.Background(), shell)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "/usr/local/bin:/usr/bin" {
		t.Fatalf("Resolve() = %q, want /usr/local/bin:/usr/bin", got)
	}

	pid := readPID(t, pidPath)
	deadline := time.Now().Add(500 * time.Millisecond)
	for processExists(pid) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if processExists(pid) {
		t.Fatalf("login-shell descendant pid %d survived Resolve return", pid)
	}
}

func writeShell(t *testing.T, mode os.FileMode, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "shell")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), mode); err != nil {
		t.Fatalf("write shell: %v", err)
	}
	return path
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read descendant pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		t.Fatalf("parse descendant pid: %q", data)
	}
	return pid
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// A failed probe is currently indistinguishable in the daemon log from a missing
// shell, which is why diagnosing one costs a support round trip. Reason names the
// class without ever carrying the shell's output, the candidate path, or the
// underlying message.
func TestReasonNamesTheFailureClassWithoutDisclosingDetails(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "invalid shell", err: fmt.Errorf("resolve user path: %w", errInvalidShell), want: "invalid_shell"},
		{name: "timeout", err: fmt.Errorf("resolve user path: %w", errProbeTimedOut), want: "timeout"},
		{name: "probe failed", err: fmt.Errorf("resolve user path: %w", errProbeFailed), want: "probe_failed"},
		{name: "too large", err: fmt.Errorf("resolve user path: %w", errOutputTooLarge), want: "output_too_large"},
		{name: "delimiter", err: fmt.Errorf("resolve user path: %w", errDelimiterAbsent), want: "delimiter_missing"},
		{name: "empty", err: fmt.Errorf("resolve user path: %w", errEmptyResult), want: "empty_result"},
		{name: "invalid", err: fmt.Errorf("resolve user path: %w", errInvalidResult), want: "invalid_result"},
		{name: "unknown", err: errors.New("/secret/candidate/path rejected"), want: "unknown"},
		{name: "nil", err: nil, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Reason(tc.err); got != tc.want {
				t.Fatalf("Reason() = %q, want %q", got, tc.want)
			}
		})
	}
}
