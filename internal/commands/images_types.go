package commands

import (
	"encoding/json"

	"github.com/alekzonder/tariboy/internal/agentskills"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagesource"
	"github.com/invopop/jsonschema"
)

// Response types of the image.go and image_source.go commands. Each command
// declares its type with registry.WithResult, which derives the OpenAPI result
// schema from it. Types of other packages are copied under an Image prefix, so
// their component names cannot collide with another domain's.

// ImageBuilt is one ref an image build published.
type ImageBuilt struct {
	Name   string `json:"name"`
	Tag    string `json:"tag"`
	Digest string `json:"digest"`
	Layers int    `json:"layers"`
}

// ImageBuildResult is the result of image.build. A build that publishes one
// ref, or the default tags, returns that ref's keys; a build with several
// explicit tags returns only images, one entry per tag.
type ImageBuildResult struct {
	Name   string       `json:"name,omitempty"`
	Tag    string       `json:"tag,omitempty"`
	Digest string       `json:"digest,omitempty"`
	Layers *int         `json:"layers,omitempty"`
	Images []ImageBuilt `json:"images,omitempty"`
}

// ImageDiagnostic is one validation problem or warning and the source
// location it refers to.
type ImageDiagnostic struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ImageSkill is the metadata of one skill packed into an image.
type ImageSkill agentskills.Metadata

// ImageTemplateEntry is one runtime or file entry of a prompt template.
type ImageTemplateEntry image.TemplateEntry

// ImagePromptTemplate is the ordered prompt template of an image
// (image.template).
type ImagePromptTemplate struct {
	SchemaVersion int                  `json:"schema_version"`
	Entries       []ImageTemplateEntry `json:"entries"`
	SHA256        string               `json:"sha256"`
}

// ImageValidateTemplate is the resolved prompt template of a valid source and
// null for an invalid one.
type ImageValidateTemplate struct {
	*ImagePromptTemplate
}

func (t ImageValidateTemplate) MarshalJSON() ([]byte, error) {
	if t.ImagePromptTemplate == nil {
		return []byte("null"), nil
	}
	return json.Marshal(t.ImagePromptTemplate)
}

func (ImageValidateTemplate) JSONSchemaExtend(s *jsonschema.Schema) {
	s.Extras = map[string]any{"nullable": true}
}

// ImageValidateResult is the result of image.validate.
type ImageValidateResult struct {
	Valid         bool                  `json:"valid"`
	SchemaVersion int                   `json:"schema_version"`
	Plugins       []string              `json:"plugins"`
	Skills        []ImageSkill          `json:"skills"`
	Template      ImageValidateTemplate `json:"template"`
	Diagnostics   []ImageDiagnostic     `json:"diagnostics"`
	Warnings      []ImageDiagnostic     `json:"warnings"`
}

// ImageNullableString is a string that encodes as null when not Valid.
type ImageNullableString struct {
	Value string
	Valid bool
}

func (s ImageNullableString) MarshalJSON() ([]byte, error) {
	if !s.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(s.Value)
}

func (ImageNullableString) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Extras: map[string]any{"nullable": true}}
}

// ImageProvenance is the local source provenance of an image ref
// (image.provenance). source_cwd is null when no provenance matches the
// current build; the snapshot keys appear only when a source snapshot of the
// build exists.
type ImageProvenance struct {
	Ref             string              `json:"ref"`
	Digest          *string             `json:"digest,omitempty"`
	SourceCWD       ImageNullableString `json:"source_cwd"`
	BuiltAt         *string             `json:"built_at,omitempty"`
	SourceAvailable bool                `json:"source_available"`
	SourceName      *string             `json:"source_name,omitempty"`
	SourceDigest    *string             `json:"source_digest,omitempty"`
	RepositoryID    *string             `json:"repository_id,omitempty"`
	GitCommit       *string             `json:"git_commit,omitempty"`
	LockDigest      *string             `json:"lock_digest,omitempty"`
}

