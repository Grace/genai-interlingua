# What a schema transformation format has to express

**Status: withdrawn as a proposal. Retargeted as evidence for two open upstream
questions.** See [README.md](README.md).

## Withdrawn, and why

This document used to propose Telemetry Schema File Format **1.2.0**, adding
transformations to the sections that already accept `rename_attributes`.

That proposal is withdrawn.
[OTEP 4815](https://github.com/open-telemetry/opentelemetry-specification/blob/main/oteps/4815-semantic-conventions-schema-v2.md)
merged on 2026-05-17 and says:

> We will stop publishing current schema file format 1.1.0 which has Development
> status. This is a breaking change for components that do schema transformation
> (such as Collector schemaprocessor).

> Schema transformations (diffs) will not be published.

Proposing a 1.2.0 increment to a format that was discontinued four months earlier
is not a small error of framing. It would have announced, in one line, that its
author had not read the merged OTEP governing the thing being proposed.

## What it is now

The measurements were never really about a format version. They answer the
question "what would *any* transformation format need to express", and that
question is open upstream in two places, both unassigned and both explicitly
asking for input:

- [weaver#613](https://github.com/open-telemetry/weaver/issues/613), *Formalize
  allowed transformations for V2.0 based on what weaver diff currently supports*.
- [weaver#614](https://github.com/open-telemetry/weaver/issues/614), *Decide on a
  transformation language for migrations* — custom definitions, OTTL, CEL, Lua.
  "No decision has been made yet."

So this document is the working behind two issue comments rather than a proposal
of its own. That is a better fit and a much lower bar, and it is what the
sequencing in [README.md](README.md) was pointing at all along without knowing
the issues existed.

4815 also moves diffs from published artifacts to on-demand `weaver registry
diff`. That does not affect anything below: the shortfall measured here is in the
transformation *vocabulary*, not in when or where a diff is computed.

## The answer to #614, and the limit that decides it

OTTL. It is governed by OpenTelemetry, ships in every Collector distribution, and
[`internal/emit/ottl`](../../internal/emit/ottl/) renders this repository's rules
into it for five instrumentation libraries, verified against a real Collector by
[`equivalence.sh`](../../internal/emit/ottl/testdata/equivalence.sh) rather than
against a golden file.

With one limit that a decision on #614 should turn on, because it is the only
thing the export genuinely cannot do rather than merely does awkwardly:

**OTTL cannot iterate.** A rule that translates values *and* can receive a list —
`gen_ai.response.finish_reasons` arrives as an array from one span and a scalar
from another — is only half expressible. The generated statements compare the
whole value against each table key and match nothing when it is an array. Go maps
over the elements; OTTL has no construct that does.

That is
[collector-contrib#29289](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/29289),
"Determine approach to looping", which is open and whose candidate fixes are a
for-each syntax, user-defined functions, or filter/map/reduce. The contribution
here is not the opinion — it is that the limitation turned up as a concrete case
from a real emitter, found by running both implementations over the same spans
and diffing, rather than by reading the grammar and speculating.

Prior art worth naming: Honeycomb's HTTP semantic convention migration guidance
already used OTTL for this shape of problem, which the Tooling WG discussed in
December 2024 when weighing OTTL against CEL for downgrade paths.

## The answer to #613: which transformations are actually needed

[Schema File format 1.1.0](https://opentelemetry.io/docs/specs/otel/schemas/file_format_v1.1.0/)
supported four transformations: `rename_attributes`, `rename_events`,
`rename_metrics`, and a metrics-only `split`. Every one of them changes a
**name**.

Real schema migrations also change **values**. When the GenAI conventions settled
on `gen_ai.provider.name`, they also settled on a closed set of values for it,
and emitters in the wild write `bedrock` where the conventions write
`aws.bedrock`, `OpenAI` where the conventions write `openai`, `vertex_ai` where
the conventions write `gcp.vertex_ai`. A schema file can rename the attribute. It
cannot fix the value, so applying one alone produces the target's attribute name
carrying a value the target schema does not define — arguably worse than leaving
it alone, because it now looks conformant.

The same shortfall covers unit changes (`ai.response.msToFirstChunk` in
milliseconds against `gen_ai.response.time_to_first_chunk` in seconds) and type
changes (a scalar finish reason against the array the conventions define).

The type change is left out of the proposal below, deliberately and with less
confidence than the rest. It is real — it turns up twice in the measurement —
but wrapping a scalar in a list is the point at which "describe the migration"
starts becoming "compute the payload", and the boundary matters more than the
two mappings. It is raised in Open questions rather than proposed.

[OTEP 0152](https://github.com/open-telemetry/opentelemetry-specification/blob/main/oteps/0152-telemetry-schemas.md)
anticipated this. It held the transformation set to "the bare minimum that is
necessary to handle the most common changes ... with more types of
transformations potentially proposed in the future", and specified that changing
`file_format` must go through the OTEP process. This is a draft of that
proposal.

## What the missing transformations look like

Not proposed as a 1.2.0 increment any more — see above — but the shapes are what
#613 is asking to enumerate, so they are written out concretely rather than
described.

```yaml
# Illustrative. The version marker is whatever the v2 format settles on; what
# matters here is the shape of the four operations, not the header.
file_format: <v2>
versions:
  1.42.0:
    all:
      changes:
        # existing, unchanged
        - rename_attributes:
            attribute_map:
              gen_ai.system: gen_ai.provider.name

        # new: rewrite values of one attribute through a lookup
        - transform_values:
            attribute: gen_ai.provider.name
            value_map:
              bedrock: aws.bedrock
              vertex_ai: gcp.vertex_ai
            # a value the map does not mention passes through unchanged
            unmatched: keep     # keep | drop

        # new: scale a numeric attribute
        - change_unit:
            attribute: gen_ai.response.time_to_first_chunk
            from: ms
            to: s

        # new: several source spellings into one attribute, first present wins
        - coalesce_attributes:
            attribute: gen_ai.usage.input_tokens
            from:
              - gen_ai.usage.input_tokens
              - ai.usage.promptTokens
              - ai.usage.tokens
```

`coalesce_attributes` was added to this draft *after* the measurement below, and
at 22 of the 44 fixable gaps it is the largest single one by count. `rename_attributes` can already map
two old names onto one new name, but `attribute_map` is a map: it says nothing
about what to do when a payload carries both, so the result depends on iteration
order. That is not a hypothetical. The Vercel AI SDK emits `ai.usage.promptTokens`
on the span a user's code creates and `gen_ai.usage.input_tokens` on the span its
provider adapter creates, and a trace routinely contains both.

An ordered list makes the precedence explicit and makes the transformation
reversible in exactly the cases a plain rename already is — the first entry is
the one a reverse conversion writes back to.

`transform_values` is reversible when the map is injective, in the same sense and
with the same caveat as `rename_attributes`. `change_unit` is reversible up to
floating point.

`unmatched: drop` is the part that matters most and is easiest to overlook. The
conventions define closed value sets; an emitter can produce a value outside one;
and the correct handling is to drop the attribute rather than carry a
non-conformant value under a conformant name. A schema file has no conditional,
so this is the only way to express it.

## Evidence, including the part that undercuts this

[`docs/export-gap.md`](../export-gap.md) is generated from the rule tables of the
dialects this repository has migrated, and measures what each export format can
carry. The relevant number: of the mappings a schema file could not express,
**four** fail for `value_transform` and **eight** fail because they are not
transformations of an attribute at all — reassembling indexed keys, lifting
fields out of a JSON blob, disambiguating by a sibling attribute.

Two things follow, and the second is not in this proposal's favour.

**The first:** the `value_transform` failures are exactly what a format change would fix,
and they are the mappings that would otherwise produce falsely-conformant spans.
That is the real case for this.

**The second:** they are a minority of the shortfall. Most of what a schema file
cannot carry, it cannot carry because the work is a reading of a span, and *no
declarative schema format should try to express that*. Adding `transform_values`
and `change_unit` does not make schema files sufficient for normalizing GenAI
telemetry. It makes them sufficient for a larger fraction of it, while the
remainder stays in code regardless.

So this proposal should not be argued as "this would let schema files solve
normalization". It would not. It should be argued as: schema files already claim
to express schema migration, real schema migrations change values as well as
names, and the format is currently unable to describe migrations that have
already happened in the conventions it serves.

That is a narrower claim and it is the one the evidence supports.

### What the sample did to those numbers, twice

Everything above this heading was written against one migrated dialect, and
closed by saying: *if the `value_transform` count does not grow with the sample,
this proposal does not have a case.* The sample has grown twice since, and the
answer changed both times.

| | 1 dialect | 2 dialects | 5 dialects |
| --- | ---: | ---: | ---: |
| `value_transform` | 4 | 10 | 16 |
| `scalar_to_list` | — | 2 | 4 |
| `ambiguous_precedence` | — | 12 | 22 |
| `closed_value_set` | — | — | 2 |
| **fixable by a format change** | **4** | **24** | **44** |
| `not_a_name` | 8 | 16 | 76 |
| fixable share of the shortfall | 33% | 60% | 37% |

At one dialect the fixable gaps were a minority. At two they were a clear
majority, and this section previously said so, and said that the fixable share
*grows with the sample*. At five they are a minority again.

**That second reading was wrong, and it was wrong in the most ordinary way: n=2.**
The dialect that produced it, the Vercel AI SDK, is unusually multi-key —
it emits the same fact under an `ai.*` name on the span a user's code creates and
a `gen_ai.*` name on the span its provider adapter creates, so almost every rule
it has carries two spellings. Twelve of the twenty-four fixable gaps at that
point were `ambiguous_precedence` from that one emitter. Sampling a second
emitter with an unusual shape moved the ratio; sampling three more moved it back.

So the first conclusion was right and the correction to it was not. The fixable
gaps are a minority of the shortfall, they sit somewhere around a third of it,
and the honest summary is the one this section started with rather than the one
it reached in the middle.

Both readings are left standing, in order. A proposal whose evidence section
shows its own conclusion being overturned and then restored is worth more than
one that shows a single number, because the thing a reviewer most needs to know
about a measurement like this is how much it moves when you look again.

**What survives.** 44 real mappings across five real instrumentation libraries
fail only because the format has no transformation for them, and the four
categories they fall into are specific rather than vague. That is the case, and
it does not depend on the share. A format that cannot say `bedrock` and
`aws.bedrock` are the same provider cannot describe a migration the conventions
have already made, whether that is a third of the shortfall or all of it.

**What does not.** Any claim that this proposal makes schema files sufficient for
normalizing GenAI telemetry. It does not, by a wider margin than was apparent at
two dialects. `not_a_name` is 76 of 120 — reassembly, blob lifting,
disambiguation by sibling — and none of it should ever be expressible in a
declarative schema format. The largest single contributor is OpenInference,
which packs ten request parameters into one JSON string, and no transformation
type proposed here or plausibly anywhere would reach them.

The sample is now five of six. The sixth, `raw`, is a fallback for spans nobody
designed and is deliberately not migratable: its mapping resolves keys against
the entire semantic-convention registry at runtime, so stating it as data would
either duplicate the registry or admit it carries nothing. It will not move these
numbers.

## What any of this costs, whoever specifies it

These do not become free by moving to a v2 format or to a general-purpose
language, and they are the parts #613 and #614 will actually have to argue about:

- Parser work in every consumer, whatever the format is.
- A decision about whether `transform_values` applies before or after
  `rename_attributes` within a version. (It must be after, so the map is written
  against the new attribute name — but that is a decision, not an obvious fact.)
- The same decision for `coalesce_attributes`, which must run *before*
  `transform_values` and interacts with `rename_attributes` in a way a spec has
  to pin down rather than leave to implementations.
- Reversibility. `transform_values` is reversible when its map is injective;
  `coalesce_attributes` is reversible only to its first entry. The Tooling WG has
  wanted bidirectional migration since at least January 2025 -- "stable
  conventions migration must have upgrade and downgrade path" -- so any answer to
  #613 has to say which of these survive going backwards, and the honest answer
  for a lookup table is "only sometimes".

## Alternatives

**Custom definitions, as the format has today.** The four operations above are
small, declarative and reversible in stated cases, which is what a *claim* about
two schema versions should be. An OTTL config is an imperative program that runs
in a collector; a schema entry is a reviewable assertion that two names mean the
same fact. Those are different artifacts, and #614 is partly a question about
which of the two the format is meant to be.

**OTTL.** What this repository actually exports, and what it recommends -- with
the iteration limit above stated rather than discovered. Its decisive advantage
is that it already exists, is governed, and ships everywhere; its decisive
weakness is #29289.

**CEL.** Prototyped in the Tooling WG in March 2025 for exactly this reason.
Nothing measured here bears on CEL either way, which is worth saying plainly
rather than implying the evidence favours the option the author happens to have
built against.

**Scalar-to-list.** Two mappings need an attribute the conventions type as an
array where the emitter writes a scalar. A `wrap_in_list` transformation would be
trivial to specify and is the first thing on the slope: once a schema file can
change a type it will be asked to parse a JSON string, then to reassemble indexed
attributes, and the answer to those has to be no. Two mappings is not enough to
justify standing at the top of that slope, so this draft does not propose it and
says why.

**A registry-level artifact instead of a version-migration one.** Cross-registry
value equivalence may belong with Weaver's multi-registry composition rather than
in anything migration-shaped. Since #613 and #614 both live in the Weaver
repository, this may already be the direction and is worth asking about before
assuming otherwise.
