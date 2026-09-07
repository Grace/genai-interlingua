# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///
"""A deterministic OpenAI-compatible chat completions server.

Fixtures captured against the real API would change every time the model does,
and would need a key to regenerate. This returns the same response every time,
chosen to exercise the mappings that are actually interesting: a tool call
rather than a text reply, and a usage block with the cache and reasoning
details that only some dialects carry.

Only the endpoints the instrumentation libraries actually call are implemented.
An endpoint that is missing is better than one that returns a plausible lie.
"""

import json
import os
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8080

# Loopback by default: when the capture harness runs this, it is a server on a
# developer's laptop that answers as OpenAI, and it has no business being
# reachable from the network. The demo overrides it to 0.0.0.0 because there it
# has to be reachable from another container, which is a deliberate choice made
# in one place rather than a default everyone inherits.
HOST = os.environ.get("MOCK_HOST", "127.0.0.1")

COMPLETION = {
    "id": "chatcmpl-CD8yqQ2y3kZs1o0Wm7bT",
    "object": "chat.completion",
    "created": 1789000000,
    "model": "gpt-4o-mini-2024-07-18",
    "system_fingerprint": "fp_capture0",
    "choices": [
        {
            "index": 0,
            "message": {
                "role": "assistant",
                "content": None,
                "tool_calls": [
                    {
                        "id": "call_9RtYbK2mXqLp",
                        "type": "function",
                        "function": {
                            "name": "lookup_order",
                            "arguments": '{"order_id": "A-1187"}',
                        },
                    }
                ],
            },
            "logprobs": None,
            "finish_reason": "tool_calls",
        }
    ],
    "usage": {
        "prompt_tokens": 412,
        "completion_tokens": 27,
        "total_tokens": 439,
        "prompt_tokens_details": {"cached_tokens": 256, "audio_tokens": 0},
        "completion_tokens_details": {
            "reasoning_tokens": 8,
            "audio_tokens": 0,
            "accepted_prediction_tokens": 0,
            "rejected_prediction_tokens": 0,
        },
    },
}

MODELS = {
    "object": "list",
    "data": [{"id": "gpt-4o-mini", "object": "model", "created": 1789000000, "owned_by": "mock"}],
}


class Handler(BaseHTTPRequestHandler):
    def _json(self, payload, code=200):
        body = json.dumps(payload).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        self.rfile.read(int(self.headers.get("Content-Length", 0)))
        if self.path.endswith("/chat/completions"):
            self._json(COMPLETION)
        else:
            self._json({"error": {"message": f"no mock for {self.path}"}}, 404)

    def do_GET(self):
        if self.path.endswith("/models"):
            self._json(MODELS)
        else:
            self._json({"error": {"message": f"no mock for {self.path}"}}, 404)

    def log_message(self, *args):
        pass


if __name__ == "__main__":
    print(f"mock openai listening on {HOST}:{PORT}", flush=True)
    HTTPServer((HOST, PORT), Handler).serve_forever()
