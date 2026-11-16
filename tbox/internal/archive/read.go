package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alekzonder/tariboy/tbox/internal/skill"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

// Limits of internal/image portable_validate.go.
const (
	maxCompressed  = int64(64 << 20)
	maxPathBytes   = 512
	maxMembers     = 4096
	maxExpanded    = int64(256 << 20)
	roleLimitChars = 500
)

var (
	pluginNamePattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	imageVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+)(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)
	// runtimeNames are the runtime placeholders of internal/imagecontract.
	runtimeNames    = []string{"identity", "goal", "workdir", "context", "messages", "awaiting-replies", "user-prompt", "one-shot"}
	skillCategories = []string{"current-store", "store", "plugin", "source", "absolute"}
)

// strictManifest is the schema of manifest.json in a v2 archive, decoded
// with unknown fields rejected (internal/image v2PortableManifest).
type strictManifest struct {
	SchemaVersion        int              `json:"schema_version"`
	ImageVersion         string           `json:"image_version,omitempty"`
	ID                   string           `json:"id,omitempty"`
	Name                 string           `json:"name"`
	Tag                  string           `json:"tag,omitempty"`
	BuiltAt              string           `json:"built_at"`
	Plugins              []Plugin         `json:"plugins"`
	Skills               []skill.Metadata `json:"skills"`
	PromptTemplateSHA256 string           `json:"prompt_template_sha256"`
	Role                 *tfile.Role      `json:"role,omitempty"`
}

// Read reads and validates a schema-v2 archive with the rules tariboy applies
// to a portable image: member types, modes, paths and limits, a strict
// manifest and template, layer and skill integrity, no unexpected member, and
// an id equal to the recomputed content digest. Plugins and runtime entries
// are accepted.
func Read(r io.Reader) (Image, error) {
	compressed, err := io.ReadAll(io.LimitReader(r, maxCompressed+1))
	if err != nil {
		return Image{}, err
	}
	if int64(len(compressed)) > maxCompressed {
		return Image{}, errors.New("image archive exceeds compressed size limit")
	}
	members, err := readMembers(compressed)
	if err != nil {
		return Image{}, err
	}
	byName := make(map[string]Member, len(members))
	for _, m := range members {
		byName[m.Name] = m
	}
	man, err := readManifest(byName)
	if err != nil {
		return Image{}, err
	}
	template, err := readTemplate(byName, man.PromptTemplateSHA256)
	if err != nil {
		return Image{}, err
	}
	if err := checkMembers(man, template, byName); err != nil {
		return Image{}, err
	}
	if err := checkID(man, members); err != nil {
		return Image{}, err
	}
	return Image{Manifest: man, Template: template, Members: members}, nil
}

func readMembers(compressed []byte) ([]Member, error) {
	gz, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var members []Member
	seen := map[string]bool{}
	var expanded int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return members, nil
		}
		if err != nil {
			return nil, err
		}
		if err := checkHeader(h, len(members), &expanded); err != nil {
			return nil, err
		}
		if seen[h.Name] {
			return nil, fmt.Errorf("duplicate image archive member %q", h.Name)
		}
		seen[h.Name] = true
		body, err := io.ReadAll(io.LimitReader(tr, h.Size+1))
		if err != nil {
			return nil, err
		}
		if int64(len(body)) != h.Size {
			return nil, fmt.Errorf("image archive member %q size mismatch", h.Name)
		}
		members = append(members, Member{Name: h.Name, Mode: h.Mode & 0o7777, Body: body})
	}
}

// checkHeader applies the image member rules to the member after count
// earlier ones and adds its size to expanded.
func checkHeader(h *tar.Header, count int, expanded *int64) error {
	if count+1 > maxMembers {
		return errors.New("image archive has too many files")
	}
	if h.Typeflag != tar.TypeReg {
		return fmt.Errorf("invalid image archive member %q: regular files only", h.Name)
	}
	if mode := h.Mode & 0o7777; mode != modeFile && mode != modeExecutable {
		return fmt.Errorf("invalid image archive member %q mode %#o", h.Name, mode)
	}
	if len(h.Name) == 0 || len(h.Name) > maxPathBytes || !cleanRel(h.Name) {
		return fmt.Errorf("invalid image archive member path %q", h.Name)
	}
	if h.Size < 0 || *expanded > maxExpanded-h.Size {
		return errors.New("image archive exceeds expanded size limit")
	}
	*expanded += h.Size
	return nil
}

// cleanRel is internal/safefs CleanRel as a predicate.
func cleanRel(name string) bool {
	if name == "" || strings.IndexByte(name, 0) >= 0 || strings.ContainsRune(name, '\\') || path.IsAbs(name) {
		return false
	}
	if path.Clean(name) != name || name == "." {
		return false
	}
	return !slices.Contains(strings.Split(name, "/"), "..")
}

func decodeStrict(body []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("multiple JSON values: trailing data")
	}
	return nil
}

