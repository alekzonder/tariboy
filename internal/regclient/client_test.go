package regclient

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
)

// writeCertPEM PEM-encodes a server's leaf certificate to a file and returns
// its path (a pinned CA/cert to trust).
func writeCertPEM(t *testing.T, srv *httptest.Server, name string) string {
	t.Helper()
	cert := srv.Certificate()
	if cert == nil {
		t.Fatal("test server has no certificate")
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// writeUnrelatedCertPEM generates a fresh self-signed cert (unrelated to any
// test server) and writes it as a PEM file — a valid CA/cert that must NOT be
// trusted for a different server.
func writeUnrelatedCertPEM(t *testing.T, name string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "unrelated-store"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestClientTrustsPinnedCertRejectsUntrusted is the load-bearing security test:
// a client pinned to the server's own cert completes a real TLS handshake, while
// a client pinned to a DIFFERENT cert fails the handshake (verification is never
// skipped).
func TestClientTrustsPinnedCertRejectsUntrusted(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Tariboy-Digest", "sha256:deadbeef")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("blob-bytes"))
	}))
	defer srv.Close()

	trustedCA := writeCertPEM(t, srv, "trusted.pem")
	// A valid but unrelated cert — pinning to it must cause a real handshake
	// failure against srv (httptest servers all share Go's built-in test cert,
	// so we must generate an independent one).
	untrustedCA := writeUnrelatedCertPEM(t, "untrusted.pem")

	ref := image.Ref{Name: "app", Tag: "v1"}

	// Trusted: pinned to srv's own cert → handshake succeeds, body copied.
	good, err := NewClient(srv.URL, "tok", trustedCA)
	if err != nil {
		t.Fatalf("NewClient(trusted): %v", err)
	}
	var buf bytes.Buffer
	digest, err := good.Get(ref, &buf)
	if err != nil {
		t.Fatalf("Get with pinned trusted cert must succeed, got: %v", err)
	}
	if digest != "sha256:deadbeef" {
		t.Fatalf("digest = %q, want sha256:deadbeef", digest)
	}
	if buf.String() != "blob-bytes" {
		t.Fatalf("body = %q, want blob-bytes", buf.String())
	}

	// Untrusted: pinned to a different cert → real handshake failure.
	bad, err := NewClient(srv.URL, "tok", untrustedCA)
	if err != nil {
		t.Fatalf("NewClient(untrusted): %v", err)
	}
	if _, err := bad.Get(ref, &bytes.Buffer{}); err == nil {
		t.Fatal("Get with an untrusted cert must fail the TLS handshake, but it succeeded")
	} else if !strings.Contains(err.Error(), "certificate") && !strings.Contains(err.Error(), "x509") {
		t.Fatalf("expected a TLS/certificate verification error, got: %v", err)
	}
}

// TestNewClientBadCAFile: a caFile with no PEM certs is rejected up front.
func TestNewClientBadCAFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "junk.pem")
	if err := os.WriteFile(p, []byte("not a pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewClient("https://h:8443", "tok", p); err == nil {
		t.Fatal("NewClient must reject a CA file with no certificates")
	}
}
