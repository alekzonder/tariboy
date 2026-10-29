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
	"github.com/alekzonder/tariboy/internal/operations"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/plugincaps"
	"github.com/alekzonder/tariboy/internal/plugins"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/stores"
	"github.com/alekzonder/tariboy/internal/tidy"
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
	return registry.WithResult[OperationStarted](registry.Command{
		Path:        "image.build",
		AbsPathArgs: []string{"path"},
		Summary:     "Start a background build of an agent image from a Tariboyfile.yaml",
		Help:        "The daemon builds the image as an operation. The CLI prints the operation log until the build ends, then the result; --detach prints only the operation id.",
		Args: append([]registry.Arg{
			{Name: "source", Type: registry.String, Help: "Store selector (store/image)"},
			{Name: "name", Flag: "name", Type: registry.String, Help: "target image name"},
			{Name: "tag", Flag: "tag", Type: registry.String, Repeatable: true, Help: "target image tag (default image_version and latest, or only latest when absent)"},
			{Name: "path", Flag: "path", Type: registry.String, Help: "Tariboyfile.yaml or its directory"},
			{Name: "repository-id", Flag: "repository-id", Type: registry.String, Help: "source repository ID"},
			{Name: "git-commit", Flag: "git-commit", Type: registry.String, Help: "source Git commit"},
		}, operationStartArgs()...),
		HTTP:  &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/images/build"},
		Local: buildLocal("/api/images/build"),
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			params, err := parseImageBuildParams(p.Params)
			if err != nil {
				return nil, err
			}
			if params.selector == "" {
				if params.name, params.tags, err = resolveImageBuildName(params.name, params.tags); err != nil {
					return nil, err
				}
				if _, err := canonicalSourceDir(params.path); err != nil {
					return nil, api.UserError{Code: "bad_source_path", Msg: err.Error(), Status: http.StatusBadRequest}
				}
			}
			if c.Store == nil {
				return nil, api.UserError{Code: "build_failed", Msg: "image metadata store is unavailable"}
			}
			spec := operations.Spec{Kind: "image.build", Title: imageBuildTitle(params), Initiator: p.String("initiator"), Params: params.record()}
			return startOperation(c, p, spec, func(ctx context.Context, log *operations.Log) (any, error) {
				return runImageBuild(ctx, c, params, log)
			})
		},
	})
}

// runImageBuild builds the image of params; ctx is the operation's context.
func runImageBuild(ctx context.Context, c *registry.Ctx, params imageBuildParams, log *operations.Log) (ImageBuildResult, error) {
	var locks imageBuildLocks
	if params.selector != "" {
		log.Printf("prepare Store image %s", params.selector)
		catalog := storeCatalog(c)
		prepared, release, err := catalog.PrepareBuild(params.selector)
		if err != nil {
			return ImageBuildResult{}, storeError(err)
		}
		defer release()
		params.path = prepared.Path
		npx := log.Writer("")
		defer npx.Close()
		locks = imageBuildLocks{
			restore:      func() error { return catalog.RestoreBuildLocks(ctx, prepared, npx) },
			restoreStore: func() error { return catalog.RestoreStoreLock(ctx, prepared, npx) },
			install:      func(dir string) error { return stores.RestoreLock(ctx, dir, npx) },
		}
		if params.name == "" {
			params.name = prepared.Name
		}
		if params.name, params.tags, err = resolveImageBuildName(params.name, params.tags); err != nil {
			return ImageBuildResult{}, err
		}
	}
	source, cleanup, err := freezeImageSource(c, params, locks, log)
	if err != nil {
		return ImageBuildResult{}, err
	}
	defer cleanup()
	return publishImage(c, params, source, log)
}

// imageBuildTitle names an image build in the operations list.
func imageBuildTitle(params imageBuildParams) string {
	target := params.selector
	if target == "" {
		target = params.name
		if len(params.tags) > 0 {
			target += ":" + strings.Join(params.tags, ",")
		}
	}
	return "image build " + target
}

