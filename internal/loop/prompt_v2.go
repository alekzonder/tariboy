package loop

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/image"
)

func ReadPromptTemplate(imageDir, trustedSHA string) (image.PromptTemplate, error) {
	data, err := os.ReadFile(filepath.Join(imageDir, "prompt", "template.json"))
	if err != nil {
		return image.PromptTemplate{}, fmt.Errorf("read image prompt template: %w", err)
	}
	var template image.PromptTemplate
	if err := json.Unmarshal(data, &template); err != nil {
		return image.PromptTemplate{}, fmt.Errorf("parse image prompt template: %w", err)
	}
	if err := image.ValidatePromptTemplate(template); err != nil {
		return image.PromptTemplate{}, err
	}
	if trustedSHA == "" || template.SHA256 != trustedSHA {
		return image.PromptTemplate{}, fmt.Errorf("image prompt template does not match trusted iteration identity")
	}
	return template, nil
}

type RuntimePromptValues struct {
	Identity        string
	Workdir         string
	Context         string
	Messages        string
	AwaitingReplies string
	UserPrompt      string
	OneShot         string
}

func FormatRuntimeIdentity(agentName, imageRef, imageDigest, cwd, iterationID string) string {
	lines := []string{"# You are agent " + agentName, "image: " + imageRef, "image-digest: " + imageDigest, "cwd: " + cwd}
	if iterationID != "" {
		lines = append(lines, "iteration: "+iterationID)
	}
	return strings.Join(lines, "\n")
}

func FormatRuntimeWorkdir(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve managed workdir: %w", err)
	}
	return "workdir: " + absolute, nil
}

func RenderPromptTemplate(template image.PromptTemplate, imageDir string, values RuntimePromptValues) (string, error) {
	if err := image.ValidatePromptTemplate(template); err != nil {
		return "", err
	}
	runtime := map[string]string{
		"identity": values.Identity, "context": values.Context,
		"messages": values.Messages, "awaiting-replies": values.AwaitingReplies,
		"user-prompt": values.UserPrompt, "one-shot": values.OneShot,
		"workdir": values.Workdir,
	}
	root, err := filepath.Abs(imageDir)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(template.Entries))
	for i, entry := range template.Entries {
		var body string
		switch entry.Kind {
		case "runtime":
			value, ok := runtime[entry.Runtime]
			if !ok {
				return "", fmt.Errorf("template entry %d: unknown runtime placeholder %q", i, entry.Runtime)
			}
			body = value
		case "file":
			if filepath.IsAbs(entry.ArchivePath) {
				return "", fmt.Errorf("template entry %d: unsafe absolute layer path", i)
			}
			candidate := filepath.Join(root, filepath.FromSlash(entry.ArchivePath))
			rel, err := filepath.Rel(root, candidate)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return "", fmt.Errorf("template entry %d: layer path escapes image", i)
			}
			info, err := os.Lstat(candidate)
			if err != nil || !info.Mode().IsRegular() {
				return "", fmt.Errorf("template entry %d: layer is not a regular file", i)
			}
			data, err := os.ReadFile(candidate)
			if err != nil {
				return "", fmt.Errorf("template entry %d: %w", i, err)
			}
			sum := sha256.Sum256(data)
			if int64(len(data)) != entry.Size || hex.EncodeToString(sum[:]) != entry.SHA256 {
				return "", fmt.Errorf("template entry %d: layer integrity mismatch", i)
			}
			body = string(data)
		default:
			return "", fmt.Errorf("template entry %d: unknown kind %q", i, entry.Kind)
		}
		body = strings.TrimRight(body, "\n")
		if body != "" {
			parts = append(parts, body)
		}
	}
	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, "\n\n") + "\n", nil
}
