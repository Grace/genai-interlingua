# /// script
# requires-python = ">=3.11,<3.14"
# dependencies = [
#   "langchain",
#   "langchain-openai",
#   "opentelemetry-instrumentation-langchain",
#   "opentelemetry-sdk",
#   "opentelemetry-exporter-otlp-proto-http",
#   "httpx",
# ]
# ///

# SPDX-License-Identifier: Apache-2.0

"""Drives a LangChain chain with a tool, instrumented for OpenTelemetry.

LangChain is named in a lot of job descriptions as its own telemetry ecosystem,
and it is worth being exact about what that means. LangChain emits no
OpenTelemetry of its own. Its spans come from whichever instrumentation you
attach -- here Traceloop's opentelemetry-instrumentation-langchain, elsewhere
OpenInference's -- so a "LangChain span" is really an OpenLLMetry or
OpenInference span describing a LangChain call.

That is the finding rather than a problem, and it is why this fixture does not
add a seventh dialect. What it proves is that a LangChain-shaped trace, with its
chain and its nested model call, normalizes correctly -- which is a different
claim from any of the single-call fixtures, and the one an application built on
LangChain actually needs.
"""

import os

from langchain_core.prompts import ChatPromptTemplate  # noqa: E402
from langchain_openai import ChatOpenAI  # noqa: E402
from opentelemetry import trace  # noqa: E402
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter  # noqa: E402
from opentelemetry.instrumentation.langchain import LangchainInstrumentor  # noqa: E402
from opentelemetry.sdk.resources import Resource  # noqa: E402
from opentelemetry.sdk.trace import TracerProvider  # noqa: E402
from opentelemetry.sdk.trace.export import SimpleSpanProcessor  # noqa: E402

provider = TracerProvider(resource=Resource.create({"service.name": "capture"}))
provider.add_span_processor(
    SimpleSpanProcessor(OTLPSpanExporter(endpoint=os.environ["SINK"] + "/v1/traces"))
)
trace.set_tracer_provider(provider)
LangchainInstrumentor().instrument(tracer_provider=provider)

llm = ChatOpenAI(
    model="gpt-4o-mini",
    api_key="mock",
    base_url=os.environ["MOCK"] + "/v1",
    temperature=0.7,
    top_p=0.95,
    max_tokens=1024,
)

prompt = ChatPromptTemplate.from_messages([
    ("system", "You are a support agent. Use the tools you are given."),
    ("human", "{question}"),
])

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
    chain = prompt | llm.bind(tools=TOOLS)
    chain.invoke({"question": "Where is order A-1187?"})
    provider.force_flush()
    print("langchain: done", flush=True)
