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

func FormatRuntimeIdentity(agentName, imageRef, imageDigest, cwd, iterationID string) string {
	lines := []string{"# You are agent " + agentName, "image: " + imageRef, "image-digest: " + imageDigest, "cwd: " + cwd}
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
	messages := strings.TrimRight(values.Messages, "\n")
	awaitingReplies := strings.TrimRight(values.AwaitingReplies, "\n")
	for _, entry := range template.Entries {
		if entry.Kind == "runtime" && entry.Runtime == "messages" {
			if messages != "" && awaitingReplies != "" {
				messages += "\n\n"
			}
			messages += awaitingReplies
			awaitingReplies = ""
			break
		}
	}
	runtime := map[string]string{
		"identity": values.Identity, "goal": values.Goal, "context": values.Context,
		"messages": messages, "awaiting-replies": awaitingReplies,
		"user-prompt": values.UserPrompt, "one-shot": values.OneShot,
		"workdir": values.Workdir,
	}
	runtimeSkills := map[string]string{
		"identity": "whoami", "goal": "tasks", "workdir": "workdir", "context": "context",
		"messages": "messages", "awaiting-replies": "messages",
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
			body = strings.TrimRight(value, "\n")
			if body != "" {
				switch entry.Runtime {
				case "context":
					body = "# Agent Context\n\n" + body
				case "messages", "awaiting-replies":
					if body != "# Messages" && !strings.HasPrefix(body, "# Messages\n") {
						body = "# Messages\n\n" + body
					}
				}
			}
			if body != "" {
				heading := "# [runtime: " + entry.Runtime + "]"
				if skill := runtimeSkills[entry.Runtime]; skill != "" {
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
	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, "\n\n") + "\n", nil
}
