package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/teamportable"
)

func TestSaveImportOperationStopsWithImportStateUnsaved(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	service := teamportable.Service{StagingRoot: root}
	// A directory where the operation file belongs makes the final rename fail.
	if err := os.MkdirAll(filepath.Join(root, "imp1", ".operation.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	err := saveImportOperation(service, teamportable.Operation{ImportID: "imp1", Status: "running"}, "running")
	var userErr api.UserError
	if !errors.As(err, &userErr) || userErr.Code != "import_state_unsaved" {
		t.Fatalf("err = %#v", err)
	}
	if !strings.Contains(userErr.Msg, "imp1") || !strings.Contains(userErr.Msg, "running") {
		t.Fatalf("message must name operation and step: %q", userErr.Msg)
	}
}

func TestSaveImportOperationOK(t *testing.T) {
	t.Parallel()
	service := teamportable.Service{StagingRoot: t.TempDir()}
	if err := saveImportOperation(service, teamportable.Operation{ImportID: "imp2", Status: "running"}, "running"); err != nil {
		t.Fatal(err)
	}
}
