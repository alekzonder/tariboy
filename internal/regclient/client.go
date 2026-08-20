package regclient

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/alekzonder/tariboy/internal/image"
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
		http:  &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}},
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

func (c *Client) Head(ref image.Ref) (string, bool, error) {
	req, _ := http.NewRequest(http.MethodHead, c.url(ref, ""), nil)
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

func (c *Client) Put(ref image.Ref, archivePath, digest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, c.url(ref, ""), f)
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

func (c *Client) Get(ref image.Ref, w io.Writer) (string, error) {
	req, _ := http.NewRequest(http.MethodGet, c.url(ref, ""), nil)
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", decodeErr(resp)
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return "", err
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
	_ = json.NewDecoder(resp.Body).Decode(&env)
	if env.Error != nil {
		return fmt.Errorf("%s: %s", env.Error.Code, env.Error.Message)
	}
	return fmt.Errorf("registry returned %s", resp.Status)
}
