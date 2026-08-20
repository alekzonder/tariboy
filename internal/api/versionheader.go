package api

import (
	"net/http"

	"github.com/alekzonder/tariboy/internal/version"
)

// VersionHeader stamps version.Header with the daemon's build on everything
// next writes — successes, error envelopes and 404s alike. It is set before
// next runs, so it is part of the header block whichever status the handler
// ends up writing.
//
// A header rather than a dedicated route: the client learns the daemon's
// version from whatever call it was already making, so a drift shows up on the
// first command an agent runs and not only on `whoami`. Serving is never
// refused on a mismatch — an old client must keep working.
func VersionHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(version.Header, version.Version)
		next.ServeHTTP(w, r)
	})
}
