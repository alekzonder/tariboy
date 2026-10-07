package loop

import "encoding/json"

type CursorResultUsage struct {
	Model            string
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	DurationMs       int
}

// cursorUsageObserver returns a harness-stdout hook that reports each result
// event's tokens. The row names the configured model id; with no model or
// "auto" it falls back to the display name from the system init event.
func cursorUsageObserver(configuredModel string, report func(CursorResultUsage)) func(string) {
	model := ""
	if configuredModel != "auto" {
		model = configuredModel
	}
	var displayName string
	return func(line string) {
		m, usage, ok := observeCursorStreamLine(line)
		if !ok {
			return
		}
		if usage == nil {
			displayName = m
			return
		}
		usage.Model = model
		if usage.Model == "" {
			usage.Model = displayName
		}
		report(*usage)
	}
}

func observeCursorStreamLine(line string) (model string, usage *CursorResultUsage, ok bool) {
	var ev struct {
		Type       string `json:"type"`
		Model      string `json:"model"`
		DurationMs int    `json:"duration_ms"`
		Usage      *struct {
			InputTokens      int `json:"inputTokens"`
			OutputTokens     int `json:"outputTokens"`
			CacheReadTokens  int `json:"cacheReadTokens"`
			CacheWriteTokens int `json:"cacheWriteTokens"`
		} `json:"usage"`
	}
	if json.Unmarshal([]byte(line), &ev) != nil {
		return "", nil, false
	}
	if ev.Type == "system" && ev.Model != "" {
		return ev.Model, nil, true
	}
	if ev.Type == "result" && ev.Usage != nil {
		return "", &CursorResultUsage{
			InputTokens:      ev.Usage.InputTokens,
			OutputTokens:     ev.Usage.OutputTokens,
			CacheReadTokens:  ev.Usage.CacheReadTokens,
			CacheWriteTokens: ev.Usage.CacheWriteTokens,
			DurationMs:       ev.DurationMs,
		}, true
	}
	return "", nil, false
}
