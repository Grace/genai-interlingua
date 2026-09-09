// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/emit/ottl"
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

// TestExportedConfigGatesOnItsOwnCorpus holds an emitted config to the spans it
// claims to be for.
//
// A transform processor config runs only on spans matching its conditions, so
// the conditions decide whether the config does anything at all. Getting them
// wrong fails in two opposite ways and both are quiet.
//
// Too narrow and the config never fires. That is not a partial translation, it
// is no translation, under a header advertising however many mappings the rule
// table holds -- a config that silently does nothing while claiming coverage,
// which is the exact failure this repository exists to complain about, shipped
// by this repository. Braintrust was in that state: its gate was built from
// rule keys, its rule keys are all conventions spellings, and the ones already
// correct under the target were dropped as not distinctive, leaving a gate
// matching none of its own spans. It took building a real Collector to notice.
//
// Too wide and the config fires on another emitter's spans and stamps them with
// the wrong interlingua.dialect. That failure was found and fixed earlier and
// has been untested since, which is how widening the gate to fix the first one
// could quietly reintroduce it.
//
// But it is not asserted as an absence of stray matches, because the generated
// header already says the opposite in as many words: the config "is pinned to
// <dialect> and scoped by the conditions below, which only ask whether the span
// carries attributes of the right shape. If more than one GenAI library reports
// into this pipeline, route by service first -- these conditions will not tell
// them apart." Failing a test against a documented, deliberate limitation would
// be manufacturing a bug. Four of the five configs match spans another dialect
// claims, mostly through gen_ai.system and the token counts, and that is the
// disclaimer being true rather than a defect.
//
// What is asserted instead is that the gate has some discriminating power at
// all: at least one key on it appears on no other dialect's spans. A gate made
// entirely of shared spellings is the state the earlier fix was for, and this
// catches a return to it without contradicting the header.
//
// The limit worth stating: this checks the captured corpus, not everything
// these libraries can emit. A gate can still be too narrow for a shape nobody
// captured. It is a much cheaper net than the one that caught Braintrust.
func TestExportedConfigGatesOnItsOwnCorpus(t *testing.T) {
	spans := fixtureSpans(t)

	// Which dialect owns each span, decided once by the same detector the
	// normalizer uses, so the test cannot disagree with production about whose
	// span is whose.
	owner := make([]dialect.Name, len(spans))
	for i, s := range spans {
		if d, _, ok := dialect.Detect(s); ok {
			owner[i] = d.Name()
		}
	}

	for _, d := range dialect.Dialects() {
		ruled, ok := d.(dialect.Ruled)
		if !ok {
			continue
		}

		t.Run(string(d.Name()), func(t *testing.T) {
			for _, target := range semconv.Targets {
				res, err := ottl.Config(ottl.Options{Dialect: ruled, Target: target})
				if err != nil {
					t.Fatalf("emit %s/%s: %v", d.Name(), target, err)
				}
				if len(res.Gates) == 0 {
					t.Fatalf("%s/%s emits a config with no conditions; it would run on every span", d.Name(), target)
				}

				gated := func(s dialect.Span) bool {
					for _, k := range res.Gates {
						if s.Has(k) {
							return true
						}
					}
					return false
				}

				// shared collects gate keys seen on spans belonging to another
				// dialect, so what is left is the gate's exclusive part.
				shared := make(map[string]bool)

				var mine, fired, strays int
				for i, s := range spans {
					switch owner[i] {
					case d.Name():
						mine++
						if gated(s) {
							fired++
							continue
						}
						t.Errorf("%s/%s: a span this dialect claims matches none of the config's conditions"+
							"\n    the exported config would not run on it at all, while its header"+
							"\n    advertises the mappings it carries"+
							"\n    span keys:  %v"+
							"\n    gates on:   %v",
							d.Name(), target, s.Keys(), res.Gates)
					case "":
						// No dialect claims it. Nothing to say.
					default:
						if gated(s) {
							strays++
							for _, k := range res.Gates {
								if s.Has(k) {
									shared[k] = true
								}
							}
						}
					}
				}
				if mine == 0 {
					t.Fatalf("%s/%s: no fixture span is claimed by this dialect; the test is checking nothing", d.Name(), target)
				}

				var exclusive []string
				for _, k := range res.Gates {
					if !shared[k] {
						exclusive = append(exclusive, k)
					}
				}
				if len(exclusive) == 0 {
					t.Errorf("%s/%s: every key the config gates on also appears on another dialect's spans"+
						"\n    the config has no way to tell its own emitter's telemetry from anyone"+
						"\n    else's, and will stamp interlingua.dialect on whatever arrives"+
						"\n    gates on: %v", d.Name(), target, res.Gates)
				}

				t.Logf("%s/%s: %d/%d claimed spans gated, %d exclusive of %d conditions, %d spans matched that another dialect claims",
					d.Name(), target, fired, mine, len(exclusive), len(res.Gates), strays)
			}
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
