package image

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/agentskills"
)

var portablePluginName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

const (
	maxPortableImageCompressed = int64(64 << 20)
	maxPortableImageExpanded   = int64(256 << 20)
	maxPortableImageFiles      = 4096
	maxPortableImagePathBytes  = 512
)

type v2PortableManifest struct {
	SchemaVersion        int                `json:"schema_version"`
	Name                 string             `json:"name"`
	Tag                  string             `json:"tag"`
	BuiltAt              string             `json:"built_at"`
	Plugins              []v2PortablePlugin `json:"plugins"`
	Skills               []ManifestSkill    `json:"skills"`
	PromptTemplateSHA256 string             `json:"prompt_template_sha256"`
}

type v2PortablePlugin struct {
	Name string `json:"name"`
}

type portableImageMember struct {
	body []byte
	mode int64
}

// validatePortableArchive validates the runnable inner archive before it can
// enter the immutable Store or be unpacked into an agent directory.
func validatePortableArchive(archive []byte, ref Ref) (Manifest, error) {
	if int64(len(archive)) > maxPortableImageCompressed {
		return Manifest{}, errors.New("image archive exceeds compressed size limit")
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return Manifest{}, err
	}
	defer gz.Close()

	members := make(map[string]portableImageMember)
	tr := tar.NewReader(gz)
	var expanded int64
	for count := 0; ; count++ {
		h, nextErr := tr.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return Manifest{}, nextErr
		}
		if count >= maxPortableImageFiles {
			return Manifest{}, errors.New("image archive has too many files")
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return Manifest{}, fmt.Errorf("invalid image archive member %q: regular files only", h.Name)
		}
		mode := h.Mode & 0o7777
		if mode != 0o600 && mode != 0o700 {
			return Manifest{}, fmt.Errorf("invalid image archive member %q mode %#o", h.Name, mode)
		}
		if len(h.Name) == 0 || len(h.Name) > maxPortableImagePathBytes {
			return Manifest{}, fmt.Errorf("invalid image archive member path %q", h.Name)
		}
		clean, cleanErr := cleanTarPath(h.Name)
		if cleanErr != nil || clean != path.Clean(h.Name) || strings.HasPrefix(clean, "../") {
			return Manifest{}, fmt.Errorf("invalid image archive member path %q", h.Name)
		}
		if _, duplicate := members[clean]; duplicate {
			return Manifest{}, fmt.Errorf("duplicate image archive member %q", clean)
		}
		if h.Size < 0 || expanded > maxPortableImageExpanded-h.Size {
			return Manifest{}, errors.New("image archive exceeds expanded size limit")
		}
		expanded += h.Size
		body, readErr := io.ReadAll(io.LimitReader(tr, h.Size+1))
		if readErr != nil {
			return Manifest{}, readErr
		}
		if int64(len(body)) != h.Size {
			return Manifest{}, fmt.Errorf("image archive member %q size mismatch", clean)
		}
		members[clean] = portableImageMember{body: body, mode: mode}
	}

	manifestMember, ok := members["manifest.json"]
	if !ok {
		return Manifest{}, errors.New("image manifest is missing")
	}
	if manifestMember.mode != 0o600 {
		return Manifest{}, errors.New("image manifest has invalid mode")
	}
	var manifest Manifest
	manifestBody := manifestMember.body
	if err := json.Unmarshal(manifestBody, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest of %s: %w", ref.String(), err)
	}
	if manifest.SchemaVersion != 1 && manifest.SchemaVersion != 2 {
		return Manifest{}, fmt.Errorf("unsupported image schema_version %d", manifest.SchemaVersion)
	}
	if manifest.Name != ref.Name || manifest.Tag != ref.Tag {
		return Manifest{}, fmt.Errorf("archive ref %s:%s does not match %s", manifest.Name, manifest.Tag, ref.String())
	}
	if manifest.SchemaVersion == 2 {
		if err := validateV2PortableMembers(manifestBody, members, ref, &manifest); err != nil {
			return Manifest{}, err
		}
	} else {
		for _, required := range []string{"PROMPT.md", "PROMPT_TAIL.md", "BODY.md"} {
			if _, ok := members[required]; !ok {
				return Manifest{}, fmt.Errorf("schema-v1 image member %q is missing", required)
			}
		}
		for name := range members {
			if name != "manifest.json" && name != "PROMPT.md" && name != "PROMPT_TAIL.md" && name != "BODY.md" && !strings.HasPrefix(name, "skills/") {
				return Manifest{}, fmt.Errorf("unexpected schema-v1 image member %q", name)
			}
		}
	}
	sum := sha256.Sum256(archive)
	manifest.Digest = hex.EncodeToString(sum[:])
	return manifest, nil
}

func ValidatePortableArchive(archive []byte, ref Ref) (Manifest, error) {
	return validatePortableArchive(archive, ref)
}

