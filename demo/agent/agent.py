# SPDX-License-Identifier: Apache-2.0

"""A support agent, instrumented with OpenLLMetry, talking to a mock model.

This is the same library the capture harness drives, for the same reason: what
makes the demo worth looking at is that the spans arriving at the Collector were
produced by real instrumentation, not written by this repository. The model
behind it is a mock so the demo needs no API key and answers the same way every
time.

It loops rather than running once so that Jaeger has something to show whenever
you open it.
"""

import os
import time

from openai import OpenAI
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from traceloop.sdk import Traceloop
from traceloop.sdk.decorators import workflow

Traceloop.init(
    app_name="support-agent",
    disable_batch=True,
    exporter=OTLPSpanExporter(endpoint=os.environ["OTEL_ENDPOINT"] + "/v1/traces"),
)

client = OpenAI(api_key="mock", base_url=os.environ["MOCK_OPENAI"] + "/v1")

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
def triage(question):
    return client.chat.completions.create(
        model="gpt-4o-mini",
        messages=[
            {"role": "system", "content": "You are a support agent. Use the tools you are given."},
            {"role": "user", "content": question},
        ],
        tools=TOOLS,
        temperature=0.7,
        top_p=0.95,
        max_tokens=1024,
    )


QUESTIONS = [
    "Where is order A-1187?",
    "Has order B-2043 shipped yet?",
    "I need tracking for order C-9910.",
]

if __name__ == "__main__":
    i = 0
    while True:
        question = QUESTIONS[i % len(QUESTIONS)]
        triage(question)
        print(f"asked: {question}", flush=True)
        i += 1
        time.sleep(float(os.environ.get("INTERVAL_SECONDS", "10")))
