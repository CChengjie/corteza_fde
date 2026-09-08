import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

class Fixture(BaseHTTPRequestHandler):
    def log_message(self, *_args): pass
    def do_POST(self):
        if self.path != "/internal/integrations/mapping/geocode" or self.headers.get("Authorization") != "Bearer c311-system-mapping-token":
            self.send_response(401); self.end_headers(); return
        self.rfile.read(int(self.headers.get("Content-Length", "0")))
        payload = json.dumps({"address":"100 Example Street, Buffalo, NY 14201","latitude":42.9001,"longitude":-78.8801,"provider":"BENCHMARK_MAP"}).encode()
        self.send_response(200); self.send_header("Content-Type", "application/json"); self.send_header("Content-Length", str(len(payload))); self.end_headers(); self.wfile.write(payload)

ThreadingHTTPServer(("0.0.0.0", 8081), Fixture).serve_forever()
