package script

import (
	"context"
	"testing"
	"time"
)

func TestRunNormalExit(t *testing.T) {
	output, code, err := Execute(context.Background(), t.TempDir(), nil, "echo hi", 5*time.Second)
	if err != nil || code != 0 || output != "hi\n" {
		t.Fatalf("output=%q code=%d err=%v", output, code, err)
	}
}

func TestRunTimeout(t *testing.T) {
	started := time.Now()
	_, code, err := Execute(context.Background(), t.TempDir(), nil, "sleep 30 & echo started", 300*time.Millisecond)
	if time.Since(started) >= 3*time.Second || err == nil || code != -1 {
		t.Fatalf("code=%d err=%v elapsed=%v", code, err, time.Since(started))
	}
}
