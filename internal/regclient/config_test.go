package regclient

import (
	"os"
	"testing"
)

func TestRegistriesRoundTripAndMode(t *testing.T) {
	base := t.TempDir()
	rs, err := LoadRegistries(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rs.Get("https://h:8443"); ok {
		t.Fatal("empty registries must not resolve")
	}
	rs.Set("https://h:8443/", Registry{Token: "tok", CA: "/ca.pem"})
	if err := rs.Save(); err != nil {
		t.Fatal(err)
	}
	// URL normalization: trailing slash does not create a second entry.
	rs2, _ := LoadRegistries(base)
	reg, ok := rs2.Get("https://h:8443")
	if !ok || reg.Token != "tok" || reg.CA != "/ca.pem" {
		t.Fatalf("reload = %+v,%v", reg, ok)
	}
	fi, err := os.Stat(RegistriesPath(base))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("registries.json mode = %o, want 600 (holds bearer tokens)", fi.Mode().Perm())
	}
}
