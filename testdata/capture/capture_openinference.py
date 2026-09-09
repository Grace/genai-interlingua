# /// script
# requires-python = ">=3.11,<3.14"
# dependencies = [
#   "openinference-instrumentation-openai",
#   "openai",
#   "opentelemetry-sdk",
#   "opentelemetry-exporter-otlp-proto-http",
# ]
# ///

# SPDX-License-Identifier: Apache-2.0

"""Drives Arize OpenInference through one chat-with-tool-call."""

import os

from openai import OpenAI
from openinference.instrumentation.openai import OpenAIInstrumentor
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry import trace

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
    print("openinference: done", flush=True)
