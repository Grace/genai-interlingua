# Draft comment for weaver#613

*Formalize allowed transformations for V2.0 based on what weaver diff currently supports.*
Not posted. Paste-ready.

---

I have a measurement that might be useful here, and I want to lead with the method rather
than the numbers, because the method is the only reason the numbers are worth anything.

I maintain a normalizer that translates GenAI spans from six instrumentation libraries into
one `gen_ai.*` schema. Five of them have their mappings declared as data rather than
performed in code, which means I can ask mechanically: for each mapping, what kind of
transformation would a schema format need in order to express it? The answer regenerates from
the rule tables and CI fails if it drifts, so these are measurements rather than
recollections.

Across five real emitters — OpenLLMetry, OpenInference, the Vercel AI SDK, LiteLLM and
Braintrust — and both current GenAI target schemas:

| what was missing | count |
| --- | ---: |
| **value changes, not name changes** — provider aliases (`bedrock` → `aws.bedrock`), operation aliases, case folding, unit conversion | 16 |
| **precedence between two spellings of one field** — `attribute_map` carries both entries but says nothing about which wins when a payload has both | 22 |
| **scalar → array** — the conventions type the attribute as a list and the emitter writes one value | 4 |
| **conditional drop against a closed value set** — the target admits a fixed set and the emitter can produce others | 2 |
| **target has no attribute at all** for the field | 3 |
| **not a transformation of an attribute** — reassembling indexed keys into one document, lifting fields out of a JSON blob, deciding what an attribute means from a sibling | **76** |

The last row is the one I would put most weight on, and it argues *against* expanding the
format. It is the largest category by a wide margin, and none of it should ever be
expressible declaratively. The biggest single contributor is OpenInference, which packs ten
request parameters — temperature, top_p, both penalties, the seed, stop sequences and more —
into one JSON string attribute. No transformation vocabulary anyone would want to specify
reaches inside that, and it should not try.

So my suggestion for scoping V2.0 is roughly: the first four rows are worth arguing about,
44 mappings across five libraries, and the fifth is a reason to draw the line rather than
move it.

Two things I would flag if any of these get specified:

**Precedence is the largest fixable gap and the easiest to under-specify.** A map from old
names to new names has no defined result when a payload carries two of the old names. That is
not hypothetical — the Vercel AI SDK emits `ai.usage.promptTokens` on the span a user's code
creates and `gen_ai.usage.input_tokens` on the span its provider adapter creates, and a single
trace routinely contains both. An ordered list rather than a map would fix it and stays
reversible to its first entry.

**Reversibility differs per transformation, and the WG has wanted bidirectional migration for
a while.** A value lookup is reversible only when injective; a coalesce is reversible only to
its first entry; a unit change is reversible up to floating point. Worth stating per
transformation rather than per format.

Full table, including the per-dialect breakdown and how it is generated:
<https://github.com/Grace/genai-interlingua/blob/main/docs/export-gap.md>

Happy to run this against more emitters if a wider sample would help — the measurement is
automated, so it is cheap.
