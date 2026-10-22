package loop

// Trigger says what started an iteration. The iteration row, events, audit
// payloads, and the runner's trigger argument all carry the same string.
type Trigger string

// The triggers of an iteration.
const (
	TriggerManual   Trigger = "manual"
	TriggerSchedule Trigger = "schedule"
	TriggerInterval Trigger = "interval"
	TriggerMessage  Trigger = "message"
)

// Phase is the point of an iteration lifecycle an iteration event reports.
type Phase string

// The phases of an iteration event.
const (
	PhaseStart  Phase = "start"
	PhaseFinish Phase = "finish"
)

// StopReason is why a stop policy halted a loop. It is recorded after the
// "halted: " prefix of the agent's error reason.
type StopReason string

// The stop reasons that name an iteration status; an image activation failure
// builds its reason from the error text.
const (
	StopReasonTimeout      StopReason = "timeout"
	StopReasonHarnessError StopReason = "harness_error"
)
