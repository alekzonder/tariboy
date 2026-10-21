package commands

import (
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/compose"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/teamportable"
	"gopkg.in/yaml.v3"
)

func nestedMap(value any) map[string]any { result, _ := value.(map[string]any); return result }

func rewriteTeamComposeRefs(input []byte, refs map[string]string) ([]byte, error) {
	file, err := compose.Parse(input)
	if err != nil {
		return nil, err
	}
	for name, spec := range file.Agents {
		if replacement := refs[spec.Image]; replacement != "" {
			spec.Image = replacement
			file.Agents[name] = spec
		}
	}
	return yaml.Marshal(file)
}

func registryError(code string, err error) error { return api.UserError{Code: code, Msg: err.Error()} }

func teamCompose() registry.Command {
	return registry.Command{Path: "team.compose", Summary: "Render a group as tariboy-compose.yaml", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/groups/{name}/compose"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		name := p.String("name")
		if err := checkGroupName(name); err != nil {
			return nil, err
		}
		gc, err := requireGroups(c)
		if err != nil {
			return nil, err
		}
		view, err := gc.Inspect(name)
		if err != nil {
			return nil, registryError("not_found", err)
		}
		lead, _ := view["lead"].(string)
		agents, err := agent.NewStore(c.Store).ListByGroup(name)
		if err != nil {
			return nil, err
		}
		file := compose.File{Version: 1, Images: map[string]compose.ImageSpec{}, Groups: map[string]compose.GroupSpec{name: {Lead: lead}}, Agents: map[string]compose.AgentSpec{}}
		for _, member := range agents {
			ref, err := image.ParseRef(member.ImageRef)
			if err != nil {
				return nil, err
			}
			file.Images[ref.Name] = compose.ImageSpec{Context: "./images/" + ref.Name}
			enabled := member.Enabled
			timeout := ""
			if member.TimeoutS > 0 {
				timeout = (time.Duration(member.TimeoutS) * time.Second).String()
			}
			file.Agents[member.Name] = compose.AgentSpec{Image: member.ImageRef, Group: name, Cwd: member.Cwd, Harness: &compose.HarnessSpec{Type: member.HarnessType, Model: member.Model, Effort: member.Effort, Interactive: member.Interactive}, Plugins: append([]string(nil), member.Plugins...), Loop: &compose.LoopSpec{Enabled: &enabled}, Timeout: timeout}
		}
		raw, err := yaml.Marshal(file)
		if err != nil {
			return nil, err
		}
		return map[string]any{"name": name, "yaml": string(raw)}, nil
	}}
}

func teamApplyYAML() registry.Command {
	return registry.Command{Path: "team.import.yaml.apply-internal", Summary: "Apply validated team compose YAML", Args: []registry.Arg{{Name: "yaml", Type: registry.String, Required: true}}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		file, err := compose.Parse([]byte(p.String("yaml")))
		if err != nil {
			return nil, registryError("bad_compose", err)
		}
		if err := file.Validate(); err != nil {
			return nil, registryError("bad_compose", err)
		}
		for _, spec := range file.Agents {
			ref, err := image.ParseRef(spec.Image)
			if err != nil || !imageStore(c).Exists(ref) {
				return nil, api.UserError{Code: "image_missing", Msg: "import or build image " + spec.Image + " first"}
			}
		}
		gc, err := requireGroups(c)
		if err != nil {
			return nil, err
		}
		for name, group := range file.Groups {
			if _, err := gc.Create(name, group.Lead); err != nil {
				return nil, registryError("create_failed", err)
			}
		}
		results := make([]map[string]any, 0, len(file.Agents))
		for name, spec := range file.Agents {
			if existing, getErr := agent.NewStore(c.Store).Get(name); getErr == nil {
				if existing.Group != spec.Group || existing.ImageRef != spec.Image {
					results = append(results, map[string]any{"name": name, "status": "failed", "error": "existing agent has different group or image"})
				} else {
					results = append(results, map[string]any{"name": name, "status": "reused"})
				}
				continue
			}
			harness, model, effort, interactive := "", "", "", false
			if spec.Harness != nil {
				harness, model, effort, interactive = spec.Harness.Type, spec.Harness.Model, spec.Harness.Effort, spec.Harness.Interactive
			}
			timeoutS, timeoutErr := spec.TimeoutSeconds()
			created, runErr := "", timeoutErr
			if runErr == nil {
				created, runErr = c.Control.Run(registry.RunSpec{ImageRef: spec.Image, Name: name, Cwd: spec.Cwd, Harness: harness, Model: model, Effort: effort, Interactive: interactive, Plugins: spec.Plugins, TimeoutS: timeoutS, Group: spec.Group})
			}
			item := map[string]any{"name": created, "status": "created"}
			if runErr != nil {
				item["name"] = name
				item["status"] = "failed"
				item["error"] = runErr.Error()
			}
			results = append(results, item)
		}
		complete := true
		for _, item := range results {
			if item["status"] == "failed" {
				complete = false
			}
		}
		return map[string]any{"groups": len(file.Groups), "agents": results, "complete": complete}, nil
	}}
}

