package commands

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/agentskills"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/imageprovenance"
	"github.com/alekzonder/tariboy/internal/imagesnapshot"
	"github.com/alekzonder/tariboy/internal/imagesource"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/plugincaps"
	"github.com/alekzonder/tariboy/internal/plugins"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/stores"
)

var gitCommitPattern = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)

func imageStore(c *registry.Ctx) *image.Store {
	return &image.Store{Dir: paths.Paths{Base: c.BaseDir}.ImagesDir()}
}

func imagePluginResolver(c *registry.Ctx, pluginsDir string) plugincaps.ExternalResolver {
	if c.Store == nil {
		return plugins.ResolveInstalledMetadata(pluginsDir)
	}
	return plugins.ResolveEnabledInstalledMetadata(pluginsDir, plugins.NewStore(c.Store, time.Now))
}

func imageBuild() registry.Command {
	return registry.Command{
		Path:        "image.build",
		AbsPathArgs: []string{"path"},
		Summary:     "Build an agent image from a Tariboyfile.yaml",
		Args: []registry.Arg{
			{Name: "source", Type: registry.String, Help: "Store selector (store/image)"},
			{Name: "name", Flag: "name", Type: registry.String, Help: "target image name"},
			{Name: "tag", Flag: "tag", Type: registry.String, Repeatable: true, Help: "target image tag (default image_version and latest, or only latest when absent)"},
			{Name: "path", Flag: "path", Type: registry.String, Help: "Tariboyfile.yaml or its directory"},
			{Name: "repository-id", Flag: "repository-id", Type: registry.String, Help: "source repository ID"},
			{Name: "git-commit", Flag: "git-commit", Type: registry.String, Help: "source Git commit"},
		},
		HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/images/build"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			requestCtx := p.Ctx
			params, err := parseImageBuildParams(p.Params)
			if err != nil {
				return nil, err
			}
			var locks imageBuildLocks
			if params.selector != "" {
				catalog := storeCatalog(c)
				prepared, release, err := catalog.PrepareBuild(params.selector)
				if err != nil {
					return nil, storeError(err)
				}
				defer release()
				params.path = prepared.Path
				locks = imageBuildLocks{
					restore:      func() error { return catalog.RestoreBuildLocks(requestCtx, prepared) },
					restoreStore: func() error { return catalog.RestoreStoreLock(requestCtx, prepared) },
					install:      func(dir string) error { return stores.RestoreLock(requestCtx, dir) },
				}
				if params.name == "" {
					params.name = prepared.Name
				}
			}
			params.name, params.tags, err = resolveImageBuildName(params.name, params.tags)
			if err != nil {
				return nil, err
			}
			if c.Store == nil {
				return nil, api.UserError{Code: "build_failed", Msg: "image metadata store is unavailable"}
			}
			source, cleanup, err := freezeImageSource(c, params, locks)
			if err != nil {
				return nil, err
			}
			defer cleanup()
			return publishImage(c, params, source)
		},
	}
}

// imageBuildParams holds the checked request parameters of image.build.
type imageBuildParams struct {
	selector, name, path    string
	tags                    []string
	repositoryID, gitCommit string
}

// parseImageBuildParams checks the request parameters of image.build. It
// reads no store and no file.
func parseImageBuildParams(p registry.Params) (imageBuildParams, error) {
	params := imageBuildParams{
		selector:     strings.TrimSpace(p.String("source")),
		name:         p.String("name"),
		path:         p.String("path"),
		repositoryID: strings.TrimSpace(p.String("repository-id")),
		gitCommit:    strings.TrimSpace(p.String("git-commit")),
	}
	if params.selector != "" && strings.TrimSpace(params.path) != "" {
		return imageBuildParams{}, api.UserError{Code: "bad_source", Msg: "source and path are mutually exclusive", Status: http.StatusBadRequest}
	}
	if params.selector == "" && strings.TrimSpace(params.path) == "" {
		return imageBuildParams{}, api.UserError{Code: "missing_path", Msg: "image source path is required", Status: http.StatusBadRequest}
	}
	params.tags = p.Strings("tag")
	if (params.repositoryID == "") != (params.gitCommit == "") || (params.gitCommit != "" && !gitCommitPattern.MatchString(params.gitCommit)) {
		return imageBuildParams{}, api.UserError{Code: "bad_provenance", Msg: "repository-id and a 7-64 character hexadecimal git-commit must be provided together", Status: http.StatusBadRequest}
	}
	return params, nil
}

