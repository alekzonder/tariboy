// Package imagesnapshot stores immutable source trees for successfully built images.
package imagesnapshot

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/imagesource"
)

type Snapshot struct {
	Ref          string `json:"ref"`
	ImageDigest  string `json:"image_digest"`
	SourceName   string `json:"source_name"`
	SourceDigest string `json:"source_digest"`
	RelativeDir  string `json:"relative_dir"`
	CreatedAt    string `json:"created_at"`
	RepositoryID string `json:"repository_id,omitempty"`
	GitCommit    string `json:"git_commit,omitempty"`
	LockDigest   string `json:"lock_digest,omitempty"`
}

type Store struct {
	DB    *sql.DB
	Root  string
	Clock func() time.Time
}

type file struct {
	rel  string
	path string
	mode os.FileMode
}

type executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// FrozenSource is a content-addressed copy of an image source tree.
type FrozenSource struct {
	SourceDigest string
	RelativeDir  string
}

func (s Store) Capture(ctx context.Context, ref, imageDigest, sourceName, sourceDir string) (Snapshot, error) {
	return s.CaptureWithProvenance(ctx, ref, imageDigest, sourceName, sourceDir, imagesource.Provenance{})
}

func (s Store) CaptureWithProvenance(ctx context.Context, ref, imageDigest, sourceName, sourceDir string, provenance imagesource.Provenance) (Snapshot, error) {
	frozen, err := s.Freeze(sourceDir)
	if err != nil {
		return Snapshot{}, err
	}
	return s.CaptureFrozen(ctx, ref, imageDigest, sourceName, frozen, provenance)
}

