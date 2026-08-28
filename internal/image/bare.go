package image

import (
	"errors"
	"time"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

// BareRef is the built-in instructions-free image the daemon seeds at startup
// (spec: docs/superpowers/specs/2026-07-22-terminals-simple-ui-design.md §1).
var BareRef = Ref{Name: "bare", Tag: "latest"}

// EnsureBare seeds bare:latest into the store if it is absent. An existing
// image named bare:latest is left untouched (idempotent, operator-overridable).
func EnsureBare(s *Store, clock func() time.Time) error {
	if s.Exists(BareRef) {
		return nil
	}
	_, err := BuildV2(&imagefile.V2{SchemaVersion: 2, Plugins: []imagefile.V2Plugin{}, Prompts: []imagefile.PromptEntry{}}, imagefile.ResolveRoots{}, BareRef, s, clock, nil)
	if errors.Is(err, ErrExists) {
		return nil
	}
	return err
}
