// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"testing"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

// The convention a span's numbers were counted under is the one fact about
// cached tokens that cannot be recovered downstream. OpenAI counts its cached
// tokens inside the prompt total and Anthropic counts them beside it, so the
// same two integers price differently under each, and a translator that carried
// the integers across without the convention would be handing on a number that
// means two things.
func TestCachedTokensCarryTheirAccountingConvention(t *testing.T) {
	r := mustNormalize(t, chatSpan(map[string]dialect.Value{
		"gen_ai.usage.cache_read_input_tokens": dialect.Int(256),
	}), semconv.TargetGenAIMain)

	v, ok := r.Set[AttrCacheAccounting]
	if !ok {
		t.Fatalf("a span reporting 256 cached tokens said nothing about whether they are inside its input count")
	}
	// Every dialect states unknown today. That is the honest answer for a
	// pass-through vocabulary whose real answer follows the provider underneath,
	// and this test pins that it is *stated* rather than left to a reader.
	if v.Str != string(dialect.CacheAccountingUnknown) {
		t.Errorf("accounting = %q, want %q", v.Str, dialect.CacheAccountingUnknown)
	}
}

// Absence has to mean something too. Writing "unknown" onto every span in the
// corpus would make the attribute noise, and a reader could no longer tell a
// span whose cached counts are unexplained from one that reported none.
func TestSpansWithoutCachedTokensSayNothingAboutAccounting(t *testing.T) {
	r := mustNormalize(t, chatSpan(nil), semconv.TargetGenAIMain)

	if v, ok := r.Set[AttrCacheAccounting]; ok {
		t.Errorf("a span reporting no cached tokens answered a question it was never asked: %q", v.Str)
	}
}
