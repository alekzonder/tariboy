package compose

import (
	"os"
	"path/filepath"
	"strings"
)

// resolveCwd expands compose cwd variables into a concrete path, client-side:
//
//	$CWD                    -> the compose file's directory
//	$HOME / ${HOME} / ~<sep> -> the user's home directory
//
// Empty in, empty out. The daemon validates the result (absolute + existing dir);
// $CWD is a compose-only token because only the reconciler knows the file's dir.
func resolveCwd(raw, composeDir string) string {
	if raw == "" {
		return ""
	}
	s := strings.ReplaceAll(raw, "$CWD", composeDir)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		s = strings.ReplaceAll(s, "${HOME}", home)
		s = strings.ReplaceAll(s, "$HOME", home)
		if s == "~" {
			s = home
		} else if strings.HasPrefix(s, "~/") {
			s = filepath.Join(home, s[2:])
		}
	}
	return filepath.Clean(s)
}

// effectiveCwd is resolveCwd with a compose-specific default: when an agent
// omits cwd, it runs in the compose file's directory (the project) rather than
// the daemon's default agent home, so `make build/test` and git operate on the
// repo the compose file lives in. An empty composeDir
// yields empty (no default to impose).
func effectiveCwd(raw, composeDir string) string {
	if raw == "" {
		if composeDir == "" {
			return ""
		}
		return filepath.Clean(composeDir)
	}
	return resolveCwd(raw, composeDir)
}
