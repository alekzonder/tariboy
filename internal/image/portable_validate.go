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
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/internal/agentskills"
	"github.com/alekzonder/tariboy/internal/imagecontract"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/portablearchive"
	"github.com/alekzonder/tariboy/internal/safefs"
	"github.com/alekzonder/tariboy/internal/store"
)

var portablePluginName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

const MaxPortableImageCompressed = int64(64 << 20)

// portableImageLimits bounds the members of a runnable image archive. The
// allowed member types and modes are portablearchive.MemberRules rows for
// portablearchive.FormatImage.
var portableImageLimits = portablearchive.MemberLimits{
	MaxPathBytes: 512,
	MaxMembers:   4096,
	MaxExpanded:  256 << 20,
}

// portableMemberError keeps this package's texts for a member that
// portablearchive.MemberChecker refuses.
func portableMemberError(h *tar.Header, err error) error {
	switch {
	case errors.Is(err, portablearchive.ErrMemberCount):
		return errors.New("image archive has too many files")
	case errors.Is(err, portablearchive.ErrMemberType):
		return fmt.Errorf("invalid image archive member %q: regular files only", h.Name)
	case errors.Is(err, portablearchive.ErrMemberMode):
		return fmt.Errorf("invalid image archive member %q mode %#o", h.Name, h.Mode&0o7777)
	case errors.Is(err, portablearchive.ErrMemberPath):
		return fmt.Errorf("invalid image archive member path %q", h.Name)
	case errors.Is(err, portablearchive.ErrMemberSize):
		return errors.New("image archive exceeds expanded size limit")
	}
	return err
}

// v2PortableManifest is the strict schema of manifest.json inside a v2
// archive, decoded with unknown fields rejected. It is not a copy of
// Manifest, which also carries runtime fields such as digest, harness and layers.
type v2PortableManifest struct {
	SchemaVersion        int                    `json:"schema_version"`
	ImageVersion         string                 `json:"image_version,omitempty"`
	ID                   string                 `json:"id,omitempty"`
	Name                 string                 `json:"name"`
	Tag                  string                 `json:"tag,omitempty"`
	BuiltAt              string                 `json:"built_at"`
	Plugins              []v2PortablePlugin     `json:"plugins"`
	Skills               []agentskills.Metadata `json:"skills"`
	PromptTemplateSHA256 string                 `json:"prompt_template_sha256"`
}

type v2PortablePlugin struct {
	Name string `json:"name"`
}

// archivePassHook, when set by a test in this package, is called once for every
// pass that decompresses and reads an image archive.
var archivePassHook func()

func noteArchivePass() {
	if archivePassHook != nil {
		archivePassHook()
	}
}

// portableImageMember is one regular file of a validated image archive. body
// is the member's bytes in the decompressed archive held in memory.
type portableImageMember struct {
	name       string
	typ        byte
	mode       int64 // permission bits of the tar header mode
	headerMode int64 // the tar header mode as written
	body       []byte
}

// portableArchive is the parsed form of a validated image archive. Callers
// that unpack or rewrite the archive work from it instead of reading the
// archive again.
type portableArchive struct {
	members map[string]portableImageMember
	order   []string // member names in archive order
}

// validatePortableArchive validates the runnable inner archive before it can
// enter the immutable Store or be unpacked into an agent directory, and
// returns its parsed members.
func validatePortableArchive(archive []byte, ref Ref) (Manifest, portableArchive, error) {
	return validatePortableArchiveContract(archive, ref, nil, false)
}

func validatePortableArchiveContract(archive []byte, ref Ref, resolver imagecontract.ExternalResolver, enforceContract bool) (Manifest, portableArchive, error) {
	members, order, err := readPortableMembers(archive)
	if err != nil {
		return Manifest{}, portableArchive{}, err
	}
	manifest, err := validatePortableMembers(archive, members, ref, resolver, enforceContract)
	if err != nil {
		return Manifest{}, portableArchive{}, err
	}
	return manifest, portableArchive{members: members, order: order}, nil
}