// resolveImageBuildName returns the target image name and tags.
func resolveImageBuildName(name string, tags []string) (string, []string, error) {
	// Accept the pre-v2 direct-handler shape during the schema-v1
	// compatibility window. Registry-driven callers must pass --name.
	if name == "" && len(tags) == 1 && tags[0] != "" {
		if legacy, legacyErr := image.ParseRef(tags[0]); legacyErr == nil {
			name, tags = legacy.Name, []string{legacy.Tag}
		}
	}
	if name == "" {
		return "", nil, api.UserError{Code: "missing_name", Msg: "image name is required", Status: http.StatusBadRequest}
	}
	return name, tags, nil
}

// imageBuildLocks restores the lock files of a store build; it is empty for
// a build from a path.
type imageBuildLocks struct {
	restore, restoreStore func() error
	install               func(dir string) error
}

// imageBuildSource is the frozen source an image is built from.
type imageBuildSource struct {
	sourceCWD   string
	snapshots   *imagesnapshot.Store
	frozen      imagesnapshot.FrozenSource
	parsed      *imagefile.V2
	refs        []image.Ref
	defaultTags bool
}

// freezeImageSource assembles the source, restores its locks and freezes a
// snapshot of it. cleanup removes the assembled directory.
func freezeImageSource(c *registry.Ctx, params imageBuildParams, locks imageBuildLocks) (imageBuildSource, func(), error) {
	cleanup := func() {}
	sourceCWD, sourceErr := canonicalSourceDir(params.path)
	if sourceErr != nil {
		return imageBuildSource{}, cleanup, api.UserError{Code: "bad_source_path", Msg: sourceErr.Error(), Status: http.StatusBadRequest}
	}
	snapshotStore := &imagesnapshot.Store{Root: filepath.Join(c.BaseDir, "image-source-snapshots"), Clock: time.Now, DB: c.Store.DB}
	parsed, err := imagefile.ParseV2(sourceCWD)
	if err != nil {
		return imageBuildSource{}, cleanup, api.UserError{Code: "bad_imagefile", Msg: err.Error()}
	}
	refs, defaultTags, err := imageBuildRefs(params.name, params.tags, parsed)
	if err != nil {
		return imageBuildSource{}, cleanup, err
	}
	if err := image.WithPublicationGate(func() error {
		return imageStore(c).Migrate()
	}); err != nil {
		return imageBuildSource{}, cleanup, api.UserError{Code: "build_failed", Msg: err.Error()}
	}
	buildDir := sourceCWD
	if len(parsed.Extends) > 0 {
		if locks.restoreStore != nil {
			if err := locks.restoreStore(); err != nil {
				return imageBuildSource{}, cleanup, storeError(err)
			}
		}
		assembled, removeAssembled, err := imagefile.Assemble(sourceCWD, imageBuildRoot(c), imagefile.ResolveRoots{Plugins: paths.Paths{Base: c.BaseDir}.PluginsDir()}, locks.install)
		if err != nil {
			return imageBuildSource{}, cleanup, api.UserError{Code: "bad_imagefile", Msg: err.Error()}
		}
		cleanup = removeAssembled
		buildDir = assembled
	} else if locks.restore != nil {
		if err := locks.restore(); err != nil {
			return imageBuildSource{}, cleanup, storeError(err)
		}
	}
	parsed, err = imagefile.ParseV2(buildDir)
	if err != nil {
		return imageBuildSource{}, cleanup, api.UserError{Code: "bad_imagefile", Msg: err.Error()}
	}
	frozen, err := snapshotStore.Freeze(buildDir, parsed.Skills...)
	if err != nil {
		return imageBuildSource{}, cleanup, api.UserError{Code: "bad_source_path", Msg: err.Error(), Status: http.StatusBadRequest}
	}
	frozenDir, err := snapshotStore.OpenFrozen(frozen)
	if err != nil {
		return imageBuildSource{}, cleanup, api.UserError{Code: "bad_source_path", Msg: err.Error(), Status: http.StatusBadRequest}
	}
	parsed, err = imagefile.ParseV2(frozenDir)
	if err != nil {
		return imageBuildSource{}, cleanup, api.UserError{Code: "bad_imagefile", Msg: err.Error()}
	}
	if defaultTags {
		refs, _, err = imageBuildRefs(params.name, params.tags, parsed)
		if err != nil {
			return imageBuildSource{}, cleanup, err
		}
	}
	return imageBuildSource{sourceCWD: sourceCWD, snapshots: snapshotStore, frozen: frozen, parsed: parsed, refs: refs, defaultTags: defaultTags}, cleanup, nil
}

