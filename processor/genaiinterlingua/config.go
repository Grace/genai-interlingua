// SPDX-License-Identifier: Apache-2.0

// Package genaiinterlingua is an OpenTelemetry Collector processor that
// normalizes GenAI spans from any recognized instrumentation dialect into one
// chosen version of the GenAI semantic conventions.
//
// It is a thin wrapper. Every decision it makes is made by
// github.com/Grace/genai-interlingua/internal/normalize, which knows nothing
// about the Collector, so the behaviour of this processor and the behaviour of
// the cmd/interlingua CLI cannot diverge: they are the same function applied to
// two representations of a span.
package genaiinterlingua

import (
	"fmt"

	"github.com/Grace/genai-interlingua/internal/normalize"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

// Config is the processor's configuration.
type Config struct {
	// Target is the schema version to normalize to. The default is the frozen
	// cut rather than the newest schema, because the newest schema cannot be
	// pinned: gen_ai.* now lives in a repository with no tags and no releases.
	// See docs/moving-target.md.
	Target string `mapstructure:"target"`

	// PreserveOriginal keeps the emitter's own attributes on the span beside
	// the normalized ones. Defaults to true. Turning it off makes this
	// processor the last reader of its input, which is a trade worth making
	// only once you trust the mapping for the emitters you actually run.
	PreserveOriginal bool `mapstructure:"preserve_original"`
}

// Validate reports a configuration that cannot be run. An unknown target is
// named against the legal set rather than silently falling back to the default:
// a pipeline that normalizes to a different schema than its author asked for is
// worse than one that refuses to start.
func (c *Config) Validate() error {
	if _, err := semconv.ParseTarget(c.Target); err != nil {
		return fmt.Errorf("invalid target: %w", err)
	}
	return nil
}

func (c *Config) options() (normalize.Options, error) {
	target, err := semconv.ParseTarget(c.Target)
	if err != nil {
		return normalize.Options{}, err
	}
	return normalize.Options{Target: target, PreserveOriginal: c.PreserveOriginal}, nil
}
