#!/usr/bin/env python3
# Fake OTLP/HTTP collector for the e2e. Accepts POST /v1/traces and /v1/metrics,
# returns 200, and touches a hit-file (env TARIBOY_OTLP_HITFILE) with the
# path that was posted, so the e2e can assert an export happened. NEVER a real
# collector; no external network. Prints its port to stdout (like fake-upstream).
import http.server, os, sys

HITFILE = os.environ.get("TARIBOY_OTLP_HITFILE", "")

class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        _ = self.rfile.read(int(self.headers.get("Content-Length", 0) or 0))
        if HITFILE and self.path.startswith("/v1/"):
            with open(HITFILE, "a") as f:
                f.write(self.path + "\n")
        self.send_response(200)
        self.send_header("Content-Type", "application/x-protobuf")
        self.send_header("Content-Length", "0")
        self.end_headers()
    def log_message(self, *a):  # quiet
        pass

srv = http.server.HTTPServer(("127.0.0.1", 0), H)
print(srv.server_address[1], flush=True)
srv.serve_forever()
