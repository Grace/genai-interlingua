# What is being taken upstream

`processor/genainormalizer` in `opentelemetry-collector-contrib` covers this
problem and ships in the `otelcol-contrib` binary. This repository covers it
differently in four places. A second component would be worse for everyone than
one good one, so the differences go there rather than staying here.

This file is the ledger. It records what has been offered, what was accepted,
and what was declined — including the declines, because a proposal that lost an
argument is more useful to a reader than a proposal that quietly vanished.

**Status as of 2026-09-09:** one direction proposed, awaiting a code owner. Everything else
below is still intent.

## Ledger

| | What | Where | Status |
| --- | --- | --- | --- |
| 1 | Generate the semconv target-types map at build time instead of reflecting at init | [contrib#48607](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/48607) | **proposed 2026-09-09**, awaiting reply |
| 2 | LiteLLM as a built-in source | new PR | not yet offered |
| 3 | Braintrust as a built-in source | new PR | not yet offered |
| 4 | Vercel AI SDK as a built-in source | new PR | not yet offered |
| 5 | Record what a normalization dropped | new issue | not yet filed |
| 6 | Score dialects per span instead of declaring them in config | discussion | not yet raised |

## The reasoning, per item

### 1 — build-time target-types map

`internal/otelsemconv/otelsemconv.go:35` builds `targetTypes` as a
`map[string]reflect.Type{}` at package init, by reflecting over each typed
semconv constructor. kylehounslow opened #48607 asking for it to be generated
from the pinned registry instead. It is open, unassigned, and labelled
`never stale`.

[`internal/semconv/gen/main.go`](../internal/semconv/gen/main.go) here does that:
it reads the vendored upstream registries and writes `targets_gen.go`, keeping
hand-authored only the editorial layer — which concepts are modelled at all, and
the handful of keys that are not mechanically derivable.

The argument for it is the one in that generator's own header: a table somebody
typed is a claim about upstream that upstream never made. The operational version
is that a generated file changes visibly in a diff when upstream retypes an
attribute, and reflection does not.

**Proposed 2026-09-09.** The thread had stalled on one objection: build-time generation was
taken to mean losing user-selectable target versions, leaving users a `schemaprocessor` hop.
That only follows if the generator emits one table. Emitting one per target keeps selection a
runtime value, which is what this repository already does — `go generate` here prints
`72 fields modelled, 2 targets`. The honest cost, which the proposal states, is that the set is
bounded by what a maintainer has added rather than open to any version a user names.

### 2–4 — three more built-in sources

The component has two built-in dialects and asks everyone else to hand-write YAML
mapping tables. This repository has four more, and
[`docs/findings.md`](findings.md) is the reason they are worth having rather than
re-deriving: when the fixtures behind them were replaced, one at a time, with
spans captured from the libraries actually running, **five of seven captures
found something the documentation-derived version had got wrong**.

Order is by coverage: LiteLLM (24 mappings, and a proxy, so one source covers a
lot of downstream traffic), then Braintrust (13), then Vercel.

[#48385](https://github.com/open-telemetry/opentelemetry-collector-contrib/pull/48385)
added OpenLLMetry and is the shape to follow.

### 5 — record what was dropped

The component drops silently, by design, and its own source says so:

```go
// src cannot be safely coerced; callers must drop the attribute.   coerce.go:16
// Map / Slice / Bytes: do not stringify. Caller drops the rename.  coerce.go:91
```

With `remove_originals: true` the source attribute is deleted as well, so the
evidence goes with it.

[contrib#50133](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/50133)
is one instance of what that produces: OpenInference's indexed content array
yields `"parts": []`, and with originals removed the message text is gone —
*"a well-formed attribute that looks populated to downstream consumers but
carries no content."*

The point of filing this is not to fix that bug. It is claimed, and being fixed.
The point is that it is the second instance of a class
([#48421](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/48421)
was the first), and a processor that recorded what it could not carry would turn
the third through twentieth into something a user finds with a query instead of
something a maintainer triages.

[`oteps/0001-translation-provenance.md`](oteps/0001-translation-provenance.md) is
the general version of the idea, under a neutral name. The issue should be the
short version — the general one is a bigger conversation and does not need to be
won first.

### 6 — detection

Sources are declared in config, and every declared source is applied to every
span in list order. When two match one span the winner is whichever appears later
in the YAML, which is not a decision anybody made.

This is the largest change and the least welcome as an unsolicited patch, so it
is a discussion with a reproducing span attached, after the others have landed —
not a PR.

## How this repository talks to OpenTelemetry

OpenTelemetry is dealing with maintainer burden from AI-assisted contributions
([community#3649](https://github.com/open-telemetry/community/issues/3649)), and has two
documents about it. Both bear on how anything in this ledger gets offered.

[`policies/genai.md`](https://github.com/open-telemetry/community/blob/main/policies/genai.md)
lets maintainers close or hide contributions made in whole or in part with generative AI, at
their discretion, and asks for disclosure when a tool wrote the bulk of one. For code, the
requested form is an `Assisted-by:` commit trailer naming the model.

Contrib's [`AGENTS.md`](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/AGENTS.md)
is blunter, and is addressed to the tools rather than to people:

> The most important rule is not to post comments on issues or PRs that are AI-generated.
> Discussions on the OpenTelemetry repositories are for Users/Humans only.

So: every comment, issue and PR description that goes out under this repository's name is
written by a human, start to finish. Research, reading upstream source and drafting code are
all explicitly fine — the policy FAQ calls using an LLM to understand a codebase "a good idea"
— and the line falls at anything that gets published as speech.

One practical trap worth writing down. Contrib gates on **EasyCLA**, which validates every
commit author *and co-author*. A `Co-Authored-By:` trailer naming a non-signatory fails the
check outright, which is a second reason `Assisted-by:` is the right form.

## What stays here

The CLI, the OTLP/JSON codec and its fuzzing, the `-emit ottl` and
`-emit schema-file` exporters, [`registry/`](../registry/), the capture harness,
and the generated evidence in [`conformance.md`](conformance.md) and
[`export-gap.md`](export-gap.md). None of it is Collector-component-shaped, and
the export-gap measurements exist to argue about OTTL and the schema file format
in their own repositories rather than about this one.
