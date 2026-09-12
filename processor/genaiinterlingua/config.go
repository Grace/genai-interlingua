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

	// Originals is what becomes of the emitter's own attributes: keep, dedupe
	// or prune. Empty means keep. keep leaves them all beside the normalized
	// ones; dedupe removes only exact copies a plain rename left behind; prune
	// removes everything a dialect read, which makes this processor the last
	// reader of its input and is a trade worth making only once you trust the
	// mapping for the emitters you actually run.
	Originals string `mapstructure:"originals"`

	// PreserveOriginal is the v0.5.0 spelling of Originals, kept so existing
	// configs keep meaning what they meant: true is keep, false is prune. A
	// pointer, because "not set" has to be told apart from false.
	//
	// Deprecated: use Originals.
	PreserveOriginal *bool `mapstructure:"preserve_original"`
}

// Validate reports a configuration that cannot be run. An unknown target is
// named against the legal set rather than silently falling back to the default:
// a pipeline that normalizes to a different schema than its author asked for is
// worse than one that refuses to start. The same goes for originals, where the
// silent fallback would be deleting data or failing to.
func (c *Config) Validate() error {
	if _, err := semconv.ParseTarget(c.Target); err != nil {
		return fmt.Errorf("invalid target: %w", err)
	}
	_, err := c.options()
	return err
}

func (c *Config) options() (normalize.Options, error) {
	target, err := semconv.ParseTarget(c.Target)
	if err != nil {
		return normalize.Options{}, err
	}
	originals, err := c.originals()
	if err != nil {
		return normalize.Options{}, err
	}
	opts := normalize.Options{Target: target, Originals: originals}
	return opts, opts.Validate()
}

// originals resolves the mode from the current setting and the deprecated one.
// Both set and agreeing is allowed, because a config mid-migration should not
// fail to start; both set and disagreeing is refused, because there is no
// honest way to pick.
func (c *Config) originals() (normalize.Originals, error) {
	mode, err := normalize.ParseOriginals(c.Originals)
	if err != nil {
		return "", fmt.Errorf("invalid originals: %w", err)
	}
	if c.PreserveOriginal == nil {
		return mode, nil
	}
	legacy := normalize.OriginalsPrune
	if *c.PreserveOriginal {
		legacy = normalize.OriginalsKeep
	}
	if c.Originals != "" && mode != legacy {
		return "", fmt.Errorf("preserve_original: %t means originals: %s, which conflicts with originals: %s; "+
			"preserve_original is deprecated, so set originals alone", *c.PreserveOriginal, legacy, mode)
	}
	return legacy, nil
}
