// Package teamportable moves compose-owned team/runtime configuration. Runnable
// images are imported and exported independently through image portability.
package teamportable

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/alekzonder/tariboy/internal/portablearchive"
	"gopkg.in/yaml.v3"
)

// metadata is the archive metadata. Images stays in the format so a non-empty
// list written by an older exporter is detected and rejected; it is never read.
type metadata struct {
	Team   string            `json:"team"`
	Images []json.RawMessage `json:"images"`
}
type Service struct {
	StagingRoot string
}
type Preview struct {
	ImportID    string
	Team        string
	ComposeYAML []byte
	StagedDir   string
}

type archiveCompose struct {
	Version int `yaml:"version"`
	Images  map[string]struct {
		Context string `yaml:"context"`
	} `yaml:"images"`
	Groups map[string]struct{} `yaml:"groups"`
	Agents map[string]struct {
		Image string `yaml:"image"`
		Group string `yaml:"group"`
	} `yaml:"agents"`
}

// CreateFromCompose writes a compose-only portable team archive. Image context
// paths remain local authoring inputs and are never copied into portability.
func CreateFromCompose(composePath string, w io.Writer) error {
	composeYAML, err := os.ReadFile(composePath)
	if err != nil {
		return err
	}
	var file archiveCompose
	if err := yaml.Unmarshal(composeYAML, &file); err != nil {
		return fmt.Errorf("team portable: parse compose: %w", err)
	}
	if file.Version != 1 {
		return fmt.Errorf("team portable: unsupported compose version %d", file.Version)
	}
	if len(file.Groups) != 1 {
		return errors.New("team portable: compose must contain exactly one group")
	}
	composeYAML, err = withoutImageSources(composeYAML)
	if err != nil {
		return err
	}
	var team string
	for name := range file.Groups {
		team = name
	}
	temp, err := os.MkdirTemp("", "tariboy-team-compose-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	files := []portablearchive.File{}
	paths := []string{}
	if err := addFile(temp, "tariboy-compose.yaml", composeYAML, &files, &paths); err != nil {
		return err
	}

	meta := metadata{Team: team, Images: []json.RawMessage{}}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	metaRaw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return portablearchive.Write(w, temp, portablearchive.Manifest{Format: "tariboy-portable", Version: 1, Kind: "team", Files: files, Metadata: metaRaw}, paths)
}

func (s Service) PreviewYAML(team string, composeYAML []byte) (Preview, error) {
	if s.StagingRoot == "" || team == "" || len(composeYAML) == 0 {
		return Preview{}, errors.New("team portable: invalid YAML preview")
	}
	if err := os.MkdirAll(s.StagingRoot, 0o700); err != nil {
		return Preview{}, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Preview{}, err
	}
	id := hex.EncodeToString(random[:])
	destination := filepath.Join(s.StagingRoot, id)
	if err := os.Mkdir(destination, 0o700); err != nil {
		return Preview{}, err
	}
	remove := true
	defer func() {
		if remove {
			_ = os.RemoveAll(destination)
		}
	}()
	stored, err := json.Marshal(metadata{Team: team, Images: []json.RawMessage{}})
	if err != nil {
		return Preview{}, err
	}
	if err := os.WriteFile(filepath.Join(destination, ".team-preview.json"), stored, 0o600); err != nil {
		return Preview{}, err
	}
	if err := os.WriteFile(filepath.Join(destination, "tariboy-compose.yaml"), composeYAML, 0o600); err != nil {
		return Preview{}, err
	}
	operation := Operation{ImportID: id, Team: team, Status: "pending", Steps: []OperationStep{}}
	if err := s.SaveOperation(operation); err != nil {
		return Preview{}, err
	}
	remove = false
	return Preview{ImportID: id, Team: team, ComposeYAML: append([]byte(nil), composeYAML...), StagedDir: destination}, nil
}

func (s Service) Load(importID string) (Preview, error) {
	if importID == "" || filepath.Base(importID) != importID {
		return Preview{}, errors.New("team portable: invalid import id")
	}
	destination := filepath.Join(s.StagingRoot, importID)
	if err := validateStagedTeamMembers(destination); err != nil {
		return Preview{}, err
	}
	data, err := os.ReadFile(filepath.Join(destination, ".team-preview.json"))
	if err != nil {
		return Preview{}, errors.New("team portable: preview not found")
	}
	var stored metadata
	if err := json.Unmarshal(data, &stored); err != nil {
		return Preview{}, err
	}
	if len(stored.Images) != 0 {
		return Preview{}, errors.New("team portable: source-bearing image metadata is unsupported; import runnable images separately")
	}
	composeYAML, err := os.ReadFile(filepath.Join(destination, "tariboy-compose.yaml"))
	if err != nil {
		return Preview{}, err
	}
	return Preview{ImportID: importID, Team: stored.Team, ComposeYAML: composeYAML, StagedDir: destination}, nil
}

func validateStagedTeamMembers(root string) error {
	allowed := map[string]bool{
		"tariboy-compose.yaml": true,
		".team-preview.json":   true,
		".operation.json":      true,
		".operation.lease":     true,
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() || !allowed[filepath.ToSlash(rel)] {
			return fmt.Errorf("team portable: unsupported staged member %q", filepath.ToSlash(rel))
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("team portable: unsafe staged member %q", filepath.ToSlash(rel))
		}
		return nil
	})
}

