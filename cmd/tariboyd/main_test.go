package main

import (
	"errors"
	"flag"
	"testing"
)

// resolveHTTPAddr is the whole point of keeping the deprecated alias: smoke runs
// pass `--web-addr ""` to DISABLE the listener, so "flag absent" and "flag set to
// empty" must stay distinguishable.
func TestResolveHTTPAddr(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"default", nil, "127.0.0.1:9990"},
		{"http-addr explicit", []string{"--http-addr", "127.0.0.1:9993"}, "127.0.0.1:9993"},
		{"http-addr empty disables", []string{"--http-addr", ""}, ""},
		{"deprecated alias", []string{"--web-addr", "127.0.0.1:9994"}, "127.0.0.1:9994"},
		{"deprecated alias empty disables", []string{"--web-addr", ""}, ""},
		{"http-addr wins over alias", []string{"--web-addr", "127.0.0.1:1", "--http-addr", "127.0.0.1:2"}, "127.0.0.1:2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := parseFlags(tc.args)
			if err != nil {
				t.Fatalf("parseFlags: %v", err)
			}
			if got.HTTPAddr != tc.want {
				t.Fatalf("HTTPAddr = %q, want %q", got.HTTPAddr, tc.want)
			}
		})
	}
}

func TestParseFlagsVersionSwitch(t *testing.T) {
	_, showVer, err := parseFlags([]string{"--version"})
	if err != nil || !showVer {
		t.Fatalf("parseFlags(--version) = showVer %v err %v", showVer, err)
	}
}

// -h/--help must stay a SUCCESS path. The flag package reports it as ErrHelp,
// and main translates that to exit 0 — exiting non-zero would break any wrapper
// that runs `tariboyd --help` as a liveness/health check.
func TestParseFlagsHelpIsErrHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		if _, _, err := parseFlags([]string{arg}); !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("parseFlags(%q) err = %v, want flag.ErrHelp", arg, err)
		}
	}
}

// An unknown flag stays a usage ERROR, distinct from the help path above.
func TestParseFlagsRejectsUnknownFlag(t *testing.T) {
	_, _, err := parseFlags([]string{"--nope"})
	if err == nil || errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parseFlags(--nope) err = %v, want a non-help error", err)
	}
}
