# /// script
# requires-python = ">=3.11"
# dependencies = ["opentelemetry-proto>=1.29", "protobuf>=5"]
# ///
"""An OTLP/HTTP trace receiver that writes exactly one export to a file as
OTLP/JSON.

The real Collector would do this with the file exporter, but capturing fixtures
through a Collector means the fixtures are shaped by whatever that Collector's
receiver and exporter did to them. This writes down what the instrumentation
library actually put on the wire, which is the thing the dialect parsers claim
to understand.

Every SDK here exports OTLP over HTTP as protobuf, so this decodes protobuf and
re-encodes as the JSON mapping rather than asking the SDKs for JSON they cannot
all produce.
"""

import base64
import gzip
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

from google.protobuf.json_format import MessageToDict
from opentelemetry.proto.collector.trace.v1.trace_service_pb2 import (
    ExportTraceServiceRequest,
)

OUT = sys.argv[1]
PORT = int(sys.argv[2]) if len(sys.argv) > 2 else 4318

# OTLP/JSON spells trace and span IDs as lowercase hex, which is a deliberate
# deviation from the protobuf JSON mapping's base64. MessageToDict gives us
# base64, so these fields are converted back.
ID_FIELDS = ("traceId", "spanId", "parentSpanId")


def hexify(node):
    if isinstance(node, dict):
        return {
            k: (base64.b64decode(v).hex() if k in ID_FIELDS and isinstance(v, str) else hexify(v))
            for k, v in node.items()
        }
    if isinstance(node, list):
        return [hexify(v) for v in node]
    return node


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        if self.headers.get("Content-Encoding") == "gzip":
            body = gzip.decompress(body)

        req = ExportTraceServiceRequest()
        req.ParseFromString(body)

        # preserving_proto_field_name=False gives the lowerCamelCase spelling
        # OTLP/JSON uses. including_default_value_fields is left off so that an
        # unset field is absent rather than zero, matching what the Collector's
        # own JSON marshaler emits.
        out = hexify(MessageToDict(req))

        with open(OUT, "a") as f:
            f.write(json.dumps(out) + "\n")

        self.send_response(200)
        self.send_header("Content-Type", "application/x-protobuf")
        self.end_headers()
        self.wfile.write(b"")

    def log_message(self, *args):
        pass


if __name__ == "__main__":
    print(f"sink listening on :{PORT}, appending to {OUT}", flush=True)
    HTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
