package agentdir

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/internal/agentskills"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
)

var (
	bridgeDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	bridgeKeyPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
)

func (l Layout) ImageBridgeDir(digest, contractVersion, harness string) (string, error) {
	if !bridgeDigestPattern.MatchString(digest) {
		return "", errors.New("invalid image bridge digest")
	}
	if !bridgeKeyPattern.MatchString(contractVersion) {
		return "", errors.New("invalid image bridge contract version")
	}
	if !bridgeKeyPattern.MatchString(harness) {
		return "", errors.New("invalid image bridge harness")
	}
	return filepath.Join(l.ImageBridgesDir(), digest, contractVersion, harness), nil
}

type BridgeFile struct {
	Path string
	Body []byte
	Mode fs.FileMode
}

type BridgePlan struct {
	SkillDestination string
	Files            []BridgeFile
}

type bridgeSkill struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	ClientVersion string `json:"client_version,omitempty"`
	ArchiveRoot   string `json:"archive_root"`
	FileCount     int    `json:"file_count"`
	Size          int64  `json:"size"`
	TreeSHA256    string `json:"tree_sha256"`
}

type bridgeFileRecord struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
}

type BridgeManifest struct {
	ImageDigest      string                      `json:"image_digest"`
	Harness          string                      `json:"harness"`
	ContractVersion  string                      `json:"contract_version"`
	SkillDestination string                      `json:"skill_destination"`
	Skills           []bridgeSkill               `json:"skills"`
	Files            map[string]bridgeFileRecord `json:"files"`
}

func cleanBridgeRelative(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, '\\') || path.IsAbs(name) {
		return "", fmt.Errorf("invalid bridge path %q", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean != name || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid bridge path %q", name)
	}
	return clean, nil
}

func bridgeIdentity(finalDir string) (digest, contractVersion, harness string, err error) {
	clean := filepath.Clean(finalDir)
	harness = filepath.Base(clean)
	contractDir := filepath.Dir(clean)
	contractVersion = filepath.Base(contractDir)
	digest = filepath.Base(filepath.Dir(contractDir))
	if !bridgeDigestPattern.MatchString(digest) || !bridgeKeyPattern.MatchString(contractVersion) || !bridgeKeyPattern.MatchString(harness) {
		return "", "", "", errors.New("invalid final image bridge path")
	}
	return digest, contractVersion, harness, nil
}

func expectedBridgeSkills(expected []image.ManifestSkill) []bridgeSkill {
	out := make([]bridgeSkill, 0, len(expected))
	for _, skill := range expected {
		out = append(out, bridgeSkill{
			Name: skill.Name, Description: skill.Description, ArchiveRoot: skill.ArchiveRoot,
			ClientVersion: skill.ClientVersion,
			FileCount:     skill.FileCount, Size: skill.Size, TreeSHA256: skill.TreeSHA256,
		})
	}
	return out
}

func validateBridgePlan(plan BridgePlan) (string, map[string]BridgeFile, error) {
	destination, err := cleanBridgeRelative(plan.SkillDestination)
	if err != nil {
		return "", nil, err
	}
	generated := make(map[string]BridgeFile, len(plan.Files))
	for _, file := range plan.Files {
		name, err := cleanBridgeRelative(file.Path)
		if err != nil {
			return "", nil, err
		}
		if name == "bridge-manifest.json" || name == destination || strings.HasPrefix(name, destination+"/") {
			return "", nil, fmt.Errorf("generated bridge file %q overlaps skill destination", name)
		}
		mode := file.Mode.Perm()
		if mode != 0o600 && mode != 0o700 {
			return "", nil, fmt.Errorf("generated bridge file %q has invalid mode %#o", name, mode)
		}
		if _, duplicate := generated[name]; duplicate {
			return "", nil, fmt.Errorf("duplicate generated bridge file %q", name)
		}
		file.Path = name
		file.Mode = mode
		generated[name] = file
	}
	return destination, generated, nil
}

func compareSkillMetadata(prepared agentskills.Prepared, expected image.ManifestSkill) error {
	meta := prepared.Metadata
	if meta.Name != expected.Name || meta.Description != expected.Description || meta.ArchiveRoot != expected.ArchiveRoot || meta.FileCount != expected.FileCount || meta.Size != expected.Size || meta.TreeSHA256 != expected.TreeSHA256 {
		return fmt.Errorf("skill %q does not match image manifest", expected.Name)
	}
	return nil
}

