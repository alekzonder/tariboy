package commands

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/workflowfile"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

func workflowImageRegistry(c *registry.Ctx) (*workflowimage.Registry, error) {
	if c.Store == nil {
		return nil, api.UserError{Code: "build_failed", Msg: "workflow image metadata store is unavailable"}
	}
	return &workflowimage.Registry{
		Store:       &workflowimage.Store{Dir: paths.Paths{Base: c.BaseDir}.WorkflowsDir()},
		DB:          c.Store.DB,
		OnPublished: c.FollowQueueWorkflows,
	}, nil
}

func workflowImageStore(c *registry.Ctx) *workflowimage.Store {
	return &workflowimage.Store{Dir: paths.Paths{Base: c.BaseDir}.WorkflowsDir()}
}

// workflowImageError maps the workflow image errors to their API codes.
func workflowImageError(err error) error {
	var invalid *workflowimage.InvalidError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &invalid):
		return workflowInvalid(err.Error(), invalid.Errors)
	case errors.Is(err, workflowimage.ErrVersionPublished):
		return api.UserError{Code: "workflow_version_published", Msg: err.Error(), Status: http.StatusConflict}
	case errors.Is(err, workflowimage.ErrInUse):
		return api.UserError{Code: "workflow_in_use", Msg: err.Error(), Status: http.StatusConflict}
	case errors.Is(err, workflowimage.ErrNotFound):
		return api.UserError{Code: "not_found", Msg: err.Error(), Status: http.StatusNotFound}
	case errors.Is(err, workflowimage.ErrInvalid):
		return workflowInvalid(err.Error(), nil)
	default:
		return err
	}
}

func workflowInvalid(msg string, errs []workflowfile.ValidationError) error {
	ue := api.UserError{Code: "workflow_invalid", Msg: msg, Status: http.StatusBadRequest}
	if len(errs) > 0 {
		ue.Data = map[string]any{"errors": errs}
	}
	return ue
}

// workflowSourcePath resolves the source or path argument to a source
// directory. When a Store selector is given, release must be called once the
// build no longer reads the Store tree.
func workflowSourcePath(c *registry.Ctx, p registry.Params) (path string, release func(), err error) {
	selector, path := strings.TrimSpace(p.String("source")), strings.TrimSpace(p.String("path"))
	switch {
	case selector != "" && path != "":
		return "", nil, api.UserError{Code: "bad_source", Msg: "source and path are mutually exclusive", Status: http.StatusBadRequest}
	case selector != "":
		prepared, release, err := storeCatalog(c).PrepareWorkflowBuild(selector)
		if err != nil {
			return "", nil, storeError(err)
		}
		return prepared.Path, release, nil
	case path == "":
		return "", nil, api.UserError{Code: "missing_path", Msg: "workflow source path is required", Status: http.StatusBadRequest}
	}
	return path, func() {}, nil
}

// parseWorkflowSource parses the manifest at path. A manifest that cannot be
// read or decoded is reported as workflow_invalid.
func parseWorkflowSource(path string) (*workflowfile.File, error) {
	f, err := workflowfile.Parse(path)
	if err != nil {
		return nil, workflowInvalid(err.Error(), nil)
	}
	return f, nil
}

// invalidExitCode is 1 when a validate result says the source is invalid. The
// route returns validation errors in its result, not as an HTTP error, so
// without it a script could not gate on `image validate` or `workflow
// validate`. Other commands with a `valid` field, such as `image source
// validate`, keep exit 0.
func invalidExitCode(raw json.RawMessage) int {
	var result struct {
		Valid *bool `json:"valid"`
	}
	if json.Unmarshal(raw, &result) == nil && result.Valid != nil && !*result.Valid {
		return 1
	}
	return 0
}

