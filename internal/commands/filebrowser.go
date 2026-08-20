package commands

import (
	"errors"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/filebrowser"
	"github.com/alekzonder/tariboy/internal/registry"
)

// fbErr maps a *filebrowser.Error to the api JSON error envelope (400 with its
// stable code); any other error falls through to the generic 500 path.
func fbErr(err error) error {
	var fe *filebrowser.Error
	if errors.As(err, &fe) {
		return api.UserError{Code: fe.Code, Msg: fe.Msg}
	}
	return err
}

// The file-browser routes live under the singular /file namespace. Listing is
// /file/list rather than /files because GET/PUT /api/agents/{name}/files are
// already owned by the cp push/pull commands (agentPull/agentPush).

func fileList() registry.Command {
	return registry.Command{
		Path:    "agent.file.list",
		Summary: "List a directory in an agent's cwd (jailed)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "path", Type: registry.String, Help: "directory relative to cwd (empty => root)"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/file/list"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			root, err := agentCwdFor(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			entries, err := filebrowser.List(root, str(p, "path"))
			if err != nil {
				return nil, fbErr(err)
			}
			return map[string]any{"path": str(p, "path"), "entries": entries}, nil
		},
	}
}

func fileRead() registry.Command {
	return registry.Command{
		Path:    "agent.file.read",
		Summary: "Read a file from an agent's cwd (text; binary/large => marker)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "path", Type: registry.String, Required: true, Help: "file relative to cwd"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/file"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			root, err := agentCwdFor(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			content, err := filebrowser.Read(root, str(p, "path"))
			if err != nil {
				return nil, fbErr(err)
			}
			return content, nil
		},
	}
}

func fileWrite() registry.Command {
	return registry.Command{
		Path:    "agent.file.write",
		Summary: "Overwrite/save a file in an agent's cwd (jailed)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "path", Type: registry.String, Required: true, Help: "file relative to cwd"},
			{Name: "content", Type: registry.String, Required: true, Help: "new file content (text)"},
		},
		HTTP: &registry.HTTPRoute{Method: "PUT", Path: "/api/agents/{name}/file"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			root, err := agentCwdFor(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			if err := filebrowser.Write(root, str(p, "path"), []byte(str(p, "content"))); err != nil {
				return nil, fbErr(err)
			}
			return map[string]any{"path": str(p, "path"), "saved": true}, nil
		},
	}
}

func fileCreate() registry.Command {
	return registry.Command{
		Path:    "agent.file.create",
		Summary: "Create a file or folder in an agent's cwd (jailed)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "path", Type: registry.String, Required: true, Help: "path relative to cwd"},
			{Name: "type", Type: registry.String, Help: "file (default) or dir"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/file"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			root, err := agentCwdFor(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			if err := filebrowser.Create(root, str(p, "path"), str(p, "type")); err != nil {
				return nil, fbErr(err)
			}
			return map[string]any{"path": str(p, "path"), "created": true}, nil
		},
	}
}

func fileRename() registry.Command {
	return registry.Command{
		Path:    "agent.file.rename",
		Summary: "Rename/move a file within an agent's cwd (jailed)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "from", Type: registry.String, Required: true, Help: "source path relative to cwd"},
			{Name: "to", Type: registry.String, Required: true, Help: "destination path relative to cwd"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/file/rename"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			root, err := agentCwdFor(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			if err := filebrowser.Rename(root, str(p, "from"), str(p, "to")); err != nil {
				return nil, fbErr(err)
			}
			return map[string]any{"from": str(p, "from"), "to": str(p, "to"), "renamed": true}, nil
		},
	}
}

func fileDelete() registry.Command {
	return registry.Command{
		Path:    "agent.file.delete",
		Summary: "Delete a file or folder in an agent's cwd (jailed)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "path", Type: registry.String, Required: true, Help: "path relative to cwd"},
		},
		HTTP: &registry.HTTPRoute{Method: "DELETE", Path: "/api/agents/{name}/file"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			root, err := agentCwdFor(c, str(p, "name"))
			if err != nil {
				return nil, err
			}
			if err := filebrowser.Delete(root, str(p, "path")); err != nil {
				return nil, fbErr(err)
			}
			return map[string]any{"path": str(p, "path"), "deleted": true}, nil
		},
	}
}
