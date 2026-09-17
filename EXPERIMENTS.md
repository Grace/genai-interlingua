# Experiments

One entry per experiment, appended. Entries are never edited: a result that
later turns out to be wrong gets a new entry that links back to it.

## 2026-09-17 — Does an undeclared cache convention change the price?

**Claim.** A normalized span carrying an input count and a cached-token count,
with nothing saying whether one contains the other, prices differently under each
convention, so the convention has to be recorded at translation time rather than
left to the query.

**Decision it informs.** Whether to add a per-dialect declaration and emit it, or
document a query-side convention and move on.

**Test.** Rung 3, one call priced three ways. The fixture usage is the one
`testdata/capture/mockopenai.py` returns: 412 input, 256 cache read, 27 output, 8
reasoning. Priced with genai-observability's `internal/pricing.Estimate`
(`~/code/genai-observability`, commit 5827716), which takes the convention as an
input, using illustrative gpt-4o-mini rates of 0.15 input, 0.075 cache read and
0.60 output per million. The rates are list prices used for the ratio, not a
verified price table. The throwaway test was deleted after the run.

**Result.** Verified by execution, 2026-09-17.

| Convention | Cost | Components |
|---|---|---|
| undeclared | refused | `ErrAccountingUnknown`: "cache tokens reported without declaring whether input_tokens includes them" |
| `included` | $0.00005880 | input 156, cache read 256, output 27 |
| `excluded` | $0.00009720 | input 412, cache read 256, output 27 |

The same two integers price **65.3% apart**, $58.80 against $97.20 per million
calls. Nothing in the numbers says which is right.

**Corroborating, from the session "honeycomb-telemetry-schema-fix"**, run the same
day against pydantic/genai-prices (Go package, commit eb37e5e / v0.1.7,
2026-09-15): dropping the cache-read discount entirely and billing all 412 at the
full input rate overstates cost by **32.7%** on gpt-4o-mini, which they verified
by hand. Their o3 (58.5%) and claude-sonnet-4-5 (72.8%) rows were not recomputed
by hand, and the Sonnet row applies Anthropic's price table to OpenAI-shaped
usage, so it is "Sonnet's prices against an OpenAI-shaped record" rather than
what Anthropic would bill. The three failure modes are distinct: dropping the
discount, treating disjoint counts as nested, and treating nested counts as
disjoint.

**Why it cannot be recovered later.** OpenAI's `prompt_tokens` includes
`cached_tokens`; Anthropic's `input_tokens` excludes `cache_read_input_tokens`.
Both arrive as two integers. Upstream states the same problem:
open-telemetry/semantic-conventions-genai#487, open, on consumers being unable to
tell which usage attributes are subsets of a total. Cited from that session, not
read here.

**Decision.** Keep. Added `interlingua.usage.cache_included_in_input`, written
only on spans that report cached tokens, carrying what the dialect states:
`included`, `excluded` or `unknown`. Every dialect states `unknown` today,
because for a pass-through vocabulary the answer follows the provider on the span
rather than the vocabulary, and no provider's answer has a citation here yet.
`CacheIncludedInInput` takes the parsed span so that a later provider-aware
declaration needs no new plumbing.

**Next cheapest test.** Fill in one provider's answer from its own documentation
(OpenAI is the easiest to cite), declare it for a dialect that passes that
provider through, and re-run the three-way pricing. That turns one `unknown` into
a priced span and shows the attribute doing work.

## 2026-09-17 — Can any dialect stop saying "unknown" yet?

**Claim.** OpenAI documents that its cached tokens are counted inside the input
total, so a dialect that can see the provider is OpenAI could declare `included`
with a citation instead of `unknown`.

**Decision it informs.** Whether to write provider-aware answers now, or leave
every dialect at `unknown`.

**Test.** Rung 1. Read OpenAI's prompt-caching guide, then check the regenerated
fixtures span by span to confirm the declaration would reach a span that needs it.

**Result on the citation.** Documented, 2026-09-17,
developers.openai.com/api/docs/guides/prompt-caching. The guide does not say it
in prose, but its own arithmetic does:

    ordinary_input_tokens = input_tokens - cached_tokens - cache_write_tokens

So cached *and* cache-write tokens are both subsets of the input total, which
matches how genai-observability's estimator subtracts them. Cached tokens are
billed at a reduced rate rather than counted separately.

**Result on the plumbing: a bug in the previous commit.** Verified by execution.
Checking every golden span for "carries a cached count but no convention" found 4:
`openllmetry/openai.chat` and `langchain/ChatOpenAI.chat` at both targets. Those
spans already spell the count the conventions' way, so no rule reads them, nothing
lands in the parsed fields, and the gate that asked the *parse* never fired --
while the count sits on the span the whole time. The attribute was missing from
exactly the spans that are already conformant.

Fixed by asking the span as it leaves instead of the parse: a cached count this
pass wrote, or one the emitter already spelled correctly. Regression test is
`TestAlreadyConformantCachedCountsStillCarryTheConvention`. Re-checked: 0
mismatched spans across all 18 goldens, full suite green in both modules.

**Decision.** Keep the citation, do not write the declaration yet. The provider
answer belongs in a change of its own, after the parallel loss-accounting work is
committed, because it moves the mapping digest again and every dialect's answer
needs its own source. The bug fix lands now, because it is a hole in what already
shipped.

**Next cheapest test.** Declare `included` for OpenAI on one dialect that carries
the provider, re-run the three-way pricing, and confirm a fixture that priced
`unknown` now prices. The OTTL export cannot express a provider condition, so
watch `equivalence.sh`: a provider-aware answer has to be reported as unsupported
there rather than silently disagreeing with the processor.
