// SPDX-License-Identifier: Apache-2.0

package genaiinterlingua

import (
	"context"
	"path/filepath"
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// This is the number that describes the Collector, as opposed to the library:
// it includes converting pdata into the neutral representation the dialects
// read and writing the result back.
//
// The processor mutates spans in place, so each iteration works on a fresh copy.
// Without that, every iteration after the first would be re-normalizing an
// already-normalized span, which is a different and much cheaper operation and
// would make this benchmark quietly meaningless. The copy is excluded from the
// timer rather than subtracted from the result.
func BenchmarkProcessTraces(b *testing.B) {
	opts := mustOptions(b, semconv.DefaultTarget)
	p := &interlingua{opts: opts}
	src := mustReadTraces(b, filepath.Join("..", "..", "testdata", "openllmetry", "in.json"))
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		td := ptrace.NewTraces()
		src.CopyTo(td)
		b.StartTimer()

		if _, err := p.processTraces(ctx, td); err != nil {
			b.Fatalf("processTraces: %v", err)
		}
	}
}

// The same span budget for traffic the processor does not claim. See
// BenchmarkUnclaimedSpan in internal/normalize for why this is the number that
// applies to most of a pipeline.
func BenchmarkProcessTracesUnclaimed(b *testing.B) {
	opts := mustOptions(b, semconv.DefaultTarget)
	p := &interlingua{opts: opts}
	ctx := context.Background()

	src := ptrace.NewTraces()
	span := src.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("GET /orders/:id")
	span.SetKind(ptrace.SpanKindServer)
	attrs := span.Attributes()
	attrs.PutStr("http.request.method", "GET")
	attrs.PutStr("http.route", "/orders/:id")
	attrs.PutInt("http.response.status_code", 200)
	attrs.PutStr("db.system", "postgresql")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		td := ptrace.NewTraces()
		src.CopyTo(td)
		b.StartTimer()

		if _, err := p.processTraces(ctx, td); err != nil {
			b.Fatalf("processTraces: %v", err)
		}
	}
}
