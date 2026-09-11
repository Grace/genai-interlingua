// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"flag"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

const mappingsGolden = "testdata/MAPPINGS"

// TestCanonicalMappingsAreStable holds the text the digest is computed over,
// which is the honest way to pin a hash.
//
// A test that asserted the digest equals some hex constant would be true and
// useless: when it failed, it would say a sixteen-character string changed, and
// the reviewer's next question -- which mapping moved -- would have no answer.
// The golden is the answer. The digest on its first line is derived from the
// rest of the file, so the two cannot disagree, and a diff shows the rule that
// changed next to the hash that changed because of it.
//
// This is also the "and not otherwise" half of what a digest has to prove. The
// file is a function of the rule tables, the unstated declarations, the gates
// and the target tables, and of nothing else -- no build stamp, no path, no
// clock. An unrelated commit leaves it byte-identical, which is the property
// that makes stamping it on a span worth the attribute.
func TestCanonicalMappingsAreStable(t *testing.T) {
	got := "# " + Digest() + "\n\n" + canonical()

	if *update {
		if err := os.WriteFile(mappingsGolden, []byte(got), 0o644); err != nil {
			t.Fatalf("write %s: %v", mappingsGolden, err)
		}
		return
	}
	want, err := os.ReadFile(mappingsGolden)
	if err != nil {
		t.Fatalf("read %s: %v (rerun with -update)", mappingsGolden, err)
	}
	if got != string(want) {
		t.Errorf("%s has drifted. The mappings changed, and every span this build "+
			"stamps will carry a different interlingua.mapping than the last one did. "+
			"Read the diff before regenerating.", mappingsGolden)
	}
}

// TestDigestIsStableAcrossCalls is the cheap half: the same build must answer
// the same thing every time, or a span stamped at startup and one stamped an
// hour later would disagree about a mapping that never moved.
func TestDigestIsStableAcrossCalls(t *testing.T) {
	if a, b := Digest(), Digest(); a != b {
		t.Fatalf("digest is not stable: %s then %s", a, b)
	}
	// Map iteration order is the usual way a "canonical" rendering turns out not
	// to be. Rendering repeatedly is a weak check for it, but a free one, and it
	// is the failure that would otherwise appear as a digest that changes on
	// restart.
	first := canonical()
	for i := 0; i < 32; i++ {
		if canonical() != first {
			t.Fatal("canonical() is not deterministic; a map is being iterated unsorted")
		}
	}
}

// TestDigestMovesWhenAMappingMoves is the other half. A digest that never
// changes is as useless as one that always does, so this changes one rule --
// the smallest change the type allows, one character of one source key -- and
// requires a different answer.
func TestDigestMovesWhenAMappingMoves(t *testing.T) {
	base := canonical()

	// Through digestOf, the function Digest itself calls. An earlier version of
	// this test hashed with a private sha256 helper, which made it a proof that
	// SHA-256 is injective and no kind of statement about this package.
	if digestOf(base) != Digest() {
		t.Fatal("digestOf(canonical()) and Digest() disagree; the test is not exercising the real digest")
	}

	moved := strings.Replace(base, "gen_ai.usage.input_tokens", "gen_ai.usage.prompt_tokens", 1)
	if moved == base {
		t.Fatal("no rule reads gen_ai.usage.input_tokens; this test no longer changes anything")
	}
	if digestOf(moved) == digestOf(base) {
		t.Error("a changed source key left the digest where it was")
	}

	// A target's value set widening changes what a span comes out as without
	// any rule moving, so it has to move the digest too.
	widened := base + "    enum chat,embeddings\n"
	if digestOf(widened) == digestOf(base) {
		t.Error("a changed target enum left the digest where it was")
	}
}

// TestDigestCoversEveryFieldOfEveryMappingType is the one that survives the
// people who come after.
//
// Rule and Transform will grow -- rules.go says so in as many words, and
// records that Transform already grew by four fields the first time a second
// dialect was migrated. A field added to either and not written into canonical()
// is a mapping change that produces the same digest, which is the exact failure
// the digest exists to make impossible, and it would be invisible: every test
// still passes, the golden does not move, and the stamp quietly starts lying.
//
// So this does not enumerate the fields. It reflects over them, sets each one
// to a value distinguishable from its zero, and requires the rendering to
// notice. A new field fails here on the commit that adds it.
func TestDigestCoversEveryFieldOfEveryMappingType(t *testing.T) {
	zero := render(Rule{Field: semconv.RequestModel, Keys: []string{"a"}})

	rt := reflect.TypeOf(Rule{})
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		r := Rule{Field: semconv.RequestModel, Keys: []string{"a"}}
		v := reflect.ValueOf(&r).Elem().Field(i)
		if f.Name == "Transform" {
			continue // covered field by field below
		}
		if !set(v) {
			t.Fatalf("Rule.%s: test does not know how to vary a %s", f.Name, f.Type)
		}
		if render(r) == zero {
			t.Errorf("Rule.%s is not written into canonical(); changing it would not move the digest", f.Name)
		}
	}

	tt := reflect.TypeOf(Transform{})
	for i := 0; i < tt.NumField(); i++ {
		f := tt.Field(i)
		r := Rule{Field: semconv.RequestModel, Keys: []string{"a"}}
		v := reflect.ValueOf(&r.Transform).Elem().Field(i)
		if !set(v) {
			t.Fatalf("Transform.%s: test does not know how to vary a %s", f.Name, f.Type)
		}
		if render(r) == zero {
			t.Errorf("Transform.%s is not written into canonical(); changing it would not move the digest", f.Name)
		}
	}

	// Interpretation is held to the same rule, and it matters more there: it is
	// where the meaning of the hand-written half lives, and a field on it that
	// the digest ignored would be a change of meaning nothing could see.
	base := renderInterpretation(Interpretation{Key: "a"})
	it := reflect.TypeOf(Interpretation{})
	for i := 0; i < it.NumField(); i++ {
		f := it.Field(i)
		in := Interpretation{Key: "a"}
		v := reflect.ValueOf(&in).Elem().Field(i)
		if !set(v) {
			t.Fatalf("Interpretation.%s: test does not know how to vary a %s", f.Name, f.Type)
		}
		if renderInterpretation(in) == base {
			t.Errorf("Interpretation.%s is not written into canonical(); changing it would not move the digest", f.Name)
		}
	}
}

