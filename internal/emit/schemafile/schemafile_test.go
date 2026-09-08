package schemafile

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

var update = flag.Bool("update", false, "rewrite the golden schema files")

func ruled(t *testing.T) []dialect.Ruled {
	t.Helper()
	var out []dialect.Ruled
	for _, d := range dialect.Dialects() {
		if r, ok := d.(dialect.Ruled); ok {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		t.Fatal("no dialect declares rules; there is nothing to render")
	}
	return out
}

// TestGolden renders every exportable dialect into every target.
//
// The gap list in the header is the reviewable part. A change to a rule that
// moves a mapping from "expressible as a rename" to "not expressible at all"
// is a change to the argument in docs/oteps, and it should show up in a diff
// rather than in a count nobody looked at.
func TestGolden(t *testing.T) {
	for _, d := range ruled(t) {
		for _, target := range semconv.Targets {
			name := string(d.Name()) + "." + target.String()
			t.Run(name, func(t *testing.T) {
				res, err := Render(Options{Dialect: d, Target: target})
				if err != nil {
					t.Fatalf("render %s: %v", name, err)
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
					t.Errorf("%s is stale; rerun with -update to regenerate", golden)
				}
			})
		}
	}
}

// TestEveryRuleIsAccountedFor is the arithmetic the export-gap table depends on.
// A mapping that is neither rendered as a rename, nor already conformant, nor
// recorded as a gap has fallen out of the accounting, and the counts in
// docs/export-gap.md would be quietly wrong rather than visibly wrong.
func TestEveryRuleIsAccountedFor(t *testing.T) {
	for _, d := range ruled(t) {
		for _, target := range semconv.Targets {
			res, err := Render(Options{Dialect: d, Target: target})
			if err != nil {
				t.Fatal(err)
			}
			// Distinct fields, not rules plus unstated: a field can be both,
			// and is then accounted for once as a gap.
			seen := map[semconv.Field]bool{}
			for _, r := range d.Rules() {
				seen[r.Field] = true
			}
			for _, f := range d.Unstated() {
				seen[f] = true
			}
			want := len(seen)
			got := res.Renames + res.Conformant + len(res.Gaps)
			if got != want {
				t.Errorf("%s/%s: %d rules and unstated fields, but %d renames + %d conformant + %d gaps = %d",
					d.Name(), target, want, res.Renames, res.Conformant, len(res.Gaps), got)
			}
		}
	}
}
