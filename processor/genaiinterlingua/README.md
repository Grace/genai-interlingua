# genaiinterlingua processor

Normalizes GenAI spans from any recognized instrumentation dialect into one
chosen version of the GenAI semantic conventions, and records on each span what
the translation cost.

Supported pipeline types: traces.

## Why this is its own module

The core of this repository has no dependencies. Keeping the Collector's module
graph out of it means `go test ./...` at the repository root and `go run
./cmd/interlingua` need none of it, so the mapping can be read, run and reviewed
without a Collector build. Every component in `opentelemetry-collector-contrib`
is a separate module for the same reason.

It needs Go 1.26, which is the Collector's own floor rather than this
repository's; the root module is Go 1.25.

## Configuration

```yaml
processors:
  genaiinterlingua:
    # Which schema version to normalize to. One of v1.41.0 or genai-main.
    # Default: v1.41.0
    target: v1.41.0

    # Keep the emitter's own attributes on the span beside the normalized ones.
    # Default: true
    preserve_original: true
```

`target` defaults to the frozen cut and not to the newest schema, because the
newest schema cannot be pinned: `gen_ai.*` was deprecated out of
`open-telemetry/semantic-conventions` at v1.42.0 and now lives in
`open-telemetry/semantic-conventions-genai`, which has no tags and no releases.
There is no schema URL to point at. See `docs/moving-target.md`.

An unrecognized `target` fails validation rather than falling back to the
default. A pipeline normalizing to a different schema than its author asked for
is worse than one that refuses to start.

`preserve_original: false` makes this processor the last reader of its input.
That is a trade worth making only once you trust the mapping for the emitters
you actually run, and it is the only setting here that can destroy data.

### Example

```yaml
receivers:
  otlp:
    protocols:
      grpc:
      http:

processors:
  genaiinterlingua:
    target: v1.41.0

exporters:
  otlp:
    endpoint: your-backend:4317

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [genaiinterlingua]
      exporters: [otlp]
```

## What it does to a span

A span no dialect recognizes passes through untouched. This processor sits in a
pipeline carrying every span in the service, most of which have nothing to do
with GenAI, and stamping them all would be worse than recognizing nothing.

A span it does recognize gains the normalized `gen_ai.*` attributes and four of
its own:

| Attribute | Says |
|---|---|
| `interlingua.dialect` | which emitter's vocabulary was recognized |
| `interlingua.dialect.confidence` | how much more evidence the winner had than the runner-up |
| `interlingua.target` | which schema version the `gen_ai.*` keys belong to |
| `interlingua.lossy` | the keys this span is not a faithful carrier of |

`COMPATIBILITY.md` explains the loss codes and lists the transformations that
change your values without being recorded as losses. `docs/conformance.md` is
the generated table of what each dialect carries.

## Relationship to cmd/interlingua

They are the same function applied to two representations of a span. Every
decision is made by `internal/normalize`, which knows nothing about the
Collector or about OTLP/JSON; this package converts pdata to the neutral input
and applies the returned edit, and the CLI does the same for JSON.

`TestMatchesTheCLIGoldens` holds them to it: it runs every fixture through the
pdata path and compares the result against the output the CLI already committed,
span by span, for both targets.
