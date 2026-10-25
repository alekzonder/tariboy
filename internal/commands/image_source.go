package commands

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"time"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/imagesnapshot"
	"github.com/alekzonder/tariboy/internal/imagesource"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/registry"
)

func imageSourceStore(c *registry.Ctx) *imagesource.Store {
	return &imagesource.Store{
		Root:  paths.Paths{Base: c.BaseDir}.ImageSourcesDir(),
		Clock: time.Now,
	}
}

func imageSnapshotStore(c *registry.Ctx) *imagesnapshot.Store {
	return &imagesnapshot.Store{
		DB: c.Store.DB, Root: filepath.Join(c.BaseDir, "image-source-snapshots"), Clock: time.Now,
	}
}

func imageSourceUserError(err error) (api.UserError, bool) {
	switch {
	case errors.Is(err, imagesource.ErrInvalidName):
		return api.UserError{Code: "bad_source", Msg: err.Error(), Status: http.StatusBadRequest}, true
	case errors.Is(err, imagesource.ErrExists):
		return api.UserError{Code: "source_exists", Msg: err.Error(), Status: http.StatusConflict}, true
	case errors.Is(err, imagesource.ErrNotFound):
		return api.UserError{Code: "source_not_found", Msg: err.Error(), Status: http.StatusNotFound}, true
	case errors.Is(err, imagesource.ErrInvalidPath),
		errors.Is(err, imagesource.ErrInvalidUTF8),
		errors.Is(err, imagesource.ErrFileTooLarge),
		errors.Is(err, imagesource.ErrUnsafeFile):
		return api.UserError{Code: "bad_source_path", Msg: err.Error(), Status: http.StatusForbidden}, true
	default:
		return api.UserError{}, false
	}
}

func imageSourceError(err error) error {
	if userErr, ok := imageSourceUserError(err); ok {
		return userErr
	}
	return err
}

func imageSourceLs() registry.Command {
	return registry.WithResult[ImageSourceListResult](registry.Command{
		Path:    "image.source.ls",
		Summary: "List editable image sources",
		HTTP:    &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/image-sources"},
		Handler: func(c *registry.Ctx, _ registry.Request) (any, error) {
			sources, err := imageSourceStore(c).List()
			if err != nil {
				return nil, imageSourceError(err)
			}
			return ImageSourceListResult{Sources: convertSlice(sources, imageSourceView), Count: len(sources)}, nil
		},
	})
}

func imageSourceCreate() registry.Command {
	return registry.WithResult[ImageSource](registry.Command{
		Path:    "image.source.create",
		Summary: "Create an editable image source",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "source name"},
			{Name: "prompt", Flag: "prompt", Type: registry.String, Help: "initial prompt"},
		},
		HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/image-sources"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			source, err := imageSourceStore(c).Create(imagesource.CreateRequest{
				Name:   p.String("name"),
				Prompt: p.String("prompt"),
			})
			if err != nil {
				if userErr, ok := imageSourceUserError(err); ok {
					return nil, userErr
				}
				return nil, api.UserError{Code: "bad_source", Msg: err.Error(), Status: http.StatusBadRequest}
			}
			return imageSourceView(source), nil
		},
	})
}

func imageSourceInspect() registry.Command {
	return registry.WithResult[ImageSource](registry.Command{
		Path:    "image.source.inspect",
		Summary: "Show editable image source metadata",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "source name"}},
		HTTP:    &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/image-sources/{name}"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			source, err := imageSourceStore(c).Get(p.String("name"))
			if err != nil {
				return nil, imageSourceError(err)
			}
			return imageSourceView(source), nil
		},
	})
}

func imageSourceRm() registry.Command {
	return registry.WithResult[ImageRemoveResult](registry.Command{
		Path:    "image.source.rm",
		Summary: "Remove an editable image source without removing built images",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "source name"}},
		HTTP:    &registry.HTTPRoute{Method: http.MethodDelete, Path: "/api/image-sources/{name}"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			name := p.String("name")
			if err := imageSourceStore(c).Delete(name); err != nil {
				return nil, imageSourceError(err)
			}
			return ImageRemoveResult{Removed: name}, nil
		},
	})
}

func imageSourceFiles() registry.Command {
	return registry.WithResult[ImageSourceFilesResult](registry.Command{
		Path:    "image.source.files",
		Summary: "List editable files in an image source",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "source name"}},
		HTTP:    &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/image-sources/{name}/files"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			files, err := imageSourceStore(c).ListFiles(p.String("name"))
			if err != nil {
				return nil, imageSourceError(err)
			}
			return ImageSourceFilesResult{Files: convertSlice(files, func(f imagesource.FileEntry) ImageSourceFileEntry { return ImageSourceFileEntry(f) }), Count: len(files)}, nil
		},
	})
}

func imageSourceFileGet() registry.Command {
	return registry.WithResult[ImageFileContent](registry.Command{
		Path:    "image.source.file.get",
		Summary: "Read an editable image source file",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "source name"},
			{Name: "path", Type: registry.String, Required: true, Help: "source-relative file path"},
		},
		HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/image-sources/{name}/files/{path...}"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			path := p.String("path")
			content, err := imageSourceStore(c).ReadFile(p.String("name"), path)
			if err != nil {
				return nil, imageSourceError(err)
			}
			return ImageFileContent{Path: path, Content: string(content)}, nil
		},
	})
}