func teamImportYAML() registry.Command {
	return registry.Command{Path: "team.import.yaml", Summary: "Preview pasted tariboy-compose.yaml", Args: []registry.Arg{{Name: "yaml", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/team-imports/preview-yaml"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		raw := []byte(p.String("yaml"))
		file, err := compose.Parse(raw)
		if err == nil {
			err = file.Validate()
		}
		if err != nil {
			return nil, registryError("bad_compose", err)
		}
		if len(file.Groups) != 1 {
			return nil, api.UserError{Code: "bad_compose", Msg: "team import requires exactly one group"}
		}
		team := ""
		for name := range file.Groups {
			team = name
		}
		service := teamportable.Service{StagingRoot: filepath.Join(c.BaseDir, "team-imports")}
		preview, err := service.PreviewYAML(team, raw)
		if err != nil {
			return nil, registryError("preview_failed", err)
		}
		return planTeamImport(c, preview)
	}}
}

func planTeamImport(c *registry.Ctx, preview teamportable.Preview) (any, error) {
	file, err := compose.Parse(preview.ComposeYAML)
	if err == nil {
		err = file.Validate()
	}
	if err != nil {
		return nil, registryError("bad_compose", err)
	}
	agentPlans := make([]map[string]any, 0, len(file.Agents))
	for name, spec := range file.Agents {
		action, conflict, message := "create", false, ""
		parsedRef, refErr := image.ParseRef(spec.Image)
		if refErr != nil || !imageStore(c).Exists(parsedRef) {
			action, conflict, message = "blocked", true, "required image is unavailable; import/build it or edit YAML"
		} else if existing, getErr := agent.NewStore(c.Store).Get(name); getErr == nil {
			if existing.Group == spec.Group && existing.ImageRef == spec.Image {
				action = "reuse"
			} else {
				action, conflict, message = "rename", true, "destination agent has different group or image"
			}
		}
		agentPlans = append(agentPlans, map[string]any{"name": name, "action": action, "conflict": conflict, "message": message})
	}
	groupPlans := make([]map[string]any, 0, len(file.Groups))
	gc, groupErr := requireGroups(c)
	if groupErr != nil {
		return nil, groupErr
	}
	for name := range file.Groups {
		action, conflict, message := "create", false, ""
		if _, inspectErr := gc.Inspect(name); inspectErr == nil {
			action, conflict, message = "choose", true, "destination team exists; rename it in YAML or explicitly update it"
		}
		groupPlans = append(groupPlans, map[string]any{"name": name, "action": action, "conflict": conflict, "message": message})
	}
	return map[string]any{"import_id": preview.ImportID, "team": preview.Team, "yaml": string(preview.ComposeYAML), "images": []map[string]any{}, "agents": agentPlans, "groups": groupPlans}, nil
}

func teamImportArchivePlan() registry.Command {
	return registry.Command{Path: "team.import.archive.plan", Summary: "Validate and plan a staged portable team import", Args: []registry.Arg{{Name: "id", Type: registry.String, Required: true}}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		service := teamportable.Service{StagingRoot: filepath.Join(c.BaseDir, "team-imports")}
		preview, err := service.Load(p.String("id"))
		if err != nil {
			return nil, registryError("import_not_found", err)
		}
		return planTeamImport(c, preview)
	}}
}

