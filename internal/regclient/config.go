// Package regclient is the client side of the tariboy-store push/pull
// protocol: a TLS-trusting HTTP client plus the per-registry credentials file.
package regclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Registry holds one store's credentials. CA is an optional path to a CA/cert
// PEM to trust (for self-signed or private-CA stores); empty means system roots.
type Registry struct {
	Token string `json:"token"`
	CA    string `json:"ca,omitempty"`
}

// Registries is the parsed ~/.tariboy/registries.json.
type Registries struct {
	path string
	Map  map[string]Registry `json:"registries"`
}

func RegistriesPath(baseDir string) string { return filepath.Join(baseDir, "registries.json") }

func normalizeURL(u string) string { return strings.TrimRight(u, "/") }

func LoadRegistries(baseDir string) (*Registries, error) {
	p := RegistriesPath(baseDir)
	rs := &Registries{path: p, Map: map[string]Registry{}}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return rs, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, rs); err != nil {
		return nil, err
	}
	if rs.Map == nil {
		rs.Map = map[string]Registry{}
	}
	rs.path = p
	return rs, nil
}

func (rs *Registries) Get(url string) (Registry, bool) {
	reg, ok := rs.Map[normalizeURL(url)]
	return reg, ok
}

func (rs *Registries) Set(url string, reg Registry) { rs.Map[normalizeURL(url)] = reg }

// Save writes the file at 0600 (it holds bearer tokens).
func (rs *Registries) Save() error {
	b, err := json.MarshalIndent(map[string]any{"registries": rs.Map}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(rs.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(rs.path, b, 0o600)
}
