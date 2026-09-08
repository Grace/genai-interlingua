# Translation provenance

**Status: draft, not filed.** See [README.md](README.md) for why, and for what
would have to happen first.

## Motivation

Telemetry gets rewritten on its way to storage, constantly, and there is no
standard way for it to say so.

The rewriting is not exotic. A collector normalizes a vendor's attribute names
to the conventions. A backend maps inbound `gen_ai.*` attributes into its own
vocabulary at ingest — Arize AX does this today, documented, as a product
feature. A semantic convention migration renames `http.status_code` and a
translation layer bridges the two for a year. An OpenTelemetry
[Telemetry Schema File](https://opentelemetry.io/docs/specs/otel/schemas/)
converts a payload from one version to another inside the collector.

All four are load-bearing and all four are invisible in the result. A span that
arrives at a backend carrying `gen_ai.usage.input_tokens` might have been emitted
that way, or renamed from `gen_ai.usage.prompt_tokens`, or translated from
`llm.token_count.prompt` by something that also dropped three attributes it had
nowhere to put. The span does not say. Nothing in OpenTelemetry gives it a way
to say.

That is a gap with teeth, because the failure it produces is silent and
confidence-shaped:

> A translation layer that drops data silently is worse than no translation
> layer, because you will trust the result.

`schema_url` is the closest existing mechanism and it answers a different
question. It says which version of a schema the telemetry claims to conform to.
It does not say that a conversion happened, which direction, what the source
vocabulary was when the source was not a schema version at all, or what the
conversion could not carry. And it is absent precisely when it is most needed: the GenAI conventions still
have no released version to point at, which is the situation that motivated the
implementation behind this draft. OTEP 4815 has since defined the scheme those
URLs will use, and onboarding GenAI to it is queued rather than done — which
fixes the missing URL eventually and does not touch the question this draft is
about, since a schema URL says what telemetry claims to be and not what was done
to it.

## Proposal

A small attribute namespace, recorded on telemetry that was translated. Six
attributes, no new file format, no SDK changes, no parser work. The running
implementation is
[`registry/model/translation.yaml`](../../registry/model/translation.yaml),
which `weaver registry check` validates today; it uses this repository's own
`interlingua.*` prefix, which is exactly what a shared convention should not do.
The neutral naming below is the proposal.

| attribute | type | meaning |
| --- | --- | --- |
| `telemetry.translation.source` | string | The vocabulary the telemetry arrived in. A schema URL when there is one; otherwise an identifier for the emitter or convention, because the common case is that the source declared nothing. |
| `telemetry.translation.source.confidence` | int | How decisively the source was identified, when it was inferred rather than declared. Absent if the source was declared. |
| `telemetry.translation.target` | string | The schema URL the telemetry was translated into, or an identifier when no URL exists. |
| `telemetry.translation.lossy` | string[] | Attribute keys this telemetry is **not** a faithful carrier of. |
| `telemetry.translation.lossy.count` | int | The length of that list, written **even when zero**. |
| `telemetry.translation.by` | string | What performed the translation, when several implementations of one mapping exist and they do not carry the same amount. |

### Why the count is separate from the list

`telemetry.translation.lossy.count` looks redundant and is not, for two reasons.

"This translation lost nothing" and "nobody checked" are different claims, and an
absent array cannot distinguish them. Writing zero is an assertion.

And many backends will not aggregate over an array attribute but will happily
aggregate over an integer. "Which of my services is losing the most in
translation" should be a query, not an afternoon.

### Why confidence is in the list

Because in practice the source is almost never declared. Instrumentation
libraries do not stamp their own name on spans, so a translator that handles more
than one has to *infer* which it is looking at, from the shape of the attributes.
That inference is usually easy and occasionally a coin flip.

An inference recorded without its confidence is indistinguishable from a fact.
Recording the margin makes a misdetection something you can query for, rather than
something you discover months later from a dashboard that has been quietly wrong.

### What this deliberately does not do

It does not describe the mapping. Expressing *how* to translate is a separate and
much larger question, and an open one upstream —
[weaver#613](https://github.com/open-telemetry/weaver/issues/613) and
[weaver#614](https://github.com/open-telemetry/weaver/issues/614) are still
deciding which transformations a format should permit and which language should
express them. [0002](0002-schema-file-value-transforms.md) is this repository's
evidence for those, and is not a proposal.

The two are independent, and this one does not wait on them. It is worth doing
whatever those issues conclude, because the translations are happening now, in Go
and OTTL and vendor ingest pipelines, and none of them can currently leave a
trace.

## Prior art, and why it does not cover this

**`schema_url`** answers "which version does this claim to be", not "what
happened to it". Both are worth knowing and they are different.

**Schema files** describe transformations between versions of one schema, in a
file that lives away from the data. Nothing connects a payload to the fact that a
schema file was applied to it, or to what the application cost.

**Weaver's `schema-changes`** reports how a registry changed between versions.
It is a diff of the registry, not a record of anything done to telemetry.

**`otel.*` internal attributes** cover collector-internal bookkeeping such as
dropped-count semantics. Adjacent, and about the pipeline's own health rather
than about semantic fidelity.

**Weaver's lineage and provenance work** is the closest thing by name and is a
different layer, which is worth stating before someone mistakes one for the
other. Weaver tracks, inside a *resolved schema*, which dependency registry each
attribute came from -- a dictionary of `schema_url`s with the definitions
indexing into it. That is provenance of the *schema*. This proposal is provenance
of the *telemetry*: not where a definition came from, but that a span was
rewritten, from what, into what, and what the rewrite could not carry. A payload
can be translated by something that never consults a resolved schema at all, and
frequently is.

## Open questions

**Namespace.** `telemetry.translation.*` is a guess and probably not the right
one. `otel.translation.*` would put it with existing internal attributes;
`schema.translation.*` would tie it to the schema machinery it is closest to.
This is a question for the SIG, not a decision to defend here.

**Cardinality.** `lossy` is an array whose contents vary per span. On telemetry
with wide variation in shape that is real storage. The mitigation is that most
spans from one emitter lose the same things, so it compresses well — but the
honest answer is that this needs measuring on a large corpus, and it has not
been.

**Chained translation.** A span translated twice — vendor to conventions in the
SDK, conventions to a backend's vocabulary at ingest — has two stories and these
attributes hold one. A list of records would be more correct and much heavier.
This draft proposes recording the last translation and says plainly that the
general case is unsolved.

**Whether `by` earns its place.** It exists because one implementation of this
mapping carries less than another — an exported OTTL config cannot do everything
the processor does — and downstream consumers need to tell them apart. That may
be an artifact of one repository's situation rather than a general need.

## Implementation

Running in [genai-interlingua](https://github.com/Grace/genai-interlingua) under
the `interlingua.*` prefix: on every normalized span, in the CLI, in the
Collector processor, and in the OTTL configs it exports. The
[registry](../../registry/) resolves under Weaver.

What that has been worth in practice, concretely: five of seven captures of real
instrumentation libraries turned up a mapping bug that a completely green test
suite could not see, and the loss list is how they were found. The attribute
earned its place before it was proposed.
