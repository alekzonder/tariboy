package commands

import (
	"errors"
	"net/http"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/fsbrowser"
	"github.com/alekzonder/tariboy/internal/registry"
)

// fsErr maps a *fsbrowser.Error to the api JSON error envelope with a
// code-specific HTTP status (bad_path => 403, not_found => 404, not_dir => 400);
// any other error falls through to the generic 500 path.
func fsErr(err error) error {
	var fe *fsbrowser.Error
	if !errors.As(err, &fe) {
		return err
	}
	status := http.StatusBadRequest
	switch fe.Code {
	case "bad_path":
		status = http.StatusForbidden
	case "not_found":
		status = http.StatusNotFound
	case "not_dir":
		status = http.StatusBadRequest
	}
	return api.UserError{Code: fe.Code, Msg: fe.Msg, Status: status}
}

// fsList lists directories under the daemon's filesystem root
// (TARIBOY_FS_ROOT, default $HOME). It powers the UI cwd path-autocomplete
// and is deliberately separate from the agent-workdir-jailed /file routes.
func fsList() registry.Command {
	return registry.Command{
		Path:    "fs.list",
		Summary: "List directories under the daemon filesystem root ($HOME-jailed)",
		Args: []registry.Arg{
			{Name: "path", Type: registry.String, Help: "directory to list (empty/~/relative => root)"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/fs/list"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			root, err := fsbrowser.Root()
			if err != nil {
				return nil, fsErr(err)
			}
			listing, err := fsbrowser.List(root, str(p, "path"))
			if err != nil {
				return nil, fsErr(err)
			}
			return listing, nil
		},
	}
}
