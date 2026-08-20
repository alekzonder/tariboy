package storesvc

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/image"
)

// Config configures the store service.
type Config struct {
	Addr          string // listen host:port
	TLSCert       string // TLS certificate PEM file
	TLSKey        string // TLS private key PEM file
	AllowInsecure bool   // opt out of mandatory TLS (local dev only, NEVER production)
	AnonPull      bool   // allow unauthenticated GET/HEAD
	DataDir       string // blob storage directory
	DBPath        string // SQLite catalog/token DB path
	Version       string // build version, surfaced by GET /v1/info
}

// Server is the tariboy-store HTTP service.
type Server struct {
	cfg  Config
	repo *Repo
	db   *DB
	ui   http.Handler // public SPA handler for non-/v1 paths (nil = no UI)
	http *http.Server
}

// New opens the repo+DB and builds the (auth-wrapped) handler. It enforces
// mandatory TLS: without cert+key it errors unless AllowInsecure is set (spec
// §13 — the store is the only externally-exposed service).
func New(cfg Config) (*Server, error) {
	if (cfg.TLSCert == "" || cfg.TLSKey == "") && !cfg.AllowInsecure {
		return nil, errors.New("tariboy-store requires TLS: set --tls-cert and --tls-key (or --allow-insecure for local dev, NEVER production)")
	}
	db, err := Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, repo: NewRepo(cfg.DataDir), db: db}
	s.http = &http.Server{Addr: cfg.Addr, Handler: s.rootMux()}
	return s, nil
}

func (s *Server) SeedToken(tok, scope string) error { return s.db.SeedToken(tok, scope) }
func (s *Server) Handler() http.Handler             { return s.http.Handler }
func (s *Server) Close() error                      { return s.db.Close() }

// SetUI installs the embedded store SPA handler, served (publicly, no auth) for
// every non-/v1 path. The login page itself cannot require a token to load; the
// bearer is enforced only on the /v1 API. Must be called before serving.
func (s *Server) SetUI(h http.Handler) { s.ui = h }

// rootMux composes the public SPA + public /v1/info + the auth-gated /v1 API.
// Only /v1/* carries authMiddleware; the SPA (and /v1/info) are public so the
// login screen loads and can discover the anon_pull posture pre-auth.
func (s *Server) rootMux() http.Handler {
	root := http.NewServeMux()
	root.HandleFunc("GET /v1/info", s.handleInfo)  // PUBLIC (more specific than /v1/)
	root.Handle("/v1/", s.authMiddleware(s.mux())) // auth-gated API
	root.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if s.ui != nil {
			s.ui.ServeHTTP(w, r)
			return
		}
		api.WriteErr(w, http.StatusNotFound, "not_found", "no ui configured")
	})
	return root
}

// handleInfo is a PUBLIC endpoint returning the store version + whether reads are
// anonymous, so the SPA can decide whether to present the login gate before any
// authenticated request. It exposes no secrets.
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	api.WriteOK(w, map[string]any{"version": s.cfg.Version, "anon_pull": s.cfg.AnonPull})
}

// ListenAndServeTLS serves the store over TLS (mandatory in production). When
// AllowInsecure is set and no cert is provided, it serves plain HTTP (dev only).
func (s *Server) ListenAndServeTLS() error {
	if s.cfg.AllowInsecure && (s.cfg.TLSCert == "" || s.cfg.TLSKey == "") {
		return s.http.ListenAndServe()
	}
	s.http.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return s.http.ListenAndServeTLS(s.cfg.TLSCert, s.cfg.TLSKey)
}

// Shutdown gracefully stops the HTTP server, then closes the DB. Callers that
// call Shutdown need not also call Close(); calling both is harmless (the
// second db.Close() returns an error that is ignored).
func (s *Server) Shutdown(ctx context.Context) error {
	err := s.http.Shutdown(ctx)
	_ = s.db.Close()
	return err
}