func prepareSourceSkills(imageSkillsDir string, expected []image.ManifestSkill) ([]agentskills.Prepared, error) {
	entries, err := os.ReadDir(imageSkillsDir)
	if err != nil {
		return nil, err
	}
	if len(entries) != len(expected) {
		return nil, errors.New("canonical image skill directory does not match manifest")
	}
	prepared := make([]agentskills.Prepared, 0, len(expected))
	seen := make(map[string]bool, len(expected))
	for _, skill := range expected {
		if seen[skill.Name] {
			return nil, fmt.Errorf("duplicate image skill %q", skill.Name)
		}
		seen[skill.Name] = true
		dir := filepath.Join(imageSkillsDir, skill.Name)
		got, err := agentskills.Prepare(imagefile.ResolvedDirectory{Path: dir})
		if err != nil {
			return nil, err
		}
		if err := compareSkillMetadata(got, skill); err != nil {
			return nil, err
		}
		prepared = append(prepared, got)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !seen[entry.Name()] {
			return nil, fmt.Errorf("unexpected canonical image skill member %q", entry.Name())
		}
	}
	return prepared, nil
}

func writeBridgeFile(root, name string, body []byte, mode fs.FileMode) error {
	name, err := cleanBridgeRelative(name)
	if err != nil {
		return err
	}
	dir, err := ensureConfinedDir(root, filepath.FromSlash(path.Dir(name)))
	if err != nil {
		return err
	}
	target := filepath.Join(dir, filepath.Base(name))
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("duplicate bridge path %q", name)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.WriteFile(target, body, mode.Perm()); err != nil {
		return err
	}
	return os.Chmod(target, mode.Perm())
}

func fileRecord(body []byte, mode fs.FileMode) bridgeFileRecord {
	sum := sha256.Sum256(body)
	return bridgeFileRecord{SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body)), Mode: uint32(mode.Perm())}
}

func syncBridgeTree(root string) error {
	return syncBridgeTreeWith(root, func(name string) error {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		return file.Sync()
	})
}

func syncBridgeTreeWith(root string, syncPath func(string) error) error {
	return filepath.Walk(root, func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() || info.IsDir() {
			err := syncPath(name)
			if info.IsDir() && errors.Is(err, os.ErrInvalid) {
				return nil
			}
			return err
		}
		return nil
	})
}

func validatePublishedBridge(finalDir string, expected []image.ManifestSkill, plan BridgePlan) error {
	digest, contractVersion, harness, err := bridgeIdentity(finalDir)
	if err != nil {
		return err
	}
	destination, generated, err := validateBridgePlan(plan)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(filepath.Join(finalDir, "bridge-manifest.json"))
	if err != nil {
		return err
	}
	var manifest BridgeManifest
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return errors.New("image bridge manifest contains trailing JSON")
	}
	if manifest.ImageDigest != digest || manifest.ContractVersion != contractVersion || manifest.Harness != harness || manifest.SkillDestination != destination {
		return errors.New("image bridge manifest identity mismatch")
	}
	if !equalBridgeSkills(manifest.Skills, expectedBridgeSkills(expected)) {
		return errors.New("image bridge skill metadata mismatch")
	}
	for name, generatedFile := range generated {
		if manifest.Files[name] != fileRecord(generatedFile.Body, generatedFile.Mode) {
			return fmt.Errorf("generated bridge file %q metadata mismatch", name)
		}
	}
	skillPrefixes := make([]string, 0, len(expected))
	for _, skill := range expected {
		skillPrefixes = append(skillPrefixes, path.Join(destination, skill.Name)+"/")
	}
	seen := make(map[string]bool, len(manifest.Files))
	err = filepath.Walk(finalDir, func(member string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("image bridge contains a symlink")
		}
		if info.IsDir() {
			if info.Mode().Perm() != 0o700 {
				return errors.New("image bridge directory has invalid mode")
			}
			return nil
		}
		rel, err := filepath.Rel(finalDir, member)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "bridge-manifest.json" {
			return nil
		}
		allowed := false
		if _, ok := generated[rel]; ok {
			allowed = true
		}
		for _, prefix := range skillPrefixes {
			if strings.HasPrefix(rel, prefix) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("unexpected image bridge file %q", rel)
		}
		if !info.Mode().IsRegular() {
			return errors.New("image bridge contains a special file")
		}
		record, ok := manifest.Files[rel]
		if !ok || uint32(info.Mode().Perm()) != record.Mode || info.Size() != record.Size {
			return fmt.Errorf("image bridge file %q metadata mismatch", rel)
		}
		data, err := os.ReadFile(member)
		if err != nil {
			return err
		}
		if fileRecord(data, info.Mode()).SHA256 != record.SHA256 {
			return fmt.Errorf("image bridge file %q integrity mismatch", rel)
		}
		seen[rel] = true
		return nil
	})
	if err != nil {
		return err
	}
	if len(seen) != len(manifest.Files) {
		return errors.New("image bridge files are missing")
	}
	for _, skill := range expected {
		dir := filepath.Join(finalDir, filepath.FromSlash(destination), skill.Name)
		prepared, err := agentskills.Prepare(imagefile.ResolvedDirectory{Path: dir})
		if err != nil {
			return err
		}
		if err := compareSkillMetadata(prepared, skill); err != nil {
			return err
		}
	}
	return nil
}