func addFile(root, archivePath string, data []byte, files *[]portablearchive.File, paths *[]string) error {
	target := filepath.Join(root, filepath.FromSlash(archivePath))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	*files = append(*files, portablearchive.File{Path: archivePath, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])})
	*paths = append(*paths, archivePath)
	return nil
}

func (s Service) Export(ctx context.Context, team string, composeYAML []byte, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if team == "" {
		return errors.New("team portable: invalid export")
	}
	composeYAML, err := withoutImageSources(composeYAML)
	if err != nil {
		return err
	}
	temp, err := os.MkdirTemp("", "tariboy-team-export-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	var files []portablearchive.File
	var paths []string
	if err := addFile(temp, "tariboy-compose.yaml", composeYAML, &files, &paths); err != nil {
		return err
	}
	meta := metadata{Team: team, Images: []json.RawMessage{}}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	metaRaw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return portablearchive.Write(w, temp, portablearchive.Manifest{Format: "tariboy-portable", Version: 1, Kind: "team", Files: files, Metadata: metaRaw}, paths)
}

func withoutImageSources(input []byte) ([]byte, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(input, &document); err != nil {
		return nil, fmt.Errorf("team portable: parse compose: %w", err)
	}
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("team portable: compose root must be a mapping")
	}
	root := document.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "images" {
			root.Content = append(root.Content[:i], root.Content[i+2:]...)
			break
		}
	}
	return yaml.Marshal(&document)
}

func (s Service) Preview(ctx context.Context, r io.Reader, compressedSize int64) (Preview, error) {
	if err := ctx.Err(); err != nil {
		return Preview{}, err
	}
	if s.StagingRoot == "" {
		return Preview{}, errors.New("team portable: staging unavailable")
	}
	if err := os.MkdirAll(s.StagingRoot, 0o700); err != nil {
		return Preview{}, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Preview{}, err
	}
	id := hex.EncodeToString(random[:])
	destination := filepath.Join(s.StagingRoot, id)
	manifest, err := portablearchive.Stage(r, compressedSize, destination, portablearchive.DefaultLimits())
	if err != nil {
		return Preview{}, err
	}
	remove := true
	defer func() {
		if remove {
			_ = os.RemoveAll(destination)
		}
	}()
	if manifest.Kind != "team" {
		return Preview{}, errors.New("team portable: wrong archive kind")
	}
	if len(manifest.Files) != 1 || manifest.Files[0].Path != "tariboy-compose.yaml" {
		return Preview{}, errors.New("team portable: archive must contain only tariboy-compose.yaml")
	}
	var meta metadata
	if err := json.Unmarshal(manifest.Metadata, &meta); err != nil || meta.Team == "" {
		return Preview{}, errors.New("team portable: invalid metadata")
	}
	if len(meta.Images) != 0 {
		return Preview{}, errors.New("team portable: source-bearing image metadata is unsupported; import runnable images separately")
	}
	composeYAML, err := os.ReadFile(filepath.Join(destination, "tariboy-compose.yaml"))
	if err != nil {
		return Preview{}, err
	}
	stored, err := json.Marshal(meta)
	if err != nil {
		return Preview{}, err
	}
	if err := os.WriteFile(filepath.Join(destination, ".team-preview.json"), stored, 0o600); err != nil {
		return Preview{}, err
	}
	if err := s.SaveOperation(Operation{ImportID: id, Team: meta.Team, Status: "pending", Steps: []OperationStep{}}); err != nil {
		return Preview{}, err
	}
	remove = false
	return Preview{ImportID: id, Team: meta.Team, ComposeYAML: composeYAML, StagedDir: destination}, nil
}
