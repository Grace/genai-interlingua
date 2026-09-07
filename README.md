# genai-interlingua

**Normalize LLM and GenAI spans from any instrumentation library into one
`gen_ai.*` schema, and record on every span exactly what the translation cost.**

Six libraries. Six different names for the same token count. One schema out.

Runs as an [OpenTelemetry Collector processor](processor/genaiinterlingua/) or a
[standalone CLI](cmd/interlingua/). No dependencies in the core.

---

## The problem

You are running LangChain in one service, the Vercel AI SDK in another, and
LiteLLM in front of both. Your traces have three vocabularies for the same
model call:

| | prompt tokens | provider |
| --- | --- | --- |
| OpenLLMetry (older) | `gen_ai.usage.prompt_tokens` | `gen_ai.system` |
| OpenInference | `llm.token_count.prompt` | `llm.provider`, or `llm.system` |
| Vercel AI SDK | `ai.usage.promptTokens` | `ai.model.provider` |
| the conventions | `gen_ai.usage.input_tokens` | `gen_ai.provider.name` |

You cannot write one dashboard over that. So you normalize — and immediately hit
the question this repository is actually about.

**`gen_ai.*` currently has no version you can pin.** The attributes were
deprecated out of `open-telemetry/semantic-conventions` at v1.42.0 and moved to
`open-telemetry/semantic-conventions-genai`, which as of 2026-09-07 has **zero
tags, zero releases, and 21 commits in the last 30 days**. There is no schema URL
to point at. "Normalize to the GenAI conventions" is an underspecified
instruction.

So the target is an explicit choice, not a constant hidden in a table:

```
-target v1.41.0     # the last tagged cut. frozen, deprecated at source.  (default)
-target genai-main  # current. untagged, moving.
```

The default is the frozen cut, because it is the only one of the two a reader can
reconstruct six months from now. [`docs/moving-target.md`](docs/moving-target.md)
argues it out.

## Why you would use it

**You get one vocabulary.** One dashboard, one alert, one SLO over every model
call, regardless of which library instrumented it.

**You find out what it cost you.** This is the part other normalizers skip. Every
span carries `interlingua.lossy`: the list of keys this span is *not* a faithful
carrier of. A translation layer that silently drops data is worse than no
translation layer, because you will trust the result.

**It never destroys your originals by default.** `preserve_original` is on. The
emitter's own attributes stay on the span next to the normalized ones, so a
mapping this repository got wrong is a mapping you can still see through.

**Detection is a judgement, and says so.** Dialects are scored, not first-matched.
The winner's margin over the runner-up is written to the span as
`interlingua.dialect.confidence`, so a misdetection is a thing you can query for
rather than a thing you discover later.

**The claims are checked by tests, not by prose.**
[`docs/conformance.md`](docs/conformance.md) is generated from the fixtures, and
CI fails if it disagrees with them. Five of the six dialects are backed by spans
captured from the real libraries, and the table says which — a row cannot borrow
credibility it did not earn.

## Install

```console
$ go install github.com/Grace/genai-interlingua/cmd/interlingua@latest
```

