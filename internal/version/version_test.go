package version

import (
	"regexp"
	"testing"
)

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func TestUserAgent(t *testing.T) {
	if !versionPattern.MatchString(Version) {
		t.Fatalf("Version = %q, want numeric MAJOR.MINOR.PATCH with an optional prerelease suffix", Version)
	}
	if got, want := UserAgent(), "tariboy/"+Version; got != want {
		t.Fatalf("UserAgent() = %q, want %q", got, want)
	}
}
