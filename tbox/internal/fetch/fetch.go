// Package fetch downloads small files over https and extracts archives with
// the safety rules tbox applies to remote sources.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	defaultTimeout = 60 * time.Second
	maxRedirects   = 5
)

// Client downloads files. The zero value is usable.
type Client struct {
	// HTTP is the transport client; nil means a fresh http.Client. Its
	// CheckRedirect is replaced on a copy for every request.
	HTTP *http.Client
	// Timeout bounds one Get; zero means 60 seconds.
	Timeout time.Duration
	// AllowInsecure also permits http:// (tests only).
	AllowInsecure bool
}

// Get downloads rawURL and returns its body, which must be at most max
// bytes. Only https is accepted unless AllowInsecure is set; a URL with
// user-info is rejected, and at most 5 redirects are followed, each
// validated like the original URL.
func (c Client) Get(ctx context.Context, rawURL string, max int64) ([]byte, error) {
	if max <= 0 {
		return nil, errors.New("fetch: size limit must be positive")
	}
	u, err := c.checkURL(rawURL)
	if err != nil {
		return nil, err
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: invalid request", hostOf(u))
	}
	req.Header.Set("User-Agent", "tbox")
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %s", hostOf(u), describe(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: unexpected status %d", hostOf(u), resp.StatusCode)
	}
	if resp.ContentLength > max {
		return nil, fmt.Errorf("fetch %s: body exceeds %d bytes", hostOf(u), max)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, fmt.Errorf("fetch %s: read body: %s", hostOf(u), describe(err))
	}
	if int64(len(body)) > max {
		return nil, fmt.Errorf("fetch %s: body exceeds %d bytes", hostOf(u), max)
	}
	return body, nil
}

func (c Client) client() *http.Client {
	var hc http.Client
	if c.HTTP != nil {
		hc = *c.HTTP
	}
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return errors.New("too many redirects")
		}
		_, err := c.checkURL(req.URL.String())
		return err
	}
	return &hc
}

// checkURL parses rawURL and applies the scheme and user-info rules.
func (c Client) checkURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, errors.New("fetch: invalid URL")
	}
	if u.Scheme != "https" && !(c.AllowInsecure && u.Scheme == "http") {
		return nil, fmt.Errorf("fetch: unsupported URL scheme %q, want https", u.Scheme)
	}
	if u.User != nil {
		return nil, errors.New("fetch: URL must not carry user-info")
	}
	if u.Hostname() == "" {
		return nil, errors.New("fetch: URL has no host")
	}
	return u, nil
}

// hostOf is scheme, host and path of u; the query and fragment are left out
// because they may carry tokens.
func hostOf(u *url.URL) string { return u.Scheme + "://" + u.Host + u.EscapedPath() }

// describe reduces a transport error to text that carries no URL.
func describe(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return err.Error()
}
