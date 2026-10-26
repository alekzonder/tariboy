package commands

import (
	"reflect"

	"github.com/alekzonder/tariboy/internal/filebrowser"
	"github.com/alekzonder/tariboy/internal/fsbrowser"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/invopop/jsonschema"
)

// Response types of the agent setting, loop, prompt, file browser,
// interactive session and shell script commands. Each command declares its
// type with registry.WithResult, which derives the OpenAPI result schema.

// AgentCwdResult is the result of agent.cwd: the configured cwd, empty when
// the agent uses its managed workdir.
type AgentCwdResult struct {
	Name string `json:"name"`
	Cwd  string `json:"cwd"`
}

// AgentModelResult is the result of agent.model.
type AgentModelResult struct {
	Name  string `json:"name"`
	Model string `json:"model"`
}

// AgentEffortResult is the result of agent.effort.
type AgentEffortResult struct {
	Name   string `json:"name"`
	Effort string `json:"effort"`
}

// AgentHarnessResult is the result of agent.harness.
type AgentHarnessResult struct {
	Name    string `json:"name"`
	Harness string `json:"harness"`
}

// AgentInteractiveResult is the result of agent.interactive.
type AgentInteractiveResult struct {
	Name        string `json:"name"`
	Interactive bool   `json:"interactive"`
}

// AgentEnvResult is the result of agent.env.
type AgentEnvResult struct {
	Name string            `json:"name"`
	Env  map[string]string `json:"env"`
}

// AgentAliasResult is the result of agent.alias.get and agent.alias.set.
type AgentAliasResult struct {
	Name  string `json:"name"`
	Alias string `json:"alias"`
}

// AgentNotesResult is the result of agent.notes.get and agent.notes.set.
type AgentNotesResult struct {
	Name  string `json:"name"`
	Notes string `json:"notes"`
}

// AgentColorResult is the result of agent.color.get and agent.color.set: a
// #rrggbb color, empty when unset.
type AgentColorResult struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// AgentSavedResult is the result of user-prompt.set and context.set.
type AgentSavedResult struct {
	Name  string `json:"name"`
	Saved bool   `json:"saved"`
}

// AgentUserPromptResult is the result of user-prompt.get.
type AgentUserPromptResult struct {
	Name       string `json:"name"`
	UserPrompt string `json:"user_prompt"`
}

// AgentContextResult is the result of context.get: CONTEXT.md, empty when
// the file is missing.
type AgentContextResult struct {
	Name    string `json:"name"`
	Context string `json:"context"`
}

// AgentPromptLayers lists the layers of a prompt preview: the v1 manifest
// layers (image.Layer), or the v2 prompt template entries
// (image.TemplateEntry); empty for a bare image and null for a v1 manifest
// without layers.
type AgentPromptLayers []any

// JSONSchemaExtend describes each item as one of the two layer shapes. The
// component carries nullable itself: OpenAPI 3.0 ignores keywords next to a
// $ref.
func (AgentPromptLayers) JSONSchemaExtend(s *jsonschema.Schema) {
	s.Extras = map[string]any{"nullable": true}
	s.Items = &jsonschema.Schema{OneOf: []*jsonschema.Schema{
		inlineSchema(reflect.TypeFor[image.Layer]()),
		inlineSchema(reflect.TypeFor[image.TemplateEntry]()),
	}}
}

// inlineSchema reflects t as a schema without references or a $schema key.
func inlineSchema(t reflect.Type) *jsonschema.Schema {
	s := (&jsonschema.Reflector{DoNotReference: true}).ReflectFromType(t)
	s.Version, s.ID = "", ""
	return s
}

// AgentPromptPreview is the result of prompt.get.
type AgentPromptPreview struct {
	Name   string            `json:"name"`
	Prompt string            `json:"prompt"`
	Layers AgentPromptLayers `json:"layers"`
}

// AgentExecResult is the result of agent.exec; Status is the control's note.
type AgentExecResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// AgentScreenResult is the result of agent.screen.
type AgentScreenResult struct {
	Screen string `json:"screen"`
}

// AgentSendKeysResult is the result of agent.send-keys: Sent is the number of
// raw key items, or true for a line sent with Enter.
type AgentSendKeysResult struct {
	Name string `json:"name"`
	Sent any    `json:"sent" jsonschema:"oneof_type=integer;boolean"`
}

// ShellScriptResult is the result of a shell script read; empty when the
// script does not exist.
type ShellScriptResult struct {
	Script string `json:"script"`
}

// ShellScriptSaveResult is the result of a shell script write.
type ShellScriptSaveResult struct {
	Saved bool `json:"saved"`
}

// AgentStatusEvent is one status message of agent.status.history.
type AgentStatusEvent struct {
	TS          string `json:"ts"`
	Message     string `json:"message"`
	IterationID string `json:"iteration_id"`
}

