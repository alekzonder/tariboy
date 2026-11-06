package image

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/agentskills"
	"github.com/alekzonder/tariboy/internal/imagecontract"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/plugincaps"
	"github.com/alekzonder/tariboy/internal/rolefile"
	storetime "github.com/alekzonder/tariboy/internal/store"
)

var archiveBaseCleaner = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

const maxV2PromptBytes = int64(32 << 20)

type preparedV2 struct {
	plugins  []ManifestPlugin
	skills   []agentskills.Prepared
	entries  []TemplateEntry
	layers   map[string][]byte
	template PromptTemplate
}

func prepareV2(src *imagefile.V2, roots imagefile.ResolveRoots, resolver plugincaps.ExternalResolver) (preparedV2, error) {
	if src == nil || src.SchemaVersion != 2 {
		return preparedV2{}, fmt.Errorf("schema-v2 source is required")
	}
	if len(src.Extends) > 0 {
		return preparedV2{}, errors.New(imagefile.ExtendsUnassembledMessage)
	}
	plugins, err := prepareV2Plugins(src.Plugins, resolver)
	if err != nil {
		return preparedV2{}, err
	}
	skills, sourceSkills, err := prepareV2Skills(src, roots)
	if err != nil {
		return preparedV2{}, err
	}
	if err := agentskills.ValidateSet(skills); err != nil {
		return preparedV2{}, err
	}
	if err := validateV2Contract(plugins, skills, src.Prompts, resolver); err != nil {
		return preparedV2{}, err
	}
	promptRoots := roots
	promptRoots.SourceSkills = sourceSkills
	entries, layers, err := prepareV2Prompts(src, promptRoots)
	if err != nil {
		return preparedV2{}, err
	}
	if entries == nil {
		entries = []TemplateEntry{}
	}
	hash, err := templateHash(entries)
	if err != nil {
		return preparedV2{}, err
	}
	template := PromptTemplate{SchemaVersion: 2, Entries: entries, SHA256: hash}
	return preparedV2{plugins: plugins, skills: skills, entries: entries, layers: layers, template: template}, nil
}

func prepareV2Plugins(sources []imagefile.V2Plugin, resolver plugincaps.ExternalResolver) ([]ManifestPlugin, error) {
	plugins := make([]ManifestPlugin, 0, len(sources))
	for _, plugin := range sources {
		if _, err := plugincaps.ValidateExplicit([]string{plugin.Name}, resolver); err != nil {
			return nil, err
		}
		plugins = append(plugins, ManifestPlugin{Name: plugin.Name})
	}
	return plugins, nil
}

// prepareV2Skills loads each declared skill. It also returns the source skill
// roots prompts resolve against: roots.SourceSkills when set, left unchanged,
// otherwise the relative skill directories it just resolved.
func prepareV2Skills(src *imagefile.V2, roots imagefile.ResolveRoots) ([]agentskills.Prepared, map[string]string, error) {
	skills := make([]agentskills.Prepared, 0, len(src.Skills))
	sourceSkills := roots.SourceSkills
	if sourceSkills == nil {
		sourceSkills = make(map[string]string)
	}
	for i, skill := range src.Skills {
		resolved, err := imagefile.ResolveSkillDirectory(src.Dir, skill.Dir, roots)
		if err != nil {
			return nil, nil, fmt.Errorf("skills[%d]: %w", i, err)
		}
		prepared, err := agentskills.Prepare(resolved)
		if err != nil {
			return nil, nil, fmt.Errorf("skills[%d]: %w", i, err)
		}
		skills = append(skills, prepared)
		if roots.SourceSkills == nil && (strings.HasPrefix(skill.Dir, "./") || strings.HasPrefix(skill.Dir, "../")) {
			sourceSkills[skill.Dir] = resolved.Path
		}
	}
	return skills, sourceSkills, nil
}

func validateV2Contract(plugins []ManifestPlugin, skills []agentskills.Prepared, prompts []imagefile.PromptEntry, resolver plugincaps.ExternalResolver) error {
	contract := imagecontract.Input{Plugins: make([]string, 0, len(plugins)), Skills: make([]imagecontract.Skill, 0, len(skills))}
	for _, plugin := range plugins {
		contract.Plugins = append(contract.Plugins, plugin.Name)
	}
	for _, skill := range skills {
		executables := make(map[string]bool)
		for _, file := range skill.Files {
			if file.Executable {
				executables[file.RelativePath] = true
			}
		}
		contract.Skills = append(contract.Skills, imagecontract.Skill{Name: skill.Metadata.Name, Executables: executables})
	}
	for _, prompt := range prompts {
		if prompt.Runtime != "" {
			contract.Runtimes = append(contract.Runtimes, prompt.Runtime)
		}
	}
	return imagecontract.Validate(contract, resolver)
}

