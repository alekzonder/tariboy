package image

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// contentDigestHeader names the canonical form contentDigest hashes. A change
// to the form needs a new header.
const contentDigestHeader = "tariboy-image-content/1\n"

// manifestFieldsOutsideContent are the manifest keys that do not describe
// content: the build time, the tag a caller asked for, and the identity
// derived from the content itself.
var manifestFieldsOutsideContent = []string{"built_at", "tag", "digest", "id"}

// contentDigest is the identity of an image archive's content. It hashes this
// text:
//
//	tariboy-image-content/1
//	<mode> <sha256 of body> <path>
//	...
//
// with one line per member sorted by path (byte order), mode as four octal
// digits, and manifest.json hashed in canonical form: its JSON object without
// built_at, tag, digest and id, re-encoded with sorted keys and no spaces. Two
// archives with the same members and manifest fields share a digest whatever
// their build time or member order.
func contentDigest(members map[string]portableImageMember) (string, error) {
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	var text strings.Builder
	text.WriteString(contentDigestHeader)
	for _, name := range names {
		member := members[name]
		body := member.body
		if name == "manifest.json" {
			canonical, err := canonicalManifest(body)
			if err != nil {
				return "", err
			}
			body = canonical
		}
		fmt.Fprintf(&text, "%04o %s %s\n", member.mode, sha256hex(body), name)
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
