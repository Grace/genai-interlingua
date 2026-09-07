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

## The one that is not captured

`raw/` is synthetic by definition. It is the fallback for a span that is already
conformant, or nearly, and no single library emits it -- capturing "a span some
other tool produced" would just be picking one of the others again.

## Braintrust, and why it needs no key after all

Braintrust looked uncapturable because its SDK refuses to start without
`BRAINTRUST_API_KEY`. Reading the SDK shows the requirement is shallower than it
appears: the key is checked for presence, then used only to build an
`Authorization` header on the OTLP export. It is never validated, and no login
happens at init.

So `capture_braintrust.py` passes a placeholder, sets `BRAINTRUST_API_URL` to the
local sink *before importing the SDK*, and passes `api_url` explicitly. The
processor builds its endpoint as `{api_url}/otel/v1/traces`, which the sink
accepts because it ignores the request path. Nothing reaches Braintrust, no real
credential is involved, and the fixture is reproducible by anyone.

If you are changing this file, keep that ordering. `BRAINTRUST_API_URL` must be
set before the import, so that any code path which reads it at import time still
points at localhost rather than defaulting to `https://api.braintrust.dev`.

`testdata/README.md` explains why braintrust's conformance row should be read
differently from the other four even though all five are captured.

## Adding a dialect

Add a program named after the dialect that reads two environment variables,
`SINK` and `MOCK`, and makes one instrumented model call:

- Python: `capture_<dialect>.py`, with its dependencies in a PEP 723 header so `uv run`
  needs no environment management.
- Node: `capture_<dialect>.mjs`.

Then add the name to `DIALECTS` in `capture.sh`, and a `PROVENANCE` file reading
`captured` in the fixture directory. `docs/conformance.md` reads that marker, so
a captured fixture that forgets it will under-report itself, and a hand-built one
that claims `captured` is a lie the table will repeat.

## The captures are not reproducible byte for byte

Trace IDs, span IDs and timestamps differ on every run, so re-capturing always
produces a diff. That is fine: captures are run deliberately, not in CI. What CI
does check is that `-update` is idempotent, which is a property of the
normalizer rather than of the capture.
