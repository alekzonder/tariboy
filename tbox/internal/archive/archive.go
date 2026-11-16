// Package archive writes and reads image archives in the tariboy schema-v2
// byte format (internal/image build_v2.go and portable_validate.go), so an
// archive tbox builds imports into tariboy and the reverse.
package archive

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/skill"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

const (
	manifestName = "manifest.json"
	templateName = "prompt/template.json"
	layersPrefix = "prompt/layers/"
	skillsPrefix = "skills/"

	// timeLayout is the built_at format of internal/store TimeLayout.
	timeLayout = "2006-01-02T15:04:05.000000000Z"

	maxPromptBytes           = int64(32 << 20)
	maxPromptTemplateEntries = 1024

	modeFile       = int64(0o600)
	modeExecutable = int64(0o700)
)

var archiveBaseCleaner = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// Member is one regular file of an archive. Mode is the permission bits,
// 0600 or 0700.
type Member struct {
	Name string
	Mode int64
	Body []byte
}

// TemplateEntry has the JSON of internal/image TemplateEntry.
type TemplateEntry struct {
	Kind        string `json:"kind"`
	Runtime     string `json:"runtime,omitempty"`
	Source      string `json:"source,omitempty"`
	Category    string `json:"category,omitempty"`
	ArchivePath string `json:"archive_path,omitempty"`
	Size        int64  `json:"size,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

// Template has the JSON of internal/image PromptTemplate.
type Template struct {
	SchemaVersion int             `json:"schema_version"`
	Entries       []TemplateEntry `json:"entries"`
	SHA256        string          `json:"sha256"`
}

// Plugin is a manifest plugin. tbox writes none and keeps the ones of
// archives exported from tariboy.
type Plugin struct {
	Name string `json:"name"`
}

// Manifest has the JSON of the manifest marshalV2Manifest writes. Digest and
// Tag are never written: Digest equals ID after Assemble and Read, Tag is
// for callers.
type Manifest struct {
	SchemaVersion        int              `json:"schema_version"`
	ImageVersion         string           `json:"image_version,omitempty"`
	ID                   string           `json:"id,omitempty"`
	Name                 string           `json:"name"`
	BuiltAt              string           `json:"built_at"`
	Plugins              []Plugin         `json:"plugins"`
	Skills               []skill.Metadata `json:"skills"`
	PromptTemplateSHA256 string           `json:"prompt_template_sha256"`
	Role                 *tfile.Role      `json:"role,omitempty"`
	Digest               string           `json:"-"`
	Tag                  string           `json:"-"`
}

// PromptLayer is one packaged prompt file. Base is the source file's base
// name.
type PromptLayer struct {
	Source   string
	Category string
	Base     string
	Body     []byte
}

// Image is an assembled or read archive. Members are in archive order and
// start with manifest.json and prompt/template.json.
type Image struct {
	Manifest Manifest
	Template Template
	Members  []Member
}

// Assemble builds the image the way BuildV2 does for the same skills, prompt
// files and role: same members, order, modes, layer names, template hash and
// content-digest id. The image has no plugins and no runtime entries.
func Assemble(name, imageVersion string, builtAt time.Time, skills []skill.Prepared, prompts []PromptLayer, role *tfile.Role) (Image, error) {
	if name == "" {
		return Image{}, errors.New("image name is required")
	}
	if err := skill.ValidateSet(skills); err != nil {
		return Image{}, err
	}
	entries, layers, err := promptEntries(prompts)
	if err != nil {
		return Image{}, err
	}
	hash, err := templateHash(entries)
	if err != nil {
		return Image{}, err
	}
	template := Template{SchemaVersion: 2, Entries: entries, SHA256: hash}
	templateJSON, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		return Image{}, err
	}
	man := Manifest{
		SchemaVersion:        2,
		ImageVersion:         imageVersion,
		Name:                 name,
		BuiltAt:              builtAt.UTC().Format(timeLayout),
		Plugins:              []Plugin{},
		Skills:               make([]skill.Metadata, 0, len(skills)),
		PromptTemplateSHA256: hash,
		Role:                 role,
	}
	members := []Member{{Name: manifestName, Mode: modeFile}, {Name: templateName, Mode: modeFile, Body: templateJSON}}
	members = append(members, layers...)
	for _, s := range skills {
		man.Skills = append(man.Skills, s.Metadata)
		members = append(members, skillMembers(s)...)
	}
	if err := sealManifest(&man, members); err != nil {
		return Image{}, err
	}
	return Image{Manifest: man, Template: template, Members: members}, nil
}

func promptEntries(prompts []PromptLayer) ([]TemplateEntry, []Member, error) {
	if len(prompts) > maxPromptTemplateEntries {
		return nil, nil, fmt.Errorf("prompts has %d entries, maximum is %d", len(prompts), maxPromptTemplateEntries)
	}
	entries := make([]TemplateEntry, 0, len(prompts))
	layers := make([]Member, 0, len(prompts))
	var total int64
	for i, p := range prompts {
		size := int64(len(p.Body))
		if total > maxPromptBytes-size {
			return nil, nil, fmt.Errorf("prompt layers exceed %d aggregate bytes", maxPromptBytes)
		}
		total += size
		path := fmt.Sprintf("%s%03d-%s", layersPrefix, i, archiveBaseCleaner.ReplaceAllString(p.Base, "-"))
		entries = append(entries, TemplateEntry{Kind: "file", Source: p.Source, Category: p.Category, ArchivePath: path, Size: size, SHA256: sha256hex(p.Body)})
		layers = append(layers, Member{Name: path, Mode: modeFile, Body: p.Body})
	}
	return entries, layers, nil
}

func skillMembers(s skill.Prepared) []Member {
	out := make([]Member, 0, len(s.Files))
	for _, file := range s.Files {
		mode := modeFile
		if file.Executable {
			mode = modeExecutable
		}
		out = append(out, Member{Name: s.Metadata.ArchiveRoot + "/" + file.RelativePath, Mode: mode, Body: file.Body})
	}
	return out
}

// sealManifest sets the id to the content digest of members, computed with a
// manifest that does not carry it yet, and stores the final manifest JSON in
// members[0].
func sealManifest(man *Manifest, members []Member) error {
	var err error
	man.ID = ""
	if members[0].Body, err = json.MarshalIndent(man, "", "  "); err != nil {
		return err
	}
	if man.ID, err = ContentDigest(members); err != nil {
		return err
	}
	if members[0].Body, err = json.MarshalIndent(man, "", "  "); err != nil {
		return err
	}
	man.Digest = man.ID
	return nil
}

// Write writes the image as a gzip tar with the header fields of
// internal/image writeTarFileMode.
func (img Image) Write(w io.Writer) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for _, m := range img.Members {
		if err := tw.WriteHeader(&tar.Header{Name: m.Name, Mode: m.Mode, Size: int64(len(m.Body)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		if _, err := tw.Write(m.Body); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// Skill returns the metadata of the named skill and its members, named
// relative to the skill root, in archive order.
func (img Image) Skill(name string) (skill.Metadata, []Member, bool) {
	for _, meta := range img.Manifest.Skills {
		if meta.Name != name {
			continue
		}
		prefix := meta.ArchiveRoot + "/"
		var files []Member
		for _, m := range img.Members {
			if rel, ok := strings.CutPrefix(m.Name, prefix); ok {
				files = append(files, Member{Name: rel, Mode: m.Mode, Body: m.Body})
			}
		}
		return meta, files, true
	}
	return skill.Metadata{}, nil, false
}
