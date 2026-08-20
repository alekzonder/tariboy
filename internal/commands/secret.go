package commands

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/registry"
)

func secretStore() registry.Command {
	return registry.Command{
		Path:    "secret.store",
		Summary: "Store a secret value (write-only; used by 'secret set')",
		// HTTP-only: the value arg must never become a CLI positional (it would
		// leak into argv/ps/shell history). The blessed CLI path is secret.set,
		// which reads the value from --value/stdin and POSTs to this route.
		CLIHidden: true,
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "key", Type: registry.String, Required: true, Help: "secret key"},
			{Name: "value", Type: registry.String, Required: true, Help: "secret value"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/secrets"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			if _, err := getAgent(c, str(p, "name")); err != nil {
				return nil, err
			}
			if err := agentStore(c).SecretSet(str(p, "name"), str(p, "key"), str(p, "value")); err != nil {
				return nil, err
			}
			return map[string]any{"name": str(p, "name"), "key": str(p, "key"), "stored": true}, nil
		},
	}
}

func secretLs() registry.Command {
	return registry.Command{
		Path:    "secret.ls",
		Summary: "List secret keys (values are never shown)",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/secrets"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			keys, err := agentStore(c).SecretKeys(str(p, "name"))
			if err != nil {
				return nil, err
			}
			if keys == nil {
				keys = []string{}
			}
			return map[string]any{"keys": keys, "count": len(keys)}, nil
		},
	}
}

func secretRm() registry.Command {
	return registry.Command{
		Path:    "secret.rm",
		Summary: "Remove a secret",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "key", Type: registry.String, Required: true, Help: "secret key"},
		},
		HTTP: &registry.HTTPRoute{Method: "DELETE", Path: "/api/agents/{name}/secrets"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			if err := agentStore(c).SecretRemove(str(p, "name"), str(p, "key")); err != nil {
				if errors.Is(err, agent.ErrNotFound) {
					return nil, api.UserError{Code: "not_found", Msg: "secret not found"}
				}
				return nil, err // propagate real DB errors distinctly
			}
			return map[string]any{"removed": str(p, "key")}, nil
		},
	}
}

// secretSet is CLI-local so the value can be read from stdin (never argv) and
// forwarded to the daemon's secret.store endpoint.
func secretSet() registry.Command {
	return registry.Command{
		Path:    "secret.set",
		Summary: "Set a secret; value from --value or stdin",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "key", Type: registry.String, Required: true, Help: "secret key"},
			{Name: "value", Flag: "value", Type: registry.String, Help: "secret value (default: read stdin)"},
		},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			value, err := resolveSecretValue(str(p, "value"), os.Stdin)
			if err != nil {
				return nil, api.UserError{Code: "no_value", Msg: err.Error()}
			}
			cl := client.New(c.Socket)
			route := "/api/agents/" + str(p, "name") + "/secrets"
			if _, err := cl.Call("POST", route, map[string]string{
				"name": str(p, "name"), "key": str(p, "key"), "value": value,
			}); err != nil {
				return nil, err
			}
			return map[string]any{"name": str(p, "name"), "key": str(p, "key"), "stored": true}, nil
		},
	}
}

func resolveSecretValue(flagVal string, stdin io.Reader) (string, error) {
	if flagVal != "" {
		return flagVal, nil
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	v := strings.TrimRight(string(data), "\n")
	if v == "" {
		return "", errors.New("no value given (pass --value or pipe on stdin)")
	}
	return v, nil
}
