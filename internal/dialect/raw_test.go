// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

func TestRawClaimsAHandRolledSpan(t *testing.T) {
	// Three folk spellings and no library in sight.
	s := spanOf(map[string]string{
		"model":             "gpt-4o-mini",
		"prompt_tokens":     "412",
		"completion_tokens": "27",
		"temperature":       "0.7",
	})

	d, confidence, ok := Detect(s)
	if !ok {
		t.Fatal("nothing claimed a hand-rolled GenAI span")
	}
	if d.Name() != Raw {
		t.Fatalf("a hand-rolled span was claimed by %s, want %s", d.Name(), Raw)
	}
	// It did not beat anything; it was the only thing left, and the span should
	// say so rather than claim a confident identification.
	if confidence != 0 {
		t.Errorf("the fallback reported a confidence of %d, want 0", confidence)
	}
}

func TestRawNeedsCorroborationForAFolkSpelling(t *testing.T) {
	// A lone model attribute belongs to a machine learning span, a database
	// span and a hand-rolled LLM span equally.
	if n := (raw{}).Score(spanOf(map[string]string{
		"model":      "resnet50",
		"batch_size": "32",
		"db.system":  "postgresql",
	})); n != 0 {
		t.Errorf("raw scored %d on a span with one folk spelling, want 0", n)
	}
}

func TestRawDoesNotClaimANamespaceItCannotRead(t *testing.T) {
	// Sharing a prefix is not evidence.
	if n := (raw{}).Score(spanOf(map[string]string{
		"llm.rig":     "not a real attribute",
		"llm.harness": "also not one",
	})); n != 0 {
		t.Errorf("raw scored %d on unrecognized keys that merely start with llm., want 0", n)
	}
}

// The fallback must not take a share of the evidence on a span a real dialect
// identifies: the score it took would come straight out of the winner's margin.
func TestRawDoesNotDiluteAPositiveIdentification(t *testing.T) {
	s := spanOf(map[string]string{
		"gen_ai.system":              "OpenAI",
		"llm.request.type":           "chat",
		"gen_ai.usage.prompt_tokens": "412",
		"traceloop.workflow.name":    "support_triage_agent",
	})

	d, margin, ok := Detect(s)
	if !ok || d.Name() != OpenLLMetry {
		t.Fatalf("the span was claimed by %v, want openllmetry", d)
	}
	// OpenLLMetry's own score, undiminished: nothing else that scored competed.
	if want := (openLLMetry{}).Score(s); margin != want {
		t.Errorf("margin is %d, want openllmetry's full score of %d", margin, want)
	}
}

func TestRawTakesAConformantAttributeAtItsWord(t *testing.T) {
	// Someone who wrote gen_ai.usage.input_tokens by hand got it right, and no
	// library being involved does not make the attribute mean less.
	s := Span{Attributes: map[string]Value{
		"gen_ai.usage.input_tokens": Int(412),
		"gen_ai.request.model":      String("gpt-4o-mini"),
	}}
	p := mustParse(t, s)

	if got := mustField(t, p, semconv.UsageInputTokens).Int; got != 412 {
		t.Errorf("input tokens is %d, want 412", got)
	}
	if got := mustField(t, p, semconv.RequestModel).Str; got != "gpt-4o-mini" {
		t.Errorf("request model is %q, want gpt-4o-mini", got)
	}
}

func TestRawPrefersTheConformantKeyOverTheFolkOne(t *testing.T) {
	s := Span{Attributes: map[string]Value{
		"gen_ai.usage.input_tokens": Int(412),
		"prompt_tokens":             Int(999),
		"model":                     String("wrong"),
		"gen_ai.request.model":      String("gpt-4o-mini"),
	}}
	p := mustParse(t, s)

	if got := mustField(t, p, semconv.UsageInputTokens).Int; got != 412 {
		t.Errorf("input tokens is %d, want the conformant attribute's 412", got)
	}
	if got := mustField(t, p, semconv.RequestModel).Str; got != "gpt-4o-mini" {
		t.Errorf("request model is %q, want the conformant attribute's value", got)
	}
	if l := lossFor(t, p, "prompt_tokens"); l.Reason != ReasonNoField {
		t.Errorf("the shadowed folk key was recorded with reason %q", l.Reason)
	}
}

func TestRawReportsFreeFormPayloadsRatherThanGuessingAtThem(t *testing.T) {
	// What is in these varies per author, and a message list guessed out of one
	// would be a worse lie than an honest gap.
	s := spanOf(map[string]string{
		"model":         "gpt-4o-mini",
		"prompt_tokens": "412",
		"prompt":        "Where is order A-1187?",
		"completion":    "Checking now.",
	})
	p := mustParse(t, s)

	if v, ok := p.Fields[semconv.InputMessages]; ok {
		t.Errorf("a message list was invented from a free-form prompt: %v", v)
	}
	for _, k := range []string{"prompt", "completion"} {
		if l := lossFor(t, p, k); l.Reason != ReasonUnstructured {
			t.Errorf("%s was recorded with reason %q, want %q", k, l.Reason, ReasonUnstructured)
		}
	}
}

func TestRawNamesTheAttributesItCouldNotPlace(t *testing.T) {
	// This is the dialect for spans nobody designed, so the loss list is the
	// only record of what their author was trying to say.
	s := spanOf(map[string]string{
		"gen_ai.request.model": "gpt-4o-mini",
		"gen_ai.retry_count":   "2",
		"llm.cache_hit":        "true",
	})
	p := mustParse(t, s)

	for _, k := range []string{"gen_ai.retry_count", "llm.cache_hit"} {
		if l := lossFor(t, p, k); l.Reason != ReasonNoField {
			t.Errorf("%s was recorded with reason %q, want %q", k, l.Reason, ReasonNoField)
		}
	}
}

