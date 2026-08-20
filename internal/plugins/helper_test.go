package plugins

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"testing"
)

// TestMain lets the test binary re-exec itself as a real plugin process (no
// python in `go test`). The execRunner spawns os.Args[0] with TARIBOY_FAKE_PLUGIN=1.
func TestMain(m *testing.M) {
	switch os.Getenv("TARIBOY_FAKE_PLUGIN") {
	case "1":
		fakePluginMain()
		return
	case "silent":
		// Carry-forward 2 helper: a stubborn plugin that never prints a
		// handshake and ignores SIGTERM, so only a SIGKILL to its process
		// group reaps it. Used to prove the handshake-timeout kill path.
		silentPluginMain()
		return
	}
	os.Exit(m.Run())
}

// fakePluginMain is a minimal channel-source+sink plugin: it prints its
// handshake line, listens on the assigned socket, serves /health and /deliver.
func fakePluginMain() {
	name := os.Getenv("TARIBOY_PLUGIN_NAME")
	sock := os.Getenv("TARIBOY_PLUGIN_SOCKET")
	fmt.Printf(`{"name":%q,"version":"0.0.1","types":["channel-source","channel-sink"],"protocol_version":1,"socket":%q}`+"\n", name, sock)
	os.Stdout.Sync()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/deliver", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(200)
	})
	_ = http.Serve(ln, mux)
}

// silentPluginMain prints no handshake and ignores SIGTERM, blocking forever.
// The supervisor must time out the handshake and SIGKILL the process group.
func silentPluginMain() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)
	select {} // block until SIGKILL
}
