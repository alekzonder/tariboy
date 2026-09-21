package image

const ManifestSchemaVersion = 2

type ManifestPlugin struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type ManifestSkill struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Source        string `json:"source"`
	Category      string `json:"category"`
	ClientVersion string `json:"client_version,omitempty"`
	ArchiveRoot   string `json:"archive_root"`
	FileCount     int    `json:"file_count"`
	Size          int64  `json:"size"`
	TreeSHA256    string `json:"tree_sha256"`
}

type ManifestHarness struct {
	Type        string `json:"type"`
	Model       string `json:"model,omitempty"`
	Effort      string `json:"effort,omitempty"`
	Interactive bool   `json:"interactive"`
}

type ManifestPolicy struct {
	ToolsAllow []string `json:"tools_allow,omitempty"`
	ToolsDeny  []string `json:"tools_deny,omitempty"`
}

// Layer records one content-addressed piece of the prompt (system, each body
// prompt file, tail).
type Layer struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// Manifest is the versioned metadata of an image. ID is the ref identity
// derived from name and image_version at build time and stored in the archive;
// an image that declares no version leaves it empty and stays addressed by its
// archive bytes. Tag is not part of the archive: a tag only points at a ref, so
// Tag is filled from the ref the caller asked for. Digest carries the resolved
// ref id for every consumer that pins an image.
type Manifest struct {
	SchemaVersion        int               `json:"schema_version"`
	ImageVersion         string            `json:"image_version,omitempty"`
	ID                   string            `json:"id,omitempty"`
	Name                 string            `json:"name"`
	Tag                  string            `json:"tag,omitempty"`
	Digest               string            `json:"digest,omitempty"`
	BuiltAt              string            `json:"built_at"`
	Parents              []string          `json:"parents"`
	Plugins              []ManifestPlugin  `json:"plugins"`
	Skills               []ManifestSkill   `json:"skills,omitempty"`
	RequiresSecrets      []string          `json:"requires_secrets"`
	Harness              ManifestHarness   `json:"harness"`
	Env                  map[string]string `json:"env"`
	Policy               ManifestPolicy    `json:"policy"`
	Layers               []Layer           `json:"layers"`
	PromptTemplateSHA256 string            `json:"prompt_template_sha256,omitempty"`
	// Bare marks an instructions-free image: the runner launches the harness
	// with no assembled prompt, no bin shims on PATH and no i-am-done tooling.
	Bare bool `json:"bare,omitempty"`
}
