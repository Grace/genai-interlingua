// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"strings"
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// TestEveryDialectsInterpretationsAreWellFormed runs the declared readings of
// every registered dialect through the same check the synthetic tests below
// exercise. A dialect that reads one key two ways in one context fails here, on
// the commit that does it, rather than on the first span that happens to take
// the other branch.
func TestEveryDialectsInterpretationsAreWellFormed(t *testing.T) {
	for _, d := range Dialects() {
		t.Run(string(d.Name()), func(t *testing.T) {
			in := InterpretationsOf(d)
			if len(in) == 0 {
				t.Fatalf("%s declares no readings at all", d.Name())
			}
			for _, problem := range CheckInterpretations(in) {
				t.Error(problem)
			}
		})
	}
}

// The invariant this type exists for, stated with a harmless example because no
// fixture happens to carry it cleanly: one spelling, two meanings.
//
// "model" read as the requested model by one dialect and as the responding
// model by another is not a conflict. Each dialect is saying what its own
// emitter meant, and the same string meaning two things in two libraries is the
// ordinary state of GenAI telemetry. What is a conflict is one dialect saying
// both with nothing to tell them apart.
func TestOneSpellingCanMeanDifferentThingsInDifferentDialects(t *testing.T) {
	a := []Interpretation{{Key: "model", Meaning: semconv.RequestModel}}
	b := []Interpretation{{Key: "model", Meaning: semconv.ResponseModel}}

	for name, in := range map[string][]Interpretation{"a": a, "b": b} {
		if problems := CheckInterpretations(in); len(problems) != 0 {
			t.Errorf("dialect %s reading model one way was refused: %v", name, problems)
		}
	}

	read := Origin{Key: "model"}
	if !Declares(a, semconv.RequestModel, read) || Declares(a, semconv.ResponseModel, read) {
		t.Error("dialect a's model is not exactly the requested model")
	}
	if !Declares(b, semconv.ResponseModel, read) || Declares(b, semconv.RequestModel, read) {
		t.Error("dialect b's model is not exactly the responding model")
	}

	both := append(append([]Interpretation(nil), a...), b...)
	problems := CheckInterpretations(both)
	if len(problems) != 1 ||
		!strings.Contains(problems[0], "request.model") || !strings.Contains(problems[0], "response.model") {
		t.Errorf("one dialect reading model as both was not reported as one collision naming both: %v", problems)
	}

	// Saying what tells the two apart is what resolves it, and the reading a
	// parse records has to name the context it decided under.
	told := []Interpretation{
		{Key: "model", When: "the span describes the request", Meaning: semconv.RequestModel},
		{Key: "model", When: "the span describes the response", Meaning: semconv.ResponseModel},
	}
	if problems := CheckInterpretations(told); len(problems) != 0 {
		t.Errorf("two readings distinguished by context were refused: %v", problems)
	}
	if Declares(told, semconv.ResponseModel, Origin{Key: "model", When: "the span describes the request"}) {
		t.Error("a response-model reading was accepted under the request context")
	}
}

// The inverse: different spellings, one meaning, but only where a dialect says
// so. Resembling a declared key is not the same as being one.
func TestDifferentSpellingsMeanOneThingOnlyWhenDeclared(t *testing.T) {
	in := []Interpretation{
		{Key: "prompt_tokens", Meaning: semconv.UsageInputTokens},
		{Key: "input_tokens", Meaning: semconv.UsageInputTokens},
	}
	if problems := CheckInterpretations(in); len(problems) != 0 {
		t.Fatalf("two spellings of input tokens were refused: %v", problems)
	}
	for _, key := range []string{"prompt_tokens", "input_tokens"} {
		if !Declares(in, semconv.UsageInputTokens, Origin{Key: key}) {
			t.Errorf("%s was declared as input tokens and is not recognized as such", key)
		}
	}
	if Declares(in, semconv.UsageInputTokens, Origin{Key: "usage.input_tokens"}) {
		t.Error("an undeclared key converged on input tokens because its name looks like a declared one")
	}
}

// Two dialects reading one key into one field through different value tables
// are making different claims -- llm.request.type=completion is a text
// completion to one emitter and a chat call to another -- so within one dialect
// that is a collision too.
func TestAValueTableIsPartOfTheMeaning(t *testing.T) {
	problems := CheckInterpretations([]Interpretation{
		{Key: "llm.request.type", Meaning: semconv.OperationName, Values: map[string]string{"completion": "text_completion"}},
		{Key: "llm.request.type", Meaning: semconv.OperationName, Values: map[string]string{"completion": "chat"}},
	})
	if len(problems) != 1 || !strings.Contains(problems[0], "completion=chat") ||
		!strings.Contains(problems[0], "completion=text_completion") {
		t.Errorf("one key read through two value tables was not reported with both: %v", problems)
	}
}

func TestMalformedReadingsAreRefused(t *testing.T) {
	for want, i := range map[string]Interpretation{
		"one candidate":     {Key: "x", Candidates: []semconv.Field{semconv.RequestModel}},
		"either decides":    {Key: "x", Meaning: semconv.RequestModel, Candidates: []semconv.Field{semconv.RequestModel, semconv.ResponseModel}},
		"neither a meaning": {Key: "x"},
		"not a field":       {Key: "x", Meaning: "not.a.field"},
		"namespace":         {Key: "x", BySpelling: true},
		"names no key":      {Meaning: semconv.RequestModel},
	} {
		problems := CheckInterpretations([]Interpretation{i})
		if len(problems) == 0 || !strings.Contains(strings.Join(problems, "; "), want) {
			t.Errorf("%+v: want a problem mentioning %q, got %v", i, want, problems)
		}
	}

	ambiguous := []Interpretation{{Key: "x", Candidates: []semconv.Field{semconv.ResponseModel, semconv.RequestModel}}}
	if problems := CheckInterpretations(ambiguous); len(problems) != 0 {
		t.Errorf("a well-formed ambiguity was refused: %v", problems)
	}
	if !DeclaresAmbiguity(ambiguous, "x", []semconv.Field{semconv.RequestModel, semconv.ResponseModel}) {
		t.Error("an ambiguity declared in one order was not recognized in the other")
	}
}

func TestKeyMatches(t *testing.T) {
	for _, c := range []struct {
		pattern, key string
		want         bool
	}{
		{"model", "model", true},
		{"model", "model_name", false},
		{"braintrust.metrics#prompt_tokens", "braintrust.metrics", true},
		{"gen_ai.prompt.*", "gen_ai.prompt.0.content", true},
		{"gen_ai.prompt.*", "gen_ai.prompt", false},
		{"gen_ai.completion.*.finish_reason", "gen_ai.completion.0.finish_reason", true},
		{"gen_ai.completion.*.finish_reason", "gen_ai.completion.0.content", false},
		{"gen_ai.*", "gen_ai.request.model", true},
		{"gen_ai.*", "llm.request.model", false},
	} {
		if got := KeyMatches(c.pattern, c.key); got != c.want {
			t.Errorf("KeyMatches(%q, %q) = %v, want %v", c.pattern, c.key, got, c.want)
		}
	}
}