// prepareV2Prompts turns the prompts of src into template entries and the
// bodies of their file layers, keyed by archive path.
func prepareV2Prompts(src *imagefile.V2, roots imagefile.ResolveRoots) ([]TemplateEntry, map[string][]byte, error) {
	entries := make([]TemplateEntry, 0, len(src.Prompts))
	layers := map[string][]byte{}
	var totalPromptBytes int64
	for i, prompt := range src.Prompts {
		if prompt.Runtime != "" {
			entries = append(entries, TemplateEntry{Kind: "runtime", Runtime: prompt.Runtime})
			continue
		}
		resolved, err := imagefile.ResolvePromptFile(src.Dir, prompt.File, roots)
		if err != nil {
			return nil, nil, fmt.Errorf("prompts[%d]: %w", i, err)
		}
		body, err := os.ReadFile(resolved.Path)
		if err != nil {
			return nil, nil, err
		}
		if int64(len(body)) != resolved.Size || sha256hex(body) != resolved.SHA256 {
			return nil, nil, fmt.Errorf("prompts[%d]: source changed while building", i)
		}
		if totalPromptBytes > maxV2PromptBytes-resolved.Size {
			return nil, nil, fmt.Errorf("prompt layers exceed %d aggregate bytes", maxV2PromptBytes)
		}
		totalPromptBytes += resolved.Size
		base := archiveBaseCleaner.ReplaceAllString(filepath.Base(resolved.Path), "-")
		archivePath := fmt.Sprintf("prompt/layers/%03d-%s", i, base)
		layers[archivePath] = body
		entries = append(entries, TemplateEntry{Kind: "file", Source: resolved.Source, Category: resolved.Category, ArchivePath: archivePath, Size: resolved.Size, SHA256: resolved.SHA256})
	}
	return entries, layers, nil
}

type ValidationV2 struct {
	Template PromptTemplate         `json:"template"`
	Skills   []agentskills.Metadata `json:"skills"`
}

func manifestSkills(prepared []agentskills.Prepared) []agentskills.Metadata {
	out := make([]agentskills.Metadata, 0, len(prepared))
	for _, skill := range prepared {
		out = append(out, skill.Metadata)
	}
	return out
}

func ValidateV2Detailed(src *imagefile.V2, roots imagefile.ResolveRoots, resolver plugincaps.ExternalResolver) (ValidationV2, error) {
	prepared, err := prepareV2(src, roots, resolver)
	if err != nil {
		return ValidationV2{}, err
	}
	return ValidationV2{Template: prepared.template, Skills: manifestSkills(prepared.skills)}, nil
}

func ValidateV2(src *imagefile.V2, roots imagefile.ResolveRoots, resolver plugincaps.ExternalResolver) (PromptTemplate, error) {
	validated, err := ValidateV2Detailed(src, roots, resolver)
	if err != nil {
		return PromptTemplate{}, err
	}
	return validated.Template, nil
}

// BuildV2 is the one schema-v2 build mechanism. Operators, agents, Store
// sources and team imports all publish through it. It stores the archive under
// its content digest; a rebuild of an image_version that exists with other
// content fails with VersionConflictError (see Store.publish). Callers that
// accept a user-supplied ref screen it with BuildRefs or IsReserved first.
func BuildV2(src *imagefile.V2, roots imagefile.ResolveRoots, ref Ref, store *Store, clock func() time.Time, resolver plugincaps.ExternalResolver) (Manifest, error) {
	manifest, _, err := buildV2(src, roots, ref, store, clock, resolver, nil, PublishOptions{})
	return manifest, err
}

// BuildV2Publish builds through BuildV2 under opts and also returns the ids of
// the same image_version a forced build replaced.
func BuildV2Publish(src *imagefile.V2, roots imagefile.ResolveRoots, ref Ref, store *Store, clock func() time.Time, resolver plugincaps.ExternalResolver, opts PublishOptions) (Manifest, []string, error) {
	return buildV2(src, roots, ref, store, clock, resolver, nil, opts)
}

// BuildV2Archive builds through BuildV2 and also returns the published archive
// bytes, so a caller can move other tags onto the same content.
func BuildV2Archive(src *imagefile.V2, roots imagefile.ResolveRoots, ref Ref, store *Store, clock func() time.Time, resolver plugincaps.ExternalResolver) (Manifest, []byte, error) {
	var archive []byte
	manifest, _, err := buildV2(src, roots, ref, store, clock, resolver, &archive, PublishOptions{})
	return manifest, archive, err
}

