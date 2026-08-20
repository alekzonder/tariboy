package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/supportbundle"
)

type fakeSupportArchive struct {
	body string
	err  error
}

func (a fakeSupportArchive) WriteZIP(output io.Writer) error {
	if a.err != nil {
		return a.err
	}
	_, err := io.WriteString(output, a.body)
	return err
}

type fakeSupportSource struct {
	options supportbundle.Options
	archive supportbundle.Archive
	err     error
}

func (s *fakeSupportSource) Prepare(_ context.Context, options supportbundle.Options) (supportbundle.Archive, error) {
	s.options = options
	return s.archive, s.err
}

func supportTestServer(source SupportBundleSource) *Server {
	server := NewServer(registry.New(), &registry.Ctx{
		Version: "test",
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if source != nil {
		server.SetSupportBundleSource(source)
	}
	return server
}

func TestSupportBundleRouteStreamsPreparedZIP(t *testing.T) {
	source := &fakeSupportSource{archive: fakeSupportArchive{body: "zip-body"}}
	server := supportTestServer(source)
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/daemon/support-bundle?include_agent_data=1&iteration_limit=10",
		nil,
	)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("content type = %q", got)
	}
	if response.Body.String() != "zip-body" {
		t.Fatalf("body = %q", response.Body.String())
	}
	if source.options != (supportbundle.Options{IncludeAgentData: true, IterationLimit: 10}) {
		t.Fatalf("options = %+v", source.options)
	}
}

func TestSupportBundleRouteDefaultsToSafeTenIterationRequest(t *testing.T) {
	source := &fakeSupportSource{archive: fakeSupportArchive{body: "zip"}}
	server := supportTestServer(source)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/api/daemon/support-bundle", nil),
	)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if source.options.IncludeAgentData || source.options.IterationLimit != 10 {
		t.Fatalf("default options = %+v", source.options)
	}
}

func TestSupportBundleRouteRejectsMalformedQuery(t *testing.T) {
	cases := []string{
		"?include_agent_data=x",
		"?iteration_limit=0",
		"?iteration_limit=11",
		"?unknown=1",
	}
	for _, query := range cases {
		t.Run(query, func(t *testing.T) {
			source := &fakeSupportSource{archive: fakeSupportArchive{body: "zip"}}
			response := httptest.NewRecorder()
			supportTestServer(source).Handler().ServeHTTP(
				response,
				httptest.NewRequest(http.MethodGet, "/api/daemon/support-bundle"+query, nil),
			)
			if response.Code != http.StatusBadRequest ||
				!strings.Contains(response.Body.String(), `"code":"bad_support_request"`) {
				t.Fatalf("query %s = %d %s", query, response.Code, response.Body.String())
			}
		})
	}
}

func TestSupportBundleRouteMapsOversizedDataBeforeWritingZIP(t *testing.T) {
	source := &fakeSupportSource{err: supportbundle.ErrTooLarge}
	response := httptest.NewRecorder()

	supportTestServer(source).Handler().ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/api/daemon/support-bundle?include_agent_data=1", nil),
	)

	if response.Code != http.StatusRequestEntityTooLarge ||
		!strings.Contains(response.Body.String(), `"code":"support_bundle_too_large"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); strings.Contains(got, "zip") {
		t.Fatalf("error response has zip content type %q", got)
	}
}

func TestSupportBundleRouteDoesNotExposeInternalCollectorError(t *testing.T) {
	source := &fakeSupportSource{err: errors.New("/private/base read failed with token secret")}
	response := httptest.NewRecorder()

	supportTestServer(source).Handler().ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/api/daemon/support-bundle", nil),
	)

	if response.Code != http.StatusInternalServerError ||
		!strings.Contains(response.Body.String(), `"code":"internal"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	for _, forbidden := range []string{"/private/base", "token secret"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("internal error leaked %q: %s", forbidden, response.Body.String())
		}
	}
}

func TestSupportBundleRouteIsAbsentUntilConfigured(t *testing.T) {
	response := httptest.NewRecorder()
	supportTestServer(nil).Handler().ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/api/daemon/support-bundle", nil),
	)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}