// Freeze copies and content-addresses a source tree once so several builds can
// consume the identical files even if the original directory changes later.
func (s Store) Freeze(sourceDir string) (FrozenSource, error) {
	if s.Root == "" {
		return FrozenSource{}, errors.New("image snapshot: incomplete freeze request")
	}
	info, err := os.Lstat(sourceDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return FrozenSource{}, fmt.Errorf("image snapshot: unsafe source root")
	}
	var files []file
	err = filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == sourceDir {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("image snapshot: path escapes source")
		}
		if filepath.ToSlash(rel) == imagesource.MetadataFilename {
			return nil
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("image snapshot: symlink %s", rel)
		}
		if entryInfo.IsDir() {
			return nil
		}
		if !entryInfo.Mode().IsRegular() {
			return fmt.Errorf("image snapshot: non-regular file %s", rel)
		}
		if entryInfo.Size() > imagesource.MaxFileSize {
			return fmt.Errorf("image snapshot: file too large %s", rel)
		}
		files = append(files, file{rel: filepath.ToSlash(rel), path: path, mode: entryInfo.Mode()})
		return nil
	})
	if err != nil {
		return FrozenSource{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return FrozenSource{}, err
	}
	if err := os.Chmod(s.Root, 0o700); err != nil {
		return FrozenSource{}, err
	}
	stage, err := os.MkdirTemp(s.Root, ".staging-")
	if err != nil {
		return FrozenSource{}, err
	}
	defer os.RemoveAll(stage)
	h := sha256.New()
	for _, f := range files {
		mode := os.FileMode(0o600)
		if f.mode.Perm()&0o100 != 0 {
			mode = 0o700
		}
		fmt.Fprintf(h, "%s\x00%o\x00", f.rel, mode.Perm())
		dst := filepath.Join(stage, filepath.FromSlash(f.rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return FrozenSource{}, err
		}
		in, err := os.Open(f.path)
		if err != nil {
			return FrozenSource{}, err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			in.Close()
			return FrozenSource{}, err
		}
		_, copyErr := io.Copy(io.MultiWriter(out, h), in)
		closeOutErr := out.Close()
		closeInErr := in.Close()
		if copyErr != nil {
			return FrozenSource{}, copyErr
		}
		if closeOutErr != nil {
			return FrozenSource{}, closeOutErr
		}
		if closeInErr != nil {
			return FrozenSource{}, closeInErr
		}
		_, _ = h.Write([]byte{0})
	}
	digest := "sha256:" + hex.EncodeToString(h.Sum(nil))
	relativeDir := strings.TrimPrefix(digest, "sha256:")
	target := filepath.Join(s.Root, relativeDir)
	if err := os.Rename(stage, target); err != nil && !errors.Is(err, fs.ErrExist) {
		return FrozenSource{}, err
	}
	return FrozenSource{SourceDigest: digest, RelativeDir: relativeDir}, nil
}

func (s Store) OpenFrozen(frozen FrozenSource) (string, error) {
	return s.Open(Snapshot{RelativeDir: frozen.RelativeDir})
}

// CaptureFrozen associates a previously frozen source tree with one published
// image ref without reading the source directory again.
func (s Store) CaptureFrozen(ctx context.Context, ref, imageDigest, sourceName string, frozen FrozenSource, provenance imagesource.Provenance) (Snapshot, error) {
	return s.captureFrozen(ctx, s.DB, ref, imageDigest, sourceName, frozen, provenance)
}

// CaptureFrozenTx writes frozen source metadata into an existing transaction.
func (s Store) CaptureFrozenTx(ctx context.Context, tx *sql.Tx, ref, imageDigest, sourceName string, frozen FrozenSource, provenance imagesource.Provenance) (Snapshot, error) {
	return s.captureFrozen(ctx, tx, ref, imageDigest, sourceName, frozen, provenance)
}

func (s Store) captureFrozen(ctx context.Context, exec executor, ref, imageDigest, sourceName string, frozen FrozenSource, provenance imagesource.Provenance) (Snapshot, error) {
	if exec == nil || s.Root == "" || ref == "" || imageDigest == "" || sourceName == "" || frozen.SourceDigest == "" || frozen.RelativeDir == "" {
		return Snapshot{}, errors.New("image snapshot: incomplete capture request")
	}
	if _, err := s.OpenFrozen(frozen); err != nil {
		return Snapshot{}, err
	}
	now := time.Now()
	if s.Clock != nil {
		now = s.Clock()
	}
	snapshot := Snapshot{Ref: ref, ImageDigest: imageDigest, SourceName: sourceName, SourceDigest: frozen.SourceDigest, RelativeDir: frozen.RelativeDir, CreatedAt: now.UTC().Format(time.RFC3339Nano), RepositoryID: provenance.RepositoryID, GitCommit: provenance.GitCommit, LockDigest: provenance.LockDigest}
	_, err := exec.ExecContext(ctx, `INSERT INTO image_source_snapshots(image_ref,image_digest,source_name,source_digest,relative_dir,created_at,repository_id,git_commit,lock_digest) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(image_ref) DO UPDATE SET image_digest=excluded.image_digest,source_name=excluded.source_name,source_digest=excluded.source_digest,relative_dir=excluded.relative_dir,created_at=excluded.created_at,repository_id=excluded.repository_id,git_commit=excluded.git_commit,lock_digest=excluded.lock_digest`, snapshot.Ref, snapshot.ImageDigest, snapshot.SourceName, snapshot.SourceDigest, snapshot.RelativeDir, snapshot.CreatedAt, snapshot.RepositoryID, snapshot.GitCommit, snapshot.LockDigest)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (s Store) Lookup(ctx context.Context, ref string) (Snapshot, bool, error) {
	var out Snapshot
	err := s.DB.QueryRowContext(ctx, `SELECT image_ref,image_digest,source_name,source_digest,relative_dir,created_at,repository_id,git_commit,lock_digest FROM image_source_snapshots WHERE image_ref=?`, ref).Scan(&out.Ref, &out.ImageDigest, &out.SourceName, &out.SourceDigest, &out.RelativeDir, &out.CreatedAt, &out.RepositoryID, &out.GitCommit, &out.LockDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return Snapshot{}, false, nil
	}
	return out, err == nil, err
}

func (s Store) LookupDigest(ctx context.Context, digest string) (Snapshot, bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT image_ref,image_digest,source_name,source_digest,relative_dir,created_at,repository_id,git_commit,lock_digest FROM image_source_snapshots WHERE image_digest=? ORDER BY created_at,image_ref`, digest)
	if err != nil {
		return Snapshot{}, false, err
	}
	defer rows.Close()
	var first Snapshot
	found := false
	for rows.Next() {
		var current Snapshot
		if err := rows.Scan(&current.Ref, &current.ImageDigest, &current.SourceName, &current.SourceDigest, &current.RelativeDir, &current.CreatedAt, &current.RepositoryID, &current.GitCommit, &current.LockDigest); err != nil {
			return Snapshot{}, false, err
		}
		if !found {
			first, found = current, true
			continue
		}
		if current.SourceDigest != first.SourceDigest || current.RepositoryID != first.RepositoryID || current.GitCommit != first.GitCommit || current.LockDigest != first.LockDigest {
			return Snapshot{}, false, fmt.Errorf("image snapshot: conflicting provenance for digest %s", digest)
		}
	}
	if err := rows.Err(); err != nil {
		return Snapshot{}, false, err
	}
	return first, found, nil
}

func (s Store) Open(snapshot Snapshot) (string, error) {
	if snapshot.RelativeDir == "" || filepath.Base(snapshot.RelativeDir) != snapshot.RelativeDir {
		return "", errors.New("image snapshot: invalid relative directory")
	}
	path := filepath.Join(s.Root, snapshot.RelativeDir)
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("image snapshot: unavailable")
	}
	return path, nil
}
