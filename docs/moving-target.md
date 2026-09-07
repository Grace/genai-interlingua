# The moving target

Every normalizer has to answer one question before it can do anything useful:
normalize *to what*. For most semantic conventions that question is boring. You
pick a version, you write its number in a schema URL, and a consumer reading your
span three years later can look up exactly what you meant by every key on it.

For `gen_ai.*`, right now, that question has no good answer. This document is
about why, what this repository does about it, and what it costs you.

## What happened

Facts, checked against the GitHub API on 2026-09-07 rather than recalled:

| Date | Event |
| --- | --- |
| 2026-04-28 | `semantic-conventions` **v1.41.0**. Last release with live `gen_ai.*` definitions. |
| 2026-05-05 | `semantic-conventions-genai` created. |
| 2026-06-12 | `semantic-conventions` **v1.42.0**. All `gen_ai.*` deprecated and moved out. |
| 2026-07-03 | `semantic-conventions` v1.43.0. |
| 2026-08-04 | `semantic-conventions` v1.44.0. |
| 2026-09-03 | Most recent commit to `semantic-conventions-genai` `main`. |

The v1.42.0 release notes put it plainly:

> **Generative AI Migration:** All `gen_ai.*` attributes, metrics, events, and
> spans have been deprecated and moved to the dedicated
> [OpenTelemetry GenAI Semantic Conventions Repository](https://github.com/open-telemetry/semantic-conventions-genai).
> ([#3696](https://github.com/open-telemetry/semantic-conventions/issues/3696))

That covers everything under `model/gen-ai/`, `model/openai/` and `model/mcp/`.

Here is the part that matters. As of this writing, `semantic-conventions-genai`
has:

- **0 tags**
- **0 releases**
- **21 commits in the last 30 days**

The definitions are moving faster than they were before the split, and there is
now no version number anywhere on them. The main repository has shipped three
tagged releases since the move; the repository that actually owns these
attributes has shipped none.

## Why this is a normalizer's problem and not a footnote

A span normalized to a tagged schema says so, in `schema_url`. That string is the
contract: it tells a consumer which document defines `gen_ai.usage.input_tokens`,
and it lets a backend apply schema transformations to carry old data forward.

There is no such string for the current GenAI conventions, because there is no
released version to name. You cannot pin `main`. You can pin a commit, but no
consumer will know what to do with one, and nothing in the ecosystem will resolve
it.

So a GenAI normalizer is forced into a choice that a normalizer for, say, HTTP
conventions never has to make:

1. **Target the last tagged thing**, `v1.41.0`, and accept that it is frozen,
   deprecated at its source, and missing everything added in the four months
   since.
2. **Target `main`**, and accept that "conformant" is a claim with no version
   attached, which may quietly stop being true on any given Tuesday.

Both are defensible. Neither is correct. What is *not* defensible is making the
choice silently, which is what a normalizer with a hardcoded attribute table
does.

## What this repository does

The target is an explicit enum, `semconv.Target`, with two members and no default
that pretends to be neutral:

```
-target v1.41.0     # frozen, tagged, deprecated at source  (default)
-target genai-main  # current, untagged, moving
```

`v1.41.0` is the default. Not because it is better, but because it is the only
one of the two that a reader six months from now can reconstruct exactly. A
default of `genai-main` would mean the same command produces different spans in
March and in September with no record of why.

And because there is no schema URL to carry the answer, **the target is written
onto the span itself**:

```
interlingua.target = v1.41.0
```

That attribute exists purely because the thing that should carry this information
does not exist yet. When `semantic-conventions-genai` tags a release, this becomes
redundant and a real `schema_url` should replace it.

## What pinning costs, concretely

This is the same OpenLLMetry span normalized to both targets. The entire
difference:

```
  gen_ai.prompt.version                      only at genai-main
  gen_ai.usage.cache_creation.input_tokens   renamed to
  gen_ai.usage.cache_write.input_tokens      at genai-main
  interlingua.lossy                          one entry shorter at genai-main
  interlingua.target                         v1.41.0 / genai-main
```

Three things worth reading carefully there.

**A field vanishes.** OpenLLMetry emits `traceloop.prompt.version`, this
repository parses it, and `v1.41.0` has no attribute to put it in. So it is
dropped. It is not renamed to something close, and it is not passed through under
its original name pretending to be normalized output. It is dropped, and the drop
is recorded.

**A field is renamed.** `gen_ai.usage.cache_creation.input_tokens` at v1.41.0 is
`gen_ai.usage.cache_write.input_tokens` at main. Same definition, same value,
different key. This is the single rename between the two targets, and it is the
clearest possible demonstration of why "normalize to `gen_ai.*`" is not a
specification: two things that both call themselves `gen_ai.*` disagree about
what to call a number.

**The loss list shrinks.** `interlingua.lossy` is one entry shorter under
`genai-main`, and the entry that left is `gen_ai.prompt.version`. The span
reports its own fidelity, so the cost of pinning is legible on every span rather
than in this document.

The full picture across all six dialects is in
[`conformance.md`](conformance.md), under "What each target could not express."
Today it is two fields. That number is small because the fixtures are small; it
is not zero, and it grows every time the new repository adds an attribute.

## What happens when they finally tag something

Concretely, when `semantic-conventions-genai` publishes `v1.0.0` or whatever it
calls its first release:

1. It becomes a third `Target`. `genai-main` stays, because people track main.
2. It becomes the new `DefaultTarget`, because a tagged release beats a frozen
   deprecated one on every axis that made `v1.41.0` the default.
3. Spans normalized to it get a real `schema_url`, and `interlingua.target`
   becomes belt-and-braces rather than the only record.
4. `v1.41.0` stays a target for as long as anyone has a backend pinned to it.
   Removing it would be the same mistake as never having offered it.

The `Target` enum, the per-target key maps in `internal/semconv`, and the
generated conformance table are all shaped so that adding that third target is a
data change plus a regenerated table, not a rewrite. That is the actual bet this
repository is making: not that `v1.41.0` is the right answer, but that *which
version you normalize to* is a parameter, and code that treats it as a constant
will be wrong within the year.

## A note on what this is not

This is not an argument that the split was a mistake. Moving GenAI conventions
into their own repository, on their own release cadence, is a reasonable response
to a domain changing much faster than HTTP or database conventions do.

It is an argument that the window between "moved out" and "tagged something" is
real, that it is currently four months wide and open, and that everyone
normalizing GenAI telemetry is making a choice inside it whether or not they have
noticed. This repository's only claim is that the choice should be visible, on
the span, and reversible with a flag.
