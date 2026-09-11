// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

// TestEveryReadingIsDeclared holds each parser to its declarations on the
// corpus: every value it attributed to one attribute, every field it rebuilt
// from several, and every value it called ambiguous has to be a reading its
// dialect declares. An interpretation added to the Go without being declared
// fails here, which is what keeps the declarations -- and the digest computed
// over them -- an account of what the parser does rather than of what somebody
// once meant it to do.
//
// It also refuses a written field with no provenance at all: neither one source
// nor a list of inputs. That is a value the page could only call "derived" with
// nothing behind the word.
func TestEveryReadingIsDeclared(t *testing.T) {
	checked := map[string]int{}
	for _, s := range fixtureSpans(t) {
		p, ok := dialect.Parse(s)
		if !ok {
			continue
		}
		d, ok := dialect.ByName(p.Dialect)
		if !ok {
			t.Fatalf("parse names dialect %q, which is not registered", p.Dialect)
		}
		in := dialect.InterpretationsOf(d)

		for f := range p.Fields {
			_, sourced := p.Source[f]
			_, rebuilt := p.Inputs[f]
			if !sourced && !rebuilt {
				t.Errorf("%s set %s on span %q with no source and no inputs", p.Dialect, f, s.Name)
			}
		}
		for f, o := range p.Source {
			checked["source"]++
			if !dialect.Declares(in, f, o) {
				t.Errorf("%s read %s into %s (when %q, by spelling %t) on span %q, and declares no such reading",
					p.Dialect, o.Key, f, o.When, o.BySpelling, s.Name)
			}
		}
		for f, inputs := range p.Inputs {
			for _, k := range inputs {
				checked["rebuild"]++
				if !dialect.DeclaresRebuild(in, f, k) {
					t.Errorf("%s rebuilt %s from %s on span %q, and declares no such reading", p.Dialect, f, k, s.Name)
				}
			}
		}
		for _, l := range p.Loss {
			if l.Reason != dialect.ReasonAmbiguous {
				continue
			}
			checked["ambiguous"]++
			if len(l.Candidates) < 2 {
				t.Errorf("%s called %s ambiguous on span %q without naming what it could have meant",
					p.Dialect, l.Key, s.Name)
			}
			if !dialect.DeclaresAmbiguity(in, l.Key, l.Candidates) {
				t.Errorf("%s called %s ambiguous between %v on span %q, and declares no such reading",
					p.Dialect, l.Key, l.Candidates, s.Name)
			}
		}
	}
	for _, kind := range []string{"source", "rebuild", "ambiguous"} {
		if checked[kind] == 0 {
			t.Errorf("no %s reading was checked; the corpus no longer exercises that half of this test", kind)
		}
	}
}

// The pipeline half of the invariant the Interpretation type states: one
// spelling, read by two dialects as two meanings, has to come out as two
// different fields -- under two different keys, attributed to the same source
// -- and the explanation has to say which meaning each one was given.
//
// The dialects here are not registered. fromParsed is the part of Span that does
// not care who parsed, so a parse built by hand goes through exactly the
// rendering and explanation a registered dialect's would.
func TestOneSpellingTwoMeaningsThroughThePipeline(t *testing.T) {
	s := dialect.Span{Name: "call", Attributes: map[string]dialect.Value{
		"model": dialect.String("claude-sonnet-4-5"),
	}}
	readAs := func(name dialect.Name, f semconv.Field) Attribution {
		t.Helper()
		var p dialect.Parsed
		p.Dialect = name
		if !p.Take(s, "model", f) {
			t.Fatalf("%s: model not read", name)
		}
		opts := DefaultOptions()
		return attributionFrom(t, explanationOf(s, fromParsed(s, p, opts), opts), "model")
	}

	requested := readAs("requesting", semconv.RequestModel)
	responded := readAs("responding", semconv.ResponseModel)

	if requested.From != responded.From {
		t.Fatalf("the two readings do not share a source: %q and %q", requested.From, responded.From)
	}
	for _, c := range []struct {
		a                Attribution
		field, key, what string
	}{
		{requested, "request.model", "gen_ai.request.model", "the requesting dialect's model"},
		{responded, "response.model", "gen_ai.response.model", "the responding dialect's model"},
	} {
		if c.a.Field != c.field || c.a.Key != c.key {
			t.Errorf("%s came out as %s meaning %s, want %s meaning %s", c.what, c.a.Key, c.a.Field, c.key, c.field)
		}
	}
}

