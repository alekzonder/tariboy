package image

import (
	"time"

	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/version"
)

// BareRef is the built-in instructions-free image the daemon seeds at startup
// (spec: docs/superpowers/specs/2026-07-22-terminals-simple-ui-design.md §1).
var BareRef = Ref{Name: "bare", Tag: "latest"}

// EnsureBare rebuilds bare:latest on every daemon start. The daemon owns the
// image, so it always matches this release's image contract, whatever an
// earlier release left in the store. Its image_version is the daemon version:
// a restart of one release rewrites the same id, and a new release moves the
// tag to a new id while earlier archives stay for agents pinned to them.
func EnsureBare(s *Store, clock func() time.Time) error {
	return WithRefLock(BareRef.Name, func() error {
		_, err := buildV2(&imagefile.V2{SchemaVersion: 2, ImageVersion: version.Version, Plugins: []imagefile.V2Plugin{}, Prompts: []imagefile.PromptEntry{}}, imagefile.ResolveRoots{}, BareRef, s, clock, nil, nil)
		return err
	})
}
