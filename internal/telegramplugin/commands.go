package telegramplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var managementHelp = []string{
	"/help", "/agents", "/agent create NAME IMAGE", "/agent show NAME", "/agent set NAME FIELD VALUE",
	"/start NAME", "/stop NAME", "/kill NAME", "/tasks [NAME]", "/task show KEY",
	"/task create QUEUE TITLE", "/task assign KEY AGENT", "/task status KEY open|in_progress|done|cancelled",
	"/task comment KEY TEXT",
}

var agentHelp = []string{
	"/help", "/start", "/stop", "/kill", "/tasks", "/task show KEY",
	"/task create QUEUE TITLE", "/task assign KEY", "/task status KEY open|in_progress|done|cancelled",
	"/task comment KEY TEXT",
}

func (s *Server) handleCommand(ctx context.Context, message *TelegramMessage, agent string, updateID int64) (bool, error) {
	if !strings.HasPrefix(strings.TrimSpace(message.Text), "/") {
		return false, nil
	}
	text := strings.TrimSpace(message.Text)
	fields := strings.Fields(text)
	command := strings.TrimPrefix(fields[0], "/")
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	args := fields[1:]
	response, err := s.runCommand(ctx, command, args, text, agent, updateID)
	if err != nil {
		response = "Error: " + err.Error()
	}
	state := s.state.Snapshot()
	if _, err := s.bot.SendMessage(ctx, state.Token, state.ChatID, message.MessageThreadID, response); err != nil {
		return true, err
	}
	return true, nil
}

func (s *Server) runCommand(ctx context.Context, command string, args []string, raw, agent string, updateID int64) (string, error) {
	if command == "help" {
		if agent == "" {
			return strings.Join(managementHelp, "\n"), nil
		}
		return strings.Join(agentHelp, "\n"), nil
	}
	if s.daemon == nil {
		return "", fmt.Errorf("daemon API unavailable")
	}
	switch command {
	case "agents":
		if agent != "" {
			break
		}
		return s.callText(ctx, http.MethodGet, "/api/agents", nil)
	case "agent":
		if agent != "" || len(args) < 2 {
			break
		}
		switch args[0] {
		case "create":
			if len(args) != 3 {
				break
			}
			return s.callText(ctx, http.MethodPost, "/api/agents", map[string]any{"name": args[1], "image": args[2], "loop": true})
		case "show":
			if len(args) != 2 {
				break
			}
			return s.callText(ctx, http.MethodGet, "/api/agents/"+url.PathEscape(args[1]), nil)
		case "set":
			parts := strings.SplitN(raw, " ", 5)
			if len(parts) != 5 {
				break
			}
			return s.setAgent(ctx, parts[2], parts[3], parts[4])
		}
	case "start", "stop", "kill":
		target := agent
		if target == "" && len(args) == 1 {
			target = args[0]
		}
		if target == "" || agent != "" && len(args) != 0 {
			break
		}
		return s.callText(ctx, http.MethodPost, "/api/agents/"+url.PathEscape(target)+"/"+command, map[string]any{"name": target})
	case "tasks":
		target := agent
		if target == "" && len(args) == 1 {
			target = args[0]
		} else if len(args) > 0 {
			break
		}
		path := "/api/tasks"
		if target != "" {
			path += "?scope_agent=" + url.QueryEscape(target)
		}
		return s.callText(ctx, http.MethodGet, path, nil)
	case "task":
		return s.taskCommand(ctx, args, raw, agent, updateID)
	}
	return "Unknown or invalid command. Use /help.", nil
}

func (s *Server) setAgent(ctx context.Context, agent, field, value string) (string, error) {
	routeField, bodyKey, bodyValue := field, "value", any(value)
	switch field {
	case "alias", "harness", "model", "effort", "cwd":
	case "image":
		bodyKey = "image"
	case "interactive":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return "", fmt.Errorf("interactive must be true or false")
		}
		bodyValue = parsed
	case "loop":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return "", fmt.Errorf("loop must be true or false")
		}
		verb := "disable"
		if parsed {
			verb = "enable"
		}
		return s.callText(ctx, http.MethodPost, "/api/agents/"+url.PathEscape(agent)+"/loop/"+verb, map[string]any{"name": agent})
	case "interval", "timeout", "hard-timeout", "max-idle":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return "", fmt.Errorf("%s must be a non-negative integer", field)
		}
		bodyValue = n
		routeField = "loop/" + field
	case "on-timeout", "on-error":
		if value != "restart" && value != "stop" {
			return "", fmt.Errorf("%s must be restart or stop", field)
		}
		routeField = "loop/" + field
	default:
		return "", fmt.Errorf("field is not editable from Telegram")
	}
	body := map[string]any{"name": agent, bodyKey: bodyValue}
	return s.callText(ctx, http.MethodPost, "/api/agents/"+url.PathEscape(agent)+"/"+routeField, body)
}

func (s *Server) taskCommand(ctx context.Context, args []string, raw, agent string, updateID int64) (string, error) {
	if len(args) < 2 {
		return "Unknown or invalid command. Use /help.", nil
	}
	verb, key := args[0], args[1]
	switch verb {
	case "show":
		if len(args) != 2 {
			break
		}
		return s.callText(ctx, http.MethodGet, "/api/tasks/"+url.PathEscape(key), nil)
	case "create":
		parts := strings.SplitN(raw, " ", 4)
		if len(parts) != 4 {
			break
		}
		body := map[string]any{"queue": key, "title": parts[3], "idempotency_key": "telegram:" + strconv.FormatInt(updateID, 10)}
		if agent != "" {
			body["assignee"] = agent
		}
		return s.callText(ctx, http.MethodPost, "/api/tasks", body)
	case "assign":
		target := agent
		if target == "" && len(args) == 3 {
			target = args[2]
		}
		if target == "" || agent != "" && len(args) != 2 {
			break
		}
		return s.updateTask(ctx, key, map[string]any{"assignee": target})
	case "status":
		if len(args) != 3 || !validTaskStatus(args[2]) {
			break
		}
		return s.updateTask(ctx, key, map[string]any{"status": args[2]})
	case "comment":
		parts := strings.SplitN(raw, " ", 4)
		if len(parts) != 4 {
			break
		}
		return s.callText(ctx, http.MethodPost, "/api/tasks/"+url.PathEscape(key)+"/comments", map[string]any{
			"body": parts[3], "idempotency_key": "telegram:" + strconv.FormatInt(updateID, 10),
		})
	}
	return "Unknown or invalid command. Use /help.", nil
}

func (s *Server) updateTask(ctx context.Context, key string, change map[string]any) (string, error) {
	var task struct {
		Revision int64 `json:"revision"`
	}
	if err := s.daemon.Call(ctx, http.MethodGet, "/api/tasks/"+url.PathEscape(key), nil, &task); err != nil {
		return "", err
	}
	change["revision"] = task.Revision
	return s.callText(ctx, http.MethodPatch, "/api/tasks/"+url.PathEscape(key), change)
}

func (s *Server) callText(ctx context.Context, method, path string, body any) (string, error) {
	var result any
	if err := s.daemon.Call(ctx, method, path, body, &result); err != nil {
		return "", err
	}
	if result == nil {
		return "OK", nil
	}
	encoded, _ := json.Marshal(result)
	return string(encoded), nil
}

func validTaskStatus(status string) bool {
	return status == "open" || status == "in_progress" || status == "done" || status == "cancelled"
}
