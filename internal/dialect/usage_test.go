// SPDX-License-Identifier: Apache-2.0

package dialect

import "testing"

// The positive cases are the spellings the fixtures and the captures actually
// carry, plus the two hand-rolled shapes that went unrecorded until this
// detector existed. The negative cases matter more: this runs on every span in a
// pipeline where most spans are HTTP, and a detector that claims one of those
// has done more damage than the gap it closed.
func TestUsageShaped(t *testing.T) {
	yes := []string{
		"prompt_tokens",
		"completion_tokens",
		"input_tokens",
		"output_tokens",
		"tokens.in",
		"tokens.out",
		"promptTokens",
		"completionTokens",
		"llm.token_count.prompt",
		"llm.token_count.completion",
		"gen_ai.usage.input_tokens",
		"gen_ai.usage.cache_read.input_tokens",
		"gen_ai.usage.reasoning.output_tokens",
		"ai.usage.promptTokens",
		"usage.total_tokens",
		"llm.prompt_tokens",
		"TOKENS_IN",
		"output-tokens",
	}
	for _, k := range yes {
		if !UsageShaped(k) {
			t.Errorf("UsageShaped(%q) = false, want true", k)
		}
	}

	no := []string{
		// Ceilings and budgets. max_tokens is on every one of the fixtures,
		// and claiming it would report a request parameter as a spent count.
		"max_tokens",
		"gen_ai.request.max_tokens",
		"ai.settings.maxOutputTokens",
		"llm.openai.max_tokens",
		"traceloop.association.properties.ls_max_tokens",

		// Rate limiting. These ride along on HTTP spans next to real model
		// calls, which is the likeliest place to produce a false claim.
		"http.response.header.x-ratelimit-remaining-tokens",
		"x-ratelimit-limit-tokens",
		"ratelimit.tokens.remaining",
		"rate_limit.input_tokens",

		// A token that is not a count of anything.
		"auth.token",
		"next_page_token",
		"token",
		"tokens",
		"csrf_token",

		// Directions without a token word: an input is not a token count.
		"input",
		"output",
		"gen_ai.input.messages",
		"llm.prompt",
		"cache.hit",

		// HTTP and database spans, the traffic this must ignore entirely.
		"http.request.method",
		"http.response.status_code",
		"db.system",
		"service.name",

		// interlingua.usage.cache_included_in_input is written by the
		// normalizer to say whether cached tokens sit inside the input count.
		// It is a convention flag, not a quantity, and it must never be read as
		// an unrecorded token count. It carries "cache" and "input", which are
		// both direction words, and no token word -- so it is exactly the key
		// that would start matching if a cache or direction word were ever made
		// sufficient on its own. It is pinned here for that reason.
		"interlingua.usage.cache_included_in_input",
	}
	for _, k := range no {
		if UsageShaped(k) {
			t.Errorf("UsageShaped(%q) = true, want false", k)
		}
	}
}

func TestPackedUsage(t *testing.T) {
	// The LiteLLM shapes, as captured: one Python repr, one JSON.
	repr := String("{'completion_tokens': 27, 'prompt_tokens': 412, 'total_tokens': 439}")
	js := String(`{"prompt_tokens": 412, "completion_tokens": 27, "tokens": 439}`)

	yes := []struct {
		key string
		v   Value
	}{
		{"llm.openai.usage", repr},
		{"metadata.usage_object", repr},
		{"usage", js},
		{"token_usage", js},
		{"llm.anthropic.usage", repr},
	}
	for _, c := range yes {
		if !PackedUsage(c.key, c.v) {
			t.Errorf("PackedUsage(%q, ...) = false, want true", c.key)
		}
	}

	// Both halves are required, so each of these fails on one of them.
	no := []struct {
		key    string
		v      Value
		reason string
	}{
		{"llm.openai.usage", String("ok"), "usage key, no marker in the value"},
		// Braintrust packs its counts under a key that says nothing about
		// usage. The braintrust dialect reads it by name; this detector is not
		// how it is found, and widening the key list to catch it would claim
		// every metrics blob in a pipeline.
		{"braintrust.metrics", js, "markers in the value, not a usage key"},
		{"user.usage", String("heavy"), "usage key, unrelated value"},
		{"http.response.body", js, "markers in the value, not a usage key"},
		{"usage", Int(412), "not a string"},
		{"usage", String(""), "empty"},
		{"gen_ai.usage.input_tokens", Int(412), "a count, not a packed object"},
	}
	for _, c := range no {
		if PackedUsage(c.key, c.v) {
			t.Errorf("PackedUsage(%q, %v) = true, want false (%s)", c.key, c.v, c.reason)
		}
	}
}

// Both detectors run on every attribute of every span passing through a
// collector, including the HTTP ones that will never match. Allocating per key
// would be a tax on all of that traffic to answer a question about a few spans,
// so the words are compared as subslices and nothing is built.
func TestDetectorsDoNotAllocate(t *testing.T) {
	v := String("{'prompt_tokens': 412}")
	if n := testing.AllocsPerRun(100, func() {
		UsageShaped("llm.token_count.prompt")
		UsageShaped("http.response.header.x-ratelimit-remaining-tokens")
		PackedUsage("llm.openai.usage", v)
	}); n != 0 {
		t.Errorf("detectors allocated %v times per run, want 0", n)
	}
}
