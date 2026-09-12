# /// script
# requires-python = ">=3.11,<3.14"
# dependencies = ["traceloop-sdk", "openai", "httpx"]
# ///

# SPDX-License-Identifier: Apache-2.0

"""Drives OpenLLMetry (Traceloop) through one chat-with-tool-call.

The workflow decorator and the prompt association are here because the dialect
parser reads traceloop.workflow.name and traceloop.prompt.*, and a fixture that
never exercises them proves nothing about that half of the mapping.
"""

import os

from openai import OpenAI
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from traceloop.sdk import Traceloop
from traceloop.sdk.decorators import workflow

Traceloop.init(
    app_name="capture",
    disable_batch=True,
    exporter=OTLPSpanExporter(endpoint=os.environ["SINK"] + "/v1/traces"),
)

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


@workflow(name="support_triage_agent")
def triage():
    return client.chat.completions.create(
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


if __name__ == "__main__":
    triage()
    print("openllmetry: done", flush=True)
