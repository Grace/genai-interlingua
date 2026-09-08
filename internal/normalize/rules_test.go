package normalize

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

// fixtureSpans decodes every captured payload into dialect spans. It lives here
// rather than in internal/dialect because the OTLP decoder does, and
// internal/dialect cannot import this package without a cycle.
func fixtureSpans(t *testing.T) []dialect.Span {
	t.Helper()
	inputs, err := filepath.Glob(filepath.Join(testdata, "*", "in.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no fixtures under %s", testdata)
	}

	var out []dialect.Span
	for _, input := range inputs {
		var p payload
		if err := json.Unmarshal(mustReadFile(t, input), &p); err != nil {
			t.Fatalf("decode %s: %v", input, err)
		}
		for _, rs := range p.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for _, s := range ss.Spans {
					out = append(out, s.parsed())
				}
			}
		}
	}
	return out
}

// TestUnstatedIsAccurate holds a dialect's exportable claim to the corpus.
//
// A dialect that declares Rules is saying those mappings can be restated
// somewhere this binary is not running -- in an OTTL config, in a schema file --
// and that Unstated is the complete list of what such a restatement would lose.
// Nothing else in the suite can check that. The goldens compare the finished
// span, which is identical whether a field arrived by rule or by method; the
// conformance table reports what was carried, not how.
//
// So the failure this exists for is quiet by construction. Somebody adds a
// mapping to Parse as a method call because that was the shortest path, and does
// not add it to Rules or to Unstated. Every other test still passes. The only
// thing now wrong is the exported OTTL, which drops a field while advertising
// that it carries everything it does not list -- which is precisely the failure
// mode this repository exists to argue against.
func TestUnstatedIsAccurate(t *testing.T) {
	spans := fixtureSpans(t)

	for _, d := range dialect.Dialects() {
		ruled, ok := d.(dialect.Ruled)
		if !ok {
			// Not migrated yet. That is a supported state -- the dialect works,
			// it simply is not exportable, and internal/emit refuses rather
			// than emitting a partial config that looks complete.
			t.Logf("%s declares no rules; not exportable", d.Name())
			continue
		}

		t.Run(string(d.Name()), func(t *testing.T) {
			stated := make(map[semconv.Field]bool)
			for _, r := range ruled.Rules() {
				stated[r.Field] = true
			}
			admitted := make(map[semconv.Field]bool)
			for _, f := range ruled.Unstated() {
				admitted[f] = true
			}

			// Only spans this dialect actually wins are its responsibility.
			// Parsing a span that belongs to another emitter would produce
			// fields from a reading the dialect never gets asked to make.
			produced := make(map[semconv.Field]bool)
			claimed := 0
			for _, s := range spans {
				winner, _, ok := dialect.Detect(s)
				if !ok || winner.Name() != d.Name() {
					continue
				}
				claimed++
				for f := range d.Parse(s).Fields {
					produced[f] = true
				}
			}
			if claimed == 0 {
				t.Fatalf("no fixture span is claimed by %s; this test is checking nothing", d.Name())
			}

			for _, f := range sortedFields(produced) {
				if stated[f] || admitted[f] {
					continue
				}
				t.Errorf("%s produces %s on a real fixture, but no rule states it and Unstated does not admit it"+
					"\n    an OTTL export would drop it while claiming to carry it;"+
					" add it to Rules if it is a value transform, to Unstated if it is a reading of the span",
					d.Name(), f)
			}

			// The opposite direction is a weaker claim: a field admitted as
			// unstated that no fixture exercises may just be a shape the corpus
			// does not cover. Worth saying, not worth failing.
			for _, f := range ruled.Unstated() {
				if !produced[f] {
					t.Logf("Unstated names %s, which no captured span produces", f)
				}
			}
			t.Logf("%s: %d fixture spans, %d fields produced, %d stated, %d unstated",
				d.Name(), claimed, len(produced), len(stated), len(admitted))
		})
	}
}

func sortedFields(set map[semconv.Field]bool) []semconv.Field {
	out := make([]semconv.Field, 0, len(set))
	for f := range set {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
