package commands

import "github.com/alekzonder/tariboy/internal/teamportable"

// Response types of the group.go and team.go commands. Each command declares
// its type with registry.WithResult, which derives the OpenAPI result schema
// from it.

// GroupView is one group's lead, members, channels and shared directory
// (group.inspect, group.create, group.rename, group.lead.set).
type GroupView struct {
	Name      string   `json:"name"`
	Lead      string   `json:"lead"`
	Members   []string `json:"members"`
	Broadcast string   `json:"broadcast"`
	Inbox     string   `json:"inbox"`
	SharedDir string   `json:"shared_dir"`
}

// groupViewFrom reads the map the group control returns.
func groupViewFrom(m map[string]any) GroupView {
	view := GroupView{}
	view.Name, _ = m["name"].(string)
	view.Lead, _ = m["lead"].(string)
	view.Members, _ = m["members"].([]string)
	view.Broadcast, _ = m["broadcast"].(string)
	view.Inbox, _ = m["inbox"].(string)
	view.SharedDir, _ = m["shared_dir"].(string)
	return view
}

// GroupRow is one group in the group.ls list; Members counts its agents.
type GroupRow struct {
	Name    string `json:"name"`
	Lead    string `json:"lead"`
	Members int    `json:"members"`
}

// GroupList is the group.ls result.
type GroupList struct {
	Groups []GroupRow `json:"groups"`
	Count  int        `json:"count"`
}

// GroupRemoveResult is the group.rm result.
type GroupRemoveResult struct {
	Removed string `json:"removed"`
	Volumes bool   `json:"volumes"`
}

// GroupAssignment is an agent's group after group.assign or group.member.rm;
// an empty group means the agent left it.
type GroupAssignment struct {
	Agent string `json:"agent"`
	Group string `json:"group"`
}

// TeamCompose is a group rendered as tariboy-compose.yaml (team.compose).
type TeamCompose struct {
	Name string `json:"name"`
	YAML string `json:"yaml"`
}

// TeamImportEntryPlan is the planned action for one agent or group of a team
// import, with a message when it conflicts.
type TeamImportEntryPlan struct {
	Name     string `json:"name"`
	Action   string `json:"action"`
	Conflict bool   `json:"conflict"`
	Message  string `json:"message"`
}

// TeamImportImagePlan is the planned action for one image of a team import.
// Imports are compose-only, so the plan lists no images today.
type TeamImportImagePlan struct {
	Ref      string `json:"ref"`
	Action   string `json:"action"`
	Conflict bool   `json:"conflict"`
	Message  string `json:"message"`
}

// TeamImportPlan is the preview of a staged team import (team.import.yaml).
type TeamImportPlan struct {
	ImportID string                `json:"import_id"`
	Team     string                `json:"team"`
	YAML     string                `json:"yaml"`
	Images   []TeamImportImagePlan `json:"images"`
	Agents   []TeamImportEntryPlan `json:"agents"`
	Groups   []TeamImportEntryPlan `json:"groups"`
}

// TeamApplyAgent is the outcome for one agent of an applied team: created,
// reused or failed with an error.
type TeamApplyAgent struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// TeamApplyResult is the outcome of applying team compose YAML; Groups counts
// the groups created or updated.
type TeamApplyResult struct {
	Groups   int              `json:"groups"`
	Agents   []TeamApplyAgent `json:"agents"`
	Complete bool             `json:"complete"`
}

// TeamImportStep is one recorded step of a team import operation.
type TeamImportStep struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	SourceReady bool   `json:"source_ready,omitempty"`
}

// TeamImportOperation is the persisted progress of a team import
// (team.import.archive.status).
type TeamImportOperation struct {
	ImportID       string            `json:"import_id"`
	Team           string            `json:"team"`
	Status         string            `json:"status"`
	Steps          []TeamImportStep  `json:"steps" jsonschema_extras:"nullable=true"`
	Error          string            `json:"error,omitempty"`
	Updated        string            `json:"updated_at"`
	ComposeYAML    string            `json:"compose_yaml,omitempty"`
	ResolvedRefs   map[string]string `json:"resolved_refs,omitempty"`
	UpdateExisting bool              `json:"update_existing,omitempty"`
}

func teamImportOperationFrom(operation teamportable.Operation) TeamImportOperation {
	var steps []TeamImportStep
	if operation.Steps != nil {
		steps = make([]TeamImportStep, 0, len(operation.Steps))
		for _, step := range operation.Steps {
			steps = append(steps, TeamImportStep(step))
		}
	}
	return TeamImportOperation{
		ImportID: operation.ImportID, Team: operation.Team, Status: operation.Status, Steps: steps,
		Error: operation.Error, Updated: operation.Updated, ComposeYAML: operation.ComposeYAML,
		ResolvedRefs: operation.ResolvedRefs, UpdateExisting: operation.UpdateExisting,
	}
}

// TeamImportApplyResult is the team.import.archive.apply result: the applied
// team and the saved import operation.
type TeamImportApplyResult struct {
	TeamApplyResult
	ImportID  string              `json:"import_id"`
	Operation TeamImportOperation `json:"operation"`
}