// record returns the build parameters an operation shows.
func (params imageBuildParams) record() map[string]string {
	out := map[string]string{}
	for key, value := range map[string]string{
		"source": params.selector, "name": params.name, "tag": strings.Join(params.tags, ","), "path": params.path,
		"repository-id": params.repositoryID, "git-commit": params.gitCommit,
	} {
		if value != "" {
			out[key] = value
		}
	}
	return out
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
func freezeImageSource(c *registry.Ctx, params imageBuildParams, locks imageBuildLocks, log *operations.Log) (imageBuildSource, func(), error) {
	cleanup := func() {}
	sourceCWD, sourceErr := canonicalSourceDir(params.path)
	if sourceErr != nil {
		return imageBuildSource{}, cleanup, api.UserError{Code: "bad_source_path", Msg: sourceErr.Error(), Status: http.StatusBadRequest}
	}
	snapshotStore := &imagesnapshot.Store{Root: filepath.Join(c.BaseDir, "image-source-snapshots"), Clock: time.Now, DB: c.Store.DB}
	log.Printf("parse %s", filepath.Join(sourceCWD, imagefile.DefaultFilename))
	parsed, err := imagefile.ParseV2(sourceCWD)
	if err != nil {
		return imageBuildSource{}, cleanup, api.UserError{Code: "bad_imagefile", Msg: err.Error()}
	}
	refs, defaultTags, err := imageBuildRefs(params.name, params.tags, parsed)
	if err != nil {
		return imageBuildSource{}, cleanup, err
	}
	// Migration walks every image name; MigrateOnce takes the store lock for
	// it once. The publication later takes only the lock of the name it builds.
	if err := imageStore(c).MigrateOnce(); err != nil {
		return imageBuildSource{}, cleanup, api.UserError{Code: "build_failed", Msg: err.Error()}
	}
	buildDir := sourceCWD
	if len(parsed.Extends) > 0 {
		if locks.restoreStore != nil {
			log.Printf("restore the Store skills lock")
			if err := locks.restoreStore(); err != nil {
				return imageBuildSource{}, cleanup, storeError(err)
			}
		}
		log.Printf("assemble the extends chain: %s", strings.Join(parsed.Extends, ", "))
		assembled, removeAssembled, err := imagefile.Assemble(sourceCWD, imageBuildRoot(c), imagefile.ResolveRoots{Plugins: paths.Paths{Base: c.BaseDir}.PluginsDir()}, locks.install)
		if err != nil {
			return imageBuildSource{}, cleanup, api.UserError{Code: "bad_imagefile", Msg: err.Error()}
		}
		cleanup = removeAssembled
		buildDir = assembled
	} else if locks.restore != nil {
		log.Printf("restore the skills locks")
		if err := locks.restore(); err != nil {
			return imageBuildSource{}, cleanup, storeError(err)
		}
	}
	parsed, err = imagefile.ParseV2(buildDir)
	if err != nil {
		return imageBuildSource{}, cleanup, api.UserError{Code: "bad_imagefile", Msg: err.Error()}
	}
	log.Printf("freeze a snapshot of the source")
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
func publishImage(c *registry.Ctx, params imageBuildParams, source imageBuildSource, log *operations.Log) (ImageBuildResult, error) {
	refs := source.refs
	pluginsDir := paths.Paths{Base: c.BaseDir}.PluginsDir()
	resolver := imagePluginResolver(c, pluginsDir)
	builtAt := time.Now()
	clock := func() time.Time { return builtAt }
	var result ImageBuildResult
	// Every ref of one build shares the image name. freezeImageSource has
	// already migrated the store.
	log.Printf("wait for the lock of image %s", refs[0].Name)
	err := image.WithRefLock(refs[0].Name, func() error {
		log.Printf("build layers of %s", refs[0])
		store := imageStore(c)
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
		log.Printf("built %s digest %s, %d layers", refs[0], man.Digest, len(man.Layers))
		for _, ref := range refs[1:] {
			log.Printf("tag %s", ref)
			if err := store.SetTag(ref, man.Digest); err != nil {
				return restore("build_failed", err)
			}
		}
		log.Printf("record the provenance")
		results := make([]ImageBuilt, 0, len(refs))
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
			results = append(results, ImageBuilt{Name: man.Name, Tag: ref.Tag, Digest: man.Digest, Layers: len(man.Layers)})
		}
		if err := tx.Commit(); err != nil {
			return restore("provenance_failed", err)
		}
		if len(results) == 1 || source.defaultTags {
			first := results[0]
			result = ImageBuildResult{Name: first.Name, Tag: first.Tag, Digest: first.Digest, Layers: &first.Layers}
		} else {
			result = ImageBuildResult{Images: results}
		}
		return nil
	})
	if err != nil {
		return ImageBuildResult{}, err
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
	return registry.WithResult[ImageValidateResult](registry.Command{
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
				return imageValidateFailure(0, []string{}, "name/tag", diagnostic), nil
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
				return imageValidateFailure(0, []string{}, "Tariboyfile.yaml", err.Error()), nil
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
				return imageValidateFailure(2, pluginNames, "Tariboyfile.yaml", validateErr.Error()), nil
			}
			template := imagePromptTemplate(validated.Template)
			return ImageValidateResult{
				Valid: true, SchemaVersion: 2, Plugins: pluginNames, Skills: imageSkills(validated.Skills),
				Template:    ImageValidateTemplate{ImagePromptTemplate: &template},
				Diagnostics: []ImageDiagnostic{}, Warnings: v2ValidationWarnings(parsed, validated, pluginStore),
			}, nil
		},
	})
}

