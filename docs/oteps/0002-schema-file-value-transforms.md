# Value transforms in Telemetry Schema Files

**Status: draft, not filed, and weaker than [0001](0001-translation-provenance.md).**
See [README.md](README.md). This one got noticeably less convincing while its
evidence section was being generated, and the section below says how.

## Motivation

[Schema File format 1.1.0](https://opentelemetry.io/docs/specs/otel/schemas/file_format_v1.1.0/)
supports four transformations: `rename_attributes`, `rename_events`,
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
changes (a scalar finish reason against an array of them).

[OTEP 0152](https://github.com/open-telemetry/opentelemetry-specification/blob/main/oteps/0152-telemetry-schemas.md)
anticipated this. It held the transformation set to "the bare minimum that is
necessary to handle the most common changes ... with more types of
transformations potentially proposed in the future", and specified that changing
`file_format` must go through the OTEP process. This is a draft of that
proposal.

## Proposal

Schema File format **1.2.0**, adding two transformations to the sections that
already accept `rename_attributes`.

```yaml
file_format: 1.2.0
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
```

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

**The first:** the `value_transform` failures are exactly what 1.2.0 would fix,
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

The sample is also thin — one migrated dialect at the time of writing. The
counts in `export-gap.md` regenerate from the rule tables and are checked by CI,
so they will grow as more dialects migrate, and this section should be re-read
against them before anything is filed. If the `value_transform` count does not
grow with the sample, this proposal does not have a case.

## What this would cost

Not free, and worth stating in the proposal rather than discovering in review:

- An OTEP-gated `file_format` bump, which every schema file consumer must handle.
- Parser work in `go.opentelemetry.io/otel/schema` and its equivalents.
- A decision about whether `transform_values` applies before or after
  `rename_attributes` within a version. (It must be after, so the map is written
  against the new attribute name — but that is a decision, not an obvious fact.)
- Buy-in from the schema file maintainers, who have kept this format
  deliberately minimal for five years and have good reasons for it.

## Alternatives

**Do nothing; use OTTL.** Entirely reasonable, and the reason this draft is
second. OTTL expresses all of this today, and this repository
[exports it](../../internal/emit/ottl/). The difference is that an OTTL config is
an imperative program that runs in a collector, and a schema file is a portable,
reviewable, versioned *claim* about what two schema versions have to do with each
other. Those are different artifacts for different purposes, and the argument for
this proposal rests entirely on that distinction being worth something. If it is
not, this should not be filed.

**Put it in Weaver instead.** Weaver is building multi-registry composition, and
cross-registry value equivalence may fit better in a registry-level artifact than
in a version-migration one. This deserves a real look before anything is filed;
it may be that the correct proposal is a Weaver one and this draft is aimed at
the wrong repository.
