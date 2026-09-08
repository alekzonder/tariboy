package commands

import (
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/registry"
)

func imageVersionGet() registry.Command {
	return registry.Command{
		Path: "image.version.get", Summary: "Print image_version from a local Tariboyfile.yaml",
		Args:    []registry.Arg{{Name: "path", Flag: "path", Type: registry.String, Default: ".", Help: "Tariboyfile.yaml or its directory"}},
		Handler: func(_ *registry.Ctx, p registry.Params) (any, error) { return imagefile.GetVersion(str(p, "path")) },
	}
}

func imageVersionUpdate() registry.Command {
	return registry.Command{
		Path: "image.version.update", Summary: "Bump image_version in a local Tariboyfile.yaml",
		Args: []registry.Arg{
			{Name: "part", Type: registry.String, Required: true, Help: "SemVer component to increment: major, minor, or patch"},
			{Name: "path", Flag: "path", Type: registry.String, Default: ".", Help: "Tariboyfile.yaml or its directory"},
		},
		Handler: func(_ *registry.Ctx, p registry.Params) (any, error) {
			return imagefile.UpdateVersion(str(p, "path"), str(p, "part"))
		},
	}
}
