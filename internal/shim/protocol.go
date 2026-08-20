// Package shim is the per-iteration harness parent (spec §4). protocol.go holds
// the JSON-RPC 2.0 wire types and client shared with the daemon; the process
// engine lives in shim.go.
package shim

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"
)

// defaultClientTimeout bounds how long a Client waits on a single RPC
// round-trip when Client.Timeout is unset.
const defaultClientTimeout = 5 * time.Second

// serverConnTimeoutNanos bounds how long serveConn waits on a single
// connection (read of the request plus write of the response) before giving
// up on a stuck peer, stored as nanoseconds in an atomic so tests can shrink
// it (to keep a send-nothing-and-wait regression test fast) while a
// concurrently running serveConn goroutine reads it, without a data race.
var serverConnTimeoutNanos atomic.Int64

func init() {
	serverConnTimeoutNanos.Store(int64(30 * time.Second))
}

// serverConnTimeout returns the current serveConn connection timeout.
func serverConnTimeout() time.Duration {
	return time.Duration(serverConnTimeoutNanos.Load())
}

const (
	MethodStatus          = "status"
	MethodKill            = "kill"
	MethodScreen          = "screen"
	MethodSendKeys        = "sendkeys"
	MethodReport          = "report"
	MethodSetHardDeadline = "set_hard_deadline"
	MethodAttach          = "attach"
	MethodResize          = "resize"
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return e.Message }

type StatusResult struct {
	Running   bool   `json:"running"`
	PID       int    `json:"pid"`
	StartedAt string `json:"started_at"`
}

type ScreenResult struct {
	Screen string `json:"screen"`
}

// KeyItem is one unit sent to the interactive terminal: either literal Text
// (typed as-is, no Enter) or a named tmux Key (e.g. "Up", "Enter", "Escape",
// "C-c"). Exactly one field is set per item.
type KeyItem struct {
	Text string `json:"text,omitempty"`
	Key  string `json:"key,omitempty"`
}

// SendKeysParams carries either a legacy single line (Keys, sent with a trailing
// Enter) or a list of raw Items (sent with no forced Enter). Items win when set.
type SendKeysParams struct {
	Keys  string    `json:"keys,omitempty"`
	Items []KeyItem `json:"items,omitempty"`
}

// IterationResult is the final report; also written to result.json.
type IterationResult struct {
	ExitCode          int    `json:"exit_code"`
	EndedAt           string `json:"ended_at"`
	CPUMs             int    `json:"cpu_ms"`
	MemPeakKB         int    `json:"mem_peak_kb"`
	OOM               bool   `json:"oom"`
	TerminationReason string `json:"termination_reason,omitempty"`
}

type ReportResult struct {
	Finished bool             `json:"finished"`
	Result   *IterationResult `json:"result,omitempty"`
}

// Handler is implemented by the running shim.
type Handler interface {
	Status() StatusResult
	Kill() error
	Screen() (string, error)
	SendKeys(SendKeysParams) error
	Report() ReportResult
}

// HardDeadlineSetter is implemented by live shims whose watchdog can be
// resynchronised after an operator extends an iteration.
type HardDeadlineSetter interface{ SetHardDeadline(string) error }

// AttachParams carries the initial terminal size for a streaming attach.
type AttachParams struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// ResizeParams carries a terminal size update for an in-progress attach.
type ResizeParams struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// StreamHandler is implemented by shims that support a live PTY attach.
type StreamHandler interface {
	Attach(conn net.Conn, p AttachParams) error // takes over conn; returns when the stream ends
	Resize(ResizeParams) error
}

func netListenUnix(path string) (net.Listener, error) { return net.Listen("unix", path) }

// Serve accepts one request per connection until the listener is closed.
func Serve(l net.Listener, h Handler) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go serveConn(conn, h)
	}
}

