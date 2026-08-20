package commands

import (
	"io"
	"os"
	"strings"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/regclient"
	"github.com/alekzonder/tariboy/internal/registry"
)

func imagesDirFor(c *registry.Ctx) string {
	return paths.Paths{Base: c.BaseDir}.ImagesDir()
}

// clientFor loads per-registry credentials and builds a TLS-trusting client.
func clientFor(c *registry.Ctx, regURL string) (*regclient.Client, error) {
	rs, err := regclient.LoadRegistries(c.BaseDir)
	if err != nil {
		return nil, err
	}
	reg, ok := rs.Get(regURL)
	if !ok {
		return nil, api.UserError{Code: "not_logged_in", Msg: "no credentials for " + regURL + " (run: tariboy login " + regURL + ")"}
	}
	cl, err := regclient.NewClient(regURL, reg.Token, reg.CA)
	if err != nil {
		return nil, api.UserError{Code: "bad_registry", Msg: err.Error()}
	}
	return cl, nil
}

func pushCmd() registry.Command {
	return registry.Command{
		Path:    "push",
		Summary: "Push a local image to a tariboy-store registry",
		Args: []registry.Arg{
			{Name: "ref", Type: registry.String, Required: true, Help: "image ref name:tag"},
			{Name: "registry", Flag: "registry", Type: registry.String, Required: true, Help: "store base URL, e.g. https://host:8443"},
		},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			ref, err := parseImageRef(p)
			if err != nil {
				return nil, err
			}
			regURL, _ := p["registry"].(string)
			cl, err := clientFor(c, regURL)
			if err != nil {
				return nil, err
			}
			res, err := regclient.Push(imagesDirFor(c), ref, cl)
			if err != nil {
				return nil, api.UserError{Code: "push_failed", Msg: err.Error()}
			}
			return res, nil
		},
	}
}

func pullCmd() registry.Command {
	return registry.Command{
		Path:    "pull",
		Summary: "Pull an image from a tariboy-store registry into the local image store",
		Args: []registry.Arg{
			{Name: "ref", Type: registry.String, Required: true, Help: "image ref name:tag"},
			{Name: "registry", Flag: "registry", Type: registry.String, Required: true, Help: "store base URL, e.g. https://host:8443"},
		},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			ref, err := parseImageRef(p)
			if err != nil {
				return nil, err
			}
			regURL, _ := p["registry"].(string)
			cl, err := clientFor(c, regURL)
			if err != nil {
				return nil, err
			}
			res, err := regclient.Pull(imagesDirFor(c), ref, cl)
			if err != nil {
				return nil, api.UserError{Code: "pull_failed", Msg: err.Error()}
			}
			return res, nil
		},
	}
}

func loginCmd() registry.Command {
	return registry.Command{
		Path:    "login",
		Summary: "Store a bearer token for a tariboy-store registry",
		Help:    "Reads the token from --token-file, or from stdin if omitted. The token is NEVER taken from argv, to keep it out of ps output and shell history.",
		Args: []registry.Arg{
			{Name: "registry", Type: registry.String, Required: true, Help: "store base URL, e.g. https://host:8443"},
			{Name: "token-file", Flag: "token-file", Type: registry.String, Help: "file holding the bearer token (stdin if omitted)"},
			{Name: "ca", Flag: "ca", Type: registry.String, Help: "path to a CA/cert PEM to trust for this registry"},
		},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			regURL, _ := p["registry"].(string)
			tokenFile, _ := p["token-file"].(string)
			var tok string
			if tokenFile != "" {
				b, err := os.ReadFile(tokenFile)
				if err != nil {
					return nil, api.UserError{Code: "bad_token_file", Msg: err.Error()}
				}
				tok = strings.TrimSpace(string(b))
			} else {
				b, err := io.ReadAll(os.Stdin)
				if err != nil {
					return nil, api.UserError{Code: "bad_stdin", Msg: err.Error()}
				}
				tok = strings.TrimSpace(string(b))
			}
			if tok == "" {
				return nil, api.UserError{Code: "empty_token", Msg: "no token provided (use --token-file or pipe it on stdin)"}
			}
			ca, _ := p["ca"].(string)
			rs, err := regclient.LoadRegistries(c.BaseDir)
			if err != nil {
				return nil, err
			}
			rs.Set(regURL, regclient.Registry{Token: tok, CA: ca})
			if err := rs.Save(); err != nil {
				return nil, err
			}
			return map[string]any{"registry": regURL, "logged_in": true}, nil
		},
	}
}
