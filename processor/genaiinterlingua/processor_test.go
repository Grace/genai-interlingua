// SPDX-License-Identifier: Apache-2.0

package genaiinterlingua

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/Grace/genai-interlingua/internal/normalize"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

const testdata = "../../testdata"

// TestMatchesTheCLIGoldens is the test that matters. The processor and the
// cmd/interlingua CLI are supposed to be one function applied to two
// representations of a span, and the way to keep that true is to check the
// pdata path against output the OTLP/JSON path already committed. Every fixture,
// every target, span by span.
//
// Attributes are compared as maps rather than in order: pdata updates a key in
// place where the JSON codec appends it, and attribute order carries no meaning
// in OTLP. Everything else about the two results has to match exactly.
func TestMatchesTheCLIGoldens(t *testing.T) {
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
				p := &interlingua{opts: mustOptions(t, target)}

				got := mustReadTraces(t, input)
				if _, err := p.processTraces(context.Background(), got); err != nil {
					t.Fatalf("process: %v", err)
				}
				want := mustReadTraces(t, filepath.Join(dir, "out."+target.String()+".json"))

				gotSpans, wantSpans := spansOf(got), spansOf(want)
				if len(gotSpans) != len(wantSpans) {
					t.Fatalf("processed %d spans, the golden has %d", len(gotSpans), len(wantSpans))
				}
				for i := range gotSpans {
					compareAttributes(t, gotSpans[i], wantSpans[i])
				}
			})
		}
	}
}

func compareAttributes(t *testing.T, got, want ptrace.Span) {
	t.Helper()
	g, w := flatten(got.Attributes()), flatten(want.Attributes())

	for _, key := range sortedKeys(w) {
		gv, ok := g[key]
		if !ok {
			t.Errorf("span %q: the CLI produced %s=%s and the processor produced nothing",
				want.Name(), key, w[key])
			continue
		}
		if gv != w[key] {
			t.Errorf("span %q: %s is %s through the processor and %s through the CLI",
				want.Name(), key, gv, w[key])
		}
	}
	for _, key := range sortedKeys(g) {
		if _, ok := w[key]; !ok {
			t.Errorf("span %q: the processor produced %s=%s and the CLI produced nothing",
				want.Name(), key, g[key])
		}
	}
}

func flatten(attrs pcommon.Map) map[string]string {
	out := make(map[string]string, attrs.Len())
	attrs.Range(func(k string, v pcommon.Value) bool {
		out[k] = v.AsString()
		return true
	})
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func spansOf(td ptrace.Traces) []ptrace.Span {
	var out []ptrace.Span
	rs := td.ResourceSpans()
	for i := 0; i < rs.Len(); i++ {
		ss := rs.At(i).ScopeSpans()
		for j := 0; j < ss.Len(); j++ {
			spans := ss.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				out = append(out, spans.At(k))
			}
		}
	}
	return out
}

func mustReadTraces(t testing.TB, path string) ptrace.Traces {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	td, err := (&ptrace.JSONUnmarshaler{}).UnmarshalTraces(b)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return td
}

func mustOptions(t testing.TB, target semconv.Target) normalize.Options {
	t.Helper()
	cfg := createDefaultConfig().(*Config)
	cfg.Target = target.String()
	o, err := cfg.options()
	if err != nil {
		t.Fatalf("options for %s: %v", target, err)
	}
	return o
}