// publishImage builds the image from source, moves every requested tag onto
// it and records its provenance in one transaction. A failure restores the
// previous images.
func publishImage(c *registry.Ctx, params imageBuildParams, source imageBuildSource) (any, error) {
	refs := source.refs
	pluginsDir := paths.Paths{Base: c.BaseDir}.PluginsDir()
	resolver := imagePluginResolver(c, pluginsDir)
	builtAt := time.Now()
	clock := func() time.Time { return builtAt }
	var result any
	err := image.WithPublicationGate(func() error {
		store := imageStore(c)
		if err := store.Migrate(); err != nil {
			return api.UserError{Code: "build_failed", Msg: err.Error()}
		}
		publication, err := store.BeginPublication(refs)
		if err != nil {
			return api.UserError{Code: "build_failed", Msg: err.Error()}
		}
		restore := func(code string, cause error) error {
			if restoreErr := publication.Restore(); restoreErr != nil {
				return api.UserError{Code: code, Msg: fmt.Sprintf("%v; restore images: %v", cause, restoreErr)}
			}
			return api.UserError{Code: code, Msg: cause.Error()}
		}
		// One build produces one ref; every requested tag is moved onto it,
		// so a versioned tag and latest always name the same content.
		man, err := image.BuildV2(source.parsed, imagefile.ResolveRoots{Plugins: pluginsDir, SourceSkills: source.frozen.SourceSkills}, refs[0], store, clock, resolver)
		if err != nil {
			return restore("build_failed", err)
		}
		for _, ref := range refs[1:] {
			if err := store.SetTag(ref, man.Digest); err != nil {
				return restore("build_failed", err)
			}
		}
		results := make([]map[string]any, 0, len(refs))
		tx, err := c.Store.DB.BeginTx(context.Background(), nil)
		if err != nil {
			return restore("provenance_failed", err)
		}
		defer tx.Rollback()
		provenanceStore := imageprovenance.Store{DB: c.Store.DB}
		for _, ref := range refs {
			if _, err := source.snapshots.CaptureFrozenTx(context.Background(), tx, ref.String(), man.Digest, params.name, source.frozen, imagesource.Provenance{RepositoryID: params.repositoryID, GitCommit: params.gitCommit}); err != nil {
				return restore("provenance_failed", err)
			}
			if err := provenanceStore.UpsertTx(tx, imageprovenance.Record{Ref: ref.String(), Digest: man.Digest, SourceCWD: source.sourceCWD, BuiltAt: man.BuiltAt}); err != nil {
				return restore("provenance_failed", err)
			}
			results = append(results, map[string]any{"name": man.Name, "tag": ref.Tag, "digest": man.Digest, "layers": len(man.Layers)})
		}
		if err := tx.Commit(); err != nil {
			return restore("provenance_failed", err)
		}
		if len(results) == 1 || source.defaultTags {
			result = results[0]
		} else {
			result = map[string]any{"images": results}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func imageBuildRefs(name string, tags []string, parsed *imagefile.V2) ([]image.Ref, bool, error) {
	refs, defaultTags, err := image.BuildRefs(name, tags, parsed.ImageVersion)
	switch {
	case err == nil:
		return refs, defaultTags, nil
	case errors.Is(err, image.ErrDuplicateTag):
		return nil, defaultTags, api.UserError{Code: "duplicate_tag", Msg: err.Error()}
	case errors.Is(err, image.ErrTagReserved):
		return nil, defaultTags, api.UserError{Code: "reserved_image", Msg: err.Error()}
	default:
		return nil, defaultTags, api.UserError{Code: "bad_ref", Msg: err.Error()}
	}
}

// imageBuildRoot holds the temporary directories an extends chain is
// assembled in.
func imageBuildRoot(c *registry.Ctx) string {
	return filepath.Join(c.BaseDir, "image-build")
}

func canonicalSourceDir(input string) (string, error) {
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		resolved = filepath.Dir(resolved)
	}
	return resolved, nil
}

func imageValidate() registry.Command {
	return registry.Command{
		Path: "image.validate", Summary: "Validate an image source directory",
		AbsPathArgs:  []string{"path"},
		ExitCodeFrom: invalidExitCode,
		Args: []registry.Arg{
			{Name: "name", Flag: "name", Type: registry.String, Required: true, Help: "target image name"},
			{Name: "tag", Flag: "tag", Type: registry.String, Default: "latest", Help: "target image tag"},
			{Name: "path", Flag: "path", Type: registry.String, Required: true, Help: "Tariboyfile.yaml or its directory"},
		},
		HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/images/validate"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			name, tag := p.String("name"), p.String("tag")
			if tag == "" {
				tag = "latest"
			}
			ref, refErr := image.ParseRef(name + ":" + tag)
			diagnostic := ""
			switch {
			case name == "":
				diagnostic = "image name is required"
			case refErr != nil:
				diagnostic = refErr.Error()
			case image.IsReserved(ref):
				diagnostic = "image " + ref.String() + " is managed by tariboyd"
			}
			if diagnostic != "" {
				return map[string]any{"valid": false, "schema_version": 0, "plugins": []string{}, "skills": []agentskills.Metadata{}, "template": nil, "diagnostics": []map[string]string{{"path": "name/tag", "message": diagnostic}}, "warnings": []any{}}, nil
			}
			parsed, err := imagefile.ParseV2(p.String("path"))
			if err == nil && len(parsed.Extends) > 0 {
				var cleanup func()
				var assembled string
				assembled, cleanup, err = imagefile.Assemble(parsed.Dir, imageBuildRoot(c), imagefile.ResolveRoots{Plugins: paths.Paths{Base: c.BaseDir}.PluginsDir()}, nil)
				if err == nil {
					defer cleanup()
					parsed, err = imagefile.ParseV2(assembled)
				}
			}
			if err != nil {
				return map[string]any{"valid": false, "schema_version": 0, "plugins": []string{}, "skills": []agentskills.Metadata{}, "template": nil, "diagnostics": []map[string]string{{"path": "Tariboyfile.yaml", "message": err.Error()}}, "warnings": []any{}}, nil
			}
			layout := paths.Paths{Base: c.BaseDir}
			var pluginStore *plugins.Store
			if c.Store != nil {
				pluginStore = plugins.NewStore(c.Store, time.Now)
			}
			validated, validateErr := image.ValidateV2Detailed(parsed, imagefile.ResolveRoots{Plugins: layout.PluginsDir()}, imagePluginResolver(c, layout.PluginsDir()))
			pluginNames := make([]string, 0, len(parsed.Plugins))
			for _, plugin := range parsed.Plugins {
				pluginNames = append(pluginNames, plugin.Name)
			}
			if validateErr != nil {
				return map[string]any{"valid": false, "schema_version": 2, "plugins": pluginNames, "skills": []agentskills.Metadata{}, "template": nil, "diagnostics": []map[string]string{{"path": "Tariboyfile.yaml", "message": validateErr.Error()}}, "warnings": []any{}}, nil
			}
			return map[string]any{"valid": true, "schema_version": 2, "plugins": pluginNames, "skills": validated.Skills, "template": validated.Template, "diagnostics": []any{}, "warnings": v2ValidationWarnings(parsed, validated, pluginStore)}, nil
		},
	}
}

func v2ValidationWarnings(source *imagefile.V2, validated image.ValidationV2, pluginStore *plugins.Store) []map[string]string {
	warnings := []map[string]string{}
	hasIdentity, hasFinish := false, false
	staticForPlugin := map[string]bool{}
	for i, entry := range validated.Template.Entries {
		if entry.Kind == "runtime" && entry.Runtime == "identity" {
			hasIdentity = true
		}
		if entry.Kind != "file" {
			continue
		}
		if filepath.IsAbs(entry.Source) {
			warnings = append(warnings, map[string]string{"path": fmt.Sprintf("prompts[%d]", i), "message": "absolute prompt path makes the original source host-bound"})
		}
		if entry.Size >= 128<<10 {
			warnings = append(warnings, map[string]string{"path": fmt.Sprintf("prompts[%d]", i), "message": "large static prompt may exceed an interactive harness argument limit"})
		}
		if strings.HasSuffix(filepath.ToSlash(entry.Source), "/loop/finish.md") {
			hasFinish = true
		}
		for _, plugin := range source.Plugins {
			needle := "/skills/" + plugin.Name + "/"
			if strings.Contains(filepath.ToSlash(entry.Source), needle) || strings.HasPrefix(entry.Source, "$PLUGINS/"+plugin.Name+"/") {
				staticForPlugin[plugin.Name] = true
			}
		}
		if pluginStore != nil && strings.HasPrefix(entry.Source, "$PLUGINS/") {
			parts := strings.Split(strings.TrimPrefix(entry.Source, "$PLUGINS/"), "/")
			if len(parts) >= 3 {
				if active, ok, err := pluginStore.ActiveVersion(parts[0]); err == nil && ok && active != parts[1] {
					warnings = append(warnings, map[string]string{"path": fmt.Sprintf("prompts[%d]", i), "message": fmt.Sprintf("static plugin version %s/%s differs from active runtime version %s", parts[0], parts[1], active)})
				}
			}
		}
	}
	skillIndex := make(map[string]int, len(validated.Skills))
	skillNames := make([]string, 0, len(validated.Skills))
	for i, skill := range validated.Skills {
		skillIndex[skill.Name] = i
		skillNames = append(skillNames, skill.Name)
		if filepath.IsAbs(skill.Source) {
			warnings = append(warnings, map[string]string{"path": fmt.Sprintf("skills[%d]", i), "message": "absolute skill path makes the original source host-bound"})
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		for _, duplicate := range agentskills.FindScopeDuplicates(skillNames, source.Dir, home) {
			warnings = append(warnings, map[string]string{
				"path":    fmt.Sprintf("skills[%d]", skillIndex[duplicate.Name]),
				"message": fmt.Sprintf("skill %s is also visible in %s scope; native harness precedence applies", duplicate.Name, duplicate.Scope),
			})
		}
	}
	if !hasIdentity {
		warnings = append(warnings, map[string]string{"path": "prompts", "message": "identity placeholder is omitted"})
	}
	for _, plugin := range source.Plugins {
		if plugin.Name == "loop" && !hasFinish {
			warnings = append(warnings, map[string]string{"path": "prompts", "message": "loop capability has no visible finishing instruction layer"})
		}
		if !staticForPlugin[plugin.Name] {
			warnings = append(warnings, map[string]string{"path": "plugins", "message": fmt.Sprintf("plugin %s has no obvious static instruction layer", plugin.Name)})
		}
	}
	return warnings
}

func imageTemplate() registry.Command {
	return imageReadCommand(registry.Command{
		Path: "image.template", Summary: "Show an image prompt template",
		Args: []registry.Arg{{Name: "ref", Type: registry.String, Required: true, Help: "image ref name:tag"}},
		HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/images/{ref}/template"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			ref, err := parseImageRef(p.Params)
			if err != nil {
				return nil, err
			}
			template, err := imageStore(c).ReadTemplate(ref)
			if err != nil {
				return nil, api.UserError{Code: "not_found", Msg: err.Error()}
			}
			return template, nil
		},
	})
}

func imageProvenance() registry.Command {
	return imageReadCommand(registry.Command{
		Path: "image.provenance", Summary: "Show local image source provenance",
		Args: []registry.Arg{{Name: "ref", Type: registry.String, Required: true, Help: "image ref name:tag"}},
		HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/images/{ref}/provenance"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			ref, err := parseImageRef(p.Params)
			if err != nil {
				return nil, err
			}
			if c.Store == nil {
				return map[string]any{"ref": ref.String(), "source_cwd": nil, "source_available": false}, nil
			}
			record, ok, err := (imageprovenance.Store{DB: c.Store.DB}).Get(ref.String())
			if err != nil {
				return nil, err
			}
			if !ok {
				return map[string]any{"ref": ref.String(), "source_cwd": nil, "source_available": false}, nil
			}
			current, err := imageStore(c).Inspect(ref)
			if err != nil {
				return nil, err
			}
			if record.Digest != current.Digest {
				return map[string]any{"ref": ref.String(), "source_cwd": nil, "source_available": false}, nil
			}
			result := map[string]any{
				"ref": record.Ref, "digest": record.Digest, "source_cwd": record.SourceCWD,
				"built_at": record.BuiltAt, "source_available": record.SourceAvailable,
			}
			if snapshot, found, lookupErr := imageSnapshotStore(c).Lookup(context.Background(), ref.String()); lookupErr != nil {
				return nil, lookupErr
			} else if found && snapshot.ImageDigest == record.Digest {
				result["source_name"], result["source_digest"] = snapshot.SourceName, snapshot.SourceDigest
				result["repository_id"], result["git_commit"], result["lock_digest"] = snapshot.RepositoryID, snapshot.GitCommit, snapshot.LockDigest
			}
			return result, nil
		},
	})
}

func imageLs() registry.Command {
	return registry.Command{
		Path:    "image.ls",
		Summary: "List built agent images",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/images"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			mans, err := imageStore(c).List()
			if err != nil {
				return nil, err
			}
			currentAgents := map[string][]string{}
			pendingAgents := map[string][]string{}
			if c.Store != nil {
				rows, queryErr := c.Store.Reader().Query(`SELECT name, image_ref, pending_image_ref FROM agents ORDER BY name`)
				if queryErr != nil {
					return nil, queryErr
				}
				for rows.Next() {
					var name, currentRef, pendingRef string
					if scanErr := rows.Scan(&name, &currentRef, &pendingRef); scanErr != nil {
						_ = rows.Close()
						return nil, scanErr
					}
					currentAgents[currentRef] = append(currentAgents[currentRef], name)
					if pendingRef != "" {
						pendingAgents[pendingRef] = append(pendingAgents[pendingRef], name)
					}
				}
				if rowsErr := rows.Err(); rowsErr != nil {
					_ = rows.Close()
					return nil, rowsErr
				}
				if closeErr := rows.Close(); closeErr != nil {
					return nil, closeErr
				}
			}
			sourceByDigest := map[string]string{}
			sources, err := imageSourceStore(c).List()
			if err != nil {
				if c.Log != nil {
					c.Log.Warn("image source metadata unavailable while listing images", "err", err)
				}
			} else {
				for _, source := range sources {
					if source.LastBuild != nil {
						sourceByDigest[source.LastBuild.Digest] = source.Name
					}
				}
			}
			images := make([]map[string]any, 0, len(mans))
			for _, m := range mans {
				ref := m.Name + ":" + m.Tag
				item := map[string]any{
					"schema_version": m.SchemaVersion,
					"name":           m.Name,
					"tag":            m.Tag,
					"digest":         m.Digest,
					"built_at":       m.BuiltAt,
					"bare":           m.Bare,
					"current_agents": currentAgents[ref],
					"pending_agents": pendingAgents[ref],
				}
				if m.ImageVersion != "" {
					item["image_version"] = m.ImageVersion
				}
				if source := sourceByDigest[m.Digest]; source != "" {
					item["source"] = source
				}
				if c.Store != nil {
					if provenance, ok, lookupErr := (imageprovenance.Store{DB: c.Store.DB}).Get(ref); lookupErr == nil && ok && provenance.Digest == m.Digest {
						item["source_cwd"] = provenance.SourceCWD
						item["source_available"] = provenance.SourceAvailable
					}
				}
				item["exportable"] = !image.IsReserved(image.Ref{Name: m.Name, Tag: m.Tag})
				if snapshot, ok, lookupErr := imageSnapshotStore(c).Lookup(context.Background(), ref); lookupErr != nil {
					if c.Log != nil {
						c.Log.Warn("image source snapshot unavailable while listing images", "err", lookupErr)
					}
					// Legacy snapshot metadata remains readable but no longer controls artifact export.
				} else if ok && snapshot.ImageDigest == m.Digest {
					item["source_digest"] = snapshot.SourceDigest
				}
				images = append(images, item)
			}
			return map[string]any{"images": images, "count": len(images)}, nil
		},
	}
}

