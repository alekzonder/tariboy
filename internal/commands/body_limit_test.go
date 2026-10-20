package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/filebrowser"
	"github.com/alekzonder/tariboy/internal/imagesource"
)

// Text editors save a whole file as a JSON string, so their routes must accept
// the largest file they can open even when JSON escapes every byte.
func TestTextFileSaveRoutesFitEscapedMaxFile(t *testing.T) {
	for name, tc := range map[string]struct {
		limit   int64
		maxFile int64
	}{
		"agent.file.write":      {fileWrite().HTTP.MaxBodyBytes, filebrowser.MaxReadBytes},
		"image.source.file.put": {imageSourceFilePut().HTTP.MaxBodyBytes, imagesource.MaxFileSize},
	} {
		// \u0000 is the longest escape: six bytes for one file byte.
		if tc.limit < 6*tc.maxFile {
			t.Errorf("%s body limit = %d, want at least %d", name, tc.limit, 6*tc.maxFile)
		}
	}
}
