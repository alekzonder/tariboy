package judge

import (
	"crypto/sha256"
	"encoding/hex"

	storeassets "github.com/alekzonder/tariboy/store"
)

// ReviewCriteria returns the immutable rubric used by automatic and operator runs.
func ReviewCriteria() (string, string, error) {
	text, err := storeassets.ReadBundled("prompts/judge-rubric.md")
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(text)
	return string(text), hex.EncodeToString(sum[:]), nil
}