// TestDigestMovesWhenAMeaningMoves is the requirement the Interpretation type
// was added to meet. Reading "model" as the requested model and reading it as
// the responding model are different mappings of the same key, and a digest
// that treated them as one would certify two different translations as the
// same.
func TestDigestMovesWhenAMeaningMoves(t *testing.T) {
	requested := Interpretation{Key: "model", Meaning: semconv.RequestModel}
	responded := Interpretation{Key: "model", Meaning: semconv.ResponseModel}
	base := canonical()
	if digestOf(base+renderInterpretation(requested)) == digestOf(base+renderInterpretation(responded)) {
		t.Error("the same key read into a different field left the digest where it was")
	}

	// And the real rendering carries the declarations, rather than only the
	// test's own copy of them.
	for _, want := range []string{
		"means braintrust.span_attributes#type",
		"means llm.model_name when=" + `"` + openInferenceResponded + `"` + " -> response.model",
		"means traceloop.entity.name when=" + `"` + openLLMetryToolSpan + `"` + " -> tool.name",
	} {
		if !strings.Contains(base, want) {
			t.Errorf("canonical() does not render the declared reading %q", want)
		}
	}
}

// TestDigestIgnoresWhatIsNotMeaning is the "and not otherwise" half for the
// hand-written readings. The order they are declared in, and the prose a loss
// is recorded with, change what a reader of the Go sees and nothing about what
// a span becomes, so neither may move the digest.
func TestDigestIgnoresWhatIsNotMeaning(t *testing.T) {
	a := Interpretation{Key: "x", Meaning: semconv.RequestModel}
	b := Interpretation{Key: "y", When: "z", Meaning: semconv.ResponseModel}

	var forward, backward strings.Builder
	writeInterpretations(&forward, declared{a, b})
	writeInterpretations(&backward, declared{b, a})
	if forward.String() != backward.String() {
		t.Error("declaring the same readings in a different order changed the rendering")
	}

	if strings.Contains(canonical(), "the conventions carry input and output counts only") {
		t.Error("a loss detail string is part of canonical(); rewording it would move the digest")
	}
}

// declared is a dialect with nothing but declared readings, for rendering.
type declared []Interpretation

func (declared) Name() Name                          { return "declared" }
func (declared) Score(Span) int                      { return 0 }
func (declared) Parse(Span) Parsed                   { return Parsed{} }
func (d declared) Interpretations() []Interpretation { return d }

func renderInterpretation(i Interpretation) string {
	var b strings.Builder
	writeInterpretation(&b, i)
	return b.String()
}

// set gives v a value different from its zero, and reports whether it knew how.
// It returns false rather than guessing, so that a field of a type this has
// never seen fails loudly instead of being silently declared covered.
func set(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(true)
	case reflect.String:
		v.SetString("x")
	case reflect.Float64:
		v.SetFloat(3)
	case reflect.Slice:
		if v.Type().Elem().Kind() != reflect.String {
			return false
		}
		// Built element by element rather than converted, so a slice of a named
		// string type -- []semconv.Field -- is handled as well as []string.
		s := reflect.MakeSlice(v.Type(), 1, 1)
		s.Index(0).Set(reflect.ValueOf("x").Convert(v.Type().Elem()))
		v.Set(s)
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String || v.Type().Elem().Kind() != reflect.String {
			return false
		}
		m := reflect.MakeMap(v.Type())
		m.SetMapIndex(reflect.ValueOf("k").Convert(v.Type().Key()),
			reflect.ValueOf("v").Convert(v.Type().Elem()))
		v.Set(m)
	default:
		return false
	}
	return true
}

// render runs one rule through the same writer canonical() uses. Not a second
// copy of the format: a test that reimplemented the rendering would agree with
// itself about a field neither of them wrote, which is the failure this whole
// test exists to catch.
func render(r Rule) string {
	var b strings.Builder
	writeRule(&b, r)
	return b.String()
}

// update regenerates the golden above. Named and behaved like the flag the
// other packages use, so that `go test ./... -update` means one thing.
var update = flag.Bool("update", false, "rewrite the mappings golden")
