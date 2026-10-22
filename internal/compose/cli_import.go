package compose

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alekzonder/tariboy/internal/teamportable"
	"github.com/alekzonder/tariboy/internal/tidy"
)

// composeArchive writes the portable team archive of a compose file.
func composeArchive(file, output string, out, errOut io.Writer) int {
	if output == "" {
		fmt.Fprintln(errOut, "compose archive: --output is required")
		return 2
	}
	archiveOut, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		fmt.Fprintf(errOut, "compose archive: %v\n", err)
		return 1
	}
	if err := teamportable.CreateFromCompose(file, archiveOut); err != nil {
		tidy.Close(nil, archiveOut)
		tidy.Remove(nil, output)
		fmt.Fprintf(errOut, "compose archive: %v\n", err)
		return 1
	}
	if err := archiveOut.Close(); err != nil {
		tidy.Remove(nil, output)
		fmt.Fprintf(errOut, "compose archive: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, output)
	return 0
}

// composeImport uploads a team archive, prints the preview, and applies it
// after --yes or a "y" answer read from stdin.
func composeImport(call Caller, archive string, yes bool, stdin io.Reader, out, errOut io.Writer) int {
	if archive == "" {
		fmt.Fprintln(errOut, "compose import: --archive is required")
		return 2
	}
	uploader, ok := call.(interface {
		Upload(string, io.Reader, int64) (json.RawMessage, error)
	})
	if !ok {
		fmt.Fprintln(errOut, "compose import: archive upload is not available")
		return 2
	}
	in, err := os.Open(archive)
	if err != nil {
		fmt.Fprintf(errOut, "compose import: %v\n", err)
		return 1
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		fmt.Fprintf(errOut, "compose import: %v\n", err)
		return 1
	}
	preview, err := uploader.Upload("/api/team-imports", in, info.Size())
	if err != nil {
		fmt.Fprintf(errOut, "compose import: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, string(preview))
	var planned struct {
		ImportID string `json:"import_id"`
	}
	if err := json.Unmarshal(preview, &planned); err != nil || planned.ImportID == "" {
		fmt.Fprintln(errOut, "compose import: preview did not return an import id")
		return 1
	}
	if !yes {
		fmt.Fprint(errOut, "Apply this import? [y/N] ")
		if stdin == nil {
			stdin = strings.NewReader("")
		}
		var answer string
		if _, err := fmt.Fscan(stdin, &answer); err != nil || (answer != "y" && answer != "Y" && answer != "yes") {
			fmt.Fprintln(errOut, "import not applied")
			return 0
		}
	}
	result, err := call.Call("POST", "/api/team-imports/"+planned.ImportID+"/apply", map[string]any{})
	if err != nil {
		fmt.Fprintf(errOut, "compose import: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, string(result))
	return 0
}
