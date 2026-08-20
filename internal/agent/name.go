package agent

import (
	"errors"
	"math/rand"
	"regexp"
)

// ErrInvalidName is returned by Store.Create (and reported by callers) when an
// agent name fails ValidName. It exists so a traversing/malformed name can be
// distinguished from other persistence errors.
var ErrInvalidName = errors.New("invalid agent name")

// nameRE anchors the legal agent-name charset: lowercase alnum start, then
// alnum/underscore/dash. No dots, slashes or separators, so a name can never
// lexically escape a directory it is joined into (filepath.Join(agentsDir,
// name, ...)). This mirrors plugins.ValidName and the retention CLI's
// checkAgentOrKeyword, and every generated adjective-noun name satisfies it.
var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// ValidName reports whether name is a legal agent name. It is the single source
// of truth reused by the loop manager and the store to refuse path-traversal
// names such as "../victim" at agent creation — never duplicate the regex, call
// this instead.
func ValidName(name string) bool { return nameRE.MatchString(name) }

var adjectives = []string{
	"quiet", "brave", "calm", "eager", "gentle", "keen", "lively", "mellow",
	"nimble", "proud", "swift", "witty", "bold", "clever", "daring", "fond",
}

var nouns = []string{
	"otter", "falcon", "badger", "heron", "lynx", "marmot", "raven", "sparrow",
	"panda", "tapir", "gecko", "ibis", "koala", "moose", "newt", "quokka",
}

// GenerateName returns a docker-style adjective-noun label; r is injected for tests.
func GenerateName(r *rand.Rand) string {
	return adjectives[r.Intn(len(adjectives))] + "-" + nouns[r.Intn(len(nouns))]
}
