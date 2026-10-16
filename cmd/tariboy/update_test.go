package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseUpdateArgs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		version string
		force   bool
		cleanup bool
		keep    int
		wantErr bool
	}{
		{name: "bare update targets latest", args: []string{"update"}, version: "latest"},
		{name: "explicit version", args: []string{"update", "0.64.0"}, version: "0.64.0"},
		{name: "latest with force", args: []string{"update", "latest", "--force"}, version: "latest", force: true},
		{name: "cleanup keeps five by default", args: []string{"update", "--cleanup"}, version: "latest", cleanup: true, keep: 5},
		{name: "cleanup with keep", args: []string{"update", "0.82.0", "--cleanup", "--keep", "3"}, version: "0.82.0", cleanup: true, keep: 3},
		{name: "keep without cleanup", args: []string{"update", "--keep", "3"}, wantErr: true},
		{name: "keep zero", args: []string{"update", "--cleanup", "--keep", "0"}, wantErr: true},
		{name: "keep not a number", args: []string{"update", "--cleanup", "--keep", "abc"}, wantErr: true},
		{name: "keep without value", args: []string{"update", "--cleanup", "--keep"}, wantErr: true},
		{name: "unknown flag", args: []string{"update", "--yolo"}, wantErr: true},
		{name: "two versions", args: []string{"update", "1.0.0", "2.0.0"}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := parseUpdateArgs(tc.args[1:])
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseUpdateArgs(%v) accepted invalid arguments", tc.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseUpdateArgs(%v): %v", tc.args, err)
			}
			if opts.Version != tc.version || opts.Force != tc.force || opts.Cleanup != tc.cleanup || opts.Keep != tc.keep {
				t.Fatalf("parsed %+v, want version %q force %v cleanup %v keep %d", opts, tc.version, tc.force, tc.cleanup, tc.keep)
			}
		})
	}
}

func TestDispatchUpdateHelp(t *testing.T) {
	var out bytes.Buffer
	handled, code := dispatchUpdate(t.Context(), []string{"update", "--help"}, nil, &out, &out)
	if !handled || code != 0 {
		t.Fatalf("handled=%v code=%d, want true 0", handled, code)
	}
	if !strings.Contains(out.String(), "tariboy update") || !strings.Contains(out.String(), "--cleanup") {
		t.Fatalf("help output %q does not describe the command", out.String())
	}
}

func TestDispatchUpdateReportsFailure(t *testing.T) {
	var out, errOut bytes.Buffer
	getenv := func(key string) string {
		if key == "HOME" {
			return t.TempDir()
		}
		return ""
	}
	handled, code := dispatchUpdate(t.Context(), []string{"update", "0.64.0"}, getenv, &out, &errOut)
	if !handled || code != 1 {
		t.Fatalf("handled=%v code=%d, want true 1", handled, code)
	}
	if !strings.Contains(errOut.String(), "update:") {
		t.Fatalf("stderr %q does not name the failing command", errOut.String())
	}
}
