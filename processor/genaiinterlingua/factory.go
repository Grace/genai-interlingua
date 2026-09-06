package genaiinterlingua

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"

	"github.com/Grace/genai-interlingua/internal/normalize"
)

// Type is the name this processor is configured under.
var Type = component.MustNewType("genaiinterlingua")

// NewFactory returns the factory a Collector build registers.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		Type,
		createDefaultConfig,
		processor.WithTraces(createTraces, component.StabilityLevelAlpha),
	)
}

// createDefaultConfig is the frozen target with the emitter's own attributes
// kept, which is the same default the CLI and internal/normalize use.
func createDefaultConfig() component.Config {
	defaults := normalize.DefaultOptions()
	return &Config{
		Target:           defaults.Target.String(),
		PreserveOriginal: defaults.PreserveOriginal,
	}
}

func createTraces(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next consumer.Traces,
) (processor.Traces, error) {
	opts, err := cfg.(*Config).options()
	if err != nil {
		return nil, err
	}
	p := &interlingua{opts: opts}

	return processorhelper.NewTraces(
		ctx, set, cfg, next,
		p.processTraces,
		// The processor rewrites attributes in place, so the pipeline must know
		// it cannot share this batch with a parallel consumer.
		processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}),
	)
}

// A Collector API change that this wrapper needs updating for should be a build
// failure here rather than a surprise at pipeline start.
var (
	_ component.Config                                            = (*Config)(nil)
	_ func(context.Context, ptrace.Traces) (ptrace.Traces, error) = (&interlingua{}).processTraces
)
