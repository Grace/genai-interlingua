# Drafts

Two proposals, drafted here and **not filed anywhere**. That distinction is the
first thing to say, because a directory called `oteps/` invites the assumption
that something is in flight and nothing is.

They are drafted in the open for two reasons. Writing a proposal is the fastest
way to find out whether you actually have one, and generating its evidence is the
fastest way to find out whether it survives. [0002](0002-schema-file-value-transforms.md)
first got weaker under its own evidence, then recovered when a second dialect was
measured, then got weaker again when three more were — the middle reading turned
out to be one unusually-shaped emitter at n=2. All three are in the draft, in the
order they happened. Far better to learn that here than in a pull request, and
the sequence is worth more to a reviewer than the number would have been. And a proposal that already exists as a working
artifact is a different conversation from one that does not: everything
[0001](0001-translation-provenance.md) describes is running, in
[`registry/`](../../registry/), which `weaver registry check` validates today.

## The two, and what changed

**[0001 — Translation provenance](0001-translation-provenance.md).** A convention
for recording that telemetry was translated: from what, to what, and what did not
survive. No file format changes, no SDK work, no new parser — a namespace and six
attributes. It generalizes well past GenAI, because every semantic convention
migration and every vendor ingest pipeline has the same unanswerable question
under it. Unaffected by anything below and now clearly the stronger of the two.

**[0002](0002-schema-file-value-transforms.md) is withdrawn as a proposal.** It
proposed Telemetry Schema File Format 1.2.0.
[OTEP 4815](https://github.com/open-telemetry/opentelemetry-specification/blob/main/oteps/4815-semantic-conventions-schema-v2.md)
merged in May 2026 and discontinues publishing format 1.1.0 entirely, so the
document was proposing to extend something that had already been retired. It has
been reworked into evidence rather than a proposal.

## Where the work actually goes

Not an OTEP. Two issues in the Weaver repository are open, unassigned, and
explicitly asking for exactly what this repository spent a week measuring:

- [weaver#613](https://github.com/open-telemetry/weaver/issues/613) —
  *Formalize allowed transformations for V2.0*. Which transformations should a
  schema format permit? [`docs/export-gap.md`](../export-gap.md) answers that
  from five real instrumentation libraries, categorized by the specific missing
  capability, generated from the rule tables and checked by CI.
- [weaver#614](https://github.com/open-telemetry/weaver/issues/614) —
  *Decide on a transformation language for migrations*, between custom
  definitions, OTTL, CEL and Lua. No decision has been made. This repository
  exports OTTL and has verified against a real Collector exactly what that does
  and does not carry.

Two issue comments, on questions that asked for them, is a far lower bar than an
OTEP and a much better fit. It is also the "file small useful patches first" step
below — except the patches turn out to be the main contribution.

## Why 0001 still waits

It is cheap, self-contained and already implemented, and none of that makes it
urgent. The two issue comments come first because they are answers to questions
somebody asked; 0001 is an answer to a question nobody has asked yet, which is a
different and slower conversation.

## What has to happen before either is filed

Not "write the document better". The sequence, roughly in order:

1. Ship the tool and get a couple of real users. Everything below is easier with
   them and slow without them.
2. Open an issue in
   [`semantic-conventions-genai`](https://github.com/open-telemetry/semantic-conventions-genai)
   about the missing schema URL — their README still reads `Schema URL: TODO` —
   describing the problem rather than this solution.
3. Attend the GenAI SIG call twice before proposing anything.
4. File small useful patches first. One is verified and available now:
   <https://opentelemetry.io/docs/specs/otel/schemas/> still presents file format
   1.1.0 as Stable with no notice that 4815 discontinued it. The drift detector in
   [`.github/workflows/upstream.yml`](../../.github/workflows/upstream.yml) also
   knows things upstream would want, such as which reference-implementation
   attributes no convention version models.
5. Then, if there is appetite, 0001.

Standing is earned in issue threads, not in specification documents. An OTEP filed
cold by someone with no history in the project is a document that gets politely
queued, and that is a reasonable thing for a project to do with it.

## Where these would go if filed

- **0001** — `open-telemetry/semantic-conventions` (or `semantic-conventions-genai`
  if it stays scoped to GenAI, which would be the wrong scope), after discussion
  in the Semantic Conventions SIG.
- **0002** — nowhere. It is a comment on weaver#613 and weaver#614, not a
  proposal. If a spec change eventually follows from those discussions, it will
  be somebody's to write with more standing than this.

Not W3C, which owns trace *context* — the propagation format on the wire — and
not attribute semantics. Not IETF. Not CNCF directly, which hosts OpenTelemetry
but does not review its specifications.
