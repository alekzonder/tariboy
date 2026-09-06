package main

import (
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestPendingRequestTerminatesOnSignal(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "tariboy-tasks")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			// Keep the Unix socket short on hosts with long test temp paths.
			runtimeDir, err := os.MkdirTemp("", "tasks-signal-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(runtimeDir)
			socket := filepath.Join(runtimeDir, "agent.sock")
			listener, err := net.Listen("unix", socket)
			if err != nil {
				t.Fatal(err)
			}
			requested := make(chan struct{})
			release := make(chan struct{})
			server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(requested)
				<-release
			})}
			defer server.Close()
			defer close(release)
			go server.Serve(listener)
			cmd := exec.Command(binary, "mine")
			cmd.Env = append(os.Environ(), "TARIBOY_TOOLS_SOCKET="+socket, "TARIBOY_BASE_DIR="+t.TempDir(), "TARIBOY_RUNTIME_DIR="+runtimeDir)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			defer func() { _ = cmd.Process.Kill() }()
			select {
			case <-requested:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not start")
			}
			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("interrupted request succeeded")
				}
			case <-time.After(2 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				t.Fatal("pending request ignored signal")
			}
		})
	}
}