func validateV2PortableMembers(manifestBody []byte, members map[string]portableImageMember, ref Ref, manifest *Manifest) error {
	var strict v2PortableManifest
	dec := json.NewDecoder(bytes.NewReader(manifestBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&strict); err != nil {
		return fmt.Errorf("invalid schema-v2 manifest: %w", err)
	}
	if err := requireJSONEOF(dec); err != nil {
		return fmt.Errorf("invalid schema-v2 manifest: %w", err)
	}
	if strict.SchemaVersion != 2 || strict.Name != ref.Name || strict.Tag != ref.Tag || strict.PromptTemplateSHA256 == "" {
		return errors.New("invalid schema-v2 manifest identity")
	}
	if _, err := time.Parse(time.RFC3339, strict.BuiltAt); err != nil {
		return errors.New("invalid schema-v2 manifest built_at")
	}
	seenPlugins := make(map[string]bool, len(strict.Plugins))
	for _, plugin := range strict.Plugins {
		if !portablePluginName.MatchString(plugin.Name) || seenPlugins[plugin.Name] {
			return fmt.Errorf("invalid or duplicate schema-v2 plugin name %q", plugin.Name)
		}
		seenPlugins[plugin.Name] = true
	}
	templateMember, ok := members["prompt/template.json"]
	if !ok {
		return errors.New("schema-v2 prompt template is missing")
	}
	if templateMember.mode != 0o600 {
		return errors.New("schema-v2 prompt template has invalid mode")
	}
	var template PromptTemplate
	dec = json.NewDecoder(bytes.NewReader(templateMember.body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&template); err != nil {
		return fmt.Errorf("invalid schema-v2 prompt template: %w", err)
	}
	if err := requireJSONEOF(dec); err != nil {
		return fmt.Errorf("invalid schema-v2 prompt template: %w", err)
	}
	if err := ValidatePromptTemplate(template); err != nil {
		return err
	}
	if strict.PromptTemplateSHA256 != template.SHA256 {
		return errors.New("manifest prompt template digest mismatch")
	}
	want := map[string]bool{"manifest.json": true, "prompt/template.json": true}
	var totalPromptBytes int64
	for i, entry := range template.Entries {
		if entry.Kind != "file" {
			continue
		}
		if entry.ArchivePath == "" || !strings.HasPrefix(entry.ArchivePath, "prompt/layers/") {
			return fmt.Errorf("template entry %d has invalid layer path", i)
		}
		member, ok := members[entry.ArchivePath]
		if !ok {
			return fmt.Errorf("template entry %d layer is missing", i)
		}
		if member.mode != 0o600 || int64(len(member.body)) != entry.Size || sha256hex(member.body) != entry.SHA256 {
			return fmt.Errorf("template entry %d layer integrity mismatch", i)
		}
		if totalPromptBytes > maxV2PromptBytes-entry.Size {
			return fmt.Errorf("prompt layers exceed %d aggregate bytes", maxV2PromptBytes)
		}
		totalPromptBytes += entry.Size
		want[entry.ArchivePath] = true
	}
	preparedSkills := make([]agentskills.Prepared, 0, len(strict.Skills))
	seenSkillRoots := make(map[string]bool, len(strict.Skills))
	for i, skill := range strict.Skills {
		if skill.Source == "" || !validSkillCategory(skill.Category) || skill.ArchiveRoot != "skills/"+skill.Name || seenSkillRoots[skill.ArchiveRoot] {
			return fmt.Errorf("invalid schema-v2 skill metadata at index %d", i)
		}
		seenSkillRoots[skill.ArchiveRoot] = true
		prefix := skill.ArchiveRoot + "/"
		files := make([]agentskills.File, 0, skill.FileCount)
		for name, member := range members {
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			rel := strings.TrimPrefix(name, prefix)
			sum := sha256.Sum256(member.body)
			files = append(files, agentskills.File{
				RelativePath: rel,
				Size:         int64(len(member.body)),
				SHA256:       hex.EncodeToString(sum[:]),
				Executable:   member.mode == 0o700,
				Body:         member.body,
			})
			want[name] = true
		}
		sort.Slice(files, func(i, j int) bool { return files[i].RelativePath < files[j].RelativePath })
		prepared := agentskills.Prepared{Metadata: agentskills.Metadata{
			Name: skill.Name, Description: skill.Description, Source: skill.Source, Category: skill.Category,
			ArchiveRoot: skill.ArchiveRoot, FileCount: skill.FileCount, Size: skill.Size, TreeSHA256: skill.TreeSHA256,
		}, Files: files}
		if err := agentskills.ValidatePrepared(prepared); err != nil {
			return fmt.Errorf("invalid schema-v2 skill %q: %w", skill.Name, err)
		}
		preparedSkills = append(preparedSkills, prepared)
	}
	if err := agentskills.ValidateSet(preparedSkills); err != nil {
		return err
	}
	for name := range members {
		if !want[name] {
			return fmt.Errorf("unexpected schema-v2 image member %q", name)
		}
	}
	manifest.Plugins = make([]ManifestPlugin, 0, len(strict.Plugins))
	for _, plugin := range strict.Plugins {
		manifest.Plugins = append(manifest.Plugins, ManifestPlugin{Name: plugin.Name})
	}
	manifest.Skills = append([]ManifestSkill(nil), strict.Skills...)
	if manifest.Skills == nil {
		manifest.Skills = []ManifestSkill{}
	}
	return nil
}

func validSkillCategory(category string) bool {
	switch category {
	case "current-store", "store", "plugin", "source", "absolute":
		return true
	default:
		return false
	}
}

func requireJSONEOF(dec *json.Decoder) error {
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
