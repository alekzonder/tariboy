package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/schedule"
	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// internalErrorMsg is the only text an unmapped error sends to a caller; the
// error itself goes to the daemon log.
const internalErrorMsg = "internal error, see daemon log"

// sentinelError maps one domain sentinel to its public answer. An empty msg
// sends the error's own text, which is reserved for validation sentinels whose
// wrapping text is written for the caller.
type sentinelError struct {
	err    error
	status int
	code   string
	msg    string
}

var sentinelErrors = []sentinelError{
	{script.ErrNotFound, http.StatusNotFound, "not_found", "script or run not found"},
	{script.ErrActive, http.StatusConflict, "script_active", script.ErrActive.Error()},
	{script.ErrMode, http.StatusConflict, "script_mode", script.ErrMode.Error()},
	{script.ErrConflict, http.StatusConflict, "script_conflict", script.ErrConflict.Error()},
	{script.ErrInvalid, http.StatusBadRequest, "invalid_script", ""},
	{schedule.ErrNotFound, http.StatusNotFound, "not_found", "schedule not found"},
	{schedule.ErrInvalid, http.StatusBadRequest, "invalid_schedule", ""},
	{bus.ErrNotFound, http.StatusNotFound, "not_found", bus.ErrNotFound.Error()},
	{bus.ErrDeadlineUnsupported, http.StatusBadRequest, "deadline_unsupported", bus.ErrDeadlineUnsupported.Error()},
	{bus.ErrInvalid, http.StatusBadRequest, "invalid_request", ""},
}

// ErrorResponse is the one mapping from a domain error to the HTTP answer of
// the operator API and the agent socket. ok is false for an error that has no
// public answer; the caller then answers 500 without the error's text.
func ErrorResponse(err error) (status int, code, msg string, data map[string]any, ok bool) {
	var ue UserError
	if errors.As(err, &ue) {
		return statusOr400(ue.Status), ue.Code, ue.Msg, ue.Data, true
	}
	var te *tasks.Error
	if errors.As(err, &te) {
		return statusOr400(te.Status), te.Code, te.Msg, te.Data, true
	}
	var me *mail.Error
	if errors.As(err, &me) {
		return statusOr400(me.Status), me.Code, me.Error(), nil, true
	}
	var pe *registry.ParamError
	if errors.As(err, &pe) {
		return http.StatusBadRequest, pe.Code, pe.Msg, nil, true
	}
	for _, s := range sentinelErrors {
		if errors.Is(err, s.err) {
			msg := s.msg
			if msg == "" {
				msg = err.Error()
			}
			return s.status, s.code, msg, nil, true
		}
	}
	return 0, "", "", nil, false
}

func statusOr400(status int) int {
	if status == 0 {
		return http.StatusBadRequest
	}
	return status
}

// WriteError answers err through ErrorResponse. An unmapped error is answered
// 500 internal with a fixed text and logged in full under scope.
func WriteError(w http.ResponseWriter, log *slog.Logger, scope string, err error) {
	if status, code, msg, data, ok := ErrorResponse(err); ok {
		WriteErrData(w, status, code, msg, data)
		return
	}
	if log != nil {
		log.Error("request failed", "scope", scope, "err", err)
	}
	WriteErr(w, http.StatusInternalServerError, "internal", internalErrorMsg)
}

// DecodeJSONBody decodes a JSON request body of at most limit bytes into v. An
// absent or empty body leaves v unchanged. On failure it writes the answer (413
// too_large, or 400 bad_json with a fixed text) and returns false.
func DecodeJSONBody(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	if r.Body == nil {
		return true
	}
	defer r.Body.Close()
	if r.ContentLength > limit {
		WriteErr(w, http.StatusRequestEntityTooLarge, "too_large", "request body exceeds the size limit")
		return false
	}
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(v)
	var tooLarge *http.MaxBytesError
	switch {
	case err == nil || errors.Is(err, io.EOF):
		return true
	case errors.As(err, &tooLarge):
		WriteErr(w, http.StatusRequestEntityTooLarge, "too_large", "request body exceeds the size limit")
	default:
		WriteErr(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
	}
	return false
}
