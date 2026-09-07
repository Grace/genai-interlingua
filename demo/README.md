# Demo

A GenAI span's whole trip: real instrumentation, through the normalizer, into a
trace UI.

```console
$ docker compose -f demo/compose.yaml up --build
$ open http://localhost:16686                              # Jaeger, service "support-agent"
$ docker compose -f demo/compose.yaml logs -f collector    # the same spans, with the losses
```

Four services. Nothing reaches the internet at run time and no API key is needed.

| Service | What it is |
| --- | --- |
| `agent` | a support agent instrumented with **real OpenLLMetry**, asking about orders on a loop |
| `mock-openai` | the same deterministic mock the capture harness uses |
| `collector` | a Collector built by `ocb` with `genaiinterlingua` in it, exporting to Jaeger |
| `jaeger` | the trace UI |

The agent is the point. The spans arriving at the Collector were produced by the
Traceloop SDK doing what it normally does, not written by this repository, which
is the same reason `testdata/capture/` exists.

## What to look at

**In Jaeger**, the `openai.chat` span carries `gen_ai.provider.name`,
`gen_ai.operation.name` and `gen_ai.usage.input_tokens` -- none of which
OpenLLMetry emitted under those names -- alongside `interlingua.dialect`,
`interlingua.dialect.confidence` and `interlingua.target`.

**In the collector logs**, the same span carries `interlingua.lossy`. That is the
part a trace UI will not make obvious and the part this repository is actually
about: a list of the keys this span is *not* a faithful carrier of.

## Changing the target

`demo/otelcol-demo.yaml` sets `target: v1.41.0`. Change it to `genai-main`,
`docker compose -f demo/compose.yaml restart collector`, and every span's
`interlingua.target` changes.

For this agent that is the *only* thing that changes, and it is worth being
clear about why rather than implying more: everything OpenLLMetry emits here is
expressible at both targets, so pinning the frozen cut costs nothing. Pinning
costs you when an emitter produces a field the frozen cut has no attribute for.
The comment in `otelcol-demo.yaml` gives a one-liner over
`testdata/openllmetry-legacy` that shows exactly that, with
`gen_ai.prompt.version` present at one target and named in `interlingua.lossy`
at the other. See `docs/moving-target.md`.

## Sending your own span

The Collector's OTLP/HTTP port is published, so any fixture in this repository
can be pushed in from the host:

```console
$ curl -X POST localhost:4318/v1/traces \
    -H 'Content-Type: application/json' \
    -d @testdata/litellm/in.json
```

That one is worth trying: it comes back with 49 entries in `interlingua.lossy`,
because LiteLLM writes a great deal that the conventions have no words for.

## Running the Collector without Docker

`builder-config.yaml` and `otelcol.yaml` are the same thing by hand:

```console
$ go run go.opentelemetry.io/collector/cmd/builder@v0.160.0 --config demo/builder-config.yaml
$ ./demo/build/interlingua-collector --config demo/otelcol.yaml
```

`demo/build/` is generated and gitignored.
