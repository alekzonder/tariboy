package scripts

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	imagepkg "github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
)

func TestTaribossImageManagerContract(t *testing.T) {
	root := tariboyDeveloperRoot(t)
	imageDir := filepath.Join(root, "store", "images", "tariboss")
	image, err := imagefile.ParseV2(imageDir)
	if err != nil {
		t.Fatalf("parse tariboss image: %v", err)
	}

	var plugins []string
	for _, plugin := range image.Plugins {
		plugins = append(plugins, plugin.Name)
	}
	wantPlugins := []string{"whoami", "messages", "context", "status", "workdir", "loop"}
	if !slices.Equal(plugins, wantPlugins) {
		t.Fatalf("plugins = %v, want %v", plugins, wantPlugins)
	}
	validated, err := imagepkg.ValidateV2Detailed(image, imagefile.ResolveRoots{
		Store: filepath.Join(root, "store"), CurrentVersionStore: filepath.Join(root, "store"),
	}, nil)
	if err != nil {
		t.Fatalf("validate tariboss image: %v", err)
	}
	var skills []string
	for _, skill := range validated.Skills {
		skills = append(skills, skill.Name)
	}
	if !slices.Equal(skills, wantPlugins) {
		t.Fatalf("packaged skills = %v, want %v", skills, wantPlugins)
	}

	instructions, err := os.ReadFile(filepath.Join(imageDir, "instructions.md"))
	if err != nil {
		t.Fatalf("read tariboss instructions: %v", err)
	}
	for _, required := range []string{
		"tariboy agent",
		"tariboy iteration",
		"tariboy group",
		"explicit customer approval",
		"perform or delegate source-code work",
	} {
		if !strings.Contains(string(instructions), required) {
			t.Errorf("instructions do not contain %q", required)
		}
	}
}