// The inverse: two spellings, each declared by its own dialect as input tokens,
// converge on one field and one key, and each stays attributed to what its
// emitter actually wrote.
func TestTwoSpellingsOneMeaningThroughThePipeline(t *testing.T) {
	readAs := func(name dialect.Name, key string) Attribution {
		t.Helper()
		s := dialect.Span{Name: "call", Attributes: map[string]dialect.Value{key: dialect.Int(412)}}
		var p dialect.Parsed
		p.Dialect = name
		if !p.Take(s, key, semconv.UsageInputTokens) {
			t.Fatalf("%s: %s not read", name, key)
		}
		opts := DefaultOptions()
		return attributionFrom(t, explanationOf(s, fromParsed(s, p, opts), opts), key)
	}

	older := readAs("older", "prompt_tokens")
	newer := readAs("newer", "input_tokens")
	if older.Field != newer.Field || older.Key != newer.Key || older.Field != "usage.input_tokens" {
		t.Errorf("two declared spellings of input tokens diverged: %s as %s, %s as %s",
			older.Key, older.Field, newer.Key, newer.Field)
	}
	if older.From == newer.From {
		t.Error("the two spellings lost track of which one each emitter wrote")
	}
}

// A key spelled the conventions' way that no dialect read is checked against
// the target and reported when it does not conform, and it is not read into
// anything: the spelling settles conformance, not meaning.
func TestAConventionsKeyNobodyReadIsCheckedButNotRead(t *testing.T) {
	s := dialect.Span{Name: "execute_task ChatPromptTemplate", Attributes: map[string]dialect.Value{
		"traceloop.span.kind":     dialect.String("task"),
		"traceloop.workflow.name": dialect.String("RunnableSequence"),
		"gen_ai.operation.name":   dialect.String("execute_task"),
	}}
	r := mustNormalize(t, s, semconv.TargetV1_41_0)

	if _, ok := r.Sources["gen_ai.operation.name"]; ok {
		t.Error("an attribute no dialect read was attributed as though one had")
	}
	loss := targetLossFor(t, r, "gen_ai.operation.name")
	if loss.Reason != ReasonNoValue || !strings.Contains(loss.Detail, "execute_task") {
		t.Errorf("the non-conforming value was recorded as %+v", loss)
	}

	// A conforming value in the same position is nobody's business.
	s.Attributes["gen_ai.operation.name"] = dialect.String("chat")
	for _, l := range mustNormalize(t, s, semconv.TargetV1_41_0).TargetLoss {
		if l.Key == "gen_ai.operation.name" {
			t.Errorf("a conforming unread value was reported: %+v", l)
		}
	}
}

