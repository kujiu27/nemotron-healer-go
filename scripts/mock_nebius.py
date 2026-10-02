#!/usr/bin/env python3
"""Labeled mock of Nebius Token Factory + Tavily for offline pipeline smoke.

Serves OpenAI-compatible endpoints on 127.0.0.1:
  POST /v1/chat/completions  (stream:false JSON and SSE stream:true)
  POST /search, POST /extract (Tavily shapes)

The patch it returns is a REAL fix for samples/external_gjson
(TestEmptyValueQuery, upstream 0b52f9a one-liner) so the whole heal loop —
reproduce, triage, ground, synthesize, apply, pre-flight syntax, verify,
falsify, commit — can run to green with zero API keys and zero fabrication:
the server banner prints MOCK on every response header.

Usage: make eval-mock
"""
import json
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

GJSON_FIX = """```diff
--- a/gjson.go
+++ b/gjson.go
@@ -761,7 +761,7 @@
-					if len(value) > 2 && value[0] == '"' &&
+					if len(value) >= 2 && value[0] == '"' &&
 						value[len(value)-1] == '"' {
```
[TARGET_FILE]gjson.go[/TARGET_FILE]"""

TRIAGE = "ERROR_LINE: len(value) > 2\nCAUSE: array-path parser rejects the empty quoted string\nQUERY: gjson empty string query operator"

ADVERSARIAL_TEST = """```go
package gjson

import "testing"

func TestAdversarialEmptyQueryMock(t *testing.T) {
	if Get(`["a",""]`, `#(!="")#`).Raw != `["a"]` {
		t.Fatal("empty-string filter mismatch")
	}
}
```"""


def reply_for(req):
    msgs = req.get("messages", [])
    sys = " ".join(m.get("content", "") for m in msgs if m.get("role") == "system")
    user = " ".join(m.get("content", "") for m in msgs if m.get("role") == "user")
    if "triage" in sys.lower() or "ERROR_LINE" in user or "4-word" in user:
        return TRIAGE
    if "adversarial" in sys.lower() or "Red-Teamer" in sys:
        return ADVERSARIAL_TEST
    if "diff" in sys.lower() or "patch" in user.lower() or "[HYPOTHESIS" in user:
        return GJSON_FIX
    return GJSON_FIX


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def _send(self, obj, status=200):
        body = json.dumps(obj).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("X-Nemotron-Healer", "MOCK")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        req = json.loads(self.rfile.read(n) or b"{}")

        if self.path == "/search":
            return self._send({
                "query": req.get("query", ""),
                "answer": "[MOCK] gjson query operators documentation",
                "results": [{"title": "[MOCK] gjson SYNTAX.md",
                             "url": "https://github.com/tidwall/gjson/blob/master/SYNTAX.md",
                             "content": "Query operators include != comparisons; the empty string is a valid operand.",
                             "score": 0.99}]})
        if self.path == "/extract":
            url = (req.get("urls") or [""])[0]
            return self._send({"results": [{"url": url, "raw_content":
                "[MOCK EXTRACT] gjson supports #(...)# modifiers with comparison operators."}],
                "failed_results": []})

        if self.path.endswith("/chat/completions"):
            text = reply_for(req)
            if req.get("stream"):
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.send_header("X-Nemotron-Healer", "MOCK")
                self.end_headers()
                chunk = {"choices": [{"delta": {"content": text}}]}
                self.wfile.write(f"data: {json.dumps(chunk)}\n\n".encode())
                self.wfile.write(b"data: [DONE]\n\n")
                return
            return self._send({
                "choices": [{"message": {"role": "assistant", "content": text}}],
                "usage": {"prompt_tokens": 111, "completion_tokens": 222}})
        self._send({"error": "unknown path"}, 404)


if __name__ == "__main__":
    port = int(__import__("sys").argv[1]) if len(__import__("sys").argv) > 1 else 18080
    print(f"MOCK Nebius/Tavily on http://127.0.0.1:{port} (X-Nemotron-Healer: MOCK)")
    # Threaded: the DHS search path fires concurrent inference calls.
    ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()
