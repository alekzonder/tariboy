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
	"github.com/alekzonder/tariboy/internal/imagecontract"
	"github.com/alekzonder/tariboy/internal/tasks"
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
	Goal            string
	Workdir         string
	Context         string
	Messages        string
	AwaitingReplies string
	UserPrompt      string
	OneShot         string
}

func FormatRuntimeIdentity(agentName, imageRef, imageVersion, imageDigest, cwd, iterationID string) string {
	lines := []string{"# You are agent " + agentName, "image: " + imageRef}
	if imageVersion != "" {
		lines = append(lines, "image-version: "+imageVersion)
	}
	lines = append(lines, "image-digest: "+imageDigest, "cwd: "+cwd)
	if iterationID != "" {
		lines = append(lines, "iteration: "+iterationID)
	}
	return strings.Join(lines, "\n")
}

func FormatRuntimeGoal(task tasks.Task) string {
	return fmt.Sprintf("# Agent Goal\n\nA selected task is active work: complete it through its Native Task workflow. If it is `wait_customer`, wait for the customer answer recorded on the task before resuming. After recording a Pull request, set the task status to Wait customer and monitor it; do not merge it yourself.\n\nkey: %s\ntitle: %s\npriority: %s\nstatus: %s\ndescription: %s",
		task.Key, task.Title, task.Priority, task.Status, task.Description)
}

func FormatRuntimeGoalGuidance() string {
	return "# Agent Goal\n\nUse the Native Task workflow for selected work. If a task is `wait_customer`, wait for the customer answer recorded on the task before resuming. After recording a Pull request, set the task status to Wait customer and monitor it; do not merge it yourself."
}

// FormatTaskProcessingOrder is platform-owned, independent of image placeholders.
// Only generated headings are removed or nested; input text stays literal.
func FormatTaskProcessingOrder(values RuntimePromptValues) string {
	oneShot := values.OneShot
	if strings.TrimSpace(oneShot) == "" {
		oneShot = "No one-shot instruction for this iteration."
	}
	messages := strings.TrimPrefix(values.Messages, "# Messages\n")
	if strings.TrimSpace(messages) == "" {
		messages = "No incoming messages for this iteration."
	}
	if values.AwaitingReplies != "" {
		messages += "\n\n### Awaiting replies\n" + strings.TrimPrefix(values.AwaitingReplies, "# Awaiting replies\n")
	}
	goal := strings.TrimPrefix(values.Goal, "# Agent Goal\n\n")
	if strings.TrimSpace(goal) == "" {
		goal = "No goal selected for this iteration.\n\n" + strings.TrimPrefix(FormatRuntimeGoalGuidance(), "# Agent Goal\n\n")
	}
	return "# Task Processing Order\n\n" +
		"Process the following inputs in order: one-shot, then messages, then goal.\n" +
		"If an input is absent, continue to the next section; do not run commands merely to look for that absent input.\n\n" +
		"## One-shot\n\n" + oneShot + "\n\n## Messages\n\n" + messages + "\n\n## Goal\n\n" + goal
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
		// Older templates remain valid; these inputs are rendered once above them.
		"goal": "", "messages": "", "awaiting-replies": "", "one-shot": "",
		"user-prompt": values.UserPrompt,
		"workdir":     values.Workdir,
	}
	root, err := filepath.Abs(imageDir)
	if err != nil {
		return "", err
	}
	parts := []string{FormatTaskProcessingOrder(values)}
	for i, entry := range template.Entries {
		var body string
		switch entry.Kind {
		case "runtime":
			value, ok := runtime[entry.Runtime]
			if !ok {
				return "", fmt.Errorf("template entry %d: unknown runtime placeholder %q", i, entry.Runtime)
			}
			body = strings.TrimRight(value, "\n")
			if body != "" {
				switch entry.Runtime {
				case "context":
					body = "# Agent Context\n\n" + body
				}
			}
			if body != "" {
				heading := "# [runtime: " + entry.Runtime + "]"
				capability, _ := imagecontract.Runtime(entry.Runtime)
				if skill := capability.Skill; skill != "" {
					body = fmt.Sprintf("%s\n\nUse the `%s` skill for this runtime data.\n\n%s", heading, skill, body)
				} else {
					body = heading + "\n\n" + body
				}
			}
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
	return strings.Join(parts, "\n\n") + "\n", nil
}