// TestTheExplanationSaysWhatEachValueMeans checks the fields a reader needs to
// see spelling and meaning apart, on real captures rather than on shapes built
// for the test.
func TestTheExplanationSaysWhatEachValueMeans(t *testing.T) {
	explain := func(fixture, span string) Explanation {
		t.Helper()
		es, err := Explain(mustReadFile(t, filepath.Join(testdata, fixture, "in.json")), DefaultOptions())
		if err != nil {
			t.Fatalf("%s: %v", fixture, err)
		}
		for _, e := range es {
			if e.Span == span {
				return e
			}
		}
		t.Fatalf("%s has no explained span %q", fixture, span)
		return Explanation{}
	}

	// A reading that turned on evidence says what the evidence was.
	oi := attributionAt(t, explain("openinference", "ChatCompletion"), "gen_ai.response.model")
	if oi.Field != "response.model" || oi.From != "llm.model_name" || !strings.Contains(oi.When, "response") {
		t.Errorf("OpenInference's response model is explained as %+v", oi)
	}

	// A rebuilt value names every input, not one of them.
	legacy := attributionAt(t, explain("openllmetry-legacy", "openai.chat"), "gen_ai.input.messages")
	if !legacy.Derived || len(legacy.Inputs) < 2 {
		t.Errorf("rebuilt input messages are explained as %+v, want every input named", legacy)
	}
	for _, k := range legacy.Inputs {
		if !strings.HasPrefix(k, "gen_ai.prompt.") {
			t.Errorf("rebuilt input messages name %s as an input", k)
		}
	}

	// A value whose type changed says so.
	outer := attributionAt(t, explain("vercel", "ai.generateText"), "gen_ai.response.finish_reasons")
	if outer.SourceKind != "string" || outer.Kind != "string[]" {
		t.Errorf("a scalar finish reason wrapped into a list is explained as %s -> %s", outer.SourceKind, outer.Kind)
	}

	// A value read by its name alone, on a span no library claimed, says that.
	raw := attributionAt(t, explain("raw", "chat gpt-4o-mini"), "gen_ai.request.model")
	if !raw.BySpelling {
		t.Errorf("the raw fallback's request model does not say it was read by spelling: %+v", raw)
	}

	// An ambiguous value names what it could have meant.
	var found bool
	for _, l := range explain("langchain", "RunnableSequence.workflow").Lossy {
		if l.Key != "traceloop.entity.name" {
			continue
		}
		found = true
		sort.Strings(l.Candidates)
		if strings.Join(l.Candidates, ",") != "gen_ai.agent.name,gen_ai.tool.name" {
			t.Errorf("the ambiguous entity name names candidates %v", l.Candidates)
		}
	}
	if !found {
		t.Error("the LangChain workflow span no longer records traceloop.entity.name as ambiguous; this check proves nothing")
	}
}

// TestNoAttributeIsWrittenAsTwoTypes looks across the whole corpus for one
// conventions key written with different value types by different dialects.
// A backend column is one type, so that is a collision a query sees even when
// every value is individually right.
func TestNoAttributeIsWrittenAsTwoTypes(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join(testdata, "*", "in.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	seen := map[string]map[string][]string{}
	for _, input := range inputs {
		fixture := filepath.Base(filepath.Dir(input))
		for _, target := range semconv.Targets {
			opts := DefaultOptions()
			opts.Target = target
			es, err := Explain(mustReadFile(t, input), opts)
			if err != nil {
				t.Fatalf("%s: %v", fixture, err)
			}
			for _, e := range es {
				for _, a := range e.Attributes {
					if a.Own {
						continue
					}
					if seen[a.Key] == nil {
						seen[a.Key] = map[string][]string{}
					}
					seen[a.Key][a.Kind] = append(seen[a.Key][a.Kind], fixture+"/"+e.Span)
				}
			}
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if len(seen[k]) < 2 {
			continue
		}
		var parts []string
		for kind, where := range seen[k] {
			parts = append(parts, kind+" by "+where[0])
		}
		sort.Strings(parts)
		t.Errorf("%s is written as %s", k, strings.Join(parts, " and as "))
	}
}

func attributionFrom(t *testing.T, e Explanation, from string) Attribution {
	t.Helper()
	for _, a := range e.Attributes {
		if a.From == from {
			return a
		}
	}
	t.Fatalf("span %q has no attribute read from %s", e.Span, from)
	return Attribution{}
}

func attributionAt(t *testing.T, e Explanation, key string) Attribution {
	t.Helper()
	for _, a := range e.Attributes {
		if a.Key == key {
			return a
		}
	}
	t.Fatalf("span %q writes no %s", e.Span, key)
	return Attribution{}
}
