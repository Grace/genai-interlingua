// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"strings"
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// Finish reasons arrive as the list the conventions define, as one bare string,
// and -- from LiteLLM -- as a JSON array encoded into a string, which used to
// pass through unread. All three have to come out as the same list.
func TestFinishReasonsAreRespelledInEveryShapeTheyArrive(t *testing.T) {
	for name, c := range map[string]struct {
		in   Value
		want string
	}{
		"a list":                   {StrSeq([]string{"tool_calls", "stop"}), "tool_call,stop"},
		"a bare string":            {String("tool_calls"), "tool_call"},
		"a JSON array in a string": {String(`["tool_calls","stop"]`), "tool_call,stop"},
		"a reason already correct": {String("length"), "length"},
		"a reason nobody respells": {String("other"), "other"},
	} {
		var p Parsed
		s := Span{Attributes: map[string]Value{"gen_ai.response.finish_reasons": c.in}}
		if !p.takeFinishReasons(s, "gen_ai.response.finish_reasons") {
			t.Fatalf("%s: not read", name)
		}
		if got := strings.Join(mustField(t, p, semconv.ResponseFinishReasons).StrSeq, ","); got != c.want {
			t.Errorf("%s: finish reasons = %s, want %s", name, got, c.want)
		}
	}
}

// The table respells onto the conventions' values and nothing else. The message
// schema closes the set at both targets; a respelling that produced a value
// outside it would be a new second spelling rather than the removal of one.
func TestFinishReasonsRespellOnlyOntoTheSchemasValues(t *testing.T) {
	allowed := map[string]bool{"stop": true, "length": true, "content_filter": true, "tool_call": true, "error": true}
	for from, to := range finishReasons {
		if !allowed[to] {
			t.Errorf("%s is respelled to %s, which the message schema does not define", from, to)
		}
	}
}
