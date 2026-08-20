package commands

import "github.com/alekzonder/tariboy/internal/registry"

func versionCommand() registry.Command {
	return registry.Command{
		Path:    "version",
		Summary: "Print the Tariboy version",
		Handler: func(c *registry.Ctx, _ registry.Params) (any, error) {
			return c.Version, nil
		},
	}
}
