package harness

import (
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/rundir"
)

// Catalog is the fallback skill route for a harness without native skills:
// a short "name: description" list and how to read a skill through tbox.
func Catalog(run rundir.Run) string {
	var b strings.Builder
	b.WriteString("## Skills of image " + run.Name + "\n\n")
	for _, s := range run.Skills {
		b.WriteString("- " + s.Name + ": " + strings.Join(strings.Fields(s.Description), " ") + "\n")
	}
	b.WriteString("\nRead a skill with `tbox skill <name>` and run its scripts with `tbox skill exec <name> <script>`.\n")
	return b.String()
}