// readPortableMembers decompresses archive once and checks every member
// against the image member rules.
func readPortableMembers(archive []byte) (map[string]portableImageMember, []string, error) {
	if int64(len(archive)) > MaxPortableImageCompressed {
		return nil, nil, errors.New("image archive exceeds compressed size limit")
	}
	noteArchivePass()
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, nil, err
	}
	defer gz.Close()

	members := make(map[string]portableImageMember)
	var order []string
	tr := tar.NewReader(gz)
	checker := portablearchive.MemberChecker{Format: portablearchive.FormatImage, Limits: portableImageLimits}
	for {
		h, nextErr := tr.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return nil, nil, nextErr
		}
		if err := checker.Check(h); err != nil {
			return nil, nil, portableMemberError(h, err)
		}
		clean, cleanErr := safefs.CleanRel(h.Name)
		if cleanErr != nil {
			return nil, nil, fmt.Errorf("invalid image archive member path %q", h.Name)
		}
		if _, duplicate := members[clean]; duplicate {
			return nil, nil, fmt.Errorf("duplicate image archive member %q", clean)
		}
		body, readErr := io.ReadAll(io.LimitReader(tr, h.Size+1))
		if readErr != nil {
			return nil, nil, readErr
		}
		if int64(len(body)) != h.Size {
			return nil, nil, fmt.Errorf("image archive member %q size mismatch", clean)
		}
		members[clean] = portableImageMember{name: clean, typ: h.Typeflag, mode: h.Mode & 0o7777, headerMode: h.Mode, body: body}
		order = append(order, clean)
	}
	return members, order, nil
}

