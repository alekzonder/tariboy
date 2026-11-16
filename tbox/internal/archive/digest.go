package archive

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// contentDigestHeader names the canonical form ContentDigest hashes (same
// header as internal/image/content_digest.go).
const contentDigestHeader = "tariboy-image-content/1\n"

// manifestFieldsOutsideContent are the manifest keys that do not describe
// content.
var manifestFieldsOutsideContent = []string{"built_at", "tag", "digest", "id"}

// ContentDigest is the identity of an archive's content, exactly as
// internal/image contentDigest computes it: the sha256 of
//
//	tariboy-image-content/1
//	<mode> <sha256 of body> <path>
//
// with one line per member sorted by path, mode as four octal digits, and
// manifest.json hashed without built_at, tag, digest and id, re-encoded with
// sorted keys and no spaces.
func ContentDigest(members []Member) (string, error) {
	sorted := make([]Member, len(members))
	copy(sorted, members)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	var text strings.Builder
	text.WriteString(contentDigestHeader)
	for i, m := range sorted {
		if i > 0 && m.Name == sorted[i-1].Name {
			return "", fmt.Errorf("duplicate image archive member %q", m.Name)
		}
		body := m.Body
		if m.Name == manifestName {
			canonical, err := canonicalManifest(body)
			if err != nil {
				return "", err
			}
			body = canonical
		}
		fmt.Fprintf(&text, "%04o %s %s\n", m.Mode, sha256hex(body), m.Name)
	}
	sum := sha256.Sum256([]byte(text.String()))
	return hex.EncodeToString(sum[:]), nil
}

func canonicalManifest(body []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var fields map[string]any
	if err := decoder.Decode(&fields); err != nil {
		return nil, fmt.Errorf("parse image manifest: %w", err)
	}
	for _, key := range manifestFieldsOutsideContent {
		delete(fields, key)
	}
	return json.Marshal(fields)
}

// templateHash is internal/image templateHash: the sha256 of the compact JSON
// of schema_version 2 and the entries.
func templateHash(entries []TemplateEntry) (string, error) {
	canonical, err := json.Marshal(struct {
		SchemaVersion int             `json:"schema_version"`
		Entries       []TemplateEntry `json:"entries"`
	}{2, entries})
	if err != nil {
		return "", err
	}
	return sha256hex(canonical), nil
}

func sha256hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