// AgentStatusHistory is the result of agent.status.history, newest first.
type AgentStatusHistory struct {
	Events []AgentStatusEvent `json:"events"`
	Count  int                `json:"count"`
}

// AgentPullResult is the result of agent.pull: the file content in base64.
type AgentPullResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// AgentFileEntry is one directory entry of agent.file.list; it encodes as
// filebrowser.Entry.
type AgentFileEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"`
}

// AgentFileList is the result of agent.file.list, directories first.
type AgentFileList struct {
	Path    string           `json:"path"`
	Entries []AgentFileEntry `json:"entries"`
}

func newAgentFileList(path string, entries []filebrowser.Entry) AgentFileList {
	out := AgentFileList{Path: path, Entries: make([]AgentFileEntry, 0, len(entries))}
	for _, e := range entries {
		out.Entries = append(out.Entries, AgentFileEntry(e))
	}
	return out
}

// AgentFileContent is the result of agent.file.read; it encodes as
// filebrowser.Content. Content is present only for text.
type AgentFileContent struct {
	Path string        `json:"path"`
	Kind AgentFileKind `json:"kind"`
	Data string        `json:"content,omitempty"`
	Size int64         `json:"size"`
}

// AgentFileKind says whether a read file came back as text.
type AgentFileKind string

func (AgentFileKind) JSONSchemaExtend(s *jsonschema.Schema) {
	s.Enum = []any{"text", "binary", "too_large"}
}

// AgentFileSaved is the result of agent.file.write.
type AgentFileSaved struct {
	Path  string `json:"path"`
	Saved bool   `json:"saved"`
}

// AgentFileCreated is the result of agent.file.create.
type AgentFileCreated struct {
	Path    string `json:"path"`
	Created bool   `json:"created"`
}

// AgentFileDeleted is the result of agent.file.delete.
type AgentFileDeleted struct {
	Path    string `json:"path"`
	Deleted bool   `json:"deleted"`
}

// AgentFileRenamed is the result of agent.file.rename.
type AgentFileRenamed struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Renamed bool   `json:"renamed"`
}

// LoopScheduleResult is the result of loop.schedule; both values are empty
// without a schedule.
type LoopScheduleResult struct {
	Name           string `json:"name"`
	Schedule       string `json:"schedule"`
	ScheduleNextAt string `json:"schedule_next_at"`
}

// LoopIntervalResult is the result of loop.interval.
type LoopIntervalResult struct {
	Name      string `json:"name"`
	IntervalS int    `json:"interval_s"`
}

// LoopTimeoutResult is the result of loop.timeout.
type LoopTimeoutResult struct {
	Name     string `json:"name"`
	TimeoutS int    `json:"timeout_s"`
}

// LoopHardTimeoutResult is the result of loop.hard-timeout.
type LoopHardTimeoutResult struct {
	Name         string `json:"name"`
	HardTimeoutS int    `json:"hard_timeout_s"`
}

// LoopMaxIdleResult is the result of loop.max-idle.
type LoopMaxIdleResult struct {
	Name              string `json:"name"`
	MaxIdleIterations int    `json:"max_idle_iterations"`
}

// LoopAIStallTimeoutResult is the result of loop.ai-stall-timeout.
type LoopAIStallTimeoutResult struct {
	Name            string `json:"name"`
	AIStallTimeoutS int    `json:"ai_stall_timeout_s"`
}

// LoopOnTimeoutResult is the result of loop.on-timeout.
type LoopOnTimeoutResult struct {
	Name      string `json:"name"`
	OnTimeout string `json:"on_timeout"`
}

// LoopOnErrorResult is the result of loop.on-error.
type LoopOnErrorResult struct {
	Name    string `json:"name"`
	OnError string `json:"on_error"`
}

// FsEntry is one directory of fs.list; it encodes as fsbrowser.Entry. Dir is
// always true because only directories are listed.
type FsEntry struct {
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
}

// FsListing is the result of fs.list; it encodes as fsbrowser.Listing.
// Parent is empty when the listed directory is the filesystem root.
type FsListing struct {
	Path    string    `json:"path"`
	Parent  string    `json:"parent"`
	Entries []FsEntry `json:"entries"`
}

func newFsListing(l fsbrowser.Listing) FsListing {
	out := FsListing{Path: l.Path, Parent: l.Parent, Entries: make([]FsEntry, 0, len(l.Entries))}
	for _, e := range l.Entries {
		out.Entries = append(out.Entries, FsEntry(e))
	}
	return out
}

// FileUploadResult is the result of files.upload: the path relative to the
// base directory, the absolute path, and the decoded size in bytes.
type FileUploadResult struct {
	Path  string `json:"path"`
	Abs   string `json:"abs"`
	Bytes int    `json:"bytes"`
}