// ImageSummary is one built image ref in the image.ls list. The source keys
// appear only when the image's source, provenance or snapshot is known.
type ImageSummary struct {
	SchemaVersion   int      `json:"schema_version"`
	Name            string   `json:"name"`
	Tag             string   `json:"tag"`
	Digest          string   `json:"digest"`
	BuiltAt         string   `json:"built_at"`
	Bare            bool     `json:"bare"`
	CurrentAgents   []string `json:"current_agents" jsonschema_extras:"nullable=true"`
	PendingAgents   []string `json:"pending_agents" jsonschema_extras:"nullable=true"`
	ImageVersion    string   `json:"image_version,omitempty"`
	Source          string   `json:"source,omitempty"`
	SourceCWD       *string  `json:"source_cwd,omitempty"`
	SourceAvailable *bool    `json:"source_available,omitempty"`
	Exportable      bool     `json:"exportable"`
	SourceDigest    *string  `json:"source_digest,omitempty"`
}

// ImageListResult is the result of image.ls.
type ImageListResult struct {
	Images []ImageSummary `json:"images"`
	Count  int            `json:"count"`
}

// ImageManifestPlugin is one plugin an image manifest names.
type ImageManifestPlugin image.ManifestPlugin

// ImageManifestHarness is the harness of a schema v1 manifest.
type ImageManifestHarness image.ManifestHarness

// ImageManifestPolicy is the tool policy of a schema v1 manifest.
type ImageManifestPolicy image.ManifestPolicy

// ImageLayer is one content-addressed prompt layer of a schema v1 manifest.
type ImageLayer image.Layer

// ImageManifest is an image's manifest (image.inspect). The schema v1 fields
// are null in a schema v2 manifest.
type ImageManifest struct {
	SchemaVersion        int                   `json:"schema_version"`
	ImageVersion         string                `json:"image_version,omitempty"`
	ID                   string                `json:"id,omitempty"`
	Name                 string                `json:"name"`
	Tag                  string                `json:"tag,omitempty"`
	Digest               string                `json:"digest,omitempty"`
	BuiltAt              string                `json:"built_at"`
	Parents              []string              `json:"parents" jsonschema_extras:"nullable=true"`
	Plugins              []ImageManifestPlugin `json:"plugins" jsonschema_extras:"nullable=true"`
	Skills               []ImageSkill          `json:"skills,omitempty"`
	RequiresSecrets      []string              `json:"requires_secrets" jsonschema_extras:"nullable=true"`
	Harness              ImageManifestHarness  `json:"harness"`
	Env                  map[string]string     `json:"env" jsonschema_extras:"nullable=true"`
	Policy               ImageManifestPolicy   `json:"policy"`
	Layers               []ImageLayer          `json:"layers" jsonschema_extras:"nullable=true"`
	PromptTemplateSHA256 string                `json:"prompt_template_sha256,omitempty"`
	Bare                 bool                  `json:"bare,omitempty"`
}

// ImagePromptResult is an image's assembled prompt (image.prompt).
type ImagePromptResult struct {
	Prompt string `json:"prompt"`
}

// ImageFileEntry is one member of an image archive.
type ImageFileEntry image.FileEntry

// ImageFilesResult is the result of image.files.
type ImageFilesResult struct {
	Files []ImageFileEntry `json:"files" jsonschema_extras:"nullable=true"`
	Count int              `json:"count"`
}

// ImageFileContent is a file's path and UTF-8 content (image.file and
// image.source.file.get).
type ImageFileContent struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ImageRemoveResult names what image.rm or image.source.rm removed.
type ImageRemoveResult struct {
	Removed string `json:"removed"`
}

// ImageSourceBuildRecord is the last build of an editable image source.
type ImageSourceBuildRecord imagesource.BuildRecord