// imageValidateFailure is the image.validate result of an invalid source: one
// diagnostic, no skills and a null template.
func imageValidateFailure(schemaVersion int, plugins []string, path, message string) ImageValidateResult {
	return ImageValidateResult{
		SchemaVersion: schemaVersion, Plugins: plugins, Skills: []ImageSkill{},
		Diagnostics: []ImageDiagnostic{{Path: path, Message: message}}, Warnings: []ImageDiagnostic{},
	}
}

func v2ValidationWarnings(source *imagefile.V2, validated image.ValidationV2, pluginStore *plugins.Store) []ImageDiagnostic {
	warnings := []ImageDiagnostic{}
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
			warnings = append(warnings, ImageDiagnostic{Path: fmt.Sprintf("prompts[%d]", i), Message: "absolute prompt path makes the original source host-bound"})
		}
		if entry.Size >= 128<<10 {
			warnings = append(warnings, ImageDiagnostic{Path: fmt.Sprintf("prompts[%d]", i), Message: "large static prompt may exceed an interactive harness argument limit"})
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
					warnings = append(warnings, ImageDiagnostic{Path: fmt.Sprintf("prompts[%d]", i), Message: fmt.Sprintf("static plugin version %s/%s differs from active runtime version %s", parts[0], parts[1], active)})
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
			warnings = append(warnings, ImageDiagnostic{Path: fmt.Sprintf("skills[%d]", i), Message: "absolute skill path makes the original source host-bound"})
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		for _, duplicate := range agentskills.FindScopeDuplicates(skillNames, source.Dir, home) {
			warnings = append(warnings, ImageDiagnostic{
				Path:    fmt.Sprintf("skills[%d]", skillIndex[duplicate.Name]),
				Message: fmt.Sprintf("skill %s is also visible in %s scope; native harness precedence applies", duplicate.Name, duplicate.Scope),
			})
		}
	}
	if !hasIdentity {
		warnings = append(warnings, ImageDiagnostic{Path: "prompts", Message: "identity placeholder is omitted"})
	}
	for _, plugin := range source.Plugins {
		if plugin.Name == "loop" && !hasFinish {
			warnings = append(warnings, ImageDiagnostic{Path: "prompts", Message: "loop capability has no visible finishing instruction layer"})
		}
		if !staticForPlugin[plugin.Name] {
			warnings = append(warnings, ImageDiagnostic{Path: "plugins", Message: fmt.Sprintf("plugin %s has no obvious static instruction layer", plugin.Name)})
		}
	}
	return warnings
}

func imageTemplate() registry.Command {
	return registry.WithResult[ImagePromptTemplate](imageReadCommand(registry.Command{
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
			return imagePromptTemplate(template), nil
		},
	}))
}

func imageProvenance() registry.Command {
	return registry.WithResult[ImageProvenance](imageReadCommand(registry.Command{
		Path: "image.provenance", Summary: "Show local image source provenance",
		Args: []registry.Arg{{Name: "ref", Type: registry.String, Required: true, Help: "image ref name:tag"}},
		HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/images/{ref}/provenance"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			ref, err := parseImageRef(p.Params)
			if err != nil {
				return nil, err
			}
			absent := ImageProvenance{Ref: ref.String()}
			if c.Store == nil {
				return absent, nil
			}
			record, ok, err := (imageprovenance.Store{DB: c.Store.DB}).Get(ref.String())
			if err != nil {
				return nil, err
			}
			if !ok {
				return absent, nil
			}
			current, err := imageStore(c).Inspect(ref)
			if err != nil {
				return nil, err
			}
			if record.Digest != current.Digest {
				return absent, nil
			}
			result := ImageProvenance{
				Ref: record.Ref, Digest: &record.Digest, SourceCWD: ImageNullableString{Value: record.SourceCWD, Valid: true},
				BuiltAt: &record.BuiltAt, SourceAvailable: record.SourceAvailable,
			}
			if snapshot, found, lookupErr := imageSnapshotStore(c).Lookup(context.Background(), ref.String()); lookupErr != nil {
				return nil, lookupErr
			} else if found && snapshot.ImageDigest == record.Digest {
				result.SourceName, result.SourceDigest = &snapshot.SourceName, &snapshot.SourceDigest
				result.RepositoryID, result.GitCommit, result.LockDigest = &snapshot.RepositoryID, &snapshot.GitCommit, &snapshot.LockDigest
			}
			return result, nil
		},
	}))
}

