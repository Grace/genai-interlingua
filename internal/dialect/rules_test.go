package dialect

import (
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// A dialect that declares rules is making an exportable claim: these mappings
// can be restated somewhere else, in OTTL or a schema file, and mean the same
// thing. These tests are what keeps the claim true, because nothing else can.
// The compiler cannot tell a field set by a rule from one set by a method, and
// the golden fixtures cannot either -- they compare the finished span, which is
// identical whichever way it got there.

// The fixture-driven half of this contract lives in internal/normalize, as
// TestUnstatedIsAccurate: the OTLP corpus and the decoder that reads it are
// both there, and this package cannot import them without a cycle.

// ruledDialects returns every dialect that has been migrated to declared rules.
// Migration is incremental on purpose; a dialect absent here is not broken, it
// is just not exportable yet, and internal/emit says so rather than emitting a
// config that quietly does less than the processor.
func ruledDialects(t *testing.T) []Ruled {
	t.Helper()
	var out []Ruled
	for _, d := range Dialects() {
		if r, ok := d.(Ruled); ok {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		t.Fatal("no dialect declares rules; the rule driver is dead code")
	}
	return out
}

// TestRulesAreWellFormed checks the things a rule table can get wrong on its
// own, without reference to any span.
func TestRulesAreWellFormed(t *testing.T) {
	for _, d := range ruledDialects(t) {
		t.Run(string(d.Name()), func(t *testing.T) {
			seenField := make(map[semconv.Field]bool)
			seenKey := make(map[string]semconv.Field)
			for _, r := range d.Rules() {
				if len(r.Keys) == 0 {
					t.Errorf("rule for %s has no source keys", r.Field)
				}
				if seenField[r.Field] {
					// Two rules for one field is a precedence question written
					// the wrong way round; it belongs in one rule's Keys, where
					// the order is explicit and the second key is only read
					// when the first is absent.
					t.Errorf("two rules produce %s; merge them into one rule's Keys", r.Field)
				}
				seenField[r.Field] = true

				for _, k := range r.Keys {
					if prev, ok := seenKey[k]; ok {
						t.Errorf("key %q feeds both %s and %s; a source attribute means one thing", k, prev, r.Field)
					}
					seenKey[k] = r.Field
				}
				if semconv.CanonicalKey(r.Field) == "" {
					t.Errorf("rule produces %s, which no target defines", r.Field)
				}

				// A translation table whose output is also one of its inputs
				// cannot be exported as a sequence of conditional assignments
				// without the second assignment seeing the first one's result.
				// The emitter works around it with a scratch attribute; this
				// test exists so the workaround stays deliberate rather than
				// load-bearing by accident.
				for in, out := range r.Transform.Map {
					if in == out {
						continue
					}
					if _, chains := r.Transform.Map[out]; chains {
						t.Logf("%s: %q maps to %q, which is itself a key; export must not chain", r.Field, in, out)
					}
				}
			}
		})
	}
}
