# Capture harness

Records what an instrumentation library actually puts on the wire, into
`testdata/<dialect>/in.json`.

```console
$ ./testdata/capture/capture.sh openllmetry   # one dialect
$ ./testdata/capture/capture.sh all           # every dialect with a runner
$ go test ./internal/normalize -update        # regenerate goldens and the table
```

Nothing here talks to a model provider, and no API key is needed. Two local
servers stand in:

- **`mockopenai.py`** answers `/v1/chat/completions` with the same completion
  every time: a tool call rather than a text reply, and a usage block carrying
  cache and reasoning details. Deterministic, so re-running produces the same
  fixture, and chosen to exercise the mappings that are actually interesting.
- **`sink.py`** is an OTLP/HTTP receiver that writes one export to a file as
  OTLP/JSON. Every SDK here exports protobuf, so it decodes protobuf and
  re-encodes it in the JSON mapping rather than asking the SDKs for a JSON
  encoding not all of them have.

What is *real* is the instrumentation. The attribute names, their shapes, and
which of them the library bothers to set are whatever that version of the
library decided.

## Why not capture through a Collector

The Collector would do the sink's job with the file exporter, and it was tempting
because this repository already builds one. But a fixture captured through a
Collector has been shaped by that Collector's receiver and exporter, and the
thing under test is what the *library* emitted. The sink is deliberately the
dumbest possible OTLP endpoint.

## Adding a dialect

Add a program named after the dialect that reads two environment variables,
`SINK` and `MOCK`, and makes one instrumented model call:

- Python: `<dialect>.py`, with its dependencies in a PEP 723 header so `uv run`
  needs no environment management.
- Node: `<dialect>.mjs`.

Then add the name to `DIALECTS` in `capture.sh`, and a `PROVENANCE` file reading
`captured` in the fixture directory. `docs/conformance.md` reads that marker, so
a captured fixture that forgets it will under-report itself, and a hand-built one
that claims `captured` is a lie the table will repeat.

## The captures are not reproducible byte for byte

Trace IDs, span IDs and timestamps differ on every run, so re-capturing always
produces a diff. That is fine: captures are run deliberately, not in CI. What CI
does check is that `-update` is idempotent, which is a property of the
normalizer rather than of the capture.
