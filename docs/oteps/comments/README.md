# Draft comments

Two comments, drafted here and **not posted**, on issues in
[`open-telemetry/weaver`](https://github.com/open-telemetry/weaver) that are open,
unassigned, and explicitly asking for the kind of input this repository happens to have
generated.

| draft | target | asks |
| --- | --- | --- |
| [collector-contrib-29289.md](collector-contrib-29289.md) | [#29289](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/29289) | How should OTTL handle looping? Open with no comments, no assignees, awaiting input. |
| [weaver-613.md](weaver-613.md) | [#613](https://github.com/open-telemetry/weaver/issues/613) | Which transformations should a v2.0 schema format permit? |
| [weaver-614.md](weaver-614.md) | [#614](https://github.com/open-telemetry/weaver/issues/614) | Which language should express them — custom definitions, OTTL, CEL, Lua? |
| [spec-schemas-readme.md](spec-schemas-readme.md) | `opentelemetry-specification` | A Stable spec page still describes a publishing model OTEP 4815 discontinued. |

**Post #29289 first if you post only one.** It is the only one of the four that supplies
something nobody else has: a concrete failure from a shipping SDK, on a design question that
has been open for a long time collecting options rather than cases. The others are analysis;
that one is evidence.

They are drafted rather than posted for one reason: **watch a Tooling WG recording first.**
The group meets Wednesdays 07:00 PT, the sessions are recorded, and both of these may already
have been discussed in a meeting whose notes exist. Posting a measurement into a question
somebody already answered is worse than posting nothing.

## What they are careful about

Both are extraction, not new argument — every number comes from
[`../../export-gap.md`](../export-gap.md), which regenerates from the rule tables and is
checked by CI. Neither asks for anything.

Three deliberate choices worth preserving if these get edited:

**The number that argues against the author is given the most weight.** 76 of 120 gaps are
readings of a span that no declarative format should attempt. That is the largest category
and it is an argument for scoping V2.0 *narrowly* — which is not the conclusion someone with
a normalizer to promote would reach.

**What was measured is separated from what follows from it.** 614 says there is no coverage
figure for CEL, because CEL was not run — and then says what *does* transfer: the taxonomy is
a requirements list for any candidate, and the one case OTTL failed is one CEL's `map` macro
handles natively, which is a genuine discriminator. Both halves matter. An earlier draft said
"nothing measured bears on CEL", which was over-correction rather than rigour: it threw away a
real finding to avoid the appearance of advocacy. The caveat that CEL's own spec advises
implementations be able to disable macros is what keeps the corrected version from being
advocacy in the other direction.

**Neither pitches the repository.** It appears as a source for the numbers and nowhere else.

## If the reply is "we have this covered"

That is a good outcome and costs one comment. The measurement stays useful here regardless,
and knowing the answer is worth more than having contributed it.
