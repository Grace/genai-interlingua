// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// declaring is a dialect that states the convention, which no registered
// dialect does yet: every real one answers unknown until a provider's answer has
// a citation behind it. The interface still has to work, and the argument still
// has to reach it, or the first dialect to state something would be the one
// discovering the plumbing is wrong.
type declaring struct {
	answer CacheAccounting
	saw    Parsed
}

func (declaring) Name() Name        { return Name("declaring") }
func (declaring) Score(Span) int    { return 0 }
func (declaring) Parse(Span) Parsed { return Parsed{} }

func (d *declaring) CacheIncludedInInput(p Parsed) CacheAccounting {
	d.saw = p
	return d.answer
}

func TestADialectThatStatesNothingReportsUnknown(t *testing.T) {
	for _, d := range Dialects() {
		if got := CacheAccountingOf(d, Parsed{}); got != CacheAccountingUnknown {
			t.Errorf("%s answers %q; no dialect has a cited answer yet", d.Name(), got)
		}
	}
}

func TestADeclaredAnswerIsCarriedAndSeesTheSpan(t *testing.T) {
	p := Parsed{Fields: map[semconv.Field]Value{semconv.ProviderName: String("anthropic")}}
	d := &declaring{answer: CacheExcluded}

	if got := CacheAccountingOf(d, p); got != CacheExcluded {
		t.Errorf("accounting = %q, want %q", got, CacheExcluded)
	}
	// The argument is the whole reason this is a method and not a constant: a
	// pass-through vocabulary's answer follows the provider on the span, so a
	// dialect that cannot read the span cannot answer honestly.
	if _, ok := d.saw.Fields[semconv.ProviderName]; !ok {
		t.Error("the dialect was asked without being shown the span it is answering about")
	}
}

// An unrecognized return is not passed through. A dialect that answers with a
// word this package does not define has said something no consumer can act on,
// and "unknown" is what that means.
func TestAnUnrecognizedAnswerBecomesUnknown(t *testing.T) {
	d := &declaring{answer: CacheAccounting("probably")}
	if got := CacheAccountingOf(d, Parsed{}); got != CacheAccountingUnknown {
		t.Errorf("accounting = %q, want %q", got, CacheAccountingUnknown)
	}
}
