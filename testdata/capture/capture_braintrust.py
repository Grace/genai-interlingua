# /// script
# requires-python = ">=3.11,<3.14"
# dependencies = ["braintrust[otel]", "openai", "opentelemetry-sdk"]
# ///

# SPDX-License-Identifier: Apache-2.0

"""Drives Braintrust's OpenTelemetry integration through one chat-with-tool-call
plus one scoring span.

Braintrust is the odd one out among the dialects, and it is worth being precise
about what is real here.

The other four captures record attributes their library invented and wrote by
itself. Braintrust's `braintrust.*` namespace is not that: it is an *ingestion
contract*, the names Braintrust reads off incoming OTel spans and maps into its
own model (`braintrust.input_json` becomes the `input` field). Its own
`wrap_openai` does not emit OTel at all -- it logs through Braintrust's private
transport -- so nothing generates these attributes for you. An application using
Braintrust's OTel integration sets them.

So the runner sets them, from a real model call, in the shapes Braintrust
documents. What is real without any help from this file is the pipeline: a real
BraintrustSpanProcessor filters and forwards the spans, and adds its own
`braintrust.context_json` on the way out.

Nothing reaches Braintrust. BRAINTRUST_API_URL is pinned to the local sink before
the SDK is imported, and the key below is a placeholder -- the SDK requires one
to be present but only ever uses it to build an Authorization header, so no real
credential is involved and this is reproducible by anyone.
"""

import json
import os

os.environ["BRAINTRUST_API_URL"] = os.environ["SINK"]
os.environ["BRAINTRUST_API_KEY"] = "local-capture-not-a-real-key"

from braintrust.otel import BraintrustSpanProcessor  # noqa: E402
from openai import OpenAI  # noqa: E402
from opentelemetry import trace  # noqa: E402
from opentelemetry.sdk.resources import Resource  # noqa: E402
from opentelemetry.sdk.trace import TracerProvider  # noqa: E402
from opentelemetry.sdk.trace.export import SimpleSpanProcessor  # noqa: E402

provider = TracerProvider(resource=Resource.create({"service.name": "capture"}))
provider.add_span_processor(
    BraintrustSpanProcessor(
        api_url=os.environ["SINK"],
        parent="project_name:capture",
        SpanProcessor=SimpleSpanProcessor,
    )
)
trace.set_tracer_provider(provider)
tracer = trace.get_tracer("capture")

client = OpenAI(api_key="mock", base_url=os.environ["MOCK"] + "/v1")

MESSAGES = [
    {"role": "system", "content": "You are a support agent. Use the tools you are given."},
    {"role": "user", "content": "Where is order A-1187?"},
]

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


def main():
    # The model call span. One score, which is the case the conventions can
    # actually express: gen_ai.evaluation.name plus gen_ai.evaluation.score.value.
    with tracer.start_as_current_span("support_triage") as span:
        completion = client.chat.completions.create(
            model="gpt-4o-mini", messages=MESSAGES, tools=TOOLS,
            temperature=0.7, top_p=0.95, max_tokens=1024,
        )
        usage = completion.usage
        choice = completion.choices[0]

        span.set_attribute("braintrust.input_json", json.dumps(MESSAGES))
        span.set_attribute("braintrust.output_json", json.dumps([choice.message.model_dump()]))
        span.set_attribute("braintrust.expected_json", json.dumps({"tool": "lookup_order"}))
        span.set_attribute("braintrust.metrics", json.dumps({
            "prompt_tokens": usage.prompt_tokens,
            "completion_tokens": usage.completion_tokens,
            "tokens": usage.total_tokens,
        }))
        span.set_attribute("braintrust.scores", json.dumps({"correctness": 1.0}))
        span.set_attribute("braintrust.metadata", json.dumps({"model": completion.model}))
        span.set_attribute("braintrust.tags", json.dumps(["support", "triage"]))
        span.set_attribute("braintrust.span_attributes", json.dumps({"type": "llm", "name": "support_triage"}))
        span.set_attribute("gen_ai.request.model", "gpt-4o-mini")
        span.set_attribute("gen_ai.response.model", completion.model)

    # A second span carrying several scores, which the conventions cannot
    # express: one evaluation per span. This is the ReasonFlattened path, and a
    # fixture that never exercises it proves nothing about it.
    with tracer.start_as_current_span("grade") as span:
        span.set_attribute("braintrust.scores", json.dumps({
            "correctness": 1.0, "helpfulness": 0.8, "tone": 0.9,
        }))
        span.set_attribute("braintrust.span_attributes", json.dumps({"type": "score", "name": "grade"}))

    provider.force_flush()
    print("braintrust: done", flush=True)


if __name__ == "__main__":
    main()
