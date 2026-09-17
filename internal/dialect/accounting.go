// SPDX-License-Identifier: Apache-2.0

package dialect

import "github.com/Grace/genai-interlingua/internal/semconv"

// CacheAccounting is what a dialect says about whether the cached-prompt tokens
// it reports are already counted inside the input total it reports.
//
// The question has no answer in the numbers. OpenAI's usage block states
// prompt_tokens 412 with cached_tokens 256 inside it; a provider that counts the
// other way would state 412 and 256 meaning 668 tokens. Both arrive here as two
// integers, and a reader adding them, or pricing them, gets a different answer
// depending on a convention neither number carries. Nothing downstream can
// recover it later, so a dialect that reports cached tokens either states the
// convention or says it does not know.
type CacheAccounting string

const (
	// CacheIncluded: the cached tokens are part of the input count, so billed
	// input is input minus cache.
	CacheIncluded CacheAccounting = "included"

	// CacheExcluded: the counts are disjoint, so billed input is the input
	// count as stated.
	CacheExcluded CacheAccounting = "excluded"

	// CacheAccountingUnknown: nothing this translator can read says which.
	//
	// It is a value rather than an absence for the same reason
	// interlingua.lossy.count writes a zero: "nobody can say" and "this span
	// was never asked" are different facts, and only one of them should stop a
	// consumer from pricing. A consumer that treats this as a licence to guess
	// has been told, in the span, that it is guessing.
	CacheAccountingUnknown CacheAccounting = "unknown"
)

// CacheAccounted is implemented by dialects that state the convention. It is
// optional: a dialect that does not implement it reports unknown, which is the
// honest default rather than a placeholder.
//
// The declaration is a property of the vocabulary, not of one span, so it is a
// method on the dialect and not a rule. It takes the parsed span because for a
// pass-through dialect the answer follows the provider underneath -- OpenAI
// counts cached tokens inside the prompt total, and a provider that does not
// would be reported by the same LiteLLM span shape -- and a dialect that wants
// to read gen_ai.provider.name before answering must be able to. A dialect whose
// own specification fixes the answer can ignore the argument.
type CacheAccounted interface {
	Dialect
	CacheIncludedInInput(Parsed) CacheAccounting
}

// CacheAccountingOf returns what a dialect says about the span, defaulting to
// unknown for a dialect that states nothing and normalizing an unrecognized
// return to unknown rather than passing it through.
func CacheAccountingOf(d Dialect, p Parsed) CacheAccounting {
	a, ok := d.(CacheAccounted)
	if !ok {
		return CacheAccountingUnknown
	}
	switch v := a.CacheIncludedInInput(p); v {
	case CacheIncluded, CacheExcluded:
		return v
	default:
		return CacheAccountingUnknown
	}
}

// cacheFields is every field whose value is a cached-token count, including the
// per-modality ones: a span that reports only gen_ai.usage.image.cache_read.
// input_tokens raises the same question as one reporting the aggregate.
var cacheFields = map[semconv.Field]bool{
	semconv.UsageCacheReadInputTokens:      true,
	semconv.UsageCacheWriteInputTokens:     true,
	semconv.UsageTextCacheReadInputTokens:  true,
	semconv.UsageImageCacheReadInputTokens: true,
	semconv.UsageAudioCacheReadInputTokens: true,
}

// ReportsCacheTokens reports whether the parsed span carries any cached-token
// count. The accounting question exists only then: a span with no cache counts
// has nothing that could be double-counted, and stating a convention about
// numbers it does not have would be noise on every span in the corpus.
func ReportsCacheTokens(p Parsed) bool {
	for f := range p.Fields {
		if cacheFields[f] {
			return true
		}
	}
	return false
}
