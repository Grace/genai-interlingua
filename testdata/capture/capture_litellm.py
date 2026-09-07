# /// script
# requires-python = ">=3.11,<3.14"
# dependencies = [
#   "litellm",
#   "opentelemetry-sdk",
#   "opentelemetry-exporter-otlp-proto-http",
#   "opentelemetry-api",
# ]
# ///
"""Drives LiteLLM through one chat-with-tool-call.

LiteLLM emits OTel through its own callback rather than an instrumentation
package, configured by environment variable, so this sets those up before
importing it.
"""

import os

os.environ["OTEL_EXPORTER_OTLP_ENDPOINT"] = os.environ["SINK"]
os.environ["OTEL_EXPORTER_OTLP_PROTOCOL"] = "http/protobuf"
os.environ["OTEL_SERVICE_NAME"] = "capture"

import litellm  # noqa: E402

litellm.callbacks = ["otel"]

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
    litellm.completion(
        model="openai/gpt-4o-mini",
        api_base=os.environ["MOCK"] + "/v1",
        api_key="mock",
        messages=[
            {"role": "system", "content": "You are a support agent. Use the tools you are given."},
            {"role": "user", "content": "Where is order A-1187?"},
        ],
        tools=TOOLS,
        temperature=0.7,
        top_p=0.95,
        max_tokens=1024,
    )
    print("litellm: done", flush=True)