func imageSourceFilePut() registry.Command {
	return registry.WithResult[ImageSourceSaveResult](withMaxBody(registry.Command{
		Path:      "image.source.file.put",
		Summary:   "Write an editable image source file",
		CLIHidden: true,
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "source name"},
			{Name: "path", Type: registry.String, Required: true, Help: "source-relative file path"},
			{Name: "content", Type: registry.String, Required: true, Help: "UTF-8 file content"},
		},
		HTTP: &registry.HTTPRoute{Method: http.MethodPut, Path: "/api/image-sources/{name}/files/{path...}"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			path := p.String("path")
			if err := imageSourceStore(c).WriteFile(p.String("name"), path, []byte(p.String("content"))); err != nil {
				return nil, imageSourceError(err)
			}
			return ImageSourceSaveResult{Path: path, Saved: true}, nil
		},
	}, jsonEscapedBodyLimit(imagesource.MaxFileSize)))
}

func imageSourceValidate() registry.Command {
	return registry.WithResult[ImageSourceValidateResult](registry.Command{
		Path:    "image.source.validate",
		Summary: "Validate an editable image source",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "source name"}},
		HTTP:    &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/image-sources/{name}/validate"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			name := p.String("name")
			store := imageSourceStore(c)
			if _, err := store.Get(name); err != nil {
				return nil, imageSourceError(err)
			}
			dir := filepath.Join(paths.Paths{Base: c.BaseDir}.ImageSourcesDir(), name)
			parsed, err := imagefile.ParseV2(dir)
			if err != nil {
				return ImageSourceValidateResult{Diagnostics: []ImageDiagnostic{{Path: "Tariboyfile.yaml", Message: err.Error()}}}, nil
			}
			if len(parsed.Extends) > 0 {
				return ImageSourceValidateResult{Diagnostics: []ImageDiagnostic{{Path: "Tariboyfile.yaml", Message: imagefile.ExtendsUnassembledMessage}}}, nil
			}
			return ImageSourceValidateResult{Valid: true, Diagnostics: []ImageDiagnostic{}}, nil
		},
	})
}

func imageSourceBuild() registry.Command {
	return registry.WithResult[ImageSourceBuildResult](registry.Command{
		Path:    "image.source.build",
		Summary: "Build an image from an editable image source",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "source name"},
			{Name: "tag", Flag: "tag", Type: registry.String, Default: "latest", Help: "target image tag"},
		},
		HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/image-sources/{name}/build"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			name := p.String("name")
			tag := p.String("tag")
			if tag == "" {
				tag = "latest"
			}
			ref, err := image.ParseRef(name + ":" + tag)
			if err != nil {
				return nil, api.UserError{Code: "bad_ref", Msg: err.Error(), Status: http.StatusBadRequest}
			}
			if image.IsReserved(ref) {
				return nil, api.UserError{Code: "reserved_image", Msg: "image " + ref.String() + " is managed by tariboyd", Status: http.StatusConflict}
			}

			sources := imageSourceStore(c)
			source, err := sources.Get(name)
			if err != nil {
				return nil, imageSourceError(err)
			}
			var manifest image.Manifest
			var parseErr error
			store := imageStore(c)
			var record imagesource.BuildRecord
			// Migration walks every image name; MigrateOnce takes the store
			// lock for it once, before the build takes only the lock of the
			// name it publishes.
			err = store.MigrateOnce()
			if err == nil {
				err = image.WithRefLock(ref.Name, func() error {
					publication, err := store.BeginPublication([]image.Ref{ref})
					if err != nil {
						return err
					}
					record, err = sources.RecordBuild(name, func(dir string) (imagesource.BuildRecord, error) {
						parsed, err := imagefile.ParseV2(dir)
						if err != nil {
							parseErr = err
							return imagesource.BuildRecord{}, err
						}
						layout := paths.Paths{Base: c.BaseDir}
						pluginsDir := layout.PluginsDir()
						manifest, err = image.BuildV2(parsed, imagefile.ResolveRoots{Plugins: pluginsDir}, ref, store, time.Now, imagePluginResolver(c, pluginsDir))
						if err != nil {
							return imagesource.BuildRecord{}, err
						}
						if _, err := imageSnapshotStore(c).CaptureWithProvenance(
							context.Background(), ref.String(), manifest.Digest, name, dir, source.Provenance,
						); err != nil {
							return imagesource.BuildRecord{}, err
						}
						return imagesource.BuildRecord{
							Ref: ref.String(), Digest: manifest.Digest, BuiltAt: manifest.BuiltAt,
						}, nil
					}, publication.Restore)
					return err
				})
			}
			if err != nil {
				if parseErr != nil {
					return nil, api.UserError{
						Code: "bad_imagefile", Msg: parseErr.Error(), Status: http.StatusBadRequest,
					}
				}
				if userErr, ok := imageSourceUserError(err); ok {
					return nil, userErr
				}
				return nil, api.UserError{Code: "build_failed", Msg: err.Error(), Status: http.StatusBadRequest}
			}
			return ImageSourceBuildResult{
				Ref:     record.Ref,
				Digest:  record.Digest,
				BuiltAt: record.BuiltAt,
				Layers:  len(manifest.Layers),
			}, nil
		},
	})
}
