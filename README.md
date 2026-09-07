# genai-interlingua

Six GenAI instrumentation libraries. Six different names for the same token
count. One `gen_ai.*` schema out, with everything the translation cost written on
the span.

The hard part is not the renaming. It is that **`gen_ai.*` currently has no
version you can pin.** The attributes were deprecated out of
`open-telemetry/semantic-conventions` at v1.42.0 and moved to
`open-telemetry/semantic-conventions-genai`, which as of 2026-09-07 has zero tags
and zero releases, and took 21 commits in the last 30 days. There is no schema
URL to point at. So "normalize to the GenAI conventions" is an underspecified
instruction, and this repository makes you answer it:

```
-target v1.41.0     # the last tagged cut. frozen, deprecated at source.  (default)
-target genai-main  # current. untagged, moving.
```

That choice is the interesting part, and [`docs/moving-target.md`](docs/moving-target.md)
is where it is argued out.

## Try it

```console
$ cat testdata/openllmetry/in.json | go run ./cmd/interlingua -target v1.41.0
```

An OpenLLMetry span goes in with `gen_ai.usage.prompt_tokens`,
`gen_ai.prompt.0.role`, `traceloop.workflow.name` and 27 more. It comes out
still carrying all of them, plus:

```
gen_ai.provider.name              = openai
gen_ai.operation.name             = chat
gen_ai.request.model              = gpt-4o-mini
gen_ai.response.finish_reasons    = tool_calls
gen_ai.usage.input_tokens         = 412
gen_ai.usage.output_tokens        = 27
gen_ai.input.messages             = [{"role":"system","parts":[…]},…]
gen_ai.output.messages            = [{"role":"assistant","parts":[{"type":"tool_call",…}]}]
gen_ai.tool.definitions           = [{"name":"lookup_order","parameters":{…}}]

interlingua.dialect               = openllmetry
interlingua.dialect.confidence    = 7
interlingua.target                = v1.41.0
interlingua.lossy                 = [gen_ai.completion.*, gen_ai.prompt.*,
                                     gen_ai.prompt.version,
                                     gen_ai.usage.total_tokens,
                                     traceloop.association.properties.user_id]
```

The last line is the point of the project. This span is **not** a faithful
carrier of those five things, and it says so, in band, where a query can find it.
Run the same input with `-target genai-main` and `gen_ai.prompt.version` moves out
of `interlingua.lossy` and onto the span, because that target has an attribute for
it and v1.41.0 does not.

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

One dialect is captured so far. `testdata/capture/capture.sh openllmetry` runs
the real Traceloop SDK against a local mock OpenAI server -- no API key, nothing
sent anywhere -- and records what it puts on the wire. That first capture
immediately found that OpenLLMetry has migrated to the conventions, and that
four attributes it now emits were being silently walked past, including
`gen_ai.usage.reasoning_tokens`: the conventions' own field, misspelled by the
emitter without its `.output` segment. Details in
[`testdata/README.md`](testdata/README.md).

## As a Collector processor

```yaml
processors:
  genaiinterlingua:
    target: v1.41.0
    preserve_original: true
```

[`processor/genaiinterlingua/`](processor/genaiinterlingua/) is a separate Go
module, so the Collector's dependency graph stays out of the core: the mapping
can be read, run and reviewed with `go run` and no Collector build.

It is the same function as the CLI applied to a different representation.
`internal/normalize` returns a *description* of the edit rather than performing
one, so the JSON codec and the pdata processor apply one decision to two shapes.
`TestMatchesTheCLIGoldens` holds them to it by running every fixture through
pdata and comparing against the goldens the CLI committed.

`demo/builder-config.yaml` builds a real Collector with it via `ocb`. That has
been run, not just compiled: a fixture POSTed over OTLP/HTTP comes back with
`gen_ai.usage.input_tokens: Int(412)` and `interlingua.dialect: Str(openllmetry)`,
and the HTTP span sharing its batch comes back with its three original attributes
and nothing added.

## Layout

```
internal/semconv     what each target schema can express. no emitters.
internal/dialect     what each emitter says. no schema versions.
internal/normalize   the only place the two meet, plus the OTLP/JSON codec.
cmd/interlingua      stdin to stdout.
processor/…          the same, as a Collector processor. own module.
testdata/            six dialects x two targets, golden files.
```

`internal/semconv` and `internal/dialect` do not import each other and neither
knows the other exists. Normalizing is exactly the question of what happens when
a dialect says something a target has no words for, and it is worth having one
file where that is the only question being asked.

## Preserving originals

`preserve_original` defaults to **true**, and `-strip-original` inverts it. The
default keeps the emitter's own attributes beside the normalized ones: the
OpenLLMetry fixture goes in with 30 attributes and comes out with 47.

That is the right default anyway: a normalizer that is sometimes wrong should not
also be the last reader of its input. Turn it off once you trust the mapping for
the emitters you actually run.

## Status

Go 1.25 for the core, 1.26 for the processor module (the Collector's floor).
`gofmt`, `go vet ./...` and `go test -race ./...` are green in both modules, and
`-update` is idempotent for the goldens and the conformance table.

CI runs both modules, checks the generated files regenerate identically, and
builds a real Collector to assert the processor registers in it.

Not done yet: captures for the remaining five dialects, and a containerized demo
with Jaeger.