func imageInspect() registry.Command {
	return imageReadCommand(registry.Command{
		Path:    "image.inspect",
		Summary: "Show an image manifest",
		Args:    []registry.Arg{{Name: "ref", Type: registry.String, Required: true, Help: "image ref name:tag"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/images/{ref}"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			ref, err := parseImageRef(p.Params)
			if err != nil {
				return nil, err
			}
			man, err := imageStore(c).Inspect(ref)
			if err != nil {
				return nil, api.UserError{Code: "not_found", Msg: err.Error()}
			}
			return man, nil
		},
	})
}

func imagePrompt() registry.Command {
	return imageReadCommand(registry.Command{
		Path:    "image.prompt",
		Summary: "Print an image's assembled prompt",
		Args:    []registry.Arg{{Name: "ref", Type: registry.String, Required: true, Help: "image ref name:tag"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/images/{ref}/prompt"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			ref, err := parseImageRef(p.Params)
			if err != nil {
				return nil, err
			}
			s, err := imageStore(c).RenderPrompt(ref)
			if err != nil {
				return nil, api.UserError{Code: "not_found", Msg: err.Error()}
			}
			return map[string]any{"prompt": s}, nil
		},
	})
}

func imageFiles() registry.Command {
	return imageReadCommand(registry.Command{
		Path:    "image.files",
		Summary: "List the files packed into an image",
		Args:    []registry.Arg{{Name: "ref", Type: registry.String, Required: true, Help: "image ref name:tag"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/images/{ref}/files"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			ref, err := parseImageRef(p.Params)
			if err != nil {
				return nil, err
			}
			entries, err := imageStore(c).ListFiles(ref)
			if err != nil {
				return nil, api.UserError{Code: "not_found", Msg: err.Error()}
			}
			return map[string]any{"files": entries, "count": len(entries)}, nil
		},
	})
}

func imageFileRead() registry.Command {
	return imageReadCommand(registry.Command{
		Path:    "image.file",
		Summary: "Read a single file packed into an image",
		Args: []registry.Arg{
			{Name: "ref", Type: registry.String, Required: true, Help: "image ref name:tag"},
			{Name: "path", Type: registry.String, Required: true, Help: "in-image file path"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/images/{ref}/files/{path...}"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			ref, err := parseImageRef(p.Params)
			if err != nil {
				return nil, err
			}
			name := p.String("path")
			data, err := imageStore(c).ReadFile(ref, name)
			if err != nil {
				return nil, api.UserError{Code: "not_found", Msg: err.Error()}
			}
			return map[string]any{"path": name, "content": string(data)}, nil
		},
	})
}

func imageRm() registry.Command {
	return registry.Command{
		Path:    "image.rm",
		Summary: "Remove a built image",
		Args:    []registry.Arg{{Name: "ref", Type: registry.String, Required: true, Help: "image ref name:tag"}},
		HTTP:    &registry.HTTPRoute{Method: "DELETE", Path: "/api/images/{ref}"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			var result any
			err := image.WithPublicationGate(func() error {
				ref, err := parseImageRef(p.Params)
				if err != nil {
					return err
				}
				if image.IsReserved(ref) {
					return api.UserError{Code: "reserved_image", Msg: "image " + ref.String() + " is managed by tariboyd"}
				}
				if c.Store != nil {
					var users int
					if err := c.Store.DB.QueryRow(`SELECT COUNT(*) FROM agents WHERE image_ref=? OR pending_image_ref=?`, ref.String(), ref.String()).Scan(&users); err != nil {
						return err
					}
					if users > 0 {
						return api.UserError{Code: "image_in_use", Msg: "image " + ref.String() + " is active or pending for an agent", Status: http.StatusConflict}
					}
				}
				if c.Store != nil {
					if err := (imageprovenance.Store{DB: c.Store.DB}).Delete(ref.String()); err != nil {
						return err
					}
				}
				if err := imageStore(c).Remove(ref); err != nil {
					return api.UserError{Code: "not_found", Msg: err.Error()}
				}
				result = map[string]any{"removed": ref.String()}
				return nil
			})
			return result, err
		},
	}
}

func parseImageRef(p registry.Params) (image.Ref, error) {
	raw := p.String("ref")
	ref, err := image.ParseRef(raw)
	if err != nil {
		return image.Ref{}, api.UserError{Code: "bad_ref", Msg: err.Error()}
	}
	return ref, nil
}

// imageReadCommand keeps each detail read in one published generation. Clients
// pass the displayed manifest digest to reject a ref rebuilt between requests.
func imageReadCommand(command registry.Command) registry.Command {
	command.Args = append(command.Args, registry.Arg{Name: "expected_digest", Flag: "expected-digest", Type: registry.String, Help: "reject a changed image digest"})
	handler := command.Handler
	command.Handler = func(c *registry.Ctx, p registry.Request) (any, error) {
		var result any
		err := image.WithPublicationGate(func() error {
			if expected := p.String("expected_digest"); expected != "" {
				ref, err := parseImageRef(p.Params)
				if err != nil {
					return err
				}
				manifest, err := imageStore(c).Inspect(ref)
				if err != nil {
					return api.UserError{Code: "not_found", Msg: err.Error()}
				}
				if manifest.Digest != expected {
					return api.UserError{Code: "image_changed", Msg: "Image changed; refresh to inspect the new build", Status: http.StatusConflict}
				}
			}
			var err error
			result, err = handler(c, p)
			return err
		})
		return result, err
	}
	return command
}
