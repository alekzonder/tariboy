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
		wantErr bool
	}{
		{name: "bare update targets latest", args: []string{"update"}, version: "latest"},
		{name: "explicit version", args: []string{"update", "0.64.0"}, version: "0.64.0"},
		{name: "latest with force", args: []string{"update", "latest", "--force"}, version: "latest", force: true},
		{name: "unknown flag", args: []string{"update", "--yolo"}, wantErr: true},
		{name: "two versions", args: []string{"update", "1.0.0", "2.0.0"}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			version, force, err := parseUpdateArgs(tc.args[1:])
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseUpdateArgs(%v) accepted invalid arguments", tc.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseUpdateArgs(%v): %v", tc.args, err)
			}
			if version != tc.version || force != tc.force {
				t.Fatalf("parsed version %q force %v, want %q %v", version, force, tc.version, tc.force)
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
	if !strings.Contains(out.String(), "tariboy update") {
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
