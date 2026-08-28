// Package imagesource manages editable image projects below the daemon's
// image-sources directory. Built images remain owned by package image.
package imagesource

import "errors"

const (
	MetadataFilename = ".tariboy-source.json"
	MaxFileSize      = 1 << 20
)

var (
	ErrInvalidName  = errors.New("invalid image source name")
	ErrExists       = errors.New("image source already exists")
	ErrNotFound     = errors.New("image source not found")
	ErrInvalidPath  = errors.New("invalid image source path")
	ErrInvalidUTF8  = errors.New("image source file is not valid UTF-8")
	ErrFileTooLarge = errors.New("image source file is too large")
	ErrUnsafeFile   = errors.New("unsafe image source file")
)

type CreateRequest struct {
	Name         string
	From         string
	Harness      string
	Model        string
	Effort       string
	Interactive  *bool
	Capabilities []string
	Prompt       string
	Provenance   Provenance
}

type Provenance struct {
	RepositoryID string `json:"repository_id,omitempty"`
	GitCommit    string `json:"git_commit,omitempty"`
	LockDigest   string `json:"lock_digest,omitempty"`
}

type BuildRecord struct {
	Ref     string `json:"ref"`
	Digest  string `json:"digest"`
	BuiltAt string `json:"built_at"`
}

type Source struct {
	SchemaVersion int          `json:"schema_version"`
	Name          string       `json:"name"`
	CreatedAt     string       `json:"created_at"`
	UpdatedAt     string       `json:"updated_at"`
	LastBuild     *BuildRecord `json:"last_build,omitempty"`
	Provenance    Provenance   `json:"provenance,omitempty"`
}

type FileEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}