func imageLs() registry.Command {
	return registry.WithResult[ImageListResult](registry.Command{
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
						tidy.Close(nil, rows)
						return nil, scanErr
					}
					currentAgents[currentRef] = append(currentAgents[currentRef], name)
					if pendingRef != "" {
						pendingAgents[pendingRef] = append(pendingAgents[pendingRef], name)
					}
				}
				if rowsErr := rows.Err(); rowsErr != nil {
					tidy.Close(nil, rows)
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
			images := make([]ImageSummary, 0, len(mans))
			for _, m := range mans {
				ref := m.Name + ":" + m.Tag
				item := ImageSummary{
					SchemaVersion: m.SchemaVersion,
					Name:          m.Name,
					Tag:           m.Tag,
					Digest:        m.Digest,
					BuiltAt:       m.BuiltAt,
					Bare:          m.Bare,
					CurrentAgents: currentAgents[ref],
					PendingAgents: pendingAgents[ref],
					ImageVersion:  m.ImageVersion,
					Source:        sourceByDigest[m.Digest],
				}
				if c.Store != nil {
					if provenance, ok, lookupErr := (imageprovenance.Store{DB: c.Store.DB}).Get(ref); lookupErr == nil && ok && provenance.Digest == m.Digest {
						item.SourceCWD = &provenance.SourceCWD
						item.SourceAvailable = &provenance.SourceAvailable
					}
				}
				item.Exportable = !image.IsReserved(image.Ref{Name: m.Name, Tag: m.Tag})
				if snapshot, ok, lookupErr := imageSnapshotStore(c).Lookup(context.Background(), ref); lookupErr != nil {
					if c.Log != nil {
						c.Log.Warn("image source snapshot unavailable while listing images", "err", lookupErr)
					}
					// Legacy snapshot metadata remains readable but no longer controls artifact export.
				} else if ok && snapshot.ImageDigest == m.Digest {
					item.SourceDigest = &snapshot.SourceDigest
				}
				images = append(images, item)
			}
			return ImageListResult{Images: images, Count: len(images)}, nil
		},
	})
}

func imageInspect() registry.Command {
	return registry.WithResult[ImageManifest](imageReadCommand(registry.Command{
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
			return imageManifest(man), nil
		},
	}))
}

func imagePrompt() registry.Command {
	return registry.WithResult[ImagePromptResult](imageReadCommand(registry.Command{
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
			return ImagePromptResult{Prompt: s}, nil
		},
	}))
}

func imageFiles() registry.Command {
	return registry.WithResult[ImageFilesResult](imageReadCommand(registry.Command{
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
			return ImageFilesResult{Files: convertSlice(entries, func(e image.FileEntry) ImageFileEntry { return ImageFileEntry(e) }), Count: len(entries)}, nil
		},
	}))
}

func imageFileRead() registry.Command {
	return registry.WithResult[ImageFileContent](imageReadCommand(registry.Command{
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
			return ImageFileContent{Path: name, Content: string(data)}, nil
		},
	}))
}

func imageRm() registry.Command {
	return registry.WithResult[ImageRemoveResult](registry.Command{
		Path:    "image.rm",
		Summary: "Remove a built image",
		Args:    []registry.Arg{{Name: "ref", Type: registry.String, Required: true, Help: "image ref name:tag"}},
		HTTP:    &registry.HTTPRoute{Method: "DELETE", Path: "/api/images/{ref}"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			var result any
			// The store lock, not the name lock: removal checks that no
			// agent has the ref active or pending, and assignment changes
			// those under the store lock.
			err := image.WithStoreLock(func() error {
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
				result = ImageRemoveResult{Removed: ref.String()}
				return nil
			})
			return result, err
		},
	})
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
		// A detail read touches one image name. An unparsable ref locks its
		// raw text and the handler then rejects it.
		name := p.String("ref")
		if ref, err := image.ParseRef(name); err == nil {
			name = ref.Name
		}
		err := image.WithRefLock(name, func() error {
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
