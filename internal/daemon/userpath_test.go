package daemon

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyUserPathReplacesInheritedPath(t *testing.T) {
	restoreEnv(t, "SHELL")
	restoreEnv(t, "PATH")
	t.Setenv("SHELL", "/bin/test-shell")
	t.Setenv("PATH", "/inherited/secret/path")

	var logs bytes.Buffer
	log := userPathTestLogger(&logs)
	called := 0
	applyUserPath(context.Background(), log, func(_ context.Context, shell string) (string, error) {
		called++
		if shell != "/bin/test-shell" {
			t.Fatalf("resolver shell = %q, want /bin/test-shell", shell)
		}
		if got := os.Getenv("PATH"); got != "/inherited/secret/path" {
			t.Fatalf("PATH during resolution = %q, want inherited value", got)
		}
		return "/resolved/user/path", nil
	})

	if called != 1 {
		t.Fatalf("resolver calls = %d, want 1", called)
	}
	if got := os.Getenv("PATH"); got != "/resolved/user/path" {
		t.Fatalf("PATH seen by later startup wiring = %q, want resolved value", got)
	}
	if got := logs.String(); got != "" {
		t.Fatalf("unexpected log output: %q", got)
	}
}

func TestApplyUserPathFailureKeepsInheritedPathAndLogsSafeWarning(t *testing.T) {
	restoreEnv(t, "SHELL")
	restoreEnv(t, "PATH")
	t.Setenv("SHELL", "/bin/test-shell")
	t.Setenv("PATH", "/inherited/secret/path")

	var logs bytes.Buffer
	log := userPathTestLogger(&logs)
	applyUserPath(context.Background(), log, func(context.Context, string) (string, error) {
		return "/candidate/secret/path", errors.New("probe exposed candidate output")
	})

	if got := os.Getenv("PATH"); got != "/inherited/secret/path" {
		t.Fatalf("PATH = %q, want inherited value", got)
	}
	// The class of failure must be in the log: without it a stalled probe, an
	// unusable shell and a malformed result are the same single line, and telling
	// them apart costs a support round trip.
	const wantLog = "level=WARN msg=\"resolve user PATH: keeping inherited PATH\" reason=unknown\n"
	gotLog := logs.String()
	for _, secret := range []string{
		"/inherited/secret/path",
		"/candidate/secret/path",
		"probe exposed candidate output",
	} {
		if strings.Contains(gotLog, secret) {
			t.Fatalf("log output disclosed %q: %q", secret, gotLog)
		}
	}
	if gotLog != wantLog {
		t.Fatalf("log output = %q, want %q", gotLog, wantLog)
	}
}

// A launcher that started tariboyd inside the account's login shell already
// paid for the account's startup files, and the inherited PATH is the real one.
// Probing again would spend that second twice and risk discarding a PATH that is
// already correct.
func TestApplyUserPathSkipsProbeWhenLauncherSuppliedShellEnvironment(t *testing.T) {
	restoreEnv(t, "SHELL")
	restoreEnv(t, "PATH")
	restoreEnv(t, "TARIBOY_SHELL_ENV")
	t.Setenv("SHELL", "/bin/test-shell")
	t.Setenv("PATH", "/login/shell/path")
	t.Setenv("TARIBOY_SHELL_ENV", "1")

	var logs bytes.Buffer
	log := userPathTestLogger(&logs)
	called := 0
	applyUserPath(context.Background(), log, func(context.Context, string) (string, error) {
		called++
		return "/candidate/secret/path", nil
	})

	if called != 0 {
		t.Fatalf("resolver calls = %d, want 0", called)
	}
	if got := os.Getenv("PATH"); got != "/login/shell/path" {
		t.Fatalf("PATH = %q, want the login-shell value untouched", got)
	}
	if strings.Contains(logs.String(), "WARN") {
		t.Fatalf("inheriting the login-shell environment is not a fallback: %q", logs.String())
	}
}

func TestApplyUserPathEmptyOrMissingShellFallsBackWithoutResolving(t *testing.T) {
	for _, tc := range []struct {
		name     string
		setShell bool
	}{
		{name: "empty", setShell: true},
		{name: "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restoreEnv(t, "SHELL")
			restoreEnv(t, "PATH")
			if tc.setShell {
				t.Setenv("SHELL", "")
			} else if err := os.Unsetenv("SHELL"); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", "/inherited/secret/path")

			var logs bytes.Buffer
			log := userPathTestLogger(&logs)
			called := 0
			applyUserPath(context.Background(), log, func(context.Context, string) (string, error) {
				called++
				return "/candidate/secret/path", nil
			})

			if called != 0 {
				t.Fatalf("resolver calls = %d, want 0", called)
			}
			if got := os.Getenv("PATH"); got != "/inherited/secret/path" {
				t.Fatalf("PATH = %q, want inherited value", got)
			}
			const wantLog = "level=WARN msg=\"resolve user PATH: keeping inherited PATH\" reason=no_shell\n"
			if got := logs.String(); got != wantLog {
				t.Fatalf("log output = %q, want %q", got, wantLog)
			}
		})
	}
}

func TestRunAppliesUserPathBeforeEarlyStartupFailure(t *testing.T) {
	restoreEnv(t, "SHELL")
	restoreEnv(t, "PATH")
	t.Setenv("SHELL", "/bin/test-shell")
	t.Setenv("PATH", "/inherited/secret/path")

	baseFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(baseFile, []byte("isolated startup failure"), 0o600); err != nil {
		t.Fatal(err)
	}
	called := 0
	err := Run(context.Background(), Options{
		BaseDir: baseFile,
		UserPathResolver: func(_ context.Context, shell string) (string, error) {
			called++
			if shell != "/bin/test-shell" {
				t.Fatalf("resolver shell = %q, want /bin/test-shell", shell)
			}
			return "/resolved/user/path", nil
		},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want isolated early startup failure")
	}
	if called != 1 {
		t.Fatalf("resolver calls = %d, want 1", called)
	}
	if got := os.Getenv("PATH"); got != "/resolved/user/path" {
		t.Fatalf("PATH after Run() = %q, want resolved value", got)
	}
}

func userPathTestLogger(dst *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(dst, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return attr
		},
	}))
}

func restoreEnv(t *testing.T, key string) {
	t.Helper()
	value, ok := os.LookupEnv(key)
	t.Cleanup(func() {
		var err error
		if ok {
			err = os.Setenv(key, value)
		} else {
			err = os.Unsetenv(key)
		}
		if err != nil {
			t.Errorf("restore %s: %v", key, err)
		}
	})
}
