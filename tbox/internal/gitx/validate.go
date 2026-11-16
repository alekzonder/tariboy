package gitx

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	scpPattern   = regexp.MustCompile(`^([A-Za-z0-9._-]+)@([A-Za-z0-9.-]+):(.+)$`)
	ownerRepo    = regexp.MustCompile(`^([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)$`)
	userInfoMask = regexp.MustCompile(`://[^/@\s]*@`)
	refForbidden = "~^:?*[\\"
)

const maxRefLength = 255

// NormalizeURL validates a git remote and returns its canonical form. It
// accepts https://, ssh://, scp-style user@host:path and owner/repo (GitHub).
// A URL with a password or https user-info, http://, file://, transport
// helpers such as ext::, option-like values and local paths are rejected;
// allowLocal (tests) also permits http://, file:// and local paths.
func NormalizeURL(s string, allowLocal bool) (string, error) {
	s = strings.TrimSpace(s)
	if err := checkRawURL(s); err != nil {
		return "", err
	}
	if strings.Contains(s, "://") {
		return normalizeScheme(s, allowLocal)
	}
	if strings.Contains(s, "::") {
		return "", errors.New("git URL uses a transport helper")
	}
	if m := scpPattern.FindStringSubmatch(s); m != nil {
		return normalizeSCP(s, m[2], m[3])
	}
	if m := ownerRepo.FindStringSubmatch(s); m != nil && !allDots(m[1]) && !allDots(m[2]) && !filepath.IsAbs(s) {
		return "https://github.com/" + m[1] + "/" + strings.TrimSuffix(m[2], ".git") + ".git", nil
	}
	if allowLocal && (filepath.IsAbs(s) || strings.HasPrefix(s, "./")) {
		return s, nil
	}
	return "", errors.New("unsupported git URL: want https://, ssh://, user@host:path or owner/repo")
}

func checkRawURL(s string) error {
	if s == "" {
		return errors.New("git URL is empty")
	}
	if strings.HasPrefix(s, "-") {
		return errors.New("git URL must not start with -")
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == ' ' {
			return errors.New("git URL holds a space or control character")
		}
	}
	return nil
}

func normalizeScheme(s string, allowLocal bool) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", errors.New("invalid git URL")
	}
	switch u.Scheme {
	case "https", "ssh":
	case "http", "file":
		if !allowLocal {
			return "", fmt.Errorf("git URL scheme %q is not allowed", u.Scheme)
		}
	default:
		return "", fmt.Errorf("git URL scheme %q is not allowed", u.Scheme)
	}
	if u.Scheme != "file" && (u.Hostname() == "" || strings.HasPrefix(u.Hostname(), "-")) {
		return "", errors.New("git URL has no valid host")
	}
	if u.User != nil {
		if _, hasPass := u.User.Password(); hasPass || u.Scheme != "ssh" {
			return "", errors.New("git URL must not carry credentials")
		}
	}
	return u.String(), nil
}

func normalizeSCP(s, host, p string) (string, error) {
	if strings.HasPrefix(host, "-") || strings.HasPrefix(p, "-") || strings.Contains(p, "..") {
		return "", errors.New("invalid git URL")
	}
	return s, nil
}

func allDots(s string) bool { return strings.Trim(s, ".") == "" }

// CheckRef rejects a ref that is empty after trimming, starts with "-",
// holds "..", a space or a control character, or is longer than 255
// characters. It also rejects characters git forbids in ref names.
func CheckRef(ref string) error {
	if strings.TrimSpace(ref) == "" {
		return errors.New("git ref is empty")
	}
	if len(ref) > maxRefLength {
		return fmt.Errorf("git ref is longer than %d characters", maxRefLength)
	}
	if strings.HasPrefix(ref, "-") || strings.Contains(ref, "..") || strings.Contains(ref, "@{") {
		return errors.New("invalid git ref")
	}
	for _, r := range ref {
		if r <= 0x20 || r == 0x7f || strings.ContainsRune(refForbidden, r) {
			return errors.New("invalid git ref")
		}
	}
	return nil
}

// cleanSubpath returns subpath cleaned as a slash path inside a tree, or ""
// for the tree root.
func cleanSubpath(subpath string) (string, error) {
	if subpath == "" || subpath == "." {
		return "", nil
	}
	if strings.ContainsAny(subpath, "\\\x00") || path.IsAbs(subpath) {
		return "", fmt.Errorf("invalid path %q", subpath)
	}
	clean := path.Clean(subpath)
	if clean == "." {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "-") {
		return "", fmt.Errorf("path %q escapes the tree", subpath)
	}
	return clean, nil
}

func isHex(s string, minLen, maxLen int) bool {
	if len(s) < minLen || len(s) > maxLen {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

// maskCredentials hides user-info in any URL of a git message.
func maskCredentials(s string) string {
	return userInfoMask.ReplaceAllString(s, "://***@")
}