func readManifest(members map[string]Member) (Manifest, error) {
	m, ok := members[manifestName]
	if !ok {
		return Manifest{}, errors.New("image manifest is missing")
	}
	if m.Mode != modeFile {
		return Manifest{}, errors.New("image manifest has invalid mode")
	}
	var strict strictManifest
	if err := decodeStrict(m.Body, &strict); err != nil {
		return Manifest{}, fmt.Errorf("invalid schema-v2 manifest: %w", err)
	}
	if err := checkManifestFields(strict, m.Body); err != nil {
		return Manifest{}, err
	}
	man := Manifest{
		SchemaVersion:        strict.SchemaVersion,
		ImageVersion:         strict.ImageVersion,
		ID:                   strict.ID,
		Name:                 strict.Name,
		BuiltAt:              strict.BuiltAt,
		Plugins:              append([]Plugin{}, strict.Plugins...),
		Skills:               append([]skill.Metadata{}, strict.Skills...),
		PromptTemplateSHA256: strict.PromptTemplateSHA256,
		Role:                 strict.Role,
		Digest:               strict.ID,
	}
	return man, nil
}

// checkManifestFields is internal/image validateV2ManifestFields without the
// ref: the name only has to be present.
func checkManifestFields(m strictManifest, body []byte) error {
	if m.SchemaVersion != 2 || m.Name == "" || m.PromptTemplateSHA256 == "" {
		return errors.New("invalid schema-v2 manifest identity")
	}
	if m.ImageVersion != "" && !validSemVer(m.ImageVersion) {
		return fmt.Errorf("image_version must be a SemVer version (e.g. 1.2.3), got %q", m.ImageVersion)
	}
	if _, err := time.Parse(time.RFC3339Nano, m.BuiltAt); err != nil {
		return errors.New("invalid schema-v2 manifest built_at")
	}
	if err := checkRole(m.Role, body); err != nil {
		return err
	}
	seen := make(map[string]bool, len(m.Plugins))
	for _, p := range m.Plugins {
		if !pluginNamePattern.MatchString(p.Name) || seen[p.Name] {
			return fmt.Errorf("invalid or duplicate schema-v2 plugin name %q", p.Name)
		}
		seen[p.Name] = true
	}
	return nil
}

// validSemVer is the image_version rule of internal/imagefile.
func validSemVer(value string) bool {
	match := imageVersionPattern.FindStringSubmatch(value)
	if match == nil {
		return false
	}
	for _, part := range strings.Split(strings.TrimPrefix(match[4], "-"), ".") {
		if len(part) > 1 && part[0] == '0' && strings.Trim(part, "0123456789") == "" {
			return false
		}
	}
	return true
}

// checkRole is internal/rolefile Validate on the manifest role. The JSON
// decoder replaces invalid UTF-8, so the raw manifest is checked too.
func checkRole(role *tfile.Role, body []byte) error {
	if role == nil {
		return nil
	}
	switch {
	case !utf8.Valid(body):
		return errors.New("invalid schema-v2 manifest role: manifest is not valid UTF-8")
	case strings.TrimSpace(role.Text) == "":
		return errors.New("invalid schema-v2 manifest role: text is empty")
	}
	if n := utf8.RuneCountInString(role.Text); n > roleLimitChars {
		return fmt.Errorf("invalid schema-v2 manifest role: text is %d characters, more than the limit of %d characters", n, roleLimitChars)
	}
	return nil
}

func readTemplate(members map[string]Member, wantSHA256 string) (Template, error) {
	m, ok := members[templateName]
	if !ok {
		return Template{}, errors.New("schema-v2 prompt template is missing")
	}
	if m.Mode != modeFile {
		return Template{}, errors.New("schema-v2 prompt template has invalid mode")
	}
	var template Template
	if err := decodeStrict(m.Body, &template); err != nil {
		return Template{}, fmt.Errorf("invalid schema-v2 prompt template: %w", err)
	}
	if err := validateTemplate(template); err != nil {
		return Template{}, err
	}
	if template.SHA256 != wantSHA256 {
		return Template{}, errors.New("manifest prompt template digest mismatch")
	}
	return template, nil
}

