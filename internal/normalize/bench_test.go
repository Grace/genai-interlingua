package normalize

import (
	"path/filepath"
	"testing"

	"github.com/Grace/genai-interlingua/internal/dialect"
)

// The numbers these produce belong in the README, because this package runs
// inside a Collector on every span a service emits. "How much does it cost" is
// the first question anyone sensible asks about a pipeline component, and an
// answer of "not much, probably" is not one.

// BenchmarkSpan measures the decision itself: detect a dialect, parse it, render
// it into a target. No JSON, no pdata, just the part every entry point shares.
func BenchmarkSpan(b *testing.B) {
	data := mustReadFile(b, filepath.Join(testdata, "openllmetry", "in.json"))
	spans := spansOf(b, data)
	if len(spans) == 0 {
		b.Fatal("fixture produced no spans")
	}
	span := spans[0]
	opts := DefaultOptions()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := Span(span, opts); !ok {
			b.Fatal("fixture span was not claimed")
		}
	}
}

// BenchmarkUnclaimedSpan is the one to read first.
//
// In any real pipeline the overwhelming majority of spans are HTTP handlers,
// database calls and queue operations with nothing to do with GenAI. Those spans
// pay the cost of being *offered* to every dialect and declined, and they pay it
// on every span in the system, where normalization is paid on very few. If this
// number were bad it would be bad for everybody, including the people who
// installed the processor for one service.
func BenchmarkUnclaimedSpan(b *testing.B) {
	span := dialect.Span{
		Name: "GET /orders/:id",
		Kind: "SPAN_KIND_SERVER",
		Attributes: map[string]dialect.Value{
			"http.request.method":       dialect.String("GET"),
			"http.route":                dialect.String("/orders/:id"),
			"http.response.status_code": dialect.Int(200),
			"url.path":                  dialect.String("/orders/A-1187"),
			"server.address":            dialect.String("orders.internal"),
			"db.system":                 dialect.String("postgresql"),
		},
	}
	opts := DefaultOptions()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := Span(span, opts); ok {
			b.Fatal("a span with no GenAI evidence was claimed")
		}
	}
}

// BenchmarkPayload is the whole CLI path on a real fixture: decode the OTLP
// JSON, normalize every span, encode it back. This is what `interlingua`
// delivers per request, and most of it is the codec rather than the mapping.
func BenchmarkPayload(b *testing.B) {
	data := mustReadFile(b, filepath.Join(testdata, "openllmetry", "in.json"))
	opts := DefaultOptions()

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Payload(data, opts); err != nil {
			b.Fatalf("normalize payload: %v", err)
		}
	}
}
