package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ModelInfo is one model a harness CLI reports as selectable. Efforts is set
// only when the CLI lists reasoning efforts per model (codex).
type ModelInfo struct {
	ID      string   `json:"id"`
	Label   string   `json:"label,omitempty"`
	Efforts []string `json:"efforts,omitempty"`
}

// Catalog is the model and effort list of one harness on the daemon's host.
// Error explains an empty list (CLI missing, failed, or unparseable) so the UI
// can show it next to the field instead of failing.
type Catalog struct {
	Harness string      `json:"harness"`
	Models  []ModelInfo `json:"models"`
	Efforts []string    `json:"efforts"`
	Error   string      `json:"error"`
}

// claudeCatalog is fixed: the claude CLI has no command that lists models, so
// it offers its own aliases and the --effort levels.
var claudeCatalog = Catalog{
	Models: []ModelInfo{
		{ID: "fable"}, {ID: "opus"}, {ID: "opus[1m]"}, {ID: "sonnet"}, {ID: "haiku"},
	},
	Efforts: []string{"low", "medium", "high", "xhigh", "max"},
}

type modelSource struct {
	executable string
	args       []string
	parse      func([]byte) (Catalog, error)
}

var modelSources = map[string]modelSource{
	"codex":    {executable: "codex", args: []string{"debug", "models"}, parse: parseCodexModels},
	"cursor":   {executable: "agent", args: []string{"models"}, parse: parseCursorModels},
	"opencode": {executable: "opencode", args: []string{"models"}, parse: parseOpenCodeModels},
}

var (
	modelListTimeout = 20 * time.Second
	modelCacheTTL    = 5 * time.Minute
	modelOutputLimit = 4 << 20

	modelCacheMu sync.Mutex
	modelCache   = map[string]cachedCatalog{}
)

type cachedCatalog struct {
	at      time.Time
	catalog Catalog
}

// ListModels reports the models and efforts the harness CLI installed on this
// host supports, asking the CLI in the daemon's environment. Successful
// answers are cached for modelCacheTTL; refresh bypasses the cache. Only an
// unknown harness is an error; CLI failures are reported in Catalog.Error.
func ListModels(ctx context.Context, harnessType string, refresh bool) (Catalog, error) {
	switch harnessType {
	case "claude":
		catalog := claudeCatalog
		catalog.Harness = harnessType
		return catalog, nil
	case "stub":
		return Catalog{Harness: harnessType, Models: []ModelInfo{}, Efforts: []string{}}, nil
	}
	source, ok := modelSources[harnessType]
	if !ok {
		return Catalog{}, fmt.Errorf("unknown harness %q (want claude|codex|opencode|cursor|stub)", harnessType)
	}

	modelCacheMu.Lock()
	cached, hit := modelCache[harnessType]
	modelCacheMu.Unlock()
	if hit && !refresh && time.Since(cached.at) < modelCacheTTL {
		return cached.catalog, nil
	}

	catalog, err := runModelSource(ctx, source)
	if err != nil {
		return Catalog{Harness: harnessType, Models: []ModelInfo{}, Efforts: []string{}, Error: err.Error()}, nil
	}
	catalog.Harness = harnessType
	if catalog.Efforts == nil {
		catalog.Efforts = []string{}
	}
	modelCacheMu.Lock()
	modelCache[harnessType] = cachedCatalog{at: time.Now(), catalog: catalog}
	modelCacheMu.Unlock()
	return catalog, nil
}

func runModelSource(ctx context.Context, source modelSource) (Catalog, error) {
	executable, err := FindExecutable(source.executable, os.Environ(), "")
	if err != nil {
		return Catalog{}, fmt.Errorf("%s not found on PATH", source.executable)
	}
	ctx, cancel := context.WithTimeout(ctx, modelListTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, source.args...)
	cmd.Env = os.Environ()
	cmd.WaitDelay = time.Second
	stdout := &boundedOutput{remaining: modelOutputLimit}
	stderr := &boundedOutput{remaining: 4 << 10}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	command := source.executable + " " + strings.Join(source.args, " ")
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Catalog{}, fmt.Errorf("%s timed out after %s", command, modelListTimeout)
		}
		if detail := strings.TrimSpace(stripANSI(stderr.buf.String())); detail != "" {
			return Catalog{}, fmt.Errorf("%s failed: %s", command, detail)
		}
		return Catalog{}, fmt.Errorf("%s failed: %w", command, err)
	}
	catalog, err := source.parse(stdout.buf.Bytes())
	if err != nil {
		return Catalog{}, fmt.Errorf("%s: %w", command, err)
	}
	return catalog, nil
}

func parseCodexModels(raw []byte) (Catalog, error) {
	var doc struct {
		Models []struct {
			Slug        string `json:"slug"`
			DisplayName string `json:"display_name"`
			Visibility  string `json:"visibility"`
			Levels      []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Catalog{}, fmt.Errorf("unrecognized model catalog: %w", err)
	}
	catalog := Catalog{Efforts: []string{}}
	seenEffort := map[string]bool{}
	for _, model := range doc.Models {
		if model.Slug == "" || model.Visibility != "list" {
			continue
		}
		info := ModelInfo{ID: model.Slug, Label: model.DisplayName}
		for _, level := range model.Levels {
			if level.Effort == "" {
				continue
			}
			info.Efforts = append(info.Efforts, level.Effort)
			if !seenEffort[level.Effort] {
				seenEffort[level.Effort] = true
				catalog.Efforts = append(catalog.Efforts, level.Effort)
			}
		}
		catalog.Models = append(catalog.Models, info)
	}
	if len(catalog.Models) == 0 {
		return Catalog{}, errors.New("no listed models in the catalog")
	}
	return catalog, nil
}

var (
	ansiEscape       = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	cursorModelLine  = regexp.MustCompile(`^(\S+) - (.+)$`)
	cursorLabelMarks = regexp.MustCompile(`(\s*\((current|default)\))+$`)
	openCodeModel    = regexp.MustCompile(`^[A-Za-z0-9._-]+/\S+$`)
)

func stripANSI(text string) string { return ansiEscape.ReplaceAllString(text, "") }

// parseCursorModels reads `agent models`: "id - Name" lines, where the name
// may end with "(current)" or "(default)" markers.
func parseCursorModels(raw []byte) (Catalog, error) {
	catalog := Catalog{Efforts: []string{}}
	for line := range strings.SplitSeq(stripANSI(string(raw)), "\n") {
		match := cursorModelLine.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		label := strings.TrimSpace(cursorLabelMarks.ReplaceAllString(match[2], ""))
		catalog.Models = append(catalog.Models, ModelInfo{ID: match[1], Label: label})
	}
	if len(catalog.Models) == 0 {
		return Catalog{}, errors.New("no models in the output")
	}
	return catalog, nil
}

// parseOpenCodeModels reads `opencode models`: one provider/model per line.
func parseOpenCodeModels(raw []byte) (Catalog, error) {
	catalog := Catalog{Efforts: []string{}}
	for line := range strings.SplitSeq(stripANSI(string(raw)), "\n") {
		line = strings.TrimSpace(line)
		if openCodeModel.MatchString(line) {
			catalog.Models = append(catalog.Models, ModelInfo{ID: line})
		}
	}
	if len(catalog.Models) == 0 {
		return Catalog{}, errors.New("no models in the output")
	}
	return catalog, nil
}
