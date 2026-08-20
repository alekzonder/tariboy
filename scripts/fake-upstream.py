#!/usr/bin/env python3
# Fake Anthropic /v1/messages upstream for the e2e. Echoes the requested model
# (so model-policy route rewrites are observable) and a fixed usage block. The
# response text is read per-request from FAKE_UPSTREAM_TEXTFILE (if set + present)
# so an llm-judge verdict can be steered mid-run; otherwise "ok". NEVER a real
# API. Prints its chosen port to stdout.
import http.server, json, os

TEXTFILE = os.environ.get("FAKE_UPSTREAM_TEXTFILE", "")


def current_text():
    if TEXTFILE and os.path.exists(TEXTFILE):
        try:
            t = open(TEXTFILE).read().strip()
            if t:
                return t
        except OSError:
            pass
    return "ok"


class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        raw = self.rfile.read(int(self.headers.get("Content-Length", 0) or 0))
        model = "claude-opus-4-8"
        try:
            model = json.loads(raw).get("model", model) or model
        except Exception:
            pass
        resp = json.dumps({
            "id": "msg_fake", "type": "message", "role": "assistant", "model": model,
            "content": [{"type": "text", "text": current_text()}],
            "usage": {"input_tokens": 100, "output_tokens": 50,
                      "cache_creation_input_tokens": 0, "cache_read_input_tokens": 0},
        }).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(resp)))
        self.end_headers()
        self.wfile.write(resp)

    def log_message(self, *a):  # quiet
        pass


srv = http.server.HTTPServer(("127.0.0.1", 0), H)
print(srv.server_address[1], flush=True)
srv.serve_forever()