func buildV2(src *imagefile.V2, roots imagefile.ResolveRoots, ref Ref, store *Store, clock func() time.Time, resolver plugincaps.ExternalResolver, archiveOut *[]byte, opts PublishOptions) (Manifest, []string, error) {
	prepared, err := prepareV2(src, roots, resolver)
	if err != nil {
		return Manifest{}, nil, err
	}
	manifest := Manifest{SchemaVersion: 2, ImageVersion: src.ImageVersion, Name: ref.Name, Tag: ref.Tag, BuiltAt: storetime.FormatTime(clock()), Plugins: prepared.plugins, Skills: manifestSkills(prepared.skills), PromptTemplateSHA256: prepared.template.SHA256}
	if manifest.Plugins == nil {
		manifest.Plugins = []ManifestPlugin{}
	}
	if manifest.Skills == nil {
		manifest.Skills = []agentskills.Metadata{}
	}
	if manifest.Role, err = rolefile.Read(src.Dir); err != nil {
		return Manifest{}, nil, err
	}
	digest, replaced, err := store.writeV2Archive(ref, manifest, prepared.template, prepared.layers, prepared.skills, archiveOut, opts)
	if err != nil {
		return Manifest{}, nil, err
	}
	manifest.ID, manifest.Digest = digest, digest
	return manifest, replaced, nil
}

func (s *Store) writeV2Archive(ref Ref, man Manifest, template PromptTemplate, layers map[string][]byte, skills []agentskills.Prepared, archiveOut *[]byte, opts PublishOptions) (string, []string, error) {
	if err := checkRef(ref); err != nil {
		return "", nil, err
	}
	templateJSON, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		return "", nil, err
	}
	members := []portableImageMember{{name: "manifest.json", mode: 0o600}, {name: "prompt/template.json", mode: 0o600, body: templateJSON}}
	for _, entry := range template.Entries {
		if entry.Kind == "file" {
			members = append(members, portableImageMember{name: entry.ArchivePath, mode: 0o600, body: layers[entry.ArchivePath]})
		}
	}
	for _, skill := range skills {
		for _, file := range skill.Files {
			mode := int64(0o600)
			if file.Executable {
				mode = 0o700
			}
			members = append(members, portableImageMember{name: skill.Metadata.ArchiveRoot + "/" + file.RelativePath, mode: mode, body: file.Body})
		}
	}
	// The id is the digest of everything else, so it is computed with the
	// manifest that does not carry it yet.
	man.ID = ""
	if members[0].body, err = marshalV2Manifest(man); err != nil {
		return "", nil, err
	}
	byName := make(map[string]portableImageMember, len(members))
	for _, member := range members {
		byName[member.name] = member
	}
	if man.ID, err = contentDigest(byName); err != nil {
		return "", nil, err
	}
	if members[0].body, err = marshalV2Manifest(man); err != nil {
		return "", nil, err
	}
	stageDir := s.refs().NameDir(ref.Name)
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return "", nil, err
	}
	tmp, err := os.CreateTemp(stageDir, "."+ref.Tag+"-*.tmp")
	if err != nil {
		return "", nil, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	gz := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gz)
	for _, member := range members {
		if err := writeTarFileMode(tw, member.name, member.body, member.mode); err != nil {
			tmp.Close()
			return "", nil, err
		}
	}
	if err := tw.Close(); err != nil {
		tmp.Close()
		return "", nil, err
	}
	if err := gz.Close(); err != nil {
		tmp.Close()
		return "", nil, err
	}
	if err := tmp.Close(); err != nil {
		return "", nil, err
	}
	return s.publishArchive(ref, tmpName, archiveOut, opts)
}

func marshalV2Manifest(man Manifest) ([]byte, error) {
	return json.MarshalIndent(struct {
		SchemaVersion        int                    `json:"schema_version"`
		ImageVersion         string                 `json:"image_version,omitempty"`
		ID                   string                 `json:"id,omitempty"`
		Name                 string                 `json:"name"`
		BuiltAt              string                 `json:"built_at"`
		Plugins              []ManifestPlugin       `json:"plugins"`
		Skills               []agentskills.Metadata `json:"skills"`
		PromptTemplateSHA256 string                 `json:"prompt_template_sha256"`
		Role                 *rolefile.Role         `json:"role,omitempty"`
	}{man.SchemaVersion, man.ImageVersion, man.ID, man.Name, man.BuiltAt, man.Plugins, man.Skills, man.PromptTemplateSHA256, man.Role}, "", "  ")
}

func sha256hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