// openai.* and mcp.* are namespaces the conventions themselves own -- the
// v1.42.0 release notes move model/gen-ai/, model/openai/ and model/mcp/ into
// the new repository together -- so a leftover in them is GenAI data that could
// not be placed, not an unrelated application attribute.
//
// This came from capturing OpenTelemetry's own first party instrumentation,
// which emits openai.response.system_fingerprint. It was being walked past
// silently, on the one span in the fixtures with the best claim to being
// already correct.
func TestRawNamesTheVendorNamespacesTheConventionsOwn(t *testing.T) {
	p := (raw{}).Parse(Span{Attributes: map[string]Value{
		"gen_ai.usage.input_tokens":          Int(412),
		"openai.response.system_fingerprint": String("fp_capture0"),
		"mcp.tool.invocation_id":             String("inv-1"),
	}})

	for _, key := range []string{"openai.response.system_fingerprint", "mcp.tool.invocation_id"} {
		var found bool
		for _, l := range p.Loss {
			if l.Key == key && l.Reason == ReasonNoField {
				found = true
			}
		}
		if !found {
			t.Errorf("%s was not recorded as a no_field loss; losses = %+v", key, p.Loss)
		}
	}
}

// The counterweight: this dialect claims spans that are mostly arbitrary, so
// treating every unrecognized attribute as a loss would report an HTTP span's
// own attributes as GenAI data the normalizer dropped.
func TestRawDoesNotReportUnrelatedAttributesAsLosses(t *testing.T) {
	p := (raw{}).Parse(Span{Attributes: map[string]Value{
		"gen_ai.usage.input_tokens": Int(412),
		"http.request.method":       String("POST"),
		"db.system":                 String("postgresql"),
		"service.version":           String("1.2.3"),
	}})

	for _, l := range p.Loss {
		if l.Key == "http.request.method" || l.Key == "db.system" || l.Key == "service.version" {
			t.Errorf("%s was reported as a GenAI loss; losses = %+v", l.Key, p.Loss)
		}
	}
}

// A token count is evidence on its own, and these three spans are the gaps that
// went unrecorded before it was. Each carries a quantity the span is manifestly
// reporting and no mapping reads: unclaimed, they left the pipeline with no
// interlingua.* on them at all, so nothing said the count had been walked past.
// Claimed, the count is named in interlingua.lossy. Nothing here maps it.
func TestRawClaimsASpanOnATokenCountAlone(t *testing.T) {
	for _, c := range []struct {
		name string
		span map[string]string
	}{
		{
			// Under the two-folk-spelling threshold: one alias and a count
			// nothing reads.
			name: "one alias and an unread count",
			span: map[string]string{
				"model":      "gpt-4o-mini",
				"tokens.out": "27",
			},
		},
		{
			// Spellings no library emits and rawAliases does not carry.
			name: "unaliased counts",
			span: map[string]string{
				"tokens.in":  "412",
				"tokens.out": "27",
			},
		},
		{
			name: "camelCase counts",
			span: map[string]string{
				"promptTokens":     "412",
				"completionTokens": "27",
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if n := (raw{}).Score(spanOf(c.span)); n == 0 {
				t.Fatalf("raw declined a span reporting a token count: %v", c.span)
			}
		})
	}
}

// The packed case: one string holding the whole usage object. It is claimed for
// the same reason and parsed for none -- a Python repr is not a format this
// reads, and lifting a number out of it by guessing at the shape would be the
// silent mismapping the loss list exists to prevent.
func TestRawClaimsAPackedUsageBlob(t *testing.T) {
	s := Span{Attributes: map[string]Value{
		"llm.openai.max_tokens": String("1024"),
		"llm.openai.usage": String(
			"{'completion_tokens': 27, 'prompt_tokens': 412, 'total_tokens': 439}"),
	}}
	if n := (raw{}).Score(s); n == 0 {
		t.Fatal("raw declined a span carrying a packed usage object")
	}
	p := mustParse(t, s)
	if _, ok := p.Fields[semconv.UsageInputTokens]; ok {
		t.Error("a packed usage blob was parsed into an input token count; it must be recorded, not guessed at")
	}
	var named bool
	for _, l := range p.Loss {
		if l.Key == "llm.openai.usage" {
			named = true
		}
	}
	if !named {
		t.Error("llm.openai.usage was not named in the loss list")
	}
}

// The counterweight to the three tests above. Most spans in a pipeline are HTTP,
// several of them carry a token word, and none of them is a model call. A
// detector that claims one of these is worse than the gap it closed.
func TestRawDeclinesCeilingsAndRateLimits(t *testing.T) {
	for _, c := range []struct {
		name string
		span map[string]string
	}{
		{
			name: "rate limit headers on an http span",
			span: map[string]string{
				"http.request.method":                               "POST",
				"http.response.header.x-ratelimit-remaining-tokens": "39616",
				"http.response.header.x-ratelimit-limit-tokens":     "40000",
			},
		},
		{
			// A ceiling is not a quantity spent, so it is not evidence. Two
			// folk aliases still claim a span on the older rule -- model plus
			// max_tokens scores 2 and always has -- so this pairs the ceiling
			// with a key that is not an alias, to ask only whether the ceiling
			// itself brought anything.
			name: "a request ceiling alone",
			span: map[string]string{
				"http.request.method": "POST",
				"max_tokens":          "1024",
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if n := (raw{}).Score(spanOf(c.span)); n != 0 {
				t.Errorf("raw scored %d on %s, want 0", n, c.name)
			}
		})
	}
}
