package plugins

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Handshake is the one JSON line a plugin prints to stdout on startup (spec §7.2).
type Handshake struct {
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Types           []string `json:"types"`
	ProtocolVersion int      `json:"protocol_version"`
	Socket          string   `json:"socket"`
}

// ReadHandshake reads exactly one line from r and parses it. Timeout is the
// caller's concern (the supervisor wraps this with the 5s handshake timeout).
func ReadHandshake(r io.Reader) (Handshake, error) {
	br := bufio.NewReader(r)
	line, err := br.ReadBytes('\n')
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		if err != nil {
			return Handshake{}, fmt.Errorf("read handshake: %w", err)
		}
		return Handshake{}, fmt.Errorf("empty handshake line")
	}
	var h Handshake
	if err := json.Unmarshal(line, &h); err != nil {
		return Handshake{}, fmt.Errorf("parse handshake %q: %w", line, err)
	}
	return h, nil
}

// Validate checks the handshake against the installed manifest (spec §7.2).
func (h Handshake) Validate(m Manifest) error {
	if h.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("plugin %s announced protocol_version %d (daemon speaks %d)",
			m.Name, h.ProtocolVersion, ProtocolVersion)
	}
	if h.Name != m.Name {
		return fmt.Errorf("plugin announced name %q but was installed as %q", h.Name, m.Name)
	}
	return nil
}
