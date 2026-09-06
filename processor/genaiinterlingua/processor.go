package genaiinterlingua

import (
	"context"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/normalize"
)

// spanKinds spells pdata's span kind the way the OTLP/JSON codec in
// internal/normalize does. The two entry points must agree: a dialect that
// signals on span kind would otherwise recognize a span through the CLI and not
// through the Collector, which is the kind of difference that only shows up in
// production.
var spanKinds = map[ptrace.SpanKind]string{
	ptrace.SpanKindUnspecified: "SPAN_KIND_UNSPECIFIED",
	ptrace.SpanKindInternal:    "SPAN_KIND_INTERNAL",
	ptrace.SpanKindServer:      "SPAN_KIND_SERVER",
	ptrace.SpanKindClient:      "SPAN_KIND_CLIENT",
	ptrace.SpanKindProducer:    "SPAN_KIND_PRODUCER",
	ptrace.SpanKindConsumer:    "SPAN_KIND_CONSUMER",
}

type interlingua struct {
	opts normalize.Options
}

// processTraces normalizes every span it recognizes and returns the batch. A
// span no dialect claims is left exactly as it arrived: this processor sits in
// a pipeline carrying every span in the service, most of which have nothing to
// do with GenAI, and stamping them all would be a worse outcome than
// recognizing nothing.
func (p *interlingua) processTraces(_ context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	rs := td.ResourceSpans()
	for i := 0; i < rs.Len(); i++ {
		ss := rs.At(i).ScopeSpans()
		for j := 0; j < ss.Len(); j++ {
			spans := ss.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				p.normalizeSpan(spans.At(k))
			}
		}
	}
	return td, nil
}

func (p *interlingua) normalizeSpan(span ptrace.Span) {
	attrs := span.Attributes()

	r, ok := normalize.Span(dialect.Span{
		Name:       span.Name(),
		Kind:       spanKinds[span.Kind()],
		Attributes: attributesToIR(attrs),
	}, p.opts)
	if !ok {
		return
	}

	for _, key := range r.Remove {
		attrs.Remove(key)
	}
	for _, key := range r.SetKeys() {
		putValue(attrs, key, r.Set[key])
	}
}

// attributesToIR reduces a span's attributes to the union the dialects read.
// A value shape the IR has no member for becomes the empty Value, which makes
// the attribute invisible to the dialects and leaves it untouched on the span:
// no convention uses a map or a byte string, and an attribute nothing can read
// is safer left alone than guessed at.
func attributesToIR(attrs pcommon.Map) map[string]dialect.Value {
	out := make(map[string]dialect.Value, attrs.Len())
	attrs.Range(func(k string, v pcommon.Value) bool {
		out[k] = irValue(v)
		return true
	})
	return out
}

func irValue(v pcommon.Value) dialect.Value {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return dialect.String(v.Str())
	case pcommon.ValueTypeInt:
		return dialect.Int(v.Int())
	case pcommon.ValueTypeDouble:
		return dialect.Float(v.Double())
	case pcommon.ValueTypeBool:
		return dialect.Bool(v.Bool())
	case pcommon.ValueTypeSlice:
		s := v.Slice()
		out := make([]string, 0, s.Len())
		for i := 0; i < s.Len(); i++ {
			if s.At(i).Type() != pcommon.ValueTypeStr {
				return dialect.Value{}
			}
			out = append(out, s.At(i).Str())
		}
		if len(out) == 0 {
			return dialect.Value{}
		}
		return dialect.StrSeq(out)
	default:
		return dialect.Value{}
	}
}

func putValue(attrs pcommon.Map, key string, v dialect.Value) {
	switch v.Kind {
	case dialect.KindStr:
		attrs.PutStr(key, v.Str)
	case dialect.KindInt:
		attrs.PutInt(key, v.Int)
	case dialect.KindFloat:
		attrs.PutDouble(key, v.Float)
	case dialect.KindBool:
		attrs.PutBool(key, v.Bool)
	case dialect.KindStrSeq:
		slice := attrs.PutEmptySlice(key)
		for _, s := range v.StrSeq {
			slice.AppendEmpty().SetStr(s)
		}
	}
}
