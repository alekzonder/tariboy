// Package imageportable imports and exports runnable image artifacts. Source
// directories and local provenance are deliberately outside this format.
package imageportable

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imageprovenance"
	"github.com/alekzonder/tariboy/internal/imagesnapshot"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/plugincaps"
	"github.com/alekzonder/tariboy/internal/portablearchive"
	"github.com/alekzonder/tariboy/internal/version"
)

// Deprecated compatibility fields remain so older daemon wiring compiles; the
// runnable format never reads snapshots or plugin/source data.
type Service struct {
	Snapshots       *imagesnapshot.Store
	StagingRoot     string
	BaseDir         string
	ExternalPlugins plugincaps.ExternalResolver
}

type metadata struct {
	Ref             string   `json:"ref"`
	Digest          string   `json:"digest"`
	ProducerVersion string   `json:"producer_version"`
	Plugins         []string `json:"plugins"`
}
type Preview struct {
	ImportID string `json:"import_id"`
	Ref      string `json:"ref"`
	Digest   string `json:"digest"`
}
type Result struct {
	Ref    string `json:"ref"`
	Digest string `json:"digest"`
	Reused bool   `json:"reused"`
}

func (s Service) imageStore() *image.Store {
	return &image.Store{Dir: paths.Paths{Base: s.BaseDir}.ImagesDir()}
}

