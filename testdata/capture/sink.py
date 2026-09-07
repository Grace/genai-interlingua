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
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

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
    # The JavaScript exporter opens several connections at once and keeps them
    # alive. A single-threaded server answers one and hangs up the rest, which
    # surfaces in the runner as ECONNRESET rather than as anything about this
    # file, so the server threads and speaks HTTP/1.1.
    protocol_version = "HTTP/1.1"

    def read_body(self):
        # The JavaScript exporter sends chunked bodies with no Content-Length.
        # Reading Content-Length or nothing gives an empty body, and an empty
        # body fails to parse in a way that names json rather than the transfer
        # encoding, so both framings are handled here.
        if self.headers.get("Transfer-Encoding", "").lower() == "chunked":
            chunks = []
            while True:
                size = int(self.rfile.readline().split(b";")[0], 16)
                if size == 0:
                    self.rfile.readline()  # trailing CRLF
                    break
                chunks.append(self.rfile.read(size))
                self.rfile.readline()  # CRLF after each chunk
            return b"".join(chunks)
        return self.rfile.read(int(self.headers.get("Content-Length", 0)))

    def do_POST(self):
        body = self.read_body()
        if self.headers.get("Content-Encoding") == "gzip":
            body = gzip.decompress(body)

        # The Python SDKs export protobuf; the JavaScript OTLP-HTTP exporter
        # exports JSON. Guessing wrong is silent -- ParseFromString on a JSON
        # body yields an empty message rather than an error -- so the content
        # type decides, and an unrecognized one is refused rather than parsed
        # hopefully.
        content_type = self.headers.get("Content-Type", "").split(";")[0].strip()
        if content_type == "application/json":
            # Already the JSON mapping, including hex ids. Nothing to convert.
            out = json.loads(body)
        elif content_type in ("application/x-protobuf", "application/protobuf"):
            req = ExportTraceServiceRequest()
            req.ParseFromString(body)
            # preserving_proto_field_name=False gives the lowerCamelCase
            # spelling OTLP/JSON uses. Default values are left out so that an
            # unset field is absent rather than zero, matching what the
            # Collector's own JSON marshaler emits.
            out = hexify(MessageToDict(req))
        else:
            self.send_response(415)
            self.send_header("Content-Length", "0")
            self.end_headers()
            print(f"refusing Content-Type {content_type!r}", flush=True)
            return

        # An exporter may send an empty export on shutdown. Writing it would
        # satisfy the caller's "wait until the file is non-empty" loop before
        # the real spans arrive, which is a race that looks like a capture
        # producing nothing.
        if out.get("resourceSpans"):
            with open(OUT, "a") as f:
                f.write(json.dumps(out) + "\n")

        # A partial success response is empty in both encodings, but the client
        # parses it according to the content type it is given, so echo back the
        # one it sent rather than always claiming protobuf.
        payload = b"{}" if content_type == "application/json" else b""
        self.send_response(200)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, *args):
        pass


if __name__ == "__main__":
    print(f"sink listening on :{PORT}, appending to {OUT}", flush=True)
    ThreadingHTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
