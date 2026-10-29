package commands

// Operation is one background daemon operation: an image or workflow build.
// ImageResult or WorkflowResult holds the result of a succeeded build;
// ErrorCode and ErrorMessage explain a failed or interrupted one.
type Operation struct {
	ID             int64                     `json:"id"`
	Kind           string                    `json:"kind"`
	Title          string                    `json:"title"`
	Status         string                    `json:"status"`
	Initiator      string                    `json:"initiator"`
	Params         map[string]string         `json:"params"`
	ImageResult    *ImageBuildResult         `json:"image_result,omitempty"`
	WorkflowResult *WorkflowImageBuildResult `json:"workflow_result,omitempty"`
	ErrorCode      string                    `json:"error_code,omitempty"`
	ErrorMessage   string                    `json:"error_message,omitempty"`
	CreatedAt      string                    `json:"created_at"`
	FinishedAt     string                    `json:"finished_at,omitempty"`
}

// OperationStarted is the result of a build request: the started operation,
// or the finished one when the request asked to wait.
type OperationStarted struct {
	Operation Operation `json:"operation"`
}

// OperationList is one page of operations, newest first.
type OperationList struct {
	Operations []Operation `json:"operations"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	Limit      int         `json:"limit"`
}

// OperationLog is the part of an operation log from Offset to Next. Size is
// the log size when it was read; Finished means the operation had ended, so
// the log grows no more. Available is false when the log file is gone.
type OperationLog struct {
	ID        int64  `json:"id"`
	Offset    int64  `json:"offset"`
	Next      int64  `json:"next"`
	Size      int64  `json:"size"`
	Content   string `json:"content"`
	Finished  bool   `json:"finished"`
	Available bool   `json:"available"`
}
