// SPDX-License-Identifier: Apache-2.0

package dialect

import "encoding/json"

// message is one entry in gen_ai.input.messages or gen_ai.output.messages, in
// the shape the GenAI conventions define: a role and a list of typed parts.
type message struct {
	Role         string `json:"role"`
	Parts        []part `json:"parts"`
	FinishReason string `json:"finish_reason,omitempty"`
}

// part is one element of a message. A text part carries content; a tool_call
// part carries the call identity and its arguments.
type part struct {
	Type      string `json:"type"`
	Content   string `json:"content,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments any    `json:"arguments,omitempty"`
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
