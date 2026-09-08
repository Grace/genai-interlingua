# Draft PR: note that OTEP 4815 supersedes the schema publishing model

Not filed. **Target repository is
[`open-telemetry/opentelemetry-specification`](https://github.com/open-telemetry/opentelemetry-specification),
file `specification/schemas/README.md`** — not `opentelemetry.io`. The page at
<https://opentelemetry.io/docs/specs/otel/schemas/> is rendered from that source; a PR to the
website repository would be redirected.

## The gap, verified

`specification/schemas/README.md` is marked **`Status: Stable`**, references **only OTEP
0152**, and presents schema transformations as an active, integral part of the design. It
says nothing about
[OTEP 4815](https://github.com/open-telemetry/opentelemetry-specification/blob/main/oteps/4815-semantic-conventions-schema-v2.md),
which merged on 2026-05-17 and states:

> We will stop publishing current schema file format 1.1.0 which has Development status. This
> is a breaking change for components that do schema transformation (such as Collector
> schemaprocessor).

> Schema transformations (diffs) will not be published.

So a Stable specification document currently describes a mechanism the project has decided to
stop publishing. Anyone arriving there to decide how to implement schema transformation —
which is the audience the page is for — gets the pre-4815 picture with nothing indicating it
is out of date.

## Before filing

1. **Check for an existing PR or issue.** 4815 merging without a documentation follow-up
   suggests none, but a duplicate is a poor first contribution and the check is one search.
2. **Confirm it is still true.** Verified 2026-09-08; this is exactly the kind of thing that
   gets fixed the week after you notice it.
3. **Consider asking first.** A one-line question in `#otel-weaver` — "is a note on
   `specification/schemas/README.md` about 4815 wanted, or is that page being rewritten
   wholesale?" — may save the PR entirely. If v2 is landing soon, a patch to the old text is
   churn.

## Proposed change

A note immediately after the status line, deliberately additive — no rewriting of the
existing content, which stays accurate for anyone maintaining something already built against
1.1.0.

> **Note:** [OTEP 4815](../../oteps/4815-semantic-conventions-schema-v2.md) supersedes the
> publishing model described below. OpenTelemetry will stop publishing schema file format
> 1.1.0, and schema transformations (diffs) will no longer be published; consumers needing a
> diff between two versions are expected to generate one on demand with
> `weaver registry diff`. The formats documented here remain valid for existing files.

## Why it is worth doing

It is small, factual, and useful independently of anything else. It also happens to be a
one-paragraph instance of the argument this repository spends thousands of words on:
documentation that quietly disagrees with reality, where the reader has no way to tell, and
the disagreement is invisible in the artifact itself. Fixing one is worth more than describing
the class.