func equalBridgeSkills(a, b []bridgeSkill) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func buildBridgeStage(stage, digest, contractVersion, harness string, expected []image.ManifestSkill, prepared []agentskills.Prepared, plan BridgePlan) error {
	destination, generated, err := validateBridgePlan(plan)
	if err != nil {
		return err
	}
	records := make(map[string]bridgeFileRecord)
	for _, skill := range prepared {
		for _, file := range skill.Files {
			name := path.Join(destination, skill.Metadata.Name, file.RelativePath)
			mode := fs.FileMode(0o600)
			if file.Executable {
				mode = 0o700
			}
			if err := writeBridgeFile(stage, name, file.Body, mode); err != nil {
				return err
			}
			records[name] = fileRecord(file.Body, mode)
		}
	}
	generatedNames := make([]string, 0, len(generated))
	for name := range generated {
		generatedNames = append(generatedNames, name)
	}
	sort.Strings(generatedNames)
	for _, name := range generatedNames {
		file := generated[name]
		if err := writeBridgeFile(stage, name, file.Body, file.Mode); err != nil {
			return err
		}
		records[name] = fileRecord(file.Body, file.Mode)
	}
	manifest := BridgeManifest{
		ImageDigest: digest, Harness: harness, ContractVersion: contractVersion,
		SkillDestination: destination, Skills: expectedBridgeSkills(expected), Files: records,
	}
	manifestBody, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	manifestBody = append(manifestBody, '\n')
	if err := writeBridgeFile(stage, "bridge-manifest.json", manifestBody, 0o600); err != nil {
		return err
	}
	return syncBridgeTree(stage)
}

func PrepareImageBridge(imageSkillsDir, finalDir string, expected []image.ManifestSkill, plan BridgePlan) error {
	digest, contractVersion, harness, err := bridgeIdentity(finalDir)
	if err != nil {
		return err
	}
	if len(expected) == 0 {
		return errors.New("cannot prepare an image bridge without skills")
	}
	if err := validatePublishedBridge(finalDir, expected, plan); err == nil {
		return nil
	}
	prepared, err := prepareSourceSkills(imageSkillsDir, expected)
	if err != nil {
		return err
	}
	parent := filepath.Dir(finalDir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, "."+harness+"-bridge-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := os.Chmod(stage, 0o700); err != nil {
		return err
	}
	if err := buildBridgeStage(stage, digest, contractVersion, harness, expected, prepared, plan); err != nil {
		return err
	}
	if err := os.Rename(stage, finalDir); err == nil {
		return validatePublishedBridge(finalDir, expected, plan)
	}
	if err := validatePublishedBridge(finalDir, expected, plan); err == nil {
		return nil
	}
	backup, err := os.MkdirTemp(parent, "."+harness+"-invalid-")
	if err != nil {
		return err
	}
	if err := os.Remove(backup); err != nil {
		return err
	}
	if err := os.Rename(finalDir, backup); err != nil {
		if validateErr := validatePublishedBridge(finalDir, expected, plan); validateErr == nil {
			return nil
		}
		return err
	}
	if err := os.Rename(stage, finalDir); err != nil {
		_ = os.Rename(backup, finalDir)
		return err
	}
	_ = os.RemoveAll(backup)
	return validatePublishedBridge(finalDir, expected, plan)
}