// validatePortableMembers checks the manifest and the member set of a parsed
// archive and returns the manifest the archive declares for ref.
func validatePortableMembers(archive []byte, members map[string]portableImageMember, ref Ref, resolver imagecontract.ExternalResolver, enforceContract bool) (Manifest, error) {

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
	if !IsLegacyV1(manifest) && manifest.SchemaVersion != ManifestSchemaVersion {
		return Manifest{}, fmt.Errorf("unsupported image schema_version %d", manifest.SchemaVersion)
	}
	if manifest.Name != ref.Name {
		return Manifest{}, fmt.Errorf("archive image %s does not match %s", manifest.Name, ref.String())
	}
	manifest.Tag = ref.Tag
	if manifest.SchemaVersion == 2 {
		if err := validateV2PortableMembers(manifestBody, members, ref, &manifest, resolver, enforceContract); err != nil {
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
	if manifest.ID != "" {
		manifest.Digest = manifest.ID
		return manifest, nil
	}
	sum := sha256.Sum256(archive)
	manifest.Digest = hex.EncodeToString(sum[:])
	return manifest, nil
}

func ValidatePortableArchive(archive []byte, ref Ref) (Manifest, error) {
	manifest, _, err := validatePortableArchive(archive, ref)
	return manifest, err
}

// ValidatePortableArchiveContract additionally checks daemon/plugin
// compatibility before an archive may be imported on a destination.
func ValidatePortableArchiveContract(archive []byte, ref Ref, resolver imagecontract.ExternalResolver) (Manifest, error) {
	manifest, _, err := validatePortableArchiveContract(archive, ref, resolver, true)
	return manifest, err
}

func validateV2PortableMembers(manifestBody []byte, members map[string]portableImageMember, ref Ref, manifest *Manifest, resolver imagecontract.ExternalResolver, enforceContract bool) error {
	var strict v2PortableManifest
	if err := decodeStrictMember(manifestBody, &strict); err != nil {
		return fmt.Errorf("invalid schema-v2 manifest: %w", err)
	}
	if strict.SchemaVersion != 2 || strict.Name != ref.Name || strict.PromptTemplateSHA256 == "" {
		return errors.New("invalid schema-v2 manifest identity")
	}
	if strict.ImageVersion != "" {
		if err := imagefile.ValidateImageVersion(strict.ImageVersion); err != nil {
			return err
		}
	}
	if strict.ID != "" && strict.ID != RefID(ref.Name, strict.ImageVersion) {
		return errors.New("invalid schema-v2 manifest id")
	}
	if _, err := store.ParseInputTime(strict.BuiltAt); err != nil {
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
	if err := decodeStrictMember(templateMember.body, &template); err != nil {
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
		prepared := agentskills.Prepared{Metadata: skill, Files: files}
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
	manifest.ImageVersion = strict.ImageVersion
	for _, plugin := range strict.Plugins {
		manifest.Plugins = append(manifest.Plugins, ManifestPlugin{Name: plugin.Name})
	}
	manifest.Skills = append([]agentskills.Metadata(nil), strict.Skills...)
	if manifest.Skills == nil {
		manifest.Skills = []agentskills.Metadata{}
	}
	if enforceContract {
		return validateManifestContract(*manifest, template, func(skill agentskills.Metadata, launcher string) bool {
			member, ok := members[skill.ArchiveRoot+"/"+launcher]
			return ok && member.mode&0o111 != 0
		}, resolver)
	}
	return nil
}

// ValidateUnpackedContract checks a staged schema-v2 image before any bridge,
// shim, directory, or database publication occurs.
func ValidateUnpackedContract(manifest Manifest, root string, resolver imagecontract.ExternalResolver) error {
	if manifest.SchemaVersion != 2 {
		return nil
	}
	body, err := os.ReadFile(filepath.Join(root, "prompt", "template.json"))
	if err != nil {
		return err
	}
	var template PromptTemplate
	if err := json.Unmarshal(body, &template); err != nil {
		return err
	}
	if err := ValidatePromptTemplate(template); err != nil {
		return err
	}
	return validateManifestContract(manifest, template, func(skill agentskills.Metadata, launcher string) bool {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(skill.ArchiveRoot), filepath.FromSlash(launcher)))
		return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
	}, resolver)
}

func validateManifestContract(manifest Manifest, template PromptTemplate, executable func(agentskills.Metadata, string) bool, resolver imagecontract.ExternalResolver) error {
	input := imagecontract.Input{
		Plugins: make([]string, 0, len(manifest.Plugins)),
		Skills:  make([]imagecontract.Skill, 0, len(manifest.Skills)),
	}
	for _, plugin := range manifest.Plugins {
		input.Plugins = append(input.Plugins, plugin.Name)
	}
	for _, entry := range template.Entries {
		if entry.Kind == "runtime" {
			input.Runtimes = append(input.Runtimes, entry.Runtime)
		}
	}
	for _, skill := range manifest.Skills {
		executables := map[string]bool{}
		for _, plugin := range manifest.Plugins {
			capability, ok := imagecontract.Builtin(plugin.Name)
			if ok && capability.Skill == skill.Name && capability.Launcher != "" && executable(skill, capability.Launcher) {
				executables[capability.Launcher] = true
			}
		}
		input.Skills = append(input.Skills, imagecontract.Skill{Name: skill.Name, Executables: executables})
	}
	return imagecontract.Validate(input, resolver)
}

func validSkillCategory(category string) bool {
	switch category {
	case "current-store", "store", "plugin", "source", "absolute":
		return true
	default:
		return false
	}
}

// decodeStrictMember decodes one strict JSON archive member, keeping this
// package's "multiple JSON values" text for trailing data.
func decodeStrictMember(body []byte, v any) error {
	err := safefs.DecodeStrictJSON(bytes.NewReader(body), v)
	if errors.Is(err, safefs.ErrTrailingData) {
		return fmt.Errorf("multiple JSON values: %w", err)
	}
	return err
}
