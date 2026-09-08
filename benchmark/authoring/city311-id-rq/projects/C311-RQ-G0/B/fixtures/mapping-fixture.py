#!/usr/bin/env python3
"""Deterministic City 311 mapping boundary fixture for C311-RQ-G0."""

import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Fixture(BaseHTTPRequestHandler):
    calls = 0

    def log_message(self, _format, *_args):
        pass

    def send_json(self, status, body):
        encoded = json.dumps(body).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def do_GET(self):
        if self.path == "/healthz":
            return self.send_json(200, {"status": "ok"})
        if self.path == "/calls":
            return self.send_json(200, {"geocode_calls": self.calls})
        return self.send_json(404, {"error": "NOT_FOUND"})

    def do_POST(self):
        if self.path != "/internal/integrations/mapping/geocode":
            return self.send_json(404, {"error": "NOT_FOUND"})
        if self.headers.get("Authorization") != "Bearer c311-g0-mapping-token":
            return self.send_json(401, {"error": "MAP_UNAUTHENTICATED", "retryable": False})
        try:
            payload = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        except (KeyError, ValueError, json.JSONDecodeError):
            return self.send_json(400, {"error": "INVALID_REQUEST"})
        if payload.get("address") != "100 Example Street, Buffalo, NY 14201":
            return self.send_json(404, {"error": "ADDRESS_NOT_FOUND", "message": "not found", "retryable": False})
        type(self).calls += 1
        return self.send_json(200, {
            "address": "100 Example Street, Buffalo, NY 14201",
            "latitude": 42.9001,
            "longitude": -78.8801,
            "precision_digits": 4,
            "provider": "BENCHMARK_MAP",
        })


ThreadingHTTPServer(("0.0.0.0", 8081), Fixture).serve_forever()
