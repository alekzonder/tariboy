package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/operations"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/stores"
)

// autoBuildInitiator marks the image.build calls of the Store automatic build
// cycle. Such a build of a Store selector uses only the HEAD commit and the
// skills its locks pin.
const autoBuildInitiator = "auto"

// runAutoBuild builds a Store image from a clean export of the Store's HEAD
// commit. It installs the skills locks in the export, refuses installed
// skills that differ from them, and records the commit and the lock digest
// as provenance. The export is removed afterwards.
func runAutoBuild(ctx context.Context, c *registry.Ctx, params imageBuildParams, log *operations.Log) (ImageBuildResult, error) {
	log.Printf("export the HEAD commit of Store image %s", params.selector)
	export, release, err := storeCatalog(c).PrepareAutoBuild(ctx, params.selector)
	if err != nil {
		return ImageBuildResult{}, storeError(err)
	}
	defer release()
	check, err := stores.NewLockCheck(export.Root, export.Path)
	if err != nil {
		return ImageBuildResult{}, api.UserError{Code: "bad_imagefile", Msg: err.Error()}
	}
	params.path, params.gitCommit, params.lockDigest = export.Path, export.Commit, check.Digest()
	log.Printf("commit %s, lock digest %s", params.gitCommit, params.lockDigest)
	npx := log.Writer("")
	defer npx.Close()
	install := func(dir string) error {
		if err := check.Record(dir); err != nil {
			return err
		}
		return stores.RestoreLock(ctx, dir, npx)
	}
	locks := imageBuildLocks{
		restore: func() error {
			if err := install(export.Root); err != nil {
				return err
			}
			return install(export.Path)
		},
		restoreStore: func() error { return install(export.Root) },
		install:      install,
		verify:       check.Verify,
	}
	if params.name == "" {
		params.name = export.Name
	}
	if params.name, params.tags, err = resolveImageBuildName(params.name, params.tags); err != nil {
		return ImageBuildResult{}, err
	}
	source, cleanup, err := freezeImageSource(c, params, locks, log)
	if err != nil {
		return ImageBuildResult{}, err
	}
	defer cleanup()
	// The export disappears after the build; provenance names the image in
	// the Store working copy, as a manual Store build does.
	source.sourceCWD = export.SourcePath
	return publishImage(c, params, source, log)
}

// autoBuildError starts the message of an automatic build failure with the
// Store and image it concerns.
func autoBuildError(selector string, err error) error {
	store, name, _ := strings.Cut(selector, "/")
	prefix := fmt.Sprintf("Store %s image %s: ", store, name)
	if userErr, ok := errors.AsType[api.UserError](err); ok {
		userErr.Msg = prefix + userErr.Msg
		return userErr
	}
	return fmt.Errorf("%s%w", prefix, err)
}
