// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"testing"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

// The invariant this repository's whole argument rests on, run as a test rather
// than asserted in prose: a token count either arrives intact, or the key that
// carried it is named in interlingua.lossy. Never neither.
//
// It is about token counts specifically because they are the quantity with a
// price on them. A dropped cached-token count does not merely leave a gap in a
// dashboard: cached tokens bill at a fraction of the input rate, so losing the
// count silently prices every one of them at full rate. Pricing this fixture's
// usage with pydantic/genai-prices, dropping the cache-read count overstates the
// call by 32.7% on gpt-4o-mini and by more on larger models. That is the cost of
// the third case below going unrecorded.
//
// Written as a grid over the shapes that were actually getting through, each
// with a known true answer, so a regression names which shape broke.

// The one exchange every fixture in this repository records.
const (
	truthInput  = 412
	truthOutput = 27
)

type tokenCase struct {
	name string
	span dialect.Span

	// mapped says the counts should arrive under the conventions' own keys.
	// When false, the span reports usage in a spelling no mapping reads, and
	// the requirement is the weaker but non-negotiable one: the carrier keys
	// are named in the loss list.
	mapped bool

	// carriers are the keys holding the counts, for the unmapped cases.
	carriers []string

	// claimed is false for spans that must pass through untouched.
	claimed bool
}

func tokenCases() []tokenCase {
	return []tokenCase{
		{
			name: "conventional spelling",
			span: dialect.Span{Name: "chat", Attributes: map[string]dialect.Value{
				"gen_ai.system":              dialect.String("OpenAI"),
				"gen_ai.usage.input_tokens":  dialect.Int(truthInput),
				"gen_ai.usage.output_tokens": dialect.Int(truthOutput),
			}},
			mapped: true, claimed: true,
		},
		{
			name: "folk spelling the fallback knows",
			span: dialect.Span{Name: "call_model", Attributes: map[string]dialect.Value{
				"model":             dialect.String("gpt-4o-mini"),
				"prompt_tokens":     dialect.Int(truthInput),
				"completion_tokens": dialect.Int(truthOutput),
			}},
			mapped: true, claimed: true,
		},
		{
			// Under the old two-alias threshold, so the span was not claimed
			// at all and nothing recorded that a count had been walked past.
			name: "one alias and an unread count",
			span: dialect.Span{Name: "call_model", Attributes: map[string]dialect.Value{
				"model":      dialect.String("gpt-4o-mini"),
				"tokens.in":  dialect.Int(truthInput),
				"tokens.out": dialect.Int(truthOutput),
			}},
			carriers: []string{"tokens.in", "tokens.out"}, claimed: true,
		},
		{
			name: "camelCase counts no mapping reads",
			span: dialect.Span{Name: "call_model", Attributes: map[string]dialect.Value{
				"model":            dialect.String("gpt-4o-mini"),
				"provider":         dialect.String("OpenAI"),
				"promptTokens":     dialect.Int(truthInput),
				"completionTokens": dialect.Int(truthOutput),
			}},
			carriers: []string{"promptTokens", "completionTokens"}, claimed: true,
		},
		{
			// The LiteLLM raw span's shape: the whole usage object in one
			// string, which this declines to parse and must therefore name.
			name: "packed usage object",
			span: dialect.Span{Name: "raw_gen_ai_request", Attributes: map[string]dialect.Value{
				"llm.openai.max_tokens": dialect.Int(1024),
				"llm.openai.usage": dialect.String(
					"{'completion_tokens': 27, 'prompt_tokens': 412, 'total_tokens': 439}"),
			}},
			carriers: []string{"llm.openai.usage"}, claimed: true,
		},
		{
			// The counterweight. Most spans in a pipeline are this one.
			name: "http span carrying a rate limit header",
			span: dialect.Span{Name: "POST", Attributes: map[string]dialect.Value{
				"http.request.method": dialect.String("POST"),
				"http.response.header.x-ratelimit-remaining-tokens": dialect.Int(39616),
			}},
			claimed: false,
		},
	}
}

func TestTokenCountsAreCarriedOrNamed(t *testing.T) {
	for _, target := range semconv.Targets {
		for _, originals := range AllOriginals {
			for _, c := range tokenCases() {
				t.Run(c.name+"/"+string(target)+"/"+string(originals), func(t *testing.T) {
					opts := Options{Target: target, Originals: originals}
					r, ok := Span(c.span, opts)

					if ok != c.claimed {
						t.Fatalf("claimed = %v, want %v", ok, c.claimed)
					}
					if !c.claimed {
						return
					}

					if c.mapped {
						assertCount(t, r, opts, semconv.UsageInputTokens, truthInput)
						assertCount(t, r, opts, semconv.UsageOutputTokens, truthOutput)
						return
					}

					// Unmapped: nothing may claim to know the counts, and
					// every carrier must be named.
					for _, f := range []semconv.Field{semconv.UsageInputTokens, semconv.UsageOutputTokens} {
						if key, ok := opts.Target.Key(f); ok {
							if _, written := r.Set[key]; written {
								t.Errorf("%s was written from a spelling no mapping reads; a guess is worse than a gap", key)
							}
						}
					}
					lossy := r.Lossy()
					for _, carrier := range c.carriers {
						if !contains(lossy, carrier) {
							t.Errorf("%q carries a token count that did not survive, and interlingua.lossy does not name it (lossy = %v)",
								carrier, lossy)
						}
					}
				})
			}
		}
	}
}

// A span claimed on usage evidence alone has had nothing mapped, so under prune
// there is nothing that has been carried across and every removal would be
// deleting the span's only copy of its own data on the strength of a detection
// guess.
func TestEvidenceOnlyClaimRemovesNothingUnderPrune(t *testing.T) {
	s := dialect.Span{Name: "call_model", Attributes: map[string]dialect.Value{
		"tokens.in":  dialect.Int(truthInput),
		"tokens.out": dialect.Int(truthOutput),
	}}
	opts := Options{Target: semconv.DefaultTarget, Originals: OriginalsPrune}
	r, ok := Span(s, opts)
	if !ok {
		t.Fatal("a span reporting token counts was not claimed")
	}
	for _, k := range r.Remove {
		if k == AttrLossy {
			continue // the count is written; an empty list is removed
		}
		t.Errorf("prune removed %q from a span whose counts were never carried across", k)
	}
}

func assertCount(t *testing.T, r Result, opts Options, f semconv.Field, want int64) {
	t.Helper()
	key, ok := opts.Target.Key(f)
	if !ok {
		t.Fatalf("no attribute for %v at %s", f, opts.Target)
	}
	v, written := r.Set[key]
	if !written {
		t.Fatalf("%s is missing; the emitter reported it", key)
	}
	if v.Int != want {
		t.Errorf("%s = %d, want %d", key, v.Int, want)
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
