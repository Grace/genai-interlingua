# Comment on collector-contrib#29289

*[pkg/ottl] Determine approach to looping.*

**Posted 2026-09-08** to
<https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/29289>.

Everything below the horizontal rule is what went out, kept verbatim. If the
thread prompts a revision it belongs in a reply there, not in an edit here -- a
local copy quietly diverging from what was actually said is the failure this
repository is about.

The notes immediately below are why the comment is shaped as it is, and were not
posted.

The issue is open, labelled *discussion needed*, with no assignees, no comments and no linked
PRs. Its body proposes four options and closes on "awaiting community input" — so it is a
design question that has sat without concrete cases attached, which is exactly what this
supplies.

Two things shape the draft. The author annotates option 4, functional utilities, as
"potentially overcomplicated" — so leading with a request for it would read as a wish rather
than a report. And this case genuinely does not need the general solution: option 1, an
editor with no language change, covers it. Saying so is true and is the difference between a
report and a feature request.

---

I hit this from an unusual direction and thought a concrete case might be useful, since the
thread is mostly design options so far.

I maintain a normalizer that translates GenAI spans between instrumentation libraries'
attribute vocabularies, and it exports its mappings as a `transform` processor config so
people can run them without my binary. That gives me the same mapping expressed twice — once
in Go, once in OTTL — plus a harness that runs every captured span through both and diffs the
results attribute by attribute.

One mapping does not survive the trip, and it is this issue.

`gen_ai.response.finish_reasons` is an array in the semantic conventions. The Vercel AI SDK
emits it two ways: as an array on the span its provider adapter creates, and as a single
string on the outer span its caller creates. It also spells the values with hyphens where the
conventions use underscores, so `tool-calls` has to become `tool_calls`.

The scalar case is fine. The array case is not — what I can generate is:

```
set(attributes["interlingua.__scratch"], "tool_calls")
  where attributes["gen_ai.response.finish_reasons"] == "tool-calls"
```

which compares the whole attribute against a table key. `["tool-calls"]` is not
`"tool-calls"`, so it never fires, and the array passes through unmapped. The Go
implementation maps over the elements and produces `["tool_calls"]`.

What makes that worse than a plain gap is that it fails *silently and plausibly*. The config
parses, the attribute is present, the value looks like a finish reason, and it is the wrong
one. Nothing in the output indicates a transformation was attempted and missed. I only found
it because two implementations of the same mapping disagreed on a captured span — I had read
the generated config several times without noticing.

On the options in the description: **option 1 would be enough for me.** An editor that applies
a value mapping across the elements of a slice needs no language change and covers this class.
I mention it because my case is narrow, and I would rather report it than argue for the
largest available hammer — options 2 and 4 would obviously also solve it, and I have no view
on whether the use cases in #27820 justify them.

Two things I can offer if useful. The mapping is real, from a shipping SDK, so it works as a
test case rather than a hypothetical. And the diff harness generalises: running one mapping
through two implementations and comparing outputs is a way to find this class of gap
systematically rather than by noticing. Happy to point at either.