func workflowValidate() registry.Command {
	return registry.WithResult[WorkflowImageValidation](registry.Command{
		Path:         "workflow.validate",
		AbsPathArgs:  []string{"path"},
		ExitCodeFrom: invalidExitCode,
		Summary:      "Validate a workflow source directory without building it",
		Args: []registry.Arg{
			{Name: "source", Type: registry.String, Help: "Store selector (store/workflow)"},
			{Name: "path", Flag: "path", Type: registry.String, Help: "Workflowfile.yaml or its directory"},
		},
		HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/workflow-images/validate"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			path, release, err := workflowSourcePath(c, p.Params)
			if err != nil {
				return nil, err
			}
			defer release()
			f, err := parseWorkflowSource(path)
			if err != nil {
				return nil, err
			}
			// The same validation build runs, so validate reports every
			// source build would refuse.
			errs := workflowimage.Validate(f)
			return WorkflowImageValidation{
				Valid:   len(errs) == 0,
				Name:    f.Name,
				Version: f.WorkflowVersion,
				Pools:   nonNil(f.Pools()),
				Files:   nonNil(f.Files()),
				Errors:  workflowValidationErrors(errs),
			}, nil
		},
	})
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func workflowBuild() registry.Command {
	return registry.WithResult[WorkflowImageBuildResult](registry.Command{
		Path:        "workflow.build",
		AbsPathArgs: []string{"path"},
		Summary:     "Build a workflow image from a Workflowfile.yaml",
		Args: []registry.Arg{
			{Name: "source", Type: registry.String, Help: "Store selector (store/workflow)"},
			{Name: "path", Flag: "path", Type: registry.String, Help: "Workflowfile.yaml or its directory"},
		},
		HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/workflow-images/build"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			path, release, err := workflowSourcePath(c, p.Params)
			if err != nil {
				return nil, err
			}
			defer release()
			reg, err := workflowImageRegistry(c)
			if err != nil {
				return nil, err
			}
			f, err := parseWorkflowSource(path)
			if err != nil {
				return nil, err
			}
			m, created, err := reg.Publish(f, time.Now())
			if err != nil {
				return nil, workflowImageError(err)
			}
			tags, err := reg.Store.Tags(m.Name, m.Digest)
			if err != nil {
				return nil, err
			}
			return WorkflowImageBuildResult{Name: m.Name, Version: m.Version, Digest: m.Digest, Tags: nonNil(tags), Created: created}, nil
		},
	})
}

func workflowLs() registry.Command {
	return registry.WithResult[WorkflowImageList](registry.Command{
		Path:    "workflow.ls",
		Summary: "List built workflow images",
		HTTP:    &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/workflow-images"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			listed, err := workflowImageStore(c).List()
			if err != nil {
				return nil, workflowImageError(err)
			}
			rows := make([]WorkflowImageListing, 0, len(listed))
			for _, l := range listed {
				rows = append(rows, WorkflowImageListing{Name: l.Name, Tag: l.Tag, Version: l.Version, Digest: l.Digest, BuiltAt: l.BuiltAt})
			}
			return WorkflowImageList{Workflows: rows, Count: len(rows)}, nil
		},
	})
}

func workflowInspect() registry.Command {
	return registry.WithResult[WorkflowImageManifest](registry.Command{
		Path:    "workflow.inspect",
		Summary: "Show a workflow image manifest",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "workflow name"},
			{Name: "tag", Type: registry.String, Default: "latest", Help: "tag or full digest"},
		},
		HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/workflow-images/{name}/{tag}"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			tag := p.String("tag")
			if tag == "" {
				tag = "latest"
			}
			m, err := workflowImageStore(c).Inspect(p.String("name"), tag)
			if err != nil {
				return nil, workflowImageError(err)
			}
			return workflowManifestFrom(m)
		},
	})
}

func workflowRm() registry.Command {
	return registry.WithResult[WorkflowImageRemoveResult](registry.Command{
		Path:    "workflow.rm",
		Summary: "Remove a workflow image tag, and its content when no tag remains",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "workflow name"},
			{Name: "tag", Type: registry.String, Required: true, Help: "tag to remove"},
		},
		HTTP: &registry.HTTPRoute{Method: http.MethodDelete, Path: "/api/workflow-images/{name}/{tag}"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			reg, err := workflowImageRegistry(c)
			if err != nil {
				return nil, err
			}
			name, tag := p.String("name"), p.String("tag")
			// Registry.Remove deletes the content and its row with the last tag.
			_, contentRemoved, err := reg.Remove(name, tag)
			if err != nil {
				return nil, workflowImageError(err)
			}
			return WorkflowImageRemoveResult{Name: name, Tag: tag, Removed: true, ContentRemoved: contentRemoved}, nil
		},
	})
}
