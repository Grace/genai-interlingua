package genaiinterlingua

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor/processortest"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

func TestDefaultConfigPinsTheFrozenCutAndKeepsOriginals(t *testing.T) {
	cfg := createDefaultConfig().(*Config)

	// The default is deliberately not the newest schema: the newest schema is
	// in a repository with no tags, so it cannot be pinned.
	if cfg.Target != semconv.TargetV1_41_0.String() {
		t.Errorf("default target is %q, want %q", cfg.Target, semconv.TargetV1_41_0)
	}
	if !cfg.PreserveOriginal {
		t.Error("the default discards the emitter's own attributes")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("the default configuration does not validate: %v", err)
	}
}

func TestAnUnknownTargetIsRefusedRatherThanDefaulted(t *testing.T) {
	// A pipeline normalizing to a different schema than its author asked for is
	// worse than one that refuses to start. v1.42.0 is the tempting mistake: it
	// exists upstream, and it is the release that removed gen_ai.* entirely.
	cfg := &Config{Target: "v1.42.0", PreserveOriginal: true}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("a target that defines no gen_ai attributes was accepted")
	}
	for _, want := range []string{"v1.42.0", semconv.TargetV1_41_0.String(), semconv.TargetGenAIMain.String()} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %q: %v", want, err)
		}
	}
}

func TestFactoryDeclaresThatItRewritesTheBatch(t *testing.T) {
	// The processor edits attributes in place, so the pipeline must not hand
	// the same batch to a parallel consumer.
	p, err := NewFactory().CreateTraces(
		context.Background(),
		processortest.NewNopSettings(Type),
		createDefaultConfig(),
		consumertest.NewNop(),
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if want := (consumer.Capabilities{MutatesData: true}); p.Capabilities() != want {
		t.Errorf("capabilities are %+v, want %+v", p.Capabilities(), want)
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestASpanWithNoGenAIEvidencePassesThroughUntouched(t *testing.T) {
	// This processor sits in a pipeline carrying every span in the service,
	// most of which have nothing to do with GenAI.
	sink := new(consumertest.TracesSink)
	p, err := NewFactory().CreateTraces(
		context.Background(),
		processortest.NewNopSettings(Type),
		createDefaultConfig(),
		sink,
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := p.Start(context.Background(), componenttest.NewNopHost()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = p.Shutdown(context.Background()) }()

	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("POST")
	span.SetKind(ptrace.SpanKindClient)
	span.Attributes().PutStr("http.request.method", "POST")
	span.Attributes().PutStr("server.address", "api.openai.com")
	span.Attributes().PutInt("http.response.status_code", 200)

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("consume: %v", err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	if got := out.Attributes().Len(); got != 3 {
		t.Errorf("an unrecognized span came out with %d attributes, want the 3 it arrived with", got)
	}
	for _, key := range []string{"interlingua.dialect", "interlingua.target", "interlingua.lossy"} {
		if _, ok := out.Attributes().Get(key); ok {
			t.Errorf("an unrecognized span was stamped with %s", key)
		}
	}
}

func TestSpanKindIsSpelledTheWayTheCLISpellsIt(t *testing.T) {
	// A dialect that signals on span kind must recognize a span identically
	// through both entry points.
	for kind, want := range spanKinds {
		if !strings.HasPrefix(want, "SPAN_KIND_") {
			t.Errorf("%v is spelled %q, which is not the OTLP enum name", kind, want)
		}
	}
	if got := len(spanKinds); got != 6 {
		t.Errorf("the kind map has %d entries, want all 6 OTLP span kinds", got)
	}
}
