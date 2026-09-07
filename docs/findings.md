# What capturing the real libraries found

This repository started with hand-built fixtures: OTLP payloads written from
what the documentation said each library emits, and from what the dialect
parsers had been written to expect. They passed. Every test was green and the
conformance table was full of marks.

Then the fixtures were replaced, one at a time, with spans captured from the
libraries actually running. **Five of the seven captures found something wrong.**

That ratio is the argument for the capture harness, and it is the reason
`docs/conformance.md` records per-dialect whether a row's evidence was captured
or hand-built. A fixture written from documentation tests your reading of the
documentation.

## OpenLLMetry — the library had moved

`traceloop-sdk==0.62.3`

The hand-built fixture had the indexed shape: `gen_ai.prompt.0.role`,
`gen_ai.completion.0.tool_calls.0.name`, `gen_ai.usage.prompt_tokens`,
`gen_ai.system`. That is what OpenLLMetry emitted when the parser was written.

It is not what it emits now. The library has largely migrated to the
conventions, and writes `gen_ai.input.messages`, `gen_ai.provider.name` and
`gen_ai.usage.input_tokens` directly. Detection confidence for the captured span
fell from **7 to 3**, because most of what used to identify OpenLLMetry is now
just conformance.

It also emitted four attributes the parser had never been shown, and walked past
all four in silence:

| attribute | what it is |
| --- | --- |
| `gen_ai.is_streaming` | the old `llm.is_streaming`, moved namespace |
| `gen_ai.usage.reasoning_tokens` | the conventions' own field, misspelled without `.output` |
| `gen_ai.openai.api_base` | vendor extension, no home in the conventions |
| `gen_ai.openai.response.system_fingerprint` | same |

The second one is the interesting one. `gen_ai.usage.reasoning.output_tokens`
exists at both targets; OpenLLMetry writes the same number under a name that is
one path segment short of correct. Nothing was broken and nothing was reported —
the value simply never arrived, and no test could have caught it because no test
knew the attribute existed.

The pre-migration shape did not stop existing when the library moved, so it is
kept as `testdata/openllmetry-legacy/`, hand-built, and both detect as
`openllmetry`.

## OpenInference — the provider was on the span, unread

`openinference-instrumentation-openai==0.1.58`

The parser read `llm.provider` for the provider name and recorded `llm.system`
as a redundant loss, on the reasoning that `gen_ai.provider.name` already
carries what `llm.system` says. That reasoning is sound when both are present —
a model served through Bedrock is `llm.provider: aws`, `llm.system: anthropic`.

The OpenAI instrumentation sets only `llm.system`. So the normalized span came
out with **no provider at all**, while the answer sat on the span, recorded as
redundant to a field that was not there.

`llm.finish_reason` was not read either, so no OpenInference span had ever
carried `gen_ai.response.finish_reasons`.

## LiteLLM — it won its own span by a coin flip

`litellm==1.100.0`

Detection is scored: every dialect reports how much evidence it found, highest
wins, and the margin over the runner-up is written to the span as
`interlingua.dialect.confidence`.

On a captured LiteLLM span, LiteLLM scored 3. So did OpenLLMetry, which reads
the same span as its own through `llm.request.type`, the `gen_ai.completion.N`
prefix and the total token count. LiteLLM won because it is registered first.

A detector whose answer depends on registry order will change its mind for no
reason. `gen_ai.cost.*` counts as evidence now, which breaks the tie on the
merits: the conventions price nothing at any version, so nothing else emits it.

Those same cost attributes — **twelve on one span** — were being walked past.
Costing is a large part of why anyone puts a proxy in front of a model.

The capture also showed LiteLLM writing `metadata.user_api_key_hash`,
`metadata.user_api_key_user_email` and `metadata.requester_ip_address` onto every
span by default. All are recorded as `no_field` and none are mapped, so nothing
gets promoted into a `gen_ai.*` attribute a backend would index.
`COMPATIBILITY.md` says plainly that this is not redaction: the attributes stay
where the emitter put them, and dropping them is an attributes processor's job.

## raw — a bug on the one span with the best claim to being correct

`opentelemetry-instrumentation-openai-v2==2.4b0`

`raw` is the fallback for spans that are already conformant. Its capture comes
from OpenTelemetry's own first-party instrumentation, which makes it the control
case: the reference implementation of the conventions this repository normalizes
*to*.

