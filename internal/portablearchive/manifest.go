package portablearchive

import "encoding/json"

type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Manifest struct {
	Format   string          `json:"format"`
	Version  int             `json:"version"`
	Kind     string          `json:"kind"`
	Files    []File          `json:"files"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

type Limits struct {
	MaxCompressedBytes int64
	MaxExpandedBytes   int64
	MaxFiles           int
	MaxPathBytes       int
}

func DefaultLimits() Limits {
	return Limits{MaxCompressedBytes: 64 << 20, MaxExpandedBytes: 256 << 20, MaxFiles: 4096, MaxPathBytes: 512}
}