func serveConn(conn net.Conn, h Handler) {
	// Bound the initial request read: an idle/slow-loris client that connects
	// and sends nothing must not hang this goroutine forever. This deadline
	// is cleared below on the attach path once we're committed to a
	// long-lived stream; the one-shot path keeps (and re-sets) it through to
	// the response write.
	_ = conn.SetDeadline(time.Now().Add(serverConnTimeout()))
	dec := json.NewDecoder(conn)
	var req Request
	if err := dec.Decode(&req); err != nil {
		writeResp(conn, Response{JSONRPC: "2.0", Error: &RPCError{Code: -32700, Message: "parse error: " + err.Error()}})
		conn.Close()
		return
	}

	if req.Method == MethodAttach {
		sh, ok := h.(StreamHandler)
		if !ok {
			writeResp(conn, Response{JSONRPC: "2.0", ID: req.ID, Error: &RPCError{Code: -32601, Message: "attach unsupported"}})
			conn.Close()
			return
		}
		var p AttachParams
		_ = json.Unmarshal(req.Params, &p)
		// Clear the deadline before the ACK/Attach: the attach stream is
		// long-lived and must have no deadline once Attach owns the conn.
		_ = conn.SetDeadline(time.Time{})
		writeResp(conn, Response{JSONRPC: "2.0", ID: req.ID}) // ACK
		defer conn.Close()
		// dec's internal buffer may already hold bytes read past the
		// request line: at minimum the trailing newline json.Encoder
		// always appends (a framing artifact, not stream data), and
		// possibly genuine stream bytes the client sent right after.
		// Strip the framing newline, then replay whatever's left so
		// Attach sees a contiguous, uncorrupted stream.
		streamConn := newPrefixedConn(conn, bytes.NewReader(trimEncoderNewline(dec.Buffered())))
		if err := sh.Attach(streamConn, p); err != nil {
			// Stream ended; nothing more to write on a raw tunnel.
			_ = err
		}
		return
	}

	// One-shot path (existing behavior): bound the round-trip and always
	// close the conn when done.
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(serverConnTimeout()))
	resp := Response{JSONRPC: "2.0", ID: req.ID}
	result, rpcErr := dispatch(h, req)
	if rpcErr != nil {
		resp.Error = rpcErr
	} else {
		raw, err := json.Marshal(result)
		if err != nil {
			resp.Error = &RPCError{Code: -32603, Message: err.Error()}
		} else {
			resp.Result = raw
		}
	}
	writeResp(conn, resp)
}

