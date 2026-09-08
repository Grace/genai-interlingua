# Draft comment for weaver#614

*Decide on a transformation language for migrations.* Not posted. Paste-ready.

---

I have been using OTTL for this in anger and can offer a measured answer rather than a
preference, including the one place it genuinely does not reach.

Context: I maintain a normalizer that translates GenAI spans from six instrumentation
libraries into one `gen_ai.*` schema. Five have their mappings declared as data, and the tool
exports those to a `transform` processor config so people can run the mapping without my
binary. So I have the same mapping expressed twice — once in Go, once in OTTL — and a check
that runs every captured span through both and diffs the results attribute by attribute.

**What OTTL carried, per emitter, out of what the Go implementation produces:**

| emitter | carried | of |
| --- | ---: | ---: |
| LiteLLM | 20 | 24 |
| OpenLLMetry | 20 | 27 |
| Vercel AI SDK | 20 | 25 |
| OpenInference | 15 | 31 |
| Braintrust | 6 | 13 |

Most of the shortfall is not OTTL's fault and no language would fix it — it is reassembling
indexed attributes into one document, lifting fields out of JSON blobs, deciding what an
attribute means from a sibling. That work belongs in code regardless of what gets chosen
here.

**One part is a real limitation, and it is already tracked:** OTTL has no iteration. A
mapping that translates values *and* can receive a list is only half expressible — the
generated statements compare the whole value against each table key and match nothing when it
is an array. Concretely, `gen_ai.response.finish_reasons` arrives as an array from a provider
adapter span and a scalar from the parent span; Go maps over the elements, OTTL cannot.

That is #29289 ("Determine approach to looping"), and I mention it because the case arrived
from the other direction: I did not read the grammar and predict it, the equivalence check
produced a wrong value on a real captured span and I traced it back. If for-each, user-defined
functions or filter/map/reduce land, this closes.

**Where that leaves the decision, from someone who only has evidence about one of the
options:**

- *OTTL* — exists, is governed here, ships in every Collector. Carried 81 of 120 mappings in
  my corpus, and the residue is dominated by one tracked gap plus work no declarative language
  should attempt. Also has precedent for exactly this shape of problem: Honeycomb's HTTP
  semantic convention migration guidance used OTTL, which I believe this WG discussed in
  December 2024.
- *CEL* — I have not run it, so I have no coverage figure to offer, and I would rather say
  that than imply an OTTL measurement settles a three-way choice. But two things above do
  bear on it. The taxonomy in #613 is language-agnostic: value rewriting, precedence between
  spellings, scalar-to-array and conditional drop is a requirements list any candidate has to
  meet. And the one case where OTTL failed is one CEL handles natively — `map` and `filter`
  are standard comprehension macros, so applying a lookup across the elements of a list is a
  one-liner there. That is a real discriminator on this axis rather than a preference.

  With a caveat that belongs next to it, from CEL's own spec: macros "can lead to exponential
  behavior when nested or chained", and implementations are advised to be able to limit or
  disable them. A schema-transformation context is plausibly somewhere you would want them
  off — at which point CEL is back where OTTL is. So "CEL has `map`" is not "CEL solves
  this", and I do not think this measurement decides the issue in either direction.
- *Custom definitions* — worth noting these are a different kind of artifact rather than a
  weaker version of the same one. A migration entry is a reviewable *claim* that two names
  mean the same fact; an OTTL config is an imperative program. Which one the format wants to
  be is arguably the real question under this issue.

Everything above regenerates from the rule tables and is checked by CI:
<https://github.com/Grace/genai-interlingua/blob/main/docs/export-gap.md>

The equivalence check is
[`equivalence.sh`](https://github.com/Grace/genai-interlingua/blob/main/internal/emit/ottl/testdata/equivalence.sh)
if the method is more interesting than the conclusion. It found four bugs in my own emitter
on its first run, all behind configs that parsed cleanly — which is roughly the argument for
choosing a language people can test against a running collector.
