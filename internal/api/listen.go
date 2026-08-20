package api

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

type authenticatedRequestKey struct{}

func requestAuthenticated(r *http.Request) bool {
	authenticated, _ := r.Context().Value(authenticatedRequestKey{}).(bool)
	return authenticated
}

type Listen struct {
	Network   string // "unix" or "tcp"
	Addr      string // socket path (may be "" = default) or host:port
	AuthToken string
}

func ParseListen(spec, authTokenFile string) (Listen, error) {
	token := ""
	if authTokenFile != "" {
		b, err := os.ReadFile(authTokenFile)
		if err != nil {
			return Listen{}, fmt.Errorf("auth token file: %w", err)
		}
		token = strings.TrimSpace(string(b))
		if token == "" {
			return Listen{}, fmt.Errorf("auth token file %s is empty", authTokenFile)
		}
	}
	switch {
	case spec == "unix":
		return Listen{Network: "unix", AuthToken: token}, nil
	case strings.HasPrefix(spec, "unix:"):
		p := strings.TrimPrefix(spec, "unix:")
		if p == "" {
			return Listen{}, fmt.Errorf("empty unix socket path in %q", spec)
		}
		return Listen{Network: "unix", Addr: p, AuthToken: token}, nil
	case strings.HasPrefix(spec, "tcp:"):
		addr := strings.TrimPrefix(spec, "tcp:")
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return Listen{}, fmt.Errorf("bad tcp address %q: %w", addr, err)
		}
		ip := net.ParseIP(host)
		loopback := ip != nil && ip.IsLoopback()
		if !loopback && token == "" {
			return Listen{}, fmt.Errorf("listening on non-loopback %s requires --auth-token-file (spec §13)", addr)
		}
		return Listen{Network: "tcp", Addr: addr, AuthToken: token}, nil
	default:
		return Listen{}, fmt.Errorf("bad --listen %q: want unix | unix:/path.sock | tcp:HOST:PORT", spec)
	}
}

func AuthMiddleware(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get("Authorization")
		// EventSource and WebSocket browser APIs cannot set an Authorization header,
		// so the SSE stream route (/events) and terminal websocket route (/terminal)
		// ALONE also accept the bearer as a ?token= query param. Scoped to paths
		// ending in "/events" or "/terminal", plus the exact Tasks socket route,
		// so no other route exposes a token-in-URL
		// surface. accessLog logs only r.URL.Path (not the query), so the token is
		// not logged; federated daemons must run over TLS (the token rides in the URL).
		queryTokenRoute := strings.HasSuffix(r.URL.Path, "/events") ||
			strings.HasSuffix(r.URL.Path, "/terminal") ||
			r.URL.Path == "/api/tasks/ws"
		if provided == "" && queryTokenRoute {
			if q := r.URL.Query().Get("token"); q != "" {
				provided = "Bearer " + q
			}
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte("Bearer "+token)) != 1 {
			WriteErr(w, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
			return
		}
		ctx := context.WithValue(r.Context(), authenticatedRequestKey{}, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
