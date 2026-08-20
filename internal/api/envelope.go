package api

import (
	"encoding/json"
	"net/http"
)

// UserError is a handler error surfaced to the caller with a JSON error
// envelope. It defaults to HTTP 400; set Status to override (e.g. 403/404) when
// a handler needs a code-specific status.
type UserError struct {
	Code   string
	Msg    string
	Status int // 0 => 400 Bad Request
	Data   map[string]any
}

func (e UserError) Error() string { return e.Code + ": " + e.Msg }

func WriteOK(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

func WriteErr(w http.ResponseWriter, status int, code, msg string) {
	WriteErrData(w, status, code, msg, nil)
}

func WriteErrData(w http.ResponseWriter, status int, code, msg string, data map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	errBody := map[string]any{"code": code, "message": msg}
	if len(data) > 0 {
		errBody["details"] = data
	}
	json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": errBody,
	})
}