// authMiddleware gates the API by scope. Safe methods (GET/HEAD) need a read
// token (or are open under AnonPull); mutating methods need a readwrite token.
// It reuses the daemon's bearer technique (a token compared only via its sha256
// key — no timing oracle on the secret) and the api envelope for 401/403.
//
// Scope checks are fail-closed: SeedToken does no scope-value validation, so a
// stored scope that is neither "read" nor "readwrite" (e.g. a typo) grants
// nothing — an unrecognized scope is treated as insufficient (403), never as an
// implicit read grant.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		safe := r.Method == http.MethodGet || r.Method == http.MethodHead
		if safe && s.cfg.AnonPull {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		tok := ""
		if strings.HasPrefix(auth, "Bearer ") {
			tok = strings.TrimPrefix(auth, "Bearer ")
		}
		if tok == "" {
			api.WriteErr(w, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
			return
		}
		scope, ok, err := s.db.LookupToken(tok)
		if err != nil {
			api.WriteErr(w, http.StatusInternalServerError, "auth_error", "token lookup failed")
			return
		}
		if !ok {
			api.WriteErr(w, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
			return
		}
		if !safe {
			// Mutating routes require an exact readwrite scope.
			if scope != ScopeReadWrite {
				api.WriteErr(w, http.StatusForbidden, "forbidden", "token lacks readwrite scope")
				return
			}
		} else {
			// Safe routes require a recognized read-capable scope; an unknown
			// stored scope fails closed rather than silently granting reads.
			if scope != ScopeRead && scope != ScopeReadWrite {
				api.WriteErr(w, http.StatusForbidden, "forbidden", "token lacks read scope")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("HEAD /v1/images/{name}/{tag}", s.handleHead)
	mux.HandleFunc("PUT /v1/images/{name}/{tag}", s.handlePut)
	// A method-specific literal route ("GET .../tags") cannot coexist with the
	// broader-method wildcard ("HEAD .../{tag}") in net/http's ServeMux: a GET
	// pattern also serves HEAD, so GET .../tags and HEAD .../{tag} overlap on a
	// HEAD .../tags request with neither dominating, which panics at registration
	// (Go 1.22+). We therefore keep the wire path GET /v1/images/{name}/tags but
	// route it through the {tag} handler, which dispatches "tags" to handleTags
	// (literal "tags" beats a real tag named "tags" — the registry convention).
	mux.HandleFunc("GET /v1/images/{name}/{tag}", s.handleGet)
	mux.HandleFunc("GET /v1/images/{name}/{tag}/manifest", s.handleManifest)
	mux.HandleFunc("GET /v1/images", s.handleCatalog)
	return mux
}

// isDotComponent reports whether s is a pure-dot path component ("." or "..").
// image.ParseRef's regex ([a-z0-9._-]+) accepts these, but as a name/tag they are
// a one-level path-traversal escape — the store accepts name/tag over the network
// from authenticated-but-untrusted pushers, so parseRef rejects them explicitly
// (defense-in-depth, beyond net/http's own path cleaning).
func isDotComponent(s string) bool { return s == "." || s == ".." }

// parseRef validates name+tag through image.ParseRef BEFORE any path is built
// (the path-traversal guard). On failure it writes a 400 and returns ok=false.
func (s *Server) parseRef(w http.ResponseWriter, r *http.Request) (image.Ref, bool) {
	ref, err := image.ParseRef(r.PathValue("name") + ":" + r.PathValue("tag"))
	if err != nil {
		api.WriteErr(w, http.StatusBadRequest, "bad_ref", err.Error())
		return image.Ref{}, false
	}
	if isDotComponent(ref.Name) || isDotComponent(ref.Tag) {
		api.WriteErr(w, http.StatusBadRequest, "bad_ref", "name/tag must not be a pure-dot path component")
		return image.Ref{}, false
	}
	return ref, true
}

func (s *Server) handleHead(w http.ResponseWriter, r *http.Request) {
	ref, ok := s.parseRef(w, r)
	if !ok {
		return
	}
	digest, exists := s.repo.Head(ref)
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("X-Tariboy-Digest", digest)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handlePut(w http.ResponseWriter, r *http.Request) {
	ref, ok := s.parseRef(w, r)
	if !ok {
		return
	}
	claimed := strings.TrimSpace(r.Header.Get("X-Tariboy-Digest"))
	if claimed == "" {
		api.WriteErr(w, http.StatusBadRequest, "missing_digest", "X-Tariboy-Digest header is required")
		return
	}
	defer r.Body.Close()
	digest, err := s.repo.Put(ref, r.Body, claimed)
	if err != nil {
		if errors.Is(err, ErrDigestMismatch) {
			api.WriteErr(w, http.StatusBadRequest, "digest_mismatch", err.Error())
			return
		}
		api.WriteErr(w, http.StatusInternalServerError, "put_failed", err.Error())
		return
	}
	built := ""
	if m, ierr := s.repo.Inspect(ref); ierr == nil { // best-effort manifest index
		built = m.BuiltAt
	}
	if err := s.db.RecordPush(ref.Name, ref.Tag, digest, built); err != nil {
		api.WriteErr(w, http.StatusInternalServerError, "record_failed", err.Error())
		return
	}
	api.WriteOK(w, map[string]any{"name": ref.Name, "tag": ref.Tag, "digest": digest, "stored": true})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("tag") == "tags" { // GET /v1/images/{name}/tags — list tags
		s.handleTags(w, r)
		return
	}
	ref, ok := s.parseRef(w, r)
	if !ok {
		return
	}
	rc, digest, err := s.repo.Open(ref)
	if err != nil {
		api.WriteErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("X-Tariboy-Digest", digest)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	if _, err := image.ParseRef(r.PathValue("name") + ":latest"); err != nil {
		api.WriteErr(w, http.StatusBadRequest, "bad_ref", err.Error())
		return
	}
	rows, err := s.db.TagsFor(r.PathValue("name"))
	if err != nil {
		api.WriteErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	if rows == nil {
		rows = []PushRow{}
	}
	api.WriteOK(w, map[string]any{"name": r.PathValue("name"), "tags": rows})
}

// handleManifest returns the parsed image manifest for a repo:tag (plugins,
// harness, requires_secrets, evals, parents, digest). Repo.Inspect reads
// manifest.json from the archive and re-checks schema_version; it is the read
// model the store UI needs (the catalog only carries {name,tags}).
func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	ref, ok := s.parseRef(w, r)
	if !ok {
		return
	}
	m, err := s.repo.Inspect(ref)
	if err != nil {
		api.WriteErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	api.WriteOK(w, m)
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	mans, err := s.repo.List()
	if err != nil {
		api.WriteErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	type repoEntry struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	byName := map[string]*repoEntry{}
	var order []string
	for _, m := range mans {
		e := byName[m.Name]
		if e == nil {
			e = &repoEntry{Name: m.Name}
			byName[m.Name] = e
			order = append(order, m.Name)
		}
		e.Tags = append(e.Tags, m.Tag)
	}
	repos := make([]*repoEntry, 0, len(order))
	for _, n := range order {
		repos = append(repos, byName[n])
	}
	api.WriteOK(w, map[string]any{"repos": repos, "count": len(repos)})
}
