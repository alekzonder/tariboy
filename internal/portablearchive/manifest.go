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

// ComposeMemberBytes caps one image or workflow image archive inside a compose
// archive: the daemon's image and workflow import routes accept no more.
const ComposeMemberBytes = 64 << 20

// ComposeLimits are the limits of a compose archive. It holds the compose file
// and already gzipped image archives, so the expanded size barely exceeds the
// compressed one: 1 GiB fits 16 images of the 64 MiB member limit.
func ComposeLimits() Limits {
	return Limits{MaxCompressedBytes: 1 << 30, MaxExpandedBytes: 1 << 30, MaxFiles: 512, MaxPathBytes: 512}
}
