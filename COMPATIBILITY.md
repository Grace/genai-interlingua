# Compatibility

What normalizing a span costs, and how to find out whether it cost you anything.

`docs/conformance.md` is the data, generated from the fixtures. This file is the
explanation: what each reason code means, what to do when you see one, and which
changes are made to your values without being recorded as losses at all.

## Reading a normalized span

Every span this processor claims carries four attributes of its own:

| Attribute | Says |
|---|---|
| `interlingua.dialect` | which emitter's vocabulary was recognized |
| `interlingua.dialect.confidence` | how much more evidence the winner had than the runner-up |
| `interlingua.target` | which schema version the `gen_ai.*` keys belong to |
| `interlingua.lossy` | the keys this span is not a faithful carrier of |

`interlingua.lossy` is the one to alert on. If it is absent, everything the
emitter said arrived intact. If it is present, the keys it names are the ones to
look up here.

A confidence of `0` means one of two things, and they are distinguishable by the
dialect beside it. `interlingua.dialect=raw` with confidence `0` is the fallback:
no dialect recognized the span, and it was normalized on shape alone. Any other
dialect with confidence `0` is a tie, where two dialects presented equal evidence
and the winner was decided by registration order. Both are worth looking at. A
tie in particular usually means a span carrying two emitters' attributes at once,
which is a real thing that happens when one library instruments another.

**Nothing is destroyed by default.** `preserve_original: true` is the default, and
under it the emitter's own attributes stay on the span beside the normalized
ones. Every loss described below is a loss of *meaning carried into the target
schema*, not a loss of data, unless you set `strip_original`.

## Two vocabularies, because there are two culprits

A span can fail to arrive intact for reasons that are the emitter's doing or the
schema version's doing, and the codes are kept apart so you can tell which.

### The emitter said something the conventions have no word for

These come from `internal/dialect`. They are properties of the emitter and do not
change when you change target.

**`no_field`** -- the attribute means something real and the conventions name
nothing like it, at any version. This is the commonest code and mostly it is
fine. `gen_ai.usage.total_tokens` is here because the conventions carry input and
output counts and expect you to add them. Traceloop's
`traceloop.association.properties.*` and LiteLLM's `metadata.user_api_key_*` are
here because they are tenancy and correlation dimensions that a proxy operator
genuinely wants and a telemetry standard has no business defining.

*What to do:* if you need one of these, keep `preserve_original` on and query the
original attribute directly. Normalization is not trying to take it away from you.

**`unstructured`** -- the value is a payload whose shape is the author's own.
OpenInference's `input.value`, Vercel's `ai.response.providerMetadata`, and the
free-form `prompt` and `completion` keys on hand-rolled spans are all here. The
dialect could have guessed at a message list; a guessed message list is worse
than an honest gap, because it looks like data.

*What to do:* read the original. If the shape is stable in your system, that is a
good argument for a project-specific dialect rather than for this one guessing.

**`flattened`** -- indexed attributes were reassembled into one JSON document and
the per-index attributes are not separately represented in the output.
`gen_ai.prompt.*` and `llm.input_messages.*` are the recurring cases. The content
survives; the addressing does not, so a query that referenced
`gen_ai.prompt.3.content` has nowhere to point afterwards.

Braintrust's `braintrust.scores` is also here, and it is the one case where the
content does *not* survive: the conventions carry one evaluation per span, and a
span with several scores has none of them mapped. Picking one would be inventing
a verdict the emitter never reached.

*What to do:* query the JSON. For Braintrust's multi-score spans, the scores are
still on the span under their original key.

**`coerced`** -- reserved for a value that survives with the wrong type, most
often a number or boolean the emitter wrote as a string. No dialect currently
performs such a coercion. Its one use today is the other direction: Vercel's
`ai.response.msToFirstChunk` is recorded as `coerced` when it is not a number at
all, in which case the field is left unset rather than converted from something
that is not a duration.

**`ambiguous`** -- the source carried a value the dialect could not attach to one
field with confidence. OpenLLMetry's `traceloop.entity.name` is the example: it
names an agent on an agent span, a tool on a tool span, and nothing identifiable
when `traceloop.span.kind` is missing. Recording it beats guessing which.

`ambiguous` reaches `docs/conformance.md` through the captured OpenLLMetry
workflow span, which sets `traceloop.entity.name` with no `traceloop.span.kind`
beside it -- exactly the case above. `coerced` still appears nowhere: no fixture
triggers it, and it is described here because the pipeline can emit it, not
because you are likely to see it today.

### The target version cannot express it

These come from `internal/normalize`. They are properties of the target you
chose, and they change when you change it. This is the cost of pinning.

**`no_attribute`** -- the field parsed fine and the target names no attribute for
it. Normalizing to `v1.41.0` loses everything the new repository added after the
split this way. In the current fixtures that is `gen_ai.prompt.version` from
OpenLLMetry and `gen_ai.usage.audio.input_tokens` from OpenInference; the full
list per target is generated in `docs/conformance.md`.

**`no_value`** -- the target names the attribute but not the value.
`gen_ai.operation.name` exists at `v1.41.0`; `search_memory` does not. The
attribute is dropped rather than emitted, because the reason to pin a frozen cut
is to get spans that conform to it, and a span carrying a value the registry does
not define is not conformant however useful the word is.

*What to do about either:* switch to `-target genai-main` if you want the newer
vocabulary, and read `docs/moving-target.md` first for what that costs you. The
originals are still on the span either way.

## Changes that are not losses

These alter your values and are deliberately **not** in `interlingua.lossy`,
because nothing was lost. They are listed here because a value changing silently
is worth knowing about even when the change is correct.

**`gen_ai.system` becomes `gen_ai.provider.name`.** The conventions renamed the
attribute. The Vercel AI SDK and LiteLLM both still write the old spelling, and
carrying it through unrenamed leaves a span looking conformant while naming a
field that no longer exists.

**Provider names are translated to the registry's value set.** Emitters name
providers after the company, the routing prefix, or the SDK package; the
conventions name them after the API surface. So OpenInference's `aws` and
LiteLLM's `bedrock` both become `aws.bedrock`, and Vercel's `openai.chat` becomes
`openai`. A provider the registry does not name passes through as the emitter
spelled it and is then dropped by the target check, which surfaces as `no_value`
rather than being hidden behind an invented name.

**Finish reasons are respelled.** The Vercel AI SDK writes `tool-calls` where
every other emitter writes `tool_calls`. `gen_ai.response.finish_reasons` has no
closed value set, so nothing downstream would reject the hyphenated form -- it
would simply sit in your backend as a second spelling of one concept.

**Milliseconds become seconds.** Vercel's `ai.response.msToFirstChunk` is
milliseconds and `gen_ai.response.time_to_first_chunk` is a double in seconds.
The conversion is exact for every millisecond value in `int64` range.

**Stringified JSON is re-parsed.** Tool call arguments and tool parameter schemas
arrive as strings containing JSON from most emitters. They are parsed so the
result is one document rather than a document with a string of JSON inside it,
which is what makes two emitters' tool calls comparable once they land in the
same backend.

## What is never carried

Vercel's `ai.request.headers.*` are enumerated as losses rather than mapped. The
conventions carry no request header attribute, and a provider's request headers
are where its API key lives. This is the one exclusion made on grounds other than
"the conventions have no field for it".
