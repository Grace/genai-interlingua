# Draft comments

Two comments, drafted here and **not posted**, on issues in
[`open-telemetry/weaver`](https://github.com/open-telemetry/weaver) that are open,
unassigned, and explicitly asking for the kind of input this repository happens to have
generated.

| draft | issue | asks |
| --- | --- | --- |
| [weaver-613.md](weaver-613.md) | [#613](https://github.com/open-telemetry/weaver/issues/613) | Which transformations should a v2.0 schema format permit? |
| [weaver-614.md](weaver-614.md) | [#614](https://github.com/open-telemetry/weaver/issues/614) | Which language should express them — custom definitions, OTTL, CEL, Lua? |

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

**The limits of the evidence are stated rather than left to be found.** 614 says plainly that
nothing measured here bears on CEL, because the alternative — presenting an OTTL measurement
as though it settled a three-way choice — is the thing that would make a reviewer stop
reading.

**Neither pitches the repository.** It appears as a source for the numbers and nowhere else.

## If the reply is "we have this covered"

That is a good outcome and costs one comment. The measurement stays useful here regardless,
and knowing the answer is worth more than having contributed it.
