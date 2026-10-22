package regclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
)

// Transport timeouts and the download cap; vars so tests can shrink them.
var (
	dialTimeout           = 10 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	responseHeaderTimeout = 30 * time.Second
	clientTimeout         = 15 * time.Minute
	maxDownloadBytes      = image.MaxPortableImageCompressed
)

// Client talks the /v1 push/pull protocol to one store over TLS.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// NewClient builds a client. When caFile is set, TLS verification trusts that
// CA/cert PEM (self-signed or private CA) — verification stays ON.
func NewClient(baseURL, token, caFile string) (*Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("read ca file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificates found in %s", caFile)
		}
		tlsCfg.RootCAs = pool
	}
	return &Client{
		base:  normalizeURL(baseURL),
		token: token,
		http: &http.Client{
			Timeout: clientTimeout,
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: dialTimeout}).DialContext,
				TLSClientConfig:       tlsCfg,
				TLSHandshakeTimeout:   tlsHandshakeTimeout,
				ResponseHeaderTimeout: responseHeaderTimeout,
			},
		},
	}, nil
}

func (c *Client) url(ref image.Ref, suffix string) string {
	return c.base + "/v1/images/" + ref.Name + "/" + ref.Tag + suffix
}

func (c *Client) auth(r *http.Request) {
	if c.token != "" {
		r.Header.Set("Authorization", "Bearer "+c.token)
	}
}

func (c *Client) Head(ctx context.Context, ref image.Ref) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.url(ref, ""), nil)
	if err != nil {
		return "", false, err
	}
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return resp.Header.Get("X-Tariboy-Digest"), true, nil
	case http.StatusNotFound:
		return "", false, nil
	case http.StatusUnauthorized:
		return "", false, fmt.Errorf("unauthorized (run: tariboy login %s)", c.base)
	default:
		return "", false, fmt.Errorf("HEAD %s: %s", c.url(ref, ""), resp.Status)
	}
}

func (c *Client) Put(ctx context.Context, ref image.Ref, archivePath, digest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.url(ref, ""), f)
	if err != nil {
		return err
	}
	req.ContentLength = fi.Size()
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("X-Tariboy-Digest", digest)
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return decodeErr(resp)
	}
	return nil
}

// Get streams the archive into w and fails once the body exceeds
// maxDownloadBytes; callers must discard w on any error.
func (c *Client) Get(ctx context.Context, ref image.Ref, w io.Writer) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(ref, ""), nil)
	if err != nil {
		return "", err
	}
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", decodeErr(resp)
	}
	n, err := io.Copy(w, io.LimitReader(resp.Body, maxDownloadBytes+1))
	if err != nil {
		return "", err
	}
	if n > maxDownloadBytes {
		return "", fmt.Errorf("download of %s exceeds the %d byte image limit", ref.String(), maxDownloadBytes)
	}
	return resp.Header.Get("X-Tariboy-Digest"), nil
}

func decodeErr(resp *http.Response) error {
	var env struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&env) // best effort: an unparsable error body falls back to the HTTP status
	if env.Error != nil {
		return fmt.Errorf("%s: %s", env.Error.Code, env.Error.Message)
	}
	return fmt.Errorf("registry returned %s", resp.Status)
}