func teamImportArchiveApply() registry.Command {
	return registry.Command{Path: "team.import.archive.apply", Summary: "Build images and create a staged portable team", Args: []registry.Arg{{Name: "id", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/team-imports/{id}/apply"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		service := teamportable.Service{StagingRoot: filepath.Join(c.BaseDir, "team-imports")}
		preview, err := service.Load(p.String("id"))
		if err != nil {
			return nil, registryError("import_not_found", err)
		}
		operation, err := service.Operation(preview.ImportID)
		if err != nil {
			return nil, registryError("import_not_found", err)
		}
		editedYAML := p.String("yaml")
		if editedYAML == "" {
			editedYAML = operation.ComposeYAML
		}
		if editedYAML != "" {
			file, parseErr := compose.Parse([]byte(editedYAML))
			if parseErr == nil {
				parseErr = file.Validate()
			}
			if parseErr != nil {
				return nil, registryError("bad_compose", parseErr)
			}
			preview.ComposeYAML = []byte(editedYAML)
		}
		resolvedRefs := map[string]string{}
		for original, replacement := range operation.ResolvedRefs {
			resolvedRefs[original] = replacement
		}
		for original, value := range nestedMap(p.Params["refs"]) {
			if replacement, ok := value.(string); ok && replacement != "" {
				resolvedRefs[original] = replacement
			}
		}
		if len(resolvedRefs) > 0 {
			preview.ComposeYAML, err = rewriteTeamComposeRefs(preview.ComposeYAML, resolvedRefs)
			if err != nil {
				return nil, registryError("bad_compose", err)
			}
		}
		updateExisting := operation.UpdateExisting
		if value, ok := p.OptBool("update_existing"); ok {
			updateExisting = value
		}
		plannedResult, planErr := planTeamImport(c, preview)
		if planErr != nil {
			return nil, planErr
		}
		if planned, ok := plannedResult.(map[string]any); ok {
			if groups, ok := planned["groups"].([]map[string]any); ok {
				for _, group := range groups {
					if conflict, _ := group["conflict"].(bool); conflict && !updateExisting {
						return nil, api.UserError{Code: "team_conflict", Msg: "destination team exists; rename it in YAML or choose update existing"}
					}
				}
			}
			if agents, ok := planned["agents"].([]map[string]any); ok {
				for _, candidate := range agents {
					if conflict, _ := candidate["conflict"].(bool); conflict {
						return nil, api.UserError{Code: "agent_conflict", Msg: fmt.Sprint(candidate["name"]) + ": " + fmt.Sprint(candidate["message"])}
					}
				}
			}
		}
		operation.ComposeYAML = string(preview.ComposeYAML)
		operation.ResolvedRefs = resolvedRefs
		operation.UpdateExisting = updateExisting
		operation.Status, operation.Error = "running", ""
		_ = service.SaveOperation(operation)
		if err := service.TouchLease(preview.ImportID); err != nil {
			return nil, err
		}
		defer service.RemoveLease(preview.ImportID)
		heartbeatDone := make(chan struct{})
		defer close(heartbeatDone)
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					_ = service.TouchLease(preview.ImportID)
				case <-heartbeatDone:
					return
				}
			}
		}()
		result, err := registry.Call(c, p.Ctx, teamApplyYAML(), map[string]any{"yaml": string(preview.ComposeYAML)})
		if err != nil {
			operation.Status, operation.Error = "failed", err.Error()
			_ = service.SaveOperation(operation)
			return nil, err
		}
		if mapped, ok := result.(map[string]any); ok {
			mapped["import_id"] = preview.ImportID
			if agents, ok := mapped["agents"].([]map[string]any); ok {
				for _, item := range agents {
					step := teamportable.OperationStep{Kind: "agent", Name: fmt.Sprint(item["name"]), Status: fmt.Sprint(item["status"])}
					if itemError, ok := item["error"].(string); ok {
						step.Error = itemError
					}
					operation.Steps = append(operation.Steps, step)
				}
			}
			if complete, _ := mapped["complete"].(bool); complete {
				operation.Status = "complete"
			} else {
				operation.Status = "failed"
			}
			_ = service.SaveOperation(operation)
			mapped["operation"] = operation
		}
		return result, nil
	}}
}

func teamImportArchiveStatus() registry.Command {
	return registry.Command{Path: "team.import.archive.status", Summary: "Inspect portable team import progress", Args: []registry.Arg{{Name: "id", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/team-imports/{id}"}, Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
		service := teamportable.Service{StagingRoot: filepath.Join(c.BaseDir, "team-imports")}
		operation, err := service.RecoverOperation(p.String("id"), time.Now(), 5*time.Minute)
		if err != nil {
			return nil, registryError("import_not_found", err)
		}
		return operation, nil
	}}
}
