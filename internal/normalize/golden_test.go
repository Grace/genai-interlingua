package normalize

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// update rewrites the golden files instead of comparing against them. Six
// dialects times two targets is twelve files of normalized OTLP, which is well
// past the point where hand-editing them stays honest: an expectation you edited
// until it matched is not an expectation. The flag exists so the goldens are
// regenerated and then read, in a diff, as the review step.
var update = flag.Bool("update", false, "rewrite the golden files from the current mapping")

const testdata = "../../testdata"

// TestGolden normalizes every captured payload into every target and compares
// the result byte for byte. The pair of goldens per dialect is the point: the
// same input under two schema versions, where every difference between them is
// something docs/moving-target.md has to be able to explain.
func TestGolden(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join(testdata, "*", "in.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no fixtures under %s", testdata)
	}

	for _, input := range inputs {
		dir := filepath.Dir(input)
		for _, target := range semconv.Targets {
			t.Run(filepath.Base(dir)+"/"+target.String(), func(t *testing.T) {
				opts := DefaultOptions()
				opts.Target = target

				got, err := Payload(mustReadFile(t, input), opts)
				if err != nil {
					t.Fatalf("normalize %s: %v", input, err)
				}

				golden := filepath.Join(dir, "out."+target.String()+".json")
				if *update {
					if err := os.WriteFile(golden, got, 0o644); err != nil {
						t.Fatalf("write %s: %v", golden, err)
					}
					return
				}
				if want := mustReadFile(t, golden); !bytes.Equal(got, want) {
					t.Errorf("%s does not match the current mapping\n%s\nrerun with -update to regenerate",
						golden, firstDifference(want, got))
				}
			})
		}
	}
}

func mustReadFile(t testing.TB, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// firstDifference reports the first line the two documents disagree on. A full
// diff of a normalized span is longer than a test failure should be, and the
// first divergence is nearly always the whole story.
func firstDifference(want, got []byte) string {
	w := strings.Split(string(want), "\n")
	g := strings.Split(string(got), "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		wl, gl := line(w, i), line(g, i)
		if wl != gl {
			return "line " + strconv.Itoa(i+1) + ":\n  want: " + wl + "\n  got:  " + gl
		}
	}
	return "documents differ only in trailing bytes"
}

func line(lines []string, i int) string {
	if i >= len(lines) {
		return "(end of file)"
	}
	return lines[i]
}
