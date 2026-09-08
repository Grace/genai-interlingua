package ottl

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

// update rewrites the golden configs. The emitted YAML is the artifact somebody
// pastes into a Collector, so it is committed and reviewed as a diff rather than
// asserted against in fragments: a config is the kind of thing whose whole shape
// matters, and a test that checked six substrings of it would pass on a file
// nobody would want.
var update = flag.Bool("update", false, "rewrite the golden OTTL configs")

func ruled(t *testing.T) []dialect.Ruled {
	t.Helper()
	var out []dialect.Ruled
	for _, d := range dialect.Dialects() {
		if r, ok := d.(dialect.Ruled); ok {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		t.Fatal("no dialect declares rules; there is nothing to export")
	}
	return out
}

// TestGolden emits every exportable dialect into every target.
//
// Both targets matter here for the same reason they matter in the normalizer's
// goldens: the pair is where the moving-target argument shows up as bytes. The
// LiteLLM config under genai-main carries a statement renaming
// gen_ai.usage.cache_creation.input_tokens, and under v1.41.0 it does not,
// because at v1.41.0 that is already the right name.
func TestGolden(t *testing.T) {
	for _, d := range ruled(t) {
		for _, target := range semconv.Targets {
			name := string(d.Name()) + "." + target.String()
			t.Run(name, func(t *testing.T) {
				res, err := Config(Options{Dialect: d, Target: target})
				if err != nil {
					t.Fatalf("emit %s: %v", name, err)
				}

				golden := filepath.Join("testdata", name+".yaml")
				if *update {
					if err := os.MkdirAll("testdata", 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(golden, res.YAML, 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				want, err := os.ReadFile(golden)
				if err != nil {
					t.Fatalf("read %s: %v (rerun with -update)", golden, err)
				}
				if !bytes.Equal(res.YAML, want) {
					t.Errorf("%s does not match the current mapping; rerun with -update to regenerate", golden)
				}
			})
		}
	}
}

// TestNoStatementReadsAnAttributeItAlreadyWrote is the correctness property the
// scratch attribute exists for.
//
// A lookup table emitted as a run of conditional assignments against the
// attribute itself is wrong whenever one of its outputs is also one of its
// inputs: the statement for the second pair sees the first pair's result and
// rewrites it again. LiteLLM's operation table contains exactly that shape.
// Every mapped assignment must therefore land in the scratch key, and the only
// statement reading scratch must be the single move that follows.
func TestNoStatementReadsAnAttributeItAlreadyWrote(t *testing.T) {
	for _, d := range ruled(t) {
		for _, target := range semconv.Targets {
			res, err := Config(Options{Dialect: d, Target: target})
			if err != nil {
				t.Fatalf("emit %s: %v", d.Name(), err)
			}
			lines := strings.Split(string(res.YAML), "\n")

			for _, r := range d.Rules() {
				if len(r.Transform.Map) == 0 {
					continue
				}
				key, ok := target.Key(r.Field)
				if !ok {
					continue
				}
				for _, line := range lines {
					if !strings.Contains(line, "where "+`attributes["`+key+`"] == "`) {
						continue
					}
					// This is a table lookup. Its assignment target must be
					// the scratch key, never the attribute being tested.
					if !strings.Contains(line, `set(attributes["`+scratch+`"]`) {
						t.Errorf("%s/%s: a lookup assigns to the attribute it tests, so a value that is\n"+
							"    both an input and an output of the table would be rewritten twice:\n    %s",
							d.Name(), target, strings.TrimSpace(line))
					}
				}
			}

			// Scratch must never survive the processor: it is an implementation
			// detail of the emitted config, and a span carrying it would be
			// leaking one.
			deletes := 0
			for _, line := range lines {
				if strings.Contains(line, `delete_key(attributes, "`+scratch+`")`) {
					deletes++
				}
			}
			sets := 0
			for _, line := range lines {
				if strings.Contains(line, `set(attributes["`+scratch+`"]`) {
					sets++
				}
			}
			if sets > 0 && deletes == 0 {
				t.Errorf("%s/%s: writes %s but never deletes it", d.Name(), target, scratch)
			}
		}
	}
}

// TestUnsupportedIsOnTheSpanAndInTheHeader keeps the two halves of the honesty
// claim from drifting. A reader gets the list from the config's comments; a
// query gets it from interlingua.export.unsupported. They have to agree, or one
// of them is a lie.
func TestUnsupportedIsOnTheSpanAndInTheHeader(t *testing.T) {
	for _, d := range ruled(t) {
		res, err := Config(Options{Dialect: d, Target: semconv.DefaultTarget})
		if err != nil {
			t.Fatal(err)
		}
		yaml := string(res.YAML)
		for _, key := range res.Unsupported {
			if !strings.Contains(yaml, "#   "+key) {
				t.Errorf("%s: %s is unsupported but the header does not list it", d.Name(), key)
			}
			if !strings.Contains(yaml, `"interlingua.export.unsupported"`) ||
				!strings.Contains(yaml, `"`+key+`"`) {
				t.Errorf("%s: %s is unsupported but no span will say so", d.Name(), key)
			}
		}
	}
}

// TestEveryCarriedAttributeIsRepresentable guards the case where a rule names a
// field the chosen target has no attribute for. The normalizer records that as
// a loss; the exported config has to leave it out entirely rather than invent a
// key, because an attribute name no schema defines is worse than a missing one.
func TestEveryCarriedAttributeIsRepresentable(t *testing.T) {
	for _, d := range ruled(t) {
		for _, target := range semconv.Targets {
			res, err := Config(Options{Dialect: d, Target: target})
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range strings.Split(string(res.YAML), "\n") {
				line = strings.TrimSpace(line)
				if !strings.HasPrefix(line, "- set(attributes[\"gen_ai.") {
					continue
				}
				key := line[len(`- set(attributes["`):]
				key = key[:strings.Index(key, `"`)]
				if _, ok := semconv.FieldForKey(key); !ok {
					t.Errorf("%s/%s: writes %q, which no target defines", d.Name(), target, key)
					continue
				}
				f, _ := semconv.FieldForKey(key)
				if !target.Represents(f) {
					t.Errorf("%s/%s: writes %q, which %s does not define", d.Name(), target, key, target)
				}
			}
		}
	}
}

func TestConfigRejectsAnUnexportableDialect(t *testing.T) {
	if _, err := Config(Options{Target: semconv.DefaultTarget}); err == nil {
		t.Error("emitting with no dialect succeeded")
	}
	if _, err := Config(Options{Dialect: ruled(t)[0], Target: "v9.99.0"}); err == nil {
		t.Error("emitting into an unknown target succeeded")
	}
}