It emits `openai.response.system_fingerprint`. The fallback ignored it.

`openai.` and `mcp.` are namespaces the conventions themselves own — the v1.42.0
release notes move `model/gen-ai/`, `model/openai/` and `model/mcp/` into the
new repository together — so a leftover in them is GenAI data that could not be
placed, not an unrelated application attribute. Leftovers there are recorded now,
and a test asserts the counterweight: an HTTP span's `http.request.method` must
*not* be reported as a GenAI loss, because a fallback that claims mostly
arbitrary spans would otherwise report the whole world as missing data.

## Braintrust — the parser was fine, the fixture was lying

`braintrust==0.37.0`

Nothing was wrong with the code. Capturing removed three marks the hand-built
fixture had been claiming: `gen_ai.provider.name`,
`gen_ai.usage.cache_read.input_tokens`, and both message fields. Braintrust's
ingestion contract defines none of them.

Same lesson as the others, pointed at the evidence instead of the code. A
fixture that over-claims makes a conformance table that over-claims, and the
table is the thing a reader is asked to trust.

It gained one mark it had never earned: `braintrust.scores` under `flattened`,
because the capture includes a second span carrying several scores and the
conventions carry one evaluation per span.

## The two that found nothing

`ai==5.0.253` and the official OTel instrumentation both came through clean. All
three Vercel span shapes matched what the hand-built fixture claimed, and
`ai.settings.maxOutputTokens` was already handled under its v5 name.

This is worth stating rather than quietly omitting. A harness that only ever
confirms the author was right is not measuring anything, and "we checked and it
was fine" is a result.

## How the captures work

Real libraries, run for real, against a **local mock OpenAI server**. No API
keys and nothing leaves the machine, so anyone who clones the repository can
reproduce every fixture. What is mocked is the model's answer; what is real is
the instrumentation — the attribute names, their shapes, and which of them each
library bothers to set.

Braintrust is the one qualified case, and `testdata/README.md` says so on its own
row: `braintrust.*` is an *ingestion contract* rather than something the SDK
emits, so the runner sets those attributes and what is real is the pipeline
around them.

Each fixture carries three generated files:

| | |
| --- | --- |
| `PROVENANCE` | `captured` or `hand-built`, read by the conformance table |
| `VERSIONS` | the library versions that produced it |
| `KEYS` | its span names and attribute keys |

`KEYS` compares key *sets* rather than bytes, because re-capturing produces new
trace ids and timestamps every time — a byte diff is dirty on every run and
therefore useless as a signal. A key appearing or vanishing is exactly what
OpenLLMetry's migration looked like, and last time that was noticed by a person
reading a capture rather than by anything automatic.

## An independent check

Honeycomb annotates span columns with their semantic-convention definitions,
which turns out to be a second opinion this repository did not have to build:

| column | Honeycomb's description |
| --- | --- |
| `gen_ai.usage.reasoning.output_tokens` | "The number of output tokens used for reasoning…" |
| `gen_ai.usage.reasoning_tokens` | *(none)* |
| `gen_ai.request.stream` | "Indicates whether the GenAI request was made in streaming mode." |
| `gen_ai.is_streaming` | *(none)* |

The described ones are conventions. The bare ones are OpenLLMetry's own
spellings — and both pairs are exactly the mappings added after the capture
above. A source that does not share this repository's assumptions agreeing about
which spelling is real is a better check than any test in it.

Honeycomb also labels `gen_ai.openai.response.system_fingerprint` as
"Deprecated, use `openai.response.system_fingerprint`", which independently
supports treating `openai.*` as a namespace the conventions own. See
[`honeycomb.md`](honeycomb.md).

## What this is evidence for

Every one of these was invisible to a green test suite. Not one was a crash, a
type error, or anything a compiler or a linter could reach. They were attributes
that quietly failed to arrive, and the only way to find them was to run the
thing that produces them and look at what came out.

That is the argument for treating loss as an output rather than a log line, and
for `interlingua.lossy` existing at all. A normalizer that silently drops data
is worse than no normalizer, because you will trust it.

See [`moving-target.md`](moving-target.md) for the other half: which version of
the conventions any of this is being normalized *to*, and why that question does
not currently have a good answer.
