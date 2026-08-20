package plugins

import (
	"strings"
	"testing"
)

func TestReadHandshake(t *testing.T) {
	line := `{"name":"echo","version":"0.1.0","types":["channel-sink"],"protocol_version":1,"socket":"/tmp/echo.sock"}` + "\n"
	h, err := ReadHandshake(strings.NewReader(line + "extra stdout noise\n"))
	if err != nil {
		t.Fatal(err)
	}
	if h.Name != "echo" || h.ProtocolVersion != 1 || h.Socket != "/tmp/echo.sock" {
		t.Fatalf("handshake = %+v", h)
	}
	if _, err := ReadHandshake(strings.NewReader("not json\n")); err == nil {
		t.Fatal("garbage line should error")
	}
	if _, err := ReadHandshake(strings.NewReader("")); err == nil {
		t.Fatal("empty stream should error")
	}
}

func TestHandshakeValidate(t *testing.T) {
	m, _ := ParseManifest([]byte(goodManifest))
	ok := Handshake{Name: "echo", ProtocolVersion: 1, Socket: "/x"}
	if err := ok.Validate(m); err != nil {
		t.Fatalf("valid handshake rejected: %v", err)
	}
	if err := (Handshake{Name: "echo", ProtocolVersion: 2}).Validate(m); err == nil {
		t.Fatal("protocol mismatch should reject")
	}
	if err := (Handshake{Name: "other", ProtocolVersion: 1}).Validate(m); err == nil {
		t.Fatal("name mismatch should reject")
	}
}