Or grab a binary from [releases](https://github.com/Grace/genai-interlingua/releases),
or build the Collector processor into your own distribution (below).

## Use it: the CLI

Reads an OTLP/JSON trace export on stdin, writes one on stdout.

```console
$ cat testdata/openllmetry-legacy/in.json | interlingua -target v1.41.0
```

That span arrives with 30 attributes in OpenLLMetry's own vocabulary. It leaves
with all 30 still on it, plus:

```
gen_ai.provider.name           = openai            # was gen_ai.system = "OpenAI"
gen_ai.operation.name          = chat              # was llm.request.type
gen_ai.request.model           = gpt-4o-mini
gen_ai.response.finish_reasons = tool_calls
gen_ai.usage.input_tokens      = 412               # was gen_ai.usage.prompt_tokens
gen_ai.usage.output_tokens     = 27                # was gen_ai.usage.completion_tokens

interlingua.dialect            = openllmetry
interlingua.dialect.confidence = 7
interlingua.target             = v1.41.0
interlingua.lossy              = [gen_ai.completion.*, gen_ai.prompt.*,
                                  gen_ai.prompt.version,
                                  gen_ai.usage.total_tokens,
                                  traceloop.association.properties.user_id]
```

**Read the last one first.** This span is not a faithful carrier of those five
things. `gen_ai.usage.total_tokens` has no home in the conventions at all. The
indexed `gen_ai.prompt.N.*` attributes were flattened into one
`gen_ai.input.messages`. And `gen_ai.prompt.version` was dropped because
**v1.41.0 has no attribute for it** — run the same command with `-target
genai-main` and it moves out of `interlingua.lossy` and onto the span.

Flags:

| | |
| --- | --- |
| `-target` | `v1.41.0` (default) or `genai-main` |
| `-strip-original` | drop the source attributes the dialect consumed. Off by default. |
| `-version` | print version and default target |

## Use it: the Collector processor

```yaml
processors:
  genaiinterlingua:
    target: v1.41.0
    preserve_original: true

service:
  pipelines:
    traces:
      processors: [genaiinterlingua, batch]
```

Build it into a distribution with [`ocb`](https://opentelemetry.io/docs/collector/custom-collector/):

```yaml
processors:
  - gomod: github.com/Grace/genai-interlingua/processor/genaiinterlingua v0.1.0
```

[`processor/genaiinterlingua/`](processor/genaiinterlingua/) is a separate Go
module, so the Collector's dependency graph stays out of the core: the mapping
can be read, run and reviewed with `go run` and no Collector build.

It is the same function as the CLI applied to a different representation.
`internal/normalize` returns a *description* of the edit rather than performing
one, so the JSON codec and the pdata processor apply one decision to two shapes.
`TestMatchesTheCLIGoldens` holds them to it by running every fixture through
pdata and comparing against the goldens the CLI committed.

An unrecognized span passes through untouched, with no `interlingua.*` on it.
This sits in a pipeline where most spans are HTTP.

## See it

```console
$ docker compose -f demo/compose.yaml up --build
$ open http://localhost:16686
```

Four services: an agent instrumented with **real OpenLLMetry**, a mock model,
this processor in an `ocb`-built Collector, and Jaeger. Nothing reaches the
internet and no API key is needed.

![A normalized GenAI span in Jaeger, showing gen_ai.* attributes alongside interlingua.dialect, interlingua.lossy and interlingua.target](docs/img/jaeger-normalized-span.jpg)

That is a real span from that stack, and it is worth reading closely:

- **`gen_ai.usage.reasoning_tokens: 8`** is what OpenLLMetry emitted — the
  conventions' field, misspelled without its `.output` segment.
  **`gen_ai.usage.reasoning.output_tokens: 8`** is the normalized one directly
  below it. *(That mapping exists because capturing a real span found it
  missing.)*
- **`gen_ai.usage.total_tokens: 439`** is on the span **and** named in
  `interlingua.lossy`. `preserve_original` did not throw it away, and the span
  says plainly that the conventions have no field for it.
- **`interlingua.dialect: openllmetry`** with **`confidence: 3`** — low, and
  honestly so: current OpenLLMetry has largely migrated to the conventions, so
  most of what used to identify it is now just conformance.
- **`interlingua.target: v1.41.0`** — which vocabulary those `gen_ai.*` keys
  belong to, on the span, because there is no schema URL to carry it.

The Collector's OTLP port is published, so you can push any fixture in from the
host — `testdata/litellm/in.json` comes back with 49 entries in
`interlingua.lossy`, because LiteLLM writes a great deal the conventions have no
words for.

## What it recognizes

| Dialect | What it is |
| --- | --- |
| `openllmetry` | Traceloop / OpenLLMetry, `gen_ai.prompt.N.*` and `traceloop.*` |
| `openinference` | Arize OpenInference, `llm.*` with indexed messages |
| `vercel` | Vercel AI SDK, `ai.*` outer spans and `gen_ai.*` adapter spans |
| `litellm` | LiteLLM, largely already conformant plus `litellm.*` and `metadata.*` |
| `braintrust` | Braintrust, scores and metrics in packed JSON blobs |
| `raw` | fallback for spans that are already conformant, or nearly |

Detection is scored, not first-match: every dialect reports how much evidence it
found, the highest score wins, and the margin over the runner-up is recorded as
`interlingua.dialect.confidence`. A confidence of `0` next to a surprising
dialect is a misdetection you can query for, which is why it goes on the span
instead of into a log line.

The fallback is deliberately kept out of that contest until every real dialect
has declined, so that backing up the dialects does not dilute their margins.

A span carrying no GenAI evidence is returned exactly as it arrived, with no
`interlingua.*` on it at all. This runs in a pipeline where most spans are HTTP.

## Loss is an output, not a log line

Two separate vocabularies, because they are two different people's fault:

- **`dialect.Reason`** — what the emitter said that the intermediate
  representation could not hold. `no_field`, `unstructured`, `flattened`,
  `coerced`, `ambiguous`.
- **`normalize.Reason`** — what survived that, and then hit a target schema with
  no attribute (`no_attribute`) or no such value (`no_value`) for it.

`interlingua.lossy` flattens both into one list, because a consumer scanning for
a key they care about does not care which half of the pipeline dropped it.

[`COMPATIBILITY.md`](COMPATIBILITY.md) explains every code, and lists the
transformations that change your data *without* being losses: provider names
rewritten, `gen_ai.system` renamed, `tool-calls` respelled to `tool_calls`,
Vercel's milliseconds divided into the conventions' seconds. Those are the ones
worth reading, because they change your values silently and correctly.

## The conformance table is generated

[`docs/conformance.md`](docs/conformance.md) is written by
`TestConformanceTable`, not by hand, from the fixtures themselves. Fields are
recovered from the rendered attribute keys, so a dialect gets credit for a field
only if the whole pipeline actually emitted it. Editing the table to claim
something the fixtures do not demonstrate fails the test.

Its header says what it is and is not: a mark is evidence, a blank is silence.
It also reports, per dialect, whether the inputs behind a row were **captured**
from a running library or **hand-built** here, read from a marker file next to
each fixture so that account cannot drift from the truth.

Five of the six are captured -- everything except `raw`, which is the fallback
for already-conformant spans and so has no library to capture from.
`testdata/capture/capture.sh` runs the real libraries against a local mock
OpenAI server -- no API key, nothing sent anywhere -- and records what they put
on the wire.

Three of those four captures found a bug that hand-built fixtures had hidden:

- **OpenLLMetry** has migrated to the conventions, and four attributes it now
  emits were being silently walked past -- including
  `gen_ai.usage.reasoning_tokens`, the conventions' own field misspelled by the
  emitter without its `.output` segment.
- **OpenInference** sets `llm.system` and no `llm.provider` on OpenAI calls, so
  the provider was being recorded as a redundant loss and discarded while the
  answer sat unread on the span. Its `llm.finish_reason` was not read at all.
- **LiteLLM** scored a dead tie with OpenLLMetry on its own span, winning only
  by being registered first. It also emits twelve `gen_ai.cost.*` attributes,
  none of which the conventions price at any version and none of which were
  being recorded.

The Vercel capture found nothing, which is the other useful outcome. The
Braintrust one found nothing wrong with the *parser* but removed three marks the
hand-built fixture had been claiming, which is the same lesson pointed at the
evidence instead of the code.

`raw` stays hand-built because it is the fallback for arbitrary conformant spans
rather than any library's output. Details, including why braintrust's row should
be read differently from the rest, in
[`testdata/README.md`](testdata/README.md).

## Layout

```
internal/semconv     what each target schema can express. no emitters.
internal/dialect     what each emitter says. no schema versions.
internal/normalize   the only place the two meet, plus the OTLP/JSON codec.
cmd/interlingua      stdin to stdout.
processor/…          the same, as a Collector processor. own module.
testdata/            seven fixtures x two targets, plus the capture harness.
```

`internal/semconv` and `internal/dialect` do not import each other and neither
knows the other exists. Normalizing is exactly the question of what happens when
a dialect says something a target has no words for, and it is worth having one
file where that is the only question being asked.

## Preserving originals

`preserve_original` defaults to **true**, and `-strip-original` inverts it. The
default keeps the emitter's own attributes beside the normalized ones: the
`openllmetry-legacy` span used above goes in with 30 attributes and comes out
with 47. (The HTTP span sharing that trace goes in with 3 and comes out with 3.)

That is the right default anyway: a normalizer that is sometimes wrong should not
also be the last reader of its input. Turn it off once you trust the mapping for
the emitters you actually run.

## Status

Go 1.25 for the core, 1.26 for the processor module (the Collector's floor).
`gofmt`, `go vet ./...` and `go test -race ./...` are green in both modules, and
`-update` is idempotent for the goldens and the conformance table.

CI runs both modules, checks the generated files regenerate identically, and
builds a real Collector to assert the processor registers in it.

Released with goreleaser on a `v*` tag, after the other three jobs pass.

Five of the six dialects are backed by captured spans; `raw` is the fallback for
already-conformant spans and has no library to capture from.
