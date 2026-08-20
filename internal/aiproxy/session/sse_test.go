package session

import "testing"

func TestSniffStreamed(t *testing.T) {
	if !sniffStreamed([]byte("event: message_start\ndata: {}\n\n")) {
		t.Fatal("SSE body not detected")
	}
	if !sniffStreamed([]byte("data: {\"x\":1}\n\n")) {
		t.Fatal("data-only SSE not detected")
	}
	if sniffStreamed([]byte("  {\"type\":\"message\"}")) {
		t.Fatal("JSON object misdetected as SSE")
	}
}

func TestReassembleSSE(t *testing.T) {
	body := []byte("event: a\ndata: {\"n\":1}\n\nevent: b\ndata: {\"n\":2}\n\n")
	data, trunc := reassembleSSE(body)
	if trunc {
		t.Fatal("unexpected truncation")
	}
	if len(data) != 2 || string(data[0]) != `{"n":1}` || string(data[1]) != `{"n":2}` {
		t.Fatalf("bad data payloads: %v", data)
	}
}

func TestReassembleSSETruncated(t *testing.T) {
	body := []byte("data: {\"n\":1}\n\n\n[transcript truncated at cap]\n")
	data, trunc := reassembleSSE(body)
	if !trunc {
		t.Fatal("truncation marker not detected")
	}
	if len(data) != 1 {
		t.Fatalf("want 1 payload, got %d", len(data))
	}
}
