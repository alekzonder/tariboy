package storesvc

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
)

const (
	ScopeRead      = "read"
	ScopeReadWrite = "readwrite"
)

func tokenHash(tok string) string {
	s := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(s[:])
}

// SeedToken upserts a token by its sha256 (the plaintext is never stored).
func (d *DB) SeedToken(tok, scope string) error {
	_, err := d.sql.Exec(`INSERT INTO tokens(token_sha256, scope, label) VALUES (?,?,?)
		ON CONFLICT(token_sha256) DO UPDATE SET scope = excluded.scope`,
		tokenHash(tok), scope, "seed")
	return err
}

// LookupToken resolves a presented bearer token to its scope. The match is by
// sha256-hex primary key: only a hash of the high-entropy secret is compared, so
// there is no per-character timing oracle on the token itself.
func (d *DB) LookupToken(tok string) (string, bool, error) {
	var scope string
	err := d.sql.QueryRow(`SELECT scope FROM tokens WHERE token_sha256 = ?`, tokenHash(tok)).Scan(&scope)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return scope, true, nil
}
