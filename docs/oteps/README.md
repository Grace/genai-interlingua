# Drafts

Two proposals, drafted here and **not filed anywhere**. That distinction is the
first thing to say, because a directory called `oteps/` invites the assumption
that something is in flight and nothing is.

They are drafted in the open for two reasons. Writing a proposal is the fastest
way to find out whether you actually have one, and generating its evidence is the
fastest way to find out whether it survives. [0002](0002-schema-file-value-transforms.md)
first got weaker under its own evidence, then recovered when a second dialect was
measured and two entirely new gap categories appeared. Both readings are in the
draft, in the order they happened. Better to learn that here than in a pull
request. And a proposal that already exists as a working
artifact is a different conversation from one that does not: everything
[0001](0001-translation-provenance.md) describes is running, in
[`registry/`](../../registry/), which `weaver registry check` validates today.

## The two

**[0001 — Translation provenance](0001-translation-provenance.md).** A convention
for recording that telemetry was translated: from what, to what, and what did not
survive. No file format changes, no SDK work, no new parser — a namespace and six
attributes. It generalizes well past GenAI, because every semantic convention
migration and every vendor ingest pipeline has the same unanswerable question
under it.

**[0002 — Value transforms in schema files](0002-schema-file-value-transforms.md).**
Telemetry Schema File format 1.2.0, adding transformations that change a value
rather than a name. Expensive: an OTEP-gated `file_format` bump, parser changes
in `go.opentelemetry.io/otel/schema`, and buy-in from the people who own the
format. Its evidence is [`docs/export-gap.md`](../export-gap.md), which is
generated from the rule tables rather than asserted.

## Why they are in this order

0001 is cheap, self-contained, and already implemented. 0002 changes a file
format that other people's parsers read.

If only one of these ever lands it should be 0001, and filing them together would
make that outcome less likely rather than more: a reviewer reading two proposals
from an unknown contributor reads them as one ambition.

## What has to happen before either is filed

Not "write the document better". The sequence, roughly in order:

1. Ship the tool and get a couple of real users. Everything below is easier with
   them and slow without them.
2. Open an issue in
   [`semantic-conventions-genai`](https://github.com/open-telemetry/semantic-conventions-genai)
   about the missing schema URL — their README still reads `Schema URL: TODO` —
   describing the problem rather than this solution.
3. Attend the GenAI SIG call twice before proposing anything.
4. File small useful patches first. The drift detector in
   [`.github/workflows/upstream.yml`](../../.github/workflows/upstream.yml)
   already knows things upstream would want, such as which reference-implementation
   attributes no convention version models.
5. Then, if there is appetite, 0001.

Standing is earned in issue threads, not in specification documents. An OTEP filed
cold by someone with no history in the project is a document that gets politely
queued, and that is a reasonable thing for a project to do with it.

## Where these would go if filed

- **0001** — `open-telemetry/semantic-conventions` (or `semantic-conventions-genai`
  if it stays scoped to GenAI, which would be the wrong scope), after discussion
  in the Semantic Conventions SIG.
- **0002** — `open-telemetry/opentelemetry-specification`, in its `oteps/`
  directory. Note the standalone `open-telemetry/oteps` repository was archived
  on 2025-11-17; proposals moved.

Not W3C, which owns trace *context* — the propagation format on the wire — and
not attribute semantics. Not IETF. Not CNCF directly, which hosts OpenTelemetry
but does not review its specifications.
