package scripts

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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

	instructions, err := os.ReadFile(filepath.Join(imageDir, "instructions.md"))
	if err != nil {
		t.Fatalf("read tariboss instructions: %v", err)
	}
	for _, required := range []string{
		"tariboy agent",
		"tariboy iteration",
		"tariboy group",
		"explicit customer approval",
	} {
		if !strings.Contains(string(instructions), required) {
			t.Errorf("instructions do not contain %q", required)
		}
	}
}
