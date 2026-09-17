// SPDX-License-Identifier: Apache-2.0

package dialect

import "strings"

// Usage evidence: recognizing that a span reports how many tokens a model call
// spent, without claiming to know which attribute the count belongs under.
//
// This exists because of the gap between the two halves of this repository's
// argument. A dialect that claims a span records what it could not carry, and
// that record is trustworthy. A span no dialect claimed records nothing at all,
// and a token count written under a spelling no mapping reads is walked past in
// silence -- which is the failure this tool exists to make impossible, occurring
// inside the tool itself.
//
// So the test here is shape, not meaning. It answers "does this key carry a
// token count" and never "which field is it", because the second question is the
// one a hand-rolled span cannot answer and guessing at it would put a number
// under a convention's name on no evidence but a word. Recognizing the shape is
// enough to say the span is in scope and to name the key in interlingua.lossy,
// which is all that is being claimed.

// The lexicon. These are slices rather than switch statements because the
// digest renders them: a word added here changes which spans this build claims
// and which keys it names, and interlingua.mapping has to move when it does.
// See usageLexicon.
var (
	// tokenWords make a key a count of tokens rather than of something else.
	// Both spellings, because llm.token_count.prompt writes the noun singular
	// and prompt_tokens writes it plural.
	tokenWords = []string{"token", "tokens"}

	// directionWords say which side of the call the count is about. A token
	// count needs one: "tokens" alone is a unit, and a key that names a unit
	// without saying what was measured is not evidence of anything.
	directionWords = []string{
		"prompt", "completion", "input", "output", "in", "out",
		"total", "reasoning", "cache", "cached",
	}

	// excludeWords mean a key is about a budget, a ceiling or a rate rather
	// than a quantity spent. max_tokens is a request parameter and
	// x-ratelimit-remaining-tokens is a header; neither is usage, and claiming
	// an HTTP span because it carries the second would be the false positive
	// that discredits the whole detector.
	excludeWords = []string{
		"max", "limit", "ratelimit", "remaining", "reset", "rate",
		"budget", "window", "quota",
	}

	// packedUsageKeys are the key spellings that hold a whole usage object
	// rather than one number: LiteLLM writes both, one as a Python repr and one
	// as JSON.
	packedUsageKeys = []string{"usage", "usage_object", "token_usage", "usagemetadata"}
)

func anyWord(list []string, w string) bool {
	for _, want := range list {
		if foldEq(w, want) {
			return true
		}
	}
	return false
}

// HasWord reports whether a key contains one word, under the same splitting
// UsageShaped uses. It is how a caller that has already established a key is a
// token count asks which direction it counts, without re-implementing the
// boundaries and drifting from them.
func HasWord(key, word string) bool {
	for w := range words(key) {
		if foldEq(w, word) {
			return true
		}
	}
	return false
}

// usageLexicon renders the word lists for the digest, in the order they are
// declared, so that a reordering that changes nothing does not move the digest
// but an addition or a removal does.
func usageLexicon() string {
	return strings.Join(tokenWords, ",") + " | " +
		strings.Join(directionWords, ",") + " | " +
		strings.Join(excludeWords, ",") + " | " +
		strings.Join(packedUsageKeys, ",")
}

// UsageShaped reports whether a key names a spent-token count, by its spelling
// alone.
//
// The key is split into words on ".", "_", "-" and camelCase boundaries, so
// prompt_tokens, promptTokens, tokens.in and llm.token_count.prompt are one
// question rather than four. A match needs a token word, a direction word, and
// no word that makes it a ceiling.
//
// It allocates nothing: every word is a subslice of the key, compared without
// being lowercased.
func UsageShaped(key string) bool {
	var token, direction bool
	for word := range words(key) {
		switch {
		case anyWord(excludeWords, word):
			return false
		case anyWord(tokenWords, word):
			token = true
		case anyWord(directionWords, word):
			direction = true
		}
	}
	return token && direction
}

// packedUsageMarkers are the field names a serialized usage object contains. The
// value is searched for the marker rather than parsed: this reports that a
// number is in there, and declines to say which number, because the shapes
// differ per provider and per serialization and a wrong lift is worse than an
// honest gap.
var packedUsageMarkers = [...]string{
	"prompt_tokens", "completion_tokens", "input_tokens", "output_tokens",
	"promptTokens", "completionTokens", "inputTokens", "outputTokens",
}

// PackedUsage reports whether a key holds a serialized usage object.
//
// Both halves are required. The key alone would claim any attribute ending in
// "usage", including a user id under user.usage; the value alone would claim an
// HTTP response body that happens to quote an API's JSON. Together they are
// specific enough to name, and naming is all this is for.
func PackedUsage(key string, v Value) bool {
	if v.Kind != KindStr || v.Str == "" {
		return false
	}
	last := key
	if i := strings.LastIndexByte(key, '.'); i >= 0 {
		last = key[i+1:]
	}
	if !anyWord(packedUsageKeys, last) {
		return false
	}
	for _, marker := range packedUsageMarkers {
		if strings.Contains(v.Str, marker) {
			return true
		}
	}
	return false
}

// words yields each word in a key: subslices, so nothing is allocated. The
// boundaries are ".", "_", "-" and the lower-to-upper transition inside
// camelCase, which is the only one of the four that is not a separator
// character.
func words(key string) func(func(string) bool) {
	return func(yield func(string) bool) {
		start := 0
		for i := 0; i < len(key); i++ {
			c := key[i]
			switch {
			case c == '.' || c == '_' || c == '-' || c == '/':
				if i > start && !yield(key[start:i]) {
					return
				}
				start = i + 1
			case i > start && isUpper(c) && isLower(key[i-1]):
				if !yield(key[start:i]) {
					return
				}
				start = i
			}
		}
		if start < len(key) {
			yield(key[start:])
		}
	}
}

func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isLower(c byte) bool { return c >= 'a' && c <= 'z' }

// foldEq compares a key's word against a lowercase literal without allocating.
// Only ASCII case is folded, which is all an attribute key needs.
func foldEq(s, lower string) bool {
	if len(s) != len(lower) {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isUpper(c) {
			c += 'a' - 'A'
		}
		if c != lower[i] {
			return false
		}
	}
	return true
}