func (s Service) Export(ctx context.Context, refText string, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ref, err := image.ParseRef(refText)
	if err != nil {
		return err
	}
	store := s.imageStore()
	manifest, err := store.Inspect(ref)
	if err != nil {
		return err
	}
	body, err := store.ArchiveBytes(ref)
	if err != nil {
		return err
	}
	temp, err := os.MkdirTemp("", "tariboy-image-artifact-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if err := os.WriteFile(filepath.Join(temp, "image.tar.gz"), body, 0o600); err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	file := portablearchive.File{Path: "image.tar.gz", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}
	pluginNames := make([]string, 0, len(manifest.Plugins))
	for _, plugin := range manifest.Plugins {
		pluginNames = append(pluginNames, plugin.Name)
	}
	meta, err := json.Marshal(metadata{Ref: ref.String(), Digest: manifest.Digest, ProducerVersion: version.Version, Plugins: pluginNames})
	if err != nil {
		return err
	}
	return portablearchive.Write(w, temp, portablearchive.Manifest{Format: "tariboy-portable", Version: 1, Kind: "image-artifact", Files: []portablearchive.File{file}, Metadata: meta}, []string{"image.tar.gz"})
}

func (s Service) Preview(ctx context.Context, r io.Reader, compressedSize int64) (Preview, error) {
	if err := ctx.Err(); err != nil {
		return Preview{}, err
	}
	if s.StagingRoot == "" {
		return Preview{}, errors.New("image portable: staging root unavailable")
	}
	if err := os.MkdirAll(s.StagingRoot, 0o700); err != nil {
		return Preview{}, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Preview{}, err
	}
	id := hex.EncodeToString(random[:])
	dest := filepath.Join(s.StagingRoot, id)
	manifest, err := portablearchive.Stage(r, compressedSize, dest, portablearchive.DefaultLimits())
	if err != nil {
		return Preview{}, err
	}
	remove := true
	defer func() {
		if remove {
			_ = os.RemoveAll(dest)
		}
	}()
	if manifest.Kind != "image-artifact" || len(manifest.Files) != 1 || manifest.Files[0].Path != "image.tar.gz" {
		return Preview{}, errors.New("image portable: archive is not a runnable image artifact")
	}
	var meta metadata
	dec := json.NewDecoder(strings.NewReader(string(manifest.Metadata)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&meta); err != nil || meta.Ref == "" || meta.Digest == "" || meta.ProducerVersion == "" || meta.Plugins == nil {
		return Preview{}, errors.New("image portable: incomplete metadata")
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return Preview{}, errors.New("image portable: invalid metadata")
	}
	ref, err := image.ParseRef(meta.Ref)
	if err != nil || image.IsReserved(ref) {
		return Preview{}, errors.New("image portable: invalid image ref")
	}
	body, err := os.ReadFile(filepath.Join(dest, "image.tar.gz"))
	if err != nil {
		return Preview{}, err
	}
	actual := sha256.Sum256(body)
	if hex.EncodeToString(actual[:]) != meta.Digest {
		return Preview{}, errors.New("image portable: image digest mismatch")
	}
	inner, err := image.ValidatePortableArchiveContract(body, ref, s.ExternalPlugins)
	if err != nil {
		return Preview{}, fmt.Errorf("image portable: invalid runnable artifact: %w", err)
	}
	if len(inner.Plugins) != len(meta.Plugins) {
		return Preview{}, errors.New("image portable: plugin metadata mismatch")
	}
	for i, plugin := range inner.Plugins {
		if plugin.Name != meta.Plugins[i] {
			return Preview{}, errors.New("image portable: plugin metadata mismatch")
		}
	}
	preview := Preview{ImportID: id, Ref: meta.Ref, Digest: meta.Digest}
	data, _ := json.Marshal(preview)
	if err := os.WriteFile(filepath.Join(dest, ".preview.json"), data, 0o600); err != nil {
		return Preview{}, err
	}
	remove = false
	return preview, nil
}

func (s Service) Apply(ctx context.Context, importID, refOverride string) (Result, error) {
	var result Result
	err := image.WithPublicationGate(func() error {
		var err error
		result, err = s.apply(ctx, importID, refOverride)
		return err
	})
	return result, err
}

func (s Service) apply(ctx context.Context, importID, refOverride string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if importID == "" || filepath.Base(importID) != importID || s.BaseDir == "" {
		return Result{}, errors.New("image portable: invalid apply request")
	}
	stage := filepath.Join(s.StagingRoot, importID)
	data, err := os.ReadFile(filepath.Join(stage, ".preview.json"))
	if err != nil {
		return Result{}, errors.New("image portable: import preview not found")
	}
	var preview Preview
	if json.Unmarshal(data, &preview) != nil || preview.ImportID != importID {
		return Result{}, errors.New("image portable: invalid import preview")
	}
	targetText := preview.Ref
	if refOverride != "" {
		targetText = refOverride
	}
	target, err := image.ParseRef(targetText)
	if err != nil || image.IsReserved(target) {
		return Result{}, errors.New("image portable: invalid target ref")
	}
	body, err := os.ReadFile(filepath.Join(stage, "image.tar.gz"))
	if err != nil {
		return Result{}, err
	}
	source, err := image.ParseRef(preview.Ref)
	if err != nil {
		return Result{}, errors.New("image portable: invalid source ref")
	}
	candidate, err := image.ValidatePortableArchiveContract(body, source, s.ExternalPlugins)
	if err != nil {
		return Result{}, fmt.Errorf("image portable: incompatible runnable artifact: %w", err)
	}
	candidateArchive := body
	if target.String() != preview.Ref {
		staged := &image.Store{Dir: filepath.Join(stage, ".retagged")}
		candidate, err = staged.RetagMutableArchiveManifest(source, target, body)
		if err != nil {
			return Result{}, err
		}
		candidateArchive, err = staged.ArchiveBytes(target)
		if err != nil {
			return Result{}, err
		}
	}
	store := s.imageStore()
	committed := func(string, string) (bool, error) { return false, nil }
	if s.Snapshots != nil && s.Snapshots.DB != nil {
		committed = (imageprovenance.Store{DB: s.Snapshots.DB}).IsCommitted
	}
	if err := store.RecoverMutablePublications(committed); err != nil {
		return Result{}, err
	}
	targetExists := store.Exists(target)
	previousDigest := ""
	if targetExists {
		manifest, err := store.Inspect(target)
		if err != nil {
			return Result{}, err
		}
		previousDigest = manifest.Digest
		if target.String() == preview.Ref && manifest.Digest == preview.Digest {
			_ = os.RemoveAll(stage)
			return Result{Ref: target.String(), Digest: manifest.Digest, Reused: true}, nil
		}
	}
	if !targetExists && s.Snapshots != nil && s.Snapshots.DB != nil {
		if err := (imageprovenance.Store{DB: s.Snapshots.DB}).Delete(target.String()); err != nil {
			return Result{}, err
		}
	}
	publication, err := store.BeginMutablePublication([]image.PublicationCandidate{{Ref: target, Digest: candidate.Digest}})
	if err != nil {
		return Result{}, err
	}
	rollback := func(cause error) error {
		if rollbackErr := publication.Rollback(); rollbackErr != nil {
			return errors.Join(cause, fmt.Errorf("roll back image import: %w", rollbackErr))
		}
		return cause
	}
	published, err := store.InstallMutableArchive(target, candidateArchive)
	if err != nil {
		return Result{}, rollback(err)
	}
	if published.Digest != candidate.Digest {
		return Result{}, rollback(errors.New("image portable: installed digest mismatch"))
	}
	if err := publication.Complete(); err != nil {
		return Result{}, rollback(err)
	}
	_ = os.RemoveAll(stage)
	return Result{Ref: target.String(), Digest: published.Digest, Reused: targetExists && previousDigest == published.Digest}, nil
}
