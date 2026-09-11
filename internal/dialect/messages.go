// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"encoding/json"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// finishReasons respells the finish reasons emitters write onto the values the
// conventions allow.
//
// The conventions' message schema closes the set at both targets -- stop,
// length, content_filter, tool_call, error, and compaction in the new repository
// -- and gen_ai.response.finish_reasons is the same reasons one per choice. So
// every dialect puts its reasons through one table rather than each keeping its
// own: the fixtures had OpenLLMetry writing tool_call, OpenInference and the
// legacy shape tool_calls, the Vercel AI SDK tool-calls, and a backend grouping
// by reason would have shown three buckets for one fact.
//
// A reason the table does not name passes through as written. The registry
// attribute itself has no closed value set, so an unfamiliar reason is not a
// conformance failure, and guessing at one would be.
var finishReasons = map[string]string{
	"tool_calls":     "tool_call",      // OpenAI, and everything that copies its response
	"function_call":  "tool_call",      // OpenAI's name for the same event before tools
	"tool-calls":     "tool_call",      // the Vercel AI SDK
	"content-filter": "content_filter", // the Vercel AI SDK
}

// finishReason respells one reason. See finishReasons.
func finishReason(r string) string {
	if to, ok := finishReasons[r]; ok {
		return to
	}
	return r
}

// takeFinishReasons reads a finish reasons attribute in any of the shapes
// emitters write -- the list the conventions define, one bare string, or a JSON
// array encoded into a string, which is what LiteLLM writes -- and respells each
// reason. It reports whether the span carried the key.
func (p *Parsed) takeFinishReasons(s Span, key string) bool {
	v, ok := s.Attr(key)
	if !ok {
		return false
	}
	p.Consumed = append(p.Consumed, key)

	reasons, ok := finishReasonList(v)
	if !ok {
		p.Loss = append(p.Loss, Loss{Key: key, Reason: ReasonCoerced,
			Detail: "finish reasons are neither a list nor a string"})
		return true
	}
	p.setFrom(semconv.ResponseFinishReasons, strSeq(reasons), key)
	return true
}

// finishReasonList reads finish reasons out of whichever shape they arrived in
// and respells each one.
func finishReasonList(v Value) ([]string, bool) {
	var reasons []string
	switch v.Kind {
	case KindStrSeq:
		reasons = v.StrSeq
	case KindStr:
		if err := json.Unmarshal([]byte(v.Str), &reasons); err != nil {
			reasons = []string{v.Str}
		}
	default:
		return nil, false
	}
	out := make([]string, len(reasons))
	for i, r := range reasons {
		out[i] = finishReason(r)
	}
	return out, true
}

// reasonsOf collects the finish reasons carried on reassembled messages.
func reasonsOf(msgs []message) []string {
	var reasons []string
	for _, m := range msgs {
		if m.FinishReason != "" {
			reasons = append(reasons, m.FinishReason)
		}
	}
	return reasons
}

// withSuffix keeps the keys that end in suffix.
func withSuffix(keys []string, suffix string) []string {
	var out []string
	for _, k := range keys {
		if len(k) >= len(suffix) && k[len(k)-len(suffix):] == suffix {
			out = append(out, k)
		}
	}
	return out
}

// rebuildWith runs read and records the field it returns as rebuilt from every
// attribute read consumed on the way. It returns those inputs, for a caller
// that derives a second field from the same run.
func (p *Parsed) rebuildWith(f semconv.Field, read func() Value) []string {
	before := len(p.Consumed)
	v := read()
	inputs := append([]string(nil), p.Consumed[before:]...)
	p.rebuild(f, v, inputs)
	return inputs
}

// message is one entry in gen_ai.input.messages or gen_ai.output.messages, in
// the shape the GenAI conventions define: a role and a list of typed parts.
type message struct {
	Role         string `json:"role"`
	Parts        []part `json:"parts"`
	FinishReason string `json:"finish_reason,omitempty"`
}

// part is one element of a message. A text part carries content; a tool_call
// part carries the call identity and its arguments; a tool_call_response part
// carries the result a tool sent back, under the id of the call it answers.
type part struct {
	Type      string `json:"type"`
	Content   string `json:"content,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments any    `json:"arguments,omitempty"`
	Response  any    `json:"response,omitempty"`
}

func textPart(content string) part { return part{Type: "text", Content: content} }

// toolCallPart builds a tool_call part, parsing arguments as JSON when the
// emitter stringified an object and leaving them as a string when it did not.
// Every emitter that flattens tool calls writes the arguments as a string, and
// most of them wrote a JSON object into it; re-parsing is what makes two
// emitters' tool calls comparable once they land in the same backend.
func toolCallPart(id, name, arguments string) part {
	p := part{Type: "tool_call", ID: id, Name: name}
	if arguments == "" {
		return p
	}
	var args any
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		p.Arguments = arguments
		return p
	}
	p.Arguments = args
	return p
}

// messagesValue renders a message list as the JSON string the conventions
// expect. An empty list renders as an empty Value so callers can pass it
// straight to Parsed.Set.
func messagesValue(msgs []message) Value {
	if len(msgs) == 0 {
		return Value{}
	}
	b, err := json.Marshal(msgs)
	if err != nil {
		return Value{}
	}
	return String(string(b))
}