// ImageSourceProvenance is the Git provenance of an editable image source.
type ImageSourceProvenance imagesource.Provenance

// ImageSource is an editable image source's metadata.
type ImageSource struct {
	SchemaVersion int                     `json:"schema_version"`
	Name          string                  `json:"name"`
	CreatedAt     string                  `json:"created_at"`
	UpdatedAt     string                  `json:"updated_at"`
	LastBuild     *ImageSourceBuildRecord `json:"last_build,omitempty"`
	Provenance    ImageSourceProvenance   `json:"provenance,omitempty"`
}

// ImageSourceListResult is the result of image.source.ls.
type ImageSourceListResult struct {
	Sources []ImageSource `json:"sources"`
	Count   int           `json:"count"`
}

// ImageSourceFileEntry is one editable file of an image source.
type ImageSourceFileEntry imagesource.FileEntry

// ImageSourceFilesResult is the result of image.source.files.
type ImageSourceFilesResult struct {
	Files []ImageSourceFileEntry `json:"files" jsonschema_extras:"nullable=true"`
	Count int                    `json:"count"`
}

// ImageSourceSaveResult is the result of image.source.file.put.
type ImageSourceSaveResult struct {
	Path  string `json:"path"`
	Saved bool   `json:"saved"`
}

// ImageSourceValidateResult is the result of image.source.validate.
type ImageSourceValidateResult struct {
	Valid       bool              `json:"valid"`
	Diagnostics []ImageDiagnostic `json:"diagnostics"`
}

// ImageSourceBuildResult is the result of image.source.build.
type ImageSourceBuildResult struct {
	Ref     string `json:"ref"`
	Digest  string `json:"digest"`
	BuiltAt string `json:"built_at"`
	Layers  int    `json:"layers"`
}

// convertSlice maps in through convert and keeps a nil slice nil, so a
// field that encoded null still does.
func convertSlice[T, U any](in []T, convert func(T) U) []U {
	if in == nil {
		return nil
	}
	out := make([]U, len(in))
	for i, item := range in {
		out[i] = convert(item)
	}
	return out
}

func imageSkills(in []agentskills.Metadata) []ImageSkill {
	return convertSlice(in, func(m agentskills.Metadata) ImageSkill { return ImageSkill(m) })
}

func imagePromptTemplate(t image.PromptTemplate) ImagePromptTemplate {
	return ImagePromptTemplate{
		SchemaVersion: t.SchemaVersion,
		Entries:       convertSlice(t.Entries, func(e image.TemplateEntry) ImageTemplateEntry { return ImageTemplateEntry(e) }),
		SHA256:        t.SHA256,
	}
}

func imageManifest(m image.Manifest) ImageManifest {
	return ImageManifest{
		SchemaVersion: m.SchemaVersion, ImageVersion: m.ImageVersion, ID: m.ID,
		Name: m.Name, Tag: m.Tag, Digest: m.Digest, BuiltAt: m.BuiltAt,
		Parents:              m.Parents,
		Plugins:              convertSlice(m.Plugins, func(p image.ManifestPlugin) ImageManifestPlugin { return ImageManifestPlugin(p) }),
		Skills:               imageSkills(m.Skills),
		RequiresSecrets:      m.RequiresSecrets,
		Harness:              ImageManifestHarness(m.Harness),
		Env:                  m.Env,
		Policy:               ImageManifestPolicy(m.Policy),
		Layers:               convertSlice(m.Layers, func(l image.Layer) ImageLayer { return ImageLayer(l) }),
		PromptTemplateSHA256: m.PromptTemplateSHA256,
		Bare:                 m.Bare,
	}
}

func imageSourceView(s imagesource.Source) ImageSource {
	return ImageSource{
		SchemaVersion: s.SchemaVersion, Name: s.Name, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		LastBuild:  (*ImageSourceBuildRecord)(s.LastBuild),
		Provenance: ImageSourceProvenance(s.Provenance),
	}
}