func dispatch(h Handler, req Request) (any, *RPCError) {
	switch req.Method {
	case MethodStatus:
		return h.Status(), nil
	case MethodKill:
		if err := h.Kill(); err != nil {
			return nil, &RPCError{Code: -32000, Message: err.Error()}
		}
		return map[string]bool{"ok": true}, nil
	case MethodScreen:
		screen, err := h.Screen()
		if err != nil {
			return nil, &RPCError{Code: -32001, Message: err.Error()}
		}
		return ScreenResult{Screen: screen}, nil
	case MethodSendKeys:
		var p SendKeysParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				return nil, &RPCError{Code: -32602, Message: err.Error()}
			}
		}
		if err := h.SendKeys(p); err != nil {
			return nil, &RPCError{Code: -32002, Message: err.Error()}
		}
		return map[string]bool{"ok": true}, nil
	case MethodReport:
		return h.Report(), nil
	case MethodResize:
		sh, ok := h.(StreamHandler)
		if !ok {
			return nil, &RPCError{Code: -32601, Message: "resize unsupported"}
		}
		var p ResizeParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &RPCError{Code: -32602, Message: err.Error()}
		}
		if err := sh.Resize(p); err != nil {
			return nil, &RPCError{Code: -32003, Message: err.Error()}
		}
		return map[string]bool{"ok": true}, nil
	case MethodSetHardDeadline:
		setter, ok := h.(HardDeadlineSetter)
		if !ok {
			return nil, &RPCError{Code: -32601, Message: "hard deadline updates unsupported"}
		}
		var p struct {
			Deadline string `json:"deadline"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Deadline == "" {
			if err == nil {
				err = fmt.Errorf("deadline is required")
			}
			return nil, &RPCError{Code: -32602, Message: err.Error()}
		}
		if err := setter.SetHardDeadline(p.Deadline); err != nil {
			return nil, &RPCError{Code: -32602, Message: err.Error()}
		}
		return map[string]bool{"ok": true}, nil
	default:
		return nil, &RPCError{Code: -32601, Message: "unknown method " + req.Method}
	}
}

func writeResp(conn net.Conn, resp Response) {
	_ = json.NewEncoder(conn).Encode(resp)
}

// Client is a connection to a running shim's JSON-RPC socket.
type Client struct {
	sock string
	conn net.Conn
	// Timeout bounds how long call() waits on a single RPC round-trip
	// (dial is not included; the deadline is set after dialing). Zero
	// means defaultClientTimeout.
	Timeout time.Duration
}

func Dial(sock string) *Client { return &Client{sock: sock} }

// Connect binds a one-shot client to the shim currently listening at sock.
// Unlike Dial, later socket-path replacement cannot redirect its RPC.
func Connect(sock string) (*Client, error) {
	return ConnectTimeout(sock, defaultClientTimeout)
}

// ConnectTimeout is Connect with an explicit bound for establishing the Unix
// connection. It is primarily exposed so callers and tests can choose a
// shorter handoff budget without changing the later RPC deadline.
func ConnectTimeout(sock string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("unix", sock, timeout)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn}, nil
}

func (c *Client) open() (net.Conn, error) {
	if c.conn != nil {
		conn := c.conn
		c.conn = nil
		return conn, nil
	}
	return net.Dial("unix", c.sock)
}

func (c *Client) call(method string, params any, out any) error {
	conn, err := c.open()
	if err != nil {
		return err
	}
	defer conn.Close()
	timeout := c.Timeout
	if timeout == 0 {
		timeout = defaultClientTimeout
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	req := Request{JSONRPC: "2.0", ID: 1, Method: method}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return err
		}
		req.Params = raw
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return err
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("shim response: %w", err)
	}
	if resp.Error != nil {
		return resp.Error
	}
	if out != nil && len(resp.Result) > 0 {
		return json.Unmarshal(resp.Result, out)
	}
	return nil
}

func (c *Client) Status() (StatusResult, error) {
	var out StatusResult
	err := c.call(MethodStatus, nil, &out)
	return out, err
}

func (c *Client) Kill() error { return c.call(MethodKill, nil, nil) }

func (c *Client) Screen() (string, error) {
	var out ScreenResult
	err := c.call(MethodScreen, nil, &out)
	return out.Screen, err
}

func (c *Client) SendKeys(keys string) error {
	return c.call(MethodSendKeys, SendKeysParams{Keys: keys}, nil)
}

func (c *Client) SendKeysItems(items []KeyItem) error {
	return c.call(MethodSendKeys, SendKeysParams{Items: items}, nil)
}

func (c *Client) Report() (ReportResult, error) {
	var out ReportResult
	err := c.call(MethodReport, nil, &out)
	return out, err
}

// SetHardDeadline installs an absolute RFC3339 hard deadline. Repeating the
// same value is intentionally harmless, allowing daemon restart recovery.
func (c *Client) SetHardDeadline(deadline string) error {
	return c.call(MethodSetHardDeadline, struct {
		Deadline string `json:"deadline"`
	}{deadline}, nil)
}

// Attach opens a streaming terminal. It sends the attach request, reads a
// single JSON ACK line, then returns the still-open conn as a raw byte
// tunnel. The caller owns and must Close the conn.
func (c *Client) Attach(cols, rows int) (net.Conn, error) {
	conn, err := c.open()
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(AttachParams{Cols: cols, Rows: rows})
	req := Request{JSONRPC: "2.0", ID: 1, Method: MethodAttach, Params: raw}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		conn.Close()
		return nil, err
	}
	// Read the ACK line (a Response with Error==nil) without consuming stream bytes.
	dec := json.NewDecoder(conn)
	var resp Response
	if err := dec.Decode(&resp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("attach ack: %w", err)
	}
	if resp.Error != nil {
		conn.Close()
		return nil, resp.Error
	}
	// NOTE: dec may have buffered stream bytes past the ACK's own framing
	// newline; callers must read via the returned prefixedConn (see
	// newPrefixedConn) so no output is lost, and the framing newline
	// itself must not be mistaken for stream data.
	return newPrefixedConn(conn, bytes.NewReader(trimEncoderNewline(dec.Buffered()))), nil
}

func (c *Client) Resize(cols, rows int) error {
	return c.call(MethodResize, ResizeParams{Cols: cols, Rows: rows}, nil)
}

// prefixedConn wraps a net.Conn whose ACK-reading json.Decoder may have
// buffered bytes past the ACK line; those buffered bytes are replayed first
// so no stream data is lost.
type prefixedConn struct {
	net.Conn
	pre io.Reader // buffered bytes read by the ACK decoder, replayed first
}

func newPrefixedConn(c net.Conn, buffered io.Reader) net.Conn {
	return &prefixedConn{Conn: c, pre: io.MultiReader(buffered, c)}
}
func (p *prefixedConn) Read(b []byte) (int, error) { return p.pre.Read(b) }

// trimEncoderNewline reads r (a json.Decoder's Buffered() leftover) and
// strips exactly one leading '\n', the framing delimiter json.Encoder always
// appends after a value and that Decode never consumes. Without this, that
// delimiter would be mistaken for the first byte of a subsequent raw stream
// (e.g. the attach request/ACK immediately followed by tunnel data),
// corrupting it by one spurious byte.
//
// Caveat: this relies on the framing newline already being present in
// Buffered() by the time we read it, which holds for a well-behaved client
// that issues a single Write per Encode over a local unix-socket connection
// (the common case here). If the kernel ever delivered the newline in a
// separate read from the JSON body (e.g. body and newline split across TCP
// segments, or a client that writes the newline separately), Buffered()
// could still be empty of it at this point and it would leak through as the
// first stream byte instead of being stripped. Low risk for local unix-socket
// IPC but not airtight.
func trimEncoderNewline(r io.Reader) []byte {
	b, _ := io.ReadAll(r)
	if len(b) > 0 && b[0] == '\n' {
		return b[1:]
	}
	return b
}
