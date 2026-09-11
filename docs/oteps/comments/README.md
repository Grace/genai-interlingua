# Comments

Four of these are posted. One is held.

| | target | status |
| --- | --- | --- |
| [collector-contrib-29289.md](collector-contrib-29289.md) | [#29289](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/29289) — how should OTTL handle looping? | **posted 2026-09-08** |
| [weaver-613.md](weaver-613.md) | [#613](https://github.com/open-telemetry/weaver/issues/613) — which transformations should a v2.0 schema format permit? | **posted 2026-09-08** |
| [weaver-614.md](weaver-614.md) | [#614](https://github.com/open-telemetry/weaver/issues/614) — which language should express them? | **posted 2026-09-08** |
| [collector-contrib-48607.md](collector-contrib-48607.md) | [#48607](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/48607) — should the semconv target-types map be generated at build time? | **posted 2026-09-09** |
| [spec-schemas-readme.md](spec-schemas-readme.md) | `opentelemetry-specification` — a Stable spec page still describes a publishing model OTEP 4815 discontinued | held; the "ask first" step is not done |

Each file keeps the posted text verbatim. A revision prompted by a thread belongs in a reply
there, not in an edit here: a local copy quietly diverging from what was actually said is the
failure this whole repository is about.

## How they were written

The reusable part, and it now describes something that happened rather than an intention.

**The number that argues against the author got the most weight.** 76 of 120 gaps are readings
of a span that no declarative format should attempt. It is the largest category and it argues
for scoping V2.0 *narrowly* — not the conclusion someone with a normalizer to promote would
reach, which is exactly why it leads.

**What was measured is separated from what follows from it.** #614 says there is no coverage
figure for CEL, because CEL was not run — and then says what *does* transfer: the taxonomy is
a requirements list for any candidate, and the one case OTTL failed is one CEL's `map` macro
handles natively. An earlier draft said "nothing measured bears on CEL", which was
over-correction rather than rigour — it discarded a real finding to avoid looking like
advocacy. The caveat that CEL's own spec advises implementations be able to disable macros is
what keeps the corrected version from being advocacy in the other direction.

**The ask is the smallest sufficient one.** #29289 asks for option 1, an editor with no
language change, because that covers the case — and the issue author had already flagged the
more general option 4 as "potentially overcomplicated". Asking for the smallest thing that
works is the difference between a report and a wish.

**Nothing pitches the repository.** It appears as a source for numbers and nowhere else.

## What a reply would mean

Written down now, so it is not decided retrospectively.

**#29289 — the highest-signal thread.** It had four options and no decision, and no comment
since February 2025. (An earlier version of this file said "no comments at all before this
one," which was wrong: there are five, from TylerHelmuth and jsuereth, running 2024-01 to
2025-02, and jsuereth has a working list-comprehensions branch linked from the last of them.
That is a materially different thread from the one this described, and it makes the comment
below a contribution to a stalled design rather than the opening of an unopened one.) If a maintainer picks an option, that answers whether the list-mapping
gap ever closes; if it is option 2 or 4, the `Map`-over-a-slice rule becomes exportable and
`interlingua.export.partial` loses an entry. If anyone asks about the diff harness rather than
the bug, that is the larger opening — a general method for finding this class of gap is worth
more to them than one instance of it.

**#613 and #614.** Engagement here is the standing that makes filing
[0001](../0001-translation-provenance.md) reasonable later. The sequencing in
[`../README.md`](../README.md) — ship, get users, comment on the problem, show up, small
patches, then a proposal — is now through step four. 0001 stays unfiled until someone upstream
has reason to know who wrote it.

**Silence is a real outcome and is not a verdict.** These are old, quiet issues. #29289 sat
without a single comment before this one. No reply means the thread is as dormant as it was,
which is information about the thread rather than about the work.

## One thing worth having straight

Someone may reasonably ask whether this duplicates Weaver. It does not, and the distinction is
clean.

Weaver describes itself as "a set of tools for working with schematized telemetry", and every
command operates on registries: `check`, `resolve`, `diff`, `generate`, `live-check`, `emit`.
The two that touch live telemetry do not transform it — `live-check` validates an OTLP stream
against a registry, `emit` generates examples from one.

So Weaver tells you whether telemetry *conforms*; this repository makes non-conforming
telemetry conform and records what that cost. They are complementary, and `live-check` is the
natural consumer of a normalizer's output — normalize first, then live-check passes. Even
after 4815 moved diffs to on-demand generation, `weaver registry diff` *produces* a diff that
something else applies: Weaver sits upstream of transformation and is never the transformer.
