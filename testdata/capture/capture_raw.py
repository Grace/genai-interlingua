# /// script
# requires-python = ">=3.11,<3.14"
# dependencies = [
#   "opentelemetry-instrumentation-openai-v2",
#   "openai",
#   "opentelemetry-sdk",
#   "opentelemetry-exporter-otlp-proto-http",
#   "httpx",
# ]
# ///

# SPDX-License-Identifier: Apache-2.0

"""Drives OpenTelemetry's own first-party OpenAI instrumentation.

This is the control case, and the reason it belongs in the fixtures at all: it is
the reference implementation of the conventions this repository normalizes *to*.
A span from here should need almost no translation, and the `raw` dialect exists
to recognize exactly that -- a span already speaking the target vocabulary.

If this capture ever shows `raw` failing to claim these spans, or the normalizer
rewriting attributes that were already correct, that is a far more serious
finding than any single dialect's mapping bug.

It is also the sharpest available evidence for docs/moving-target.md. The
conventions were split out of open-telemetry/semantic-conventions at v1.42.0 and
the new repository has never tagged a release, so the official instrumentation is
tracking a moving target too, and which vocabulary it emits today is a fact worth
recording rather than assuming.

Message content is off by default in this instrumentation, on the grounds that
prompts are user data. It is switched on here because the fixture is talking to a
mock and the point is to exercise the mapping.

The switch takes NO_CONTENT / SPAN_ONLY / EVENT_ONLY / SPAN_AND_EVENT rather than
the boolean it once did, and an unrecognized value degrades quietly to
NO_CONTENT with a warning rather than failing -- so a fixture captured with the
old spelling would silently contain no messages at all.
"""

import os

os.environ["OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT"] = "SPAN_AND_EVENT"

from openai import OpenAI  # noqa: E402
from opentelemetry import trace  # noqa: E402
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter  # noqa: E402
from opentelemetry.instrumentation.openai_v2 import OpenAIInstrumentor  # noqa: E402
from opentelemetry.sdk.resources import Resource  # noqa: E402
from opentelemetry.sdk.trace import TracerProvider  # noqa: E402
from opentelemetry.sdk.trace.export import SimpleSpanProcessor  # noqa: E402

provider = TracerProvider(resource=Resource.create({"service.name": "capture"}))
provider.add_span_processor(
    SimpleSpanProcessor(OTLPSpanExporter(endpoint=os.environ["SINK"] + "/v1/traces"))
)
trace.set_tracer_provider(provider)
OpenAIInstrumentor().instrument(tracer_provider=provider)

client = OpenAI(api_key="mock", base_url=os.environ["MOCK"] + "/v1")

TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "lookup_order",
            "description": "Look up an order by its identifier.",
            "parameters": {
                "type": "object",
                "properties": {"order_id": {"type": "string"}},
                "required": ["order_id"],
            },
        },
    }
]

if __name__ == "__main__":
    client.chat.completions.create(
        model="gpt-4o-mini",
        messages=[
            {"role": "system", "content": "You are a support agent. Use the tools you are given."},
            {"role": "user", "content": "Where is order A-1187?"},
        ],
        tools=TOOLS,
        temperature=0.7,
        top_p=0.95,
        max_tokens=1024,
    )
    provider.force_flush()
    print("raw: done", flush=True)
