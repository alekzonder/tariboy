package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/alekzonder/tariboy/internal/supportbundle"
)

type SupportBundleSource interface {
	Prepare(context.Context, supportbundle.Options) (supportbundle.Archive, error)
}

func (s *Server) SetSupportBundleSource(source SupportBundleSource) {
	s.support = source
}

func (s *Server) serveSupportBundle(w http.ResponseWriter, r *http.Request) {
	for key := range r.URL.Query() {
		if key != "include_agent_data" && key != "iteration_limit" {
			WriteErr(w, http.StatusBadRequest, "bad_support_request", "unknown support bundle parameter")
			return
		}
	}
	include, err := supportBool(r.URL.Query().Get("include_agent_data"))
	if err != nil {
		WriteErr(w, http.StatusBadRequest, "bad_support_request", "include_agent_data must be 0 or 1")
		return
	}
	limit, err := supportLimit(r.URL.Query().Get("iteration_limit"))
	if err != nil {
		WriteErr(w, http.StatusBadRequest, "bad_support_request", "iteration_limit must be between 1 and 10")
		return
	}
	archive, err := s.support.Prepare(r.Context(), supportbundle.Options{
		IncludeAgentData: include,
		IterationLimit:   limit,
	})
	if err != nil {
		if errors.Is(err, supportbundle.ErrTooLarge) {
			WriteErr(w, http.StatusRequestEntityTooLarge, "support_bundle_too_large",
				"selected agent diagnostics exceed the support bundle size limit")
			return
		}
		s.cctx.Log.Error("support bundle prepare failed")
		WriteErr(w, http.StatusInternalServerError, "internal", "internal error, see daemon log")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="tariboy-daemon-support.zip"`)
	w.WriteHeader(http.StatusOK)
	if err := archive.WriteZIP(w); err != nil {
		s.cctx.Log.Error("support bundle stream failed")
	}
}

func supportBool(value string) (bool, error) {
	switch value {
	case "", "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, strconv.ErrSyntax
	}
}

func supportLimit(value string) (int, error) {
	if value == "" {
		return supportbundle.MaxIterations, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > supportbundle.MaxIterations {
		return 0, strconv.ErrRange
	}
	return limit, nil
}
