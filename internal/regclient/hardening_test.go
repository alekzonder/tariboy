package regclient

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
)

// shrinkResponseHeaderTimeout lowers the transport's header timeout for one test.
func shrinkResponseHeaderTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := responseHeaderTimeout
	responseHeaderTimeout = d
	t.Cleanup(func() { responseHeaderTimeout = old })
}

// stallingServer accepts each request and never writes a response until the
// test ends.
func stallingServer(t *testing.T) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	return srv
}

func TestHeadAndGetTimeOutOnStalledServer(t *testing.T) {
	shrinkResponseHeaderTimeout(t, 200*time.Millisecond)
	srv := stallingServer(t)
	cl, err := NewClient(srv.URL, "tok", "")
	if err != nil {
		t.Fatal(err)
	}
	ref := image.Ref{Name: "demo", Tag: "latest"}

	start := time.Now()
	if _, _, err := cl.Head(context.Background(), ref); err == nil {
		t.Fatal("Head against a stalled server must fail")
	}
	if el := time.Since(start); el > 5*time.Second {
		t.Fatalf("Head took %v, want about the header timeout", el)
	}

	start = time.Now()
	var buf bytes.Buffer
	if _, err := cl.Get(context.Background(), ref, &buf); err == nil {
		t.Fatal("Get against a stalled server must fail")
	}
	if el := time.Since(start); el > 5*time.Second {
		t.Fatalf("Get took %v, want about the header timeout", el)
	}
}

func TestHeadAndGetAbortOnContextCancel(t *testing.T) {
	srv := stallingServer(t) // default (long) header timeout
	cl, err := NewClient(srv.URL, "tok", "")
	if err != nil {
		t.Fatal(err)
	}
	ref := image.Ref{Name: "demo", Tag: "latest"}

	for name, call := range map[string]func(ctx context.Context) error{
		"Head": func(ctx context.Context) error { _, _, err := cl.Head(ctx, ref); return err },
		"Get":  func(ctx context.Context) error { _, err := cl.Get(ctx, ref, &bytes.Buffer{}); return err },
	} {
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(100*time.Millisecond, cancel)
		start := time.Now()
		err := call(ctx)
		cancel()
		if err == nil {
			t.Fatalf("%s must fail when ctx is cancelled", name)
		}
		if el := time.Since(start); el > 5*time.Second {
			t.Fatalf("%s took %v after cancel, want an immediate abort", name, el)
		}
	}
}

func TestGetRejectsOversizedBody(t *testing.T) {
	old := maxDownloadBytes
	maxDownloadBytes = 16
	t.Cleanup(func() { maxDownloadBytes = old })

	body := []byte(strings.Repeat("x", 17))
	digest := sha256hex(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Tariboy-Digest", digest)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	cl, err := NewClient(srv.URL, "tok", "")
	if err != nil {
		t.Fatal(err)
	}
	ref := image.Ref{Name: "demo", Tag: "latest"}

	if _, err := cl.Get(context.Background(), ref, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Get of an oversized body must fail with a size error, got %v", err)
	}

	dst := t.TempDir()
	if _, err := Pull(context.Background(), dst, ref, cl); err == nil {
		t.Fatal("Pull of an oversized body must fail")
	}
	if (&image.Store{Dir: dst}).Exists(ref) {
		t.Fatal("an oversized pull must not install the archive")
	}
	requireNoTempFiles(t, dst)
}

func TestGetAcceptsBodyAtLimit(t *testing.T) {
	old := maxDownloadBytes
	maxDownloadBytes = 16
	t.Cleanup(func() { maxDownloadBytes = old })

	body := []byte(strings.Repeat("x", 16))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Tariboy-Digest", "d")
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	cl, err := NewClient(srv.URL, "tok", "")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := cl.Get(context.Background(), image.Ref{Name: "demo", Tag: "latest"}, &buf); err != nil {
		t.Fatalf("Get at exactly the limit must succeed: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), body) {
		t.Fatalf("body = %q", buf.String())
	}
}

func TestDownloadLimitIsPortableImageLimit(t *testing.T) {
	if maxDownloadBytes != image.MaxPortableImageCompressed {
		t.Fatalf("maxDownloadBytes = %d, want %d", maxDownloadBytes, image.MaxPortableImageCompressed)
	}
}

// TestResultJSONShape pins the CLI/API output to the former map[string]any keys.
func TestResultJSONShape(t *testing.T) {
	for _, tc := range []struct {
		v    any
		want string
	}{
		{PushResult{Name: "n", Tag: "t", Digest: "d", Pushed: true}, `{"digest":"d","name":"n","pushed":true,"tag":"t"}`},
		{PushResult{Name: "n", Tag: "t", Digest: "d", Skipped: true}, `{"digest":"d","name":"n","skipped":true,"tag":"t"}`},
		{PullResult{Name: "n", Tag: "t", Digest: "d", Pulled: true}, `{"digest":"d","name":"n","pulled":true,"tag":"t"}`},
	} {
		b, err := json.Marshal(tc.v)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != tc.want {
			t.Fatalf("json = %s, want %s", b, tc.want)
		}
	}
}
