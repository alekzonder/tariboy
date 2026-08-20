package supportbundle

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

const sensitiveKeyPattern = `(?:[a-z][a-z0-9]*[_-])*(?:api[_-]?key|access[_-]?key|secret(?:[_-]?key)?|token|password|passwd|credential)`

var (
	authorizationPattern = regexp.MustCompile(`(?i)(["']?authorization["']?\s*:\s*["']?(?:bearer|basic)\s+)[^"'\s,;}]+`)
	assignmentPattern    = regexp.MustCompile(`(?i)\b(` + sensitiveKeyPattern + `)\s*([=:]\s*)("[^"]*"|'[^']*'|[^\s,;]+)`)
	jsonSecretPattern    = regexp.MustCompile(`(?i)(["']` + sensitiveKeyPattern + `["']\s*:\s*["'])[^"']*`)
	urlUserinfoPattern   = regexp.MustCompile(`(https?://)[^/@\s]+@`)
	querySecretPattern   = regexp.MustCompile(`(?i)([?&](?:api[_-]?key|access[_-]?key|secret|token|password)=)[^&#\s]+`)
	providerTokenPattern = regexp.MustCompile(`\b(?:sk|ghp|github_pat|xox[baprs])[-_][A-Za-z0-9_-]{8,}\b`)
)

func redactText(body []byte, roots, environ []string) []byte {
	output := string(body)
	output = authorizationPattern.ReplaceAllString(output, `${1}<redacted>`)
	output = assignmentPattern.ReplaceAllString(output, `${1}${2}<redacted>`)
	output = jsonSecretPattern.ReplaceAllString(output, `${1}<redacted>`)
	output = urlUserinfoPattern.ReplaceAllString(output, `${1}<redacted>@`)
	output = querySecretPattern.ReplaceAllString(output, `${1}<redacted>`)
	output = providerTokenPattern.ReplaceAllString(output, "<redacted>")

	replacements := make([]string, 0, len(roots)+len(environ))
	for _, root := range roots {
		if root != "" {
			replacements = append(replacements, root)
		}
	}
	for _, item := range environ {
		_, value, ok := strings.Cut(item, "=")
		if ok && len(value) >= 8 {
			replacements = append(replacements, value)
		}
	}
	sort.Slice(replacements, func(i, j int) bool {
		return len(replacements[i]) > len(replacements[j])
	})
	for _, value := range replacements {
		output = strings.ReplaceAll(output, value, "<redacted>")
	}
	output = redactHomePath(output, "/Users/")
	output = redactHomePath(output, "/home/")
	return []byte(output)
}

func redactHomePath(input, prefix string) string {
	output := input
	for {
		start := strings.Index(output, prefix)
		if start < 0 {
			return output
		}
		userStart := start + len(prefix)
		slash := strings.IndexByte(output[userStart:], '/')
		if slash < 0 {
			output = output[:start] + "$HOME"
			return output
		}
		userEnd := userStart + slash
		output = output[:start] + "$HOME" + output[userEnd:]
	}
}

func safeDaemonLog(body []byte) []byte {
	lines := strings.Split(string(body), "\n")
	accepted := make([]string, 0, MaxDaemonLogLines)
	for index := len(lines) - 1; index >= 0 && len(accepted) < MaxDaemonLogLines; index-- {
		line := safeLifecycleLine(lines[index])
		if line == "" {
			continue
		}
		if totalLineBytes(accepted)+len(line)+1 > MaxDaemonLogBytes {
			break
		}
		accepted = append(accepted, line)
	}
	for left, right := 0, len(accepted)-1; left < right; left, right = left+1, right-1 {
		accepted[left], accepted[right] = accepted[right], accepted[left]
	}
	if len(accepted) == 0 {
		return nil
	}
	return []byte(strings.Join(accepted, "\n") + "\n")
}

func totalLineBytes(lines []string) int {
	total := 0
	for _, line := range lines {
		total += len(line) + 1
	}
	return total
}

func safeLifecycleLine(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 3 || !safeTimestamp(fields[0]) {
		return ""
	}
	component := strings.ToLower(fields[1])
	if component != "daemon" && component != "desktop" && component != "tunnel" && component != "host" {
		return ""
	}
	lifecycle := map[string]bool{
		"started": true, "starting": true, "ready": true, "stopped": true,
		"down": true, "failed": true, "connected": true, "disconnected": true,
		"connecting": true, "restarting": true, "adopted": true,
	}
	output := []string{fields[0], component}
	hasLifecycle := false
	for _, field := range fields[2:] {
		lower := strings.ToLower(strings.Trim(field, `"'(),`))
		if lifecycle[lower] {
			hasLifecycle = true
			output = append(output, lower)
			continue
		}
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(key)
		value = strings.Trim(value, `"'(),`)
		switch key {
		case "code", "error_code":
			if safeCode(value) {
				output = append(output, key+"="+strings.ToLower(value))
			}
		case "state", "phase":
			if lifecycle[strings.ToLower(value)] {
				output = append(output, key+"="+strings.ToLower(value))
			}
		}
	}
	if !hasLifecycle {
		return ""
	}
	return strings.Join(output, " ")
}

func safeTimestamp(value string) bool {
	if len(value) < len("2006-01-02T15:04:05Z") || len(value) > 35 {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func safeCode(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') &&
			!(char >= '0' && char <= '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}
