package agent

import (
	"math/rand"
	"strings"
	"testing"
)

func TestGenerateName(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	n := GenerateName(r)
	if !strings.Contains(n, "-") {
		t.Fatalf("name %q is not adjective-noun", n)
	}
	parts := strings.SplitN(n, "-", 2)
	if parts[0] == "" || parts[1] == "" {
		t.Fatalf("empty component in %q", n)
	}
}

func TestValidName(t *testing.T) {
	valid := []string{"echo", "my-agent", "a", "quiet-otter", "a1_b-2"}
	for _, n := range valid {
		if !ValidName(n) {
			t.Errorf("ValidName(%q) = false, want true", n)
		}
	}
	invalid := []string{"../x", "a/b", "..", "", "/etc", "Upper", "-lead", "sp ace", ".hidden", "a..b/c"}
	for _, n := range invalid {
		if ValidName(n) {
			t.Errorf("ValidName(%q) = true, want false", n)
		}
	}
}

func TestGeneratedNamesAreValid(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	for i := 0; i < 200; i++ {
		if n := GenerateName(r); !ValidName(n) {
			t.Fatalf("generated name %q is not valid", n)
		}
	}
}