// validateTemplate is internal/image ValidatePromptTemplate.
func validateTemplate(template Template) error {
	if template.SchemaVersion != 2 {
		return fmt.Errorf("unsupported prompt template schema_version %d", template.SchemaVersion)
	}
	if len(template.Entries) > maxPromptTemplateEntries {
		return fmt.Errorf("prompt template has %d entries, maximum is %d", len(template.Entries), maxPromptTemplateEntries)
	}
	want, err := templateHash(template.Entries)
	if err != nil {
		return err
	}
	if template.SHA256 != want {
		return errors.New("prompt template digest mismatch")
	}
	seen := map[string]bool{}
	for i, entry := range template.Entries {
		if err := validateEntry(i, entry, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateEntry(i int, entry TemplateEntry, seen map[string]bool) error {
	switch entry.Kind {
	case "runtime":
		if !slices.Contains(runtimeNames, entry.Runtime) || entry.ArchivePath != "" || seen["runtime:"+entry.Runtime] {
			return fmt.Errorf("invalid runtime template entry %d", i)
		}
		seen["runtime:"+entry.Runtime] = true
	case "file":
		clean := path.Clean(entry.ArchivePath)
		if entry.Runtime != "" || clean != entry.ArchivePath || !strings.HasPrefix(clean, layersPrefix) || seen[clean] || entry.Size < 0 || len(entry.SHA256) != 64 {
			return fmt.Errorf("invalid file template entry %d", i)
		}
		seen[clean] = true
	default:
		return fmt.Errorf("invalid template entry %d kind %q", i, entry.Kind)
	}
	return nil
}

// checkMembers verifies the prompt layers and skills and rejects any member
// neither of them nor the manifest and template claim.
func checkMembers(man Manifest, template Template, members map[string]Member) error {
	want := map[string]bool{manifestName: true, templateName: true}
	if err := checkLayers(template, members, want); err != nil {
		return err
	}
	if err := checkSkills(man.Skills, members, want); err != nil {
		return err
	}
	for name := range members {
		if !want[name] {
			return fmt.Errorf("unexpected schema-v2 image member %q", name)
		}
	}
	return nil
}

func checkLayers(template Template, members map[string]Member, want map[string]bool) error {
	var total int64
	for i, entry := range template.Entries {
		if entry.Kind != "file" {
			continue
		}
		m, ok := members[entry.ArchivePath]
		if !ok {
			return fmt.Errorf("template entry %d layer is missing", i)
		}
		if m.Mode != modeFile || int64(len(m.Body)) != entry.Size || sha256hex(m.Body) != entry.SHA256 {
			return fmt.Errorf("template entry %d layer integrity mismatch", i)
		}
		if total > maxPromptBytes-entry.Size {
			return fmt.Errorf("prompt layers exceed %d aggregate bytes", maxPromptBytes)
		}
		total += entry.Size
		want[entry.ArchivePath] = true
	}
	return nil
}

// checkSkills rebuilds every manifest skill from its members and requires the
// rebuilt metadata to equal the manifest's.
func checkSkills(skills []skill.Metadata, members map[string]Member, want map[string]bool) error {
	tmp, err := os.MkdirTemp("", "tbox-archive-skills-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	prepared := make([]skill.Prepared, 0, len(skills))
	roots := make(map[string]bool, len(skills))
	for i, meta := range skills {
		if meta.Source == "" || !slices.Contains(skillCategories, meta.Category) || !validSkillName(meta.Name) || meta.ArchiveRoot != skillsPrefix+meta.Name || roots[meta.ArchiveRoot] {
			return fmt.Errorf("invalid schema-v2 skill metadata at index %d", i)
		}
		roots[meta.ArchiveRoot] = true
		p, err := rebuildSkill(tmp, meta, members, want)
		if err != nil {
			return fmt.Errorf("invalid schema-v2 skill %q: %w", meta.Name, err)
		}
		prepared = append(prepared, p)
	}
	return skill.ValidateSet(prepared)
}

func validSkillName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\\x00")
}

// rebuildSkill writes the skill's members under tmp/<name> and prepares them
// with skill.Prepare, which applies the frontmatter, size and tree-hash rules
// of agentskills.ValidatePrepared. The result must equal meta.
func rebuildSkill(tmp string, meta skill.Metadata, members map[string]Member, want map[string]bool) (skill.Prepared, error) {
	dir := filepath.Join(tmp, meta.Name)
	if err := os.Mkdir(dir, 0o700); err != nil {
		return skill.Prepared{}, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return skill.Prepared{}, err
	}
	defer root.Close()
	prefix := meta.ArchiveRoot + "/"
	for name, m := range members {
		rel, ok := strings.CutPrefix(name, prefix)
		if !ok {
			continue
		}
		if err := writeRooted(root, rel, m); err != nil {
			return skill.Prepared{}, err
		}
		want[name] = true
	}
	p, err := skill.Prepare(dir, meta.Source, meta.Category)
	if err != nil {
		return skill.Prepared{}, err
	}
	got := p.Metadata
	got.ClientVersion = meta.ClientVersion
	if got != meta {
		return skill.Prepared{}, errors.New("metadata does not match members")
	}
	p.Metadata = meta
	return p, nil
}

func writeRooted(root *os.Root, rel string, m Member) error {
	if parent := path.Dir(rel); parent != "." {
		if err := root.MkdirAll(parent, 0o700); err != nil {
			return err
		}
	}
	f, err := root.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(m.Mode))
	if err != nil {
		return err
	}
	if _, err := f.Write(m.Body); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(os.FileMode(m.Mode)); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// checkID requires the manifest id to be the content digest of the members.
func checkID(man Manifest, members []Member) error {
	if man.ID == "" {
		return errors.New("image manifest id is missing")
	}
	digest, err := ContentDigest(members)
	if err != nil {
		return err
	}
	if digest != man.ID {
		return errors.New("image manifest id does not match content digest")
	}
	return nil
}
