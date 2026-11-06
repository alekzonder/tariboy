package version

import "testing"

func mustSemVer(t *testing.T, s string) SemVer {
	t.Helper()
	v, err := ParseSemVer(s)
	if err != nil {
		t.Fatalf("ParseSemVer(%q) = %v", s, err)
	}
	return v
}

func TestParseSemVerRejectsInvalidStrings(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"", "v1.2.3", "1.2", "1.2.3.4", "01.2.3", "1.02.3", "1.2.03", "1.2.3+b", " 1.2.3", "1.2.3-", "1.2.3-01", "1.2.3-a..b", "a.b.c", "1.2.-3"} {
		if _, err := ParseSemVer(s); err == nil {
			t.Errorf("ParseSemVer(%q) = nil error, want an error", s)
		}
	}
}

func TestParseSemVerRoundTrips(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"0.0.0", "0.110.0", "1.2.3-rc.1", "10.20.30-alpha.beta-1.0"} {
		if got := mustSemVer(t, s).String(); got != s {
			t.Errorf("ParseSemVer(%q).String() = %q", s, got)
		}
	}
}

func TestCompareFollowsSemVerPrecedence(t *testing.T) {
	t.Parallel()
	// Each version is lower than the next (SemVer 2.0.0, item 11).
	ordered := []string{
		"0.9.9", "0.10.0", "0.10.1", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta",
		"1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.1.0", "2.0.0",
	}
	for i := 0; i+1 < len(ordered); i++ {
		a, b := mustSemVer(t, ordered[i]), mustSemVer(t, ordered[i+1])
		if a.Compare(b) != -1 || b.Compare(a) != 1 {
			t.Errorf("Compare(%s, %s) = %d, want -1", a, b, a.Compare(b))
		}
		if a.Compare(a) != 0 {
			t.Errorf("Compare(%s, %s) = %d, want 0", a, a, a.Compare(a))
		}
	}
}

func TestConstraintBoundsAreInclusive(t *testing.T) {
	t.Parallel()
	c, err := ParseConstraint("0.110.0", "0.120.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, ok := range []string{"0.110.0", "0.115.3", "0.120.0"} {
		if err := c.Check(mustSemVer(t, ok)); err != nil {
			t.Errorf("Check(%s) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"0.109.9", "0.110.0-rc.1", "0.120.1"} {
		if err := c.Check(mustSemVer(t, bad)); err == nil {
			t.Errorf("Check(%s) = nil, want an error", bad)
		}
	}
	err = c.Check(mustSemVer(t, "0.109.0"))
	if want := "version 0.109.0 does not satisfy >= 0.110.0, <= 0.120.0"; err == nil || err.Error() != want {
		t.Errorf("Check error = %v, want %q", err, want)
	}
	if got, want := c.String(), ">= 0.110.0, <= 0.120.0"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestConstraintWithOneBound(t *testing.T) {
	t.Parallel()
	minOnly, err := ParseConstraint("1.0.0-rc.1", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := minOnly.Check(mustSemVer(t, "1.0.0-rc.1")); err != nil {
		t.Errorf("min only: Check(1.0.0-rc.1) = %v", err)
	}
	if err := minOnly.Check(mustSemVer(t, "99.0.0")); err != nil {
		t.Errorf("min only: Check(99.0.0) = %v", err)
	}
	if err := minOnly.Check(mustSemVer(t, "1.0.0-beta")); err == nil {
		t.Error("min only: Check(1.0.0-beta) = nil, want an error")
	}
	if got := minOnly.String(); got != ">= 1.0.0-rc.1" {
		t.Errorf("min only: String() = %q", got)
	}
	maxOnly, err := ParseConstraint("", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := maxOnly.Check(mustSemVer(t, "0.0.1")); err != nil {
		t.Errorf("max only: Check(0.0.1) = %v", err)
	}
	if err := maxOnly.Check(mustSemVer(t, "1.0.1")); err == nil {
		t.Error("max only: Check(1.0.1) = nil, want an error")
	}
	if got := maxOnly.String(); got != "<= 1.0.0" {
		t.Errorf("max only: String() = %q", got)
	}
	none, err := ParseConstraint("", "")
	if err != nil || !none.IsZero() || minOnly.IsZero() {
		t.Fatalf("IsZero: none=%v (err %v), minOnly=%v", none.IsZero(), err, minOnly.IsZero())
	}
	if err := none.Check(mustSemVer(t, "0.0.0")); err != nil {
		t.Errorf("no bounds: Check = %v", err)
	}
}

func TestParseConstraintRejectsMinAboveMax(t *testing.T) {
	t.Parallel()
	_, err := ParseConstraint("0.120.0", "0.110.0")
	if want := "min 0.120.0 is greater than max 0.110.0"; err == nil || err.Error() != want {
		t.Fatalf("ParseConstraint error = %v, want %q", err, want)
	}
}

func TestParseConstraintRejectsInvalidBound(t *testing.T) {
	t.Parallel()
	if _, err := ParseConstraint("v1.0.0", ""); err == nil {
		t.Error("invalid min: want an error")
	}
	if _, err := ParseConstraint("", "1.0"); err == nil {
		t.Error("invalid max: want an error")
	}
}
