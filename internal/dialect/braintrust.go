// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

func init() { register(braintrust{}) }

// braintrust parses spans from Braintrust's SDK.
//
// Braintrust models a span as a row in an eval: input, output, expected,
// scores, metadata and metrics, each carried as its own JSON attribute under
// the braintrust namespace. That model predates the GenAI conventions and is
// wider than them in one direction and narrower in another. Wider, because a
// scored span is a first-class thing there and the conventions only learned
// about evaluations later. Narrower, because the model call itself is metadata
// rather than structure.
//
// So this dialect does more lifting out of JSON than any other here, and it is
// the one that most often has to say a thing cannot be carried.
//
// Attribute names are taken from the Braintrust OpenTelemetry integration
// documentation, which lists the braintrust.* attributes it reads and writes.
type braintrust struct{}

func (braintrust) Name() Name { return Braintrust }

// braintrustSpanTypes maps the type in braintrust.span_attributes onto
// gen_ai.operation.name. score and eval have no counterpart: an evaluation is
// modelled as an event in the new repository and as nothing at all in the
// frozen cut, so a span whose whole job is scoring is not an operation the
// conventions name.
var braintrustSpanTypes = map[string]string{
	"llm":      "chat",
	"tool":     "execute_tool",
	"function": "execute_tool",
	"task":     "invoke_workflow",
}

// braintrustMetrics are the members of braintrust.metrics that have a field.
// The rest of the object -- timings, the total -- is recorded as one loss.
var braintrustMetrics = map[string]semconv.Field{
	"prompt_tokens":               semconv.UsageInputTokens,
	"completion_tokens":           semconv.UsageOutputTokens,
	"prompt_cached_tokens":        semconv.UsageCacheReadInputTokens,
	"completion_reasoning_tokens": semconv.UsageReasoningOutputTokens,
}

// The evidence two of Braintrust's readings turn on, spelled once so that the
// Go deciding and the Interpretation declaring the decision cannot drift apart.
const (
	braintrustModelCall = "braintrust.span_attributes type=llm"
	braintrustOneScore  = "braintrust.scores holds exactly one score"
)

// Score keys off the namespace. Nothing else emits braintrust.*, and a span
// that carries any member of it was produced by this SDK, so one signal at
// weight three is the whole test.
func (braintrust) Score(s Span) int {
	if s.HasPrefix("braintrust.") {
		return 3
	}
	return 0
}

// Rules are the Braintrust mappings that can be stated. Every one of them is a
// plain rename with no transform at all, which makes this the shortest table in
// the package and says something about the emitter: Braintrust writes the
// conventions' own attribute names wherever it has a value for them.
//
// One thing this table makes visible that the imperative version hid. Every
// other dialect lowercases the provider before writing it -- litellm, vercel,
// openllmetry and openinference all do -- and this one does not, because it
// never did. Written out as a row next to the others, that reads like an
// oversight rather than a decision. It is left as it was: changing it here
// would be a behaviour change smuggled in with a refactor, and it belongs in
// its own commit with its own fixture.
func (braintrust) Rules() []Rule {
	return []Rule{
		{Field: semconv.ProviderName, Keys: []string{"gen_ai.provider.name", "gen_ai.system"}},
		{Field: semconv.RequestModel, Keys: []string{"gen_ai.request.model"}},
		{Field: semconv.ResponseModel, Keys: []string{"gen_ai.response.model"}},
		{Field: semconv.UsageInputTokens, Keys: []string{
			"gen_ai.usage.input_tokens", "gen_ai.usage.prompt_tokens"}},
		{Field: semconv.UsageOutputTokens, Keys: []string{
			"gen_ai.usage.output_tokens", "gen_ai.usage.completion_tokens"}},
		{Field: semconv.InputMessages, Keys: []string{"gen_ai.input.messages"}},
		{Field: semconv.OutputMessages, Keys: []string{"gen_ai.output.messages"}},
		{Field: semconv.OperationName, Keys: []string{"gen_ai.operation.name"}},
		{Field: semconv.ToolDefinitions, Keys: []string{"gen_ai.agent.tools"}},
	}
}

// Signature is the braintrust namespace, expanded into the keys this parser
// reads. Nothing else emits braintrust.*, which is why Score needs only the
// prefix and one weight of three to identify the emitter outright.
//
// This is the dialect the mechanism exists for. Its rule table names only
// conventions spellings -- gen_ai.request.model, gen_ai.system and the rest --
// because Braintrust writes the conventions' own names wherever it has a plain
// value for them and puts everything else in JSON. A gate built from that table
// keeps only the spellings that are not already the target's, which on a real
// Braintrust span is none of them.
func (braintrust) Signature() []string {
	return []string{
		"braintrust.span_attributes",
		"braintrust.scores",
		"braintrust.metrics",
		"braintrust.input_json",
		"braintrust.output_json",
		"braintrust.expected_json",
		"braintrust.context_json",
		"braintrust.metadata",
		"braintrust.tags",
	}
}

// Unstated is where this dialect stops being like the others.
//
// Five of these nine fields are *also* produced by a rule above, which no
// previous dialect needed. Braintrust carries the same facts twice: once as
// conventions attributes, and once inside the JSON blobs it writes for its own
// product -- braintrust.metrics, braintrust.span_attributes, and on a model
// call braintrust.input_json and braintrust.output_json. A span can arrive with
// either, or both, and Parse reads the conventions half first and lets the
// blobs fill only the gaps.
//
// So "can this be exported" has a third answer here, and it is neither yes nor
// no. An exported config carries these fields when the span spells them the
// conventions' way and silently does not when the span only has the blob. That
// is a weaker guarantee than the other rows in the table and the emitted header
// says so per field rather than averaging it away.
//
// EvaluationName is the sharpest thing in the package. Its value is a JSON
// object *key*, promoted to an attribute value -- the score is named by what
// the map calls it. No transformation of one value can produce that, in this
// type or any plausible successor.
func (braintrust) Unstated() []semconv.Field {
	return []semconv.Field{
		semconv.OperationName,
		semconv.UsageInputTokens,
		semconv.UsageOutputTokens,
		semconv.UsageCacheReadInputTokens,
		semconv.UsageReasoningOutputTokens,
		semconv.EvaluationName,
		semconv.EvaluationScoreValue,
		semconv.InputMessages,
		semconv.OutputMessages,
	}
}

// Interpretations are the readings Parse performs beyond the rule table: the
// lifts out of Braintrust's JSON blobs, and the two readings that turn on what
// else the span says.
func (braintrust) Interpretations() []Interpretation {
	in := []Interpretation{
		{Key: "braintrust.span_attributes#type", Meaning: semconv.OperationName, Values: braintrustSpanTypes},
		{Key: "braintrust.scores#name", When: braintrustOneScore, Meaning: semconv.EvaluationName},
		{Key: "braintrust.scores#value", When: braintrustOneScore, Meaning: semconv.EvaluationScoreValue},
	}
	for _, member := range slices.Sorted(maps.Keys(braintrustMetrics)) {
		in = append(in, Interpretation{Key: "braintrust.metrics#" + member, Meaning: braintrustMetrics[member]})
	}
	for _, m := range braintrustMessages {
		for _, key := range m.keys {
			in = append(in, Interpretation{Key: key, When: braintrustModelCall, Meaning: m.field})
		}
	}
	return in
}

func (d braintrust) Parse(s Span) Parsed {
	var p Parsed

	// Braintrust writes the conventions' own attributes where it has them, so
	// the gen_ai half is read first and the braintrust half only fills gaps.
	//
	// The order is load-bearing rather than tidy: spanAttributes, metrics and
	// messages all guard on `p.Fields[...]` already being set, so running the
	// rules after them would invert those precedences and let a JSON blob
	// overwrite the emitter's own conventions attribute.
	p.applyRules(s, d.Rules())

	typ := d.spanAttributes(s, &p)
	d.metrics(s, &p)
	d.scores(s, &p)
	d.messages(s, typ, &p)
	d.losses(s, &p)

	return p
}

// spanAttributes reads braintrust.span_attributes, a JSON object carrying the
// span's name and its type. The type is Braintrust's taxonomy and maps onto
// gen_ai.operation.name where the conventions have a word for it.
//
// The type is returned as well, because it is the only evidence on the span of
// what braintrust.input and braintrust.output hold. See messages.
func (braintrust) spanAttributes(s Span, p *Parsed) string {
	v, ok := s.Attr("braintrust.span_attributes")
	if !ok {
		return ""
	}
	p.Consumed = append(p.Consumed, "braintrust.span_attributes")

	var attrs struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(v.Str), &attrs); err != nil {
		p.Loss = append(p.Loss, Loss{Key: "braintrust.span_attributes",
			Reason: ReasonUnstructured, Detail: "value is not a JSON object"})
		return ""
	}
	if attrs.Type == "" {
		return ""
	}
	// An operation read from gen_ai.operation.name is the emitter's own
	// statement and outranks one inferred from the span taxonomy.
	if _, already := p.Fields[semconv.OperationName]; already {
		return attrs.Type
	}
	if op, ok := braintrustSpanTypes[attrs.Type]; ok {
		p.liftFrom(semconv.OperationName, String(op), "braintrust.span_attributes")
		return attrs.Type
	}
	p.Loss = append(p.Loss, Loss{Key: "braintrust.span_attributes", Reason: ReasonNoField,
		Detail: "no gen_ai.operation.name value for span type " + attrs.Type})
	return attrs.Type
}

// metrics lifts the token counts out of braintrust.metrics, a JSON object of
// numbers that also carries timings and cache statistics. Only the two counts
// have anywhere to go; the rest is named in one loss rather than one per key,
// because a reader wants to know the metrics object was not carried, not to
// read a list of its members.
func (braintrust) metrics(s Span, p *Parsed) {
	v, ok := s.Attr("braintrust.metrics")
	if !ok {
		return
	}
	p.Consumed = append(p.Consumed, "braintrust.metrics")

	var m map[string]any
	if err := json.Unmarshal([]byte(v.Str), &m); err != nil {
		p.Loss = append(p.Loss, Loss{Key: "braintrust.metrics",
			Reason: ReasonUnstructured, Detail: "value is not a JSON object"})
		return
	}

	var left []string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		f, ok := braintrustMetrics[k]
		if !ok {
			left = append(left, k)
			continue
		}
		// A count already read from a gen_ai attribute is the emitter speaking
		// the target's own language, and is not overwritten from the blob.
		if _, already := p.Fields[f]; already {
			continue
		}
		if val := jsonValue(m[k]); !val.Empty() {
			p.liftFrom(f, val, "braintrust.metrics")
		} else {
			left = append(left, k)
		}
	}
	if len(left) > 0 {
		p.Loss = append(p.Loss, Loss{Key: "braintrust.metrics", Reason: ReasonNoField,
			Detail: "no gen_ai field for " + strings.Join(left, ", ")})
	}
}

// scores is the interesting one. Braintrust carries a whole set of named
// scores on a span; the conventions carry exactly one evaluation, as a name and
// a value. So a span with a single score translates cleanly and a span with
// several does not translate at all: picking one of them would be inventing a
// verdict the emitter never reached, and averaging them would be worse.
func (braintrust) scores(s Span, p *Parsed) {
	v, ok := s.Attr("braintrust.scores")
	if !ok {
		return
	}
	p.Consumed = append(p.Consumed, "braintrust.scores")

	var scores map[string]any
	if err := json.Unmarshal([]byte(v.Str), &scores); err != nil {
		p.Loss = append(p.Loss, Loss{Key: "braintrust.scores",
			Reason: ReasonUnstructured, Detail: "value is not a JSON object"})
		return
	}
	names := slices.Sorted(maps.Keys(scores))
	switch len(names) {
	case 0:
		return
	case 1:
		value := jsonValue(scores[names[0]])
		if _, ok := value.Float64(); !ok {
			p.Loss = append(p.Loss, Loss{Key: "braintrust.scores", Reason: ReasonUnstructured,
				Detail: "score " + names[0] + " is not a number"})
			return
		}
		p.liftWhen(semconv.EvaluationName, String(names[0]), "braintrust.scores", braintrustOneScore)
		p.liftWhen(semconv.EvaluationScoreValue, value, "braintrust.scores", braintrustOneScore)
	default:
		p.Loss = append(p.Loss, Loss{Key: "braintrust.scores", Reason: ReasonFlattened,
			Detail: "the conventions carry one evaluation per span and this span has " +
				strings.Join(names, ", ")})
	}
}

// braintrustMessages names where Braintrust keeps a traced function's input and
// output: the _json spelling its OpenTelemetry integration documents, then the
// bare one. Output is the model's reply, which is the side that may arrive as
// OpenAI choices rather than as bare messages.
var braintrustMessages = []struct {
	field  semconv.Field
	keys   []string
	output bool
}{
	{semconv.InputMessages, []string{"braintrust.input_json", "braintrust.input"}, false},
	{semconv.OutputMessages, []string{"braintrust.output_json", "braintrust.output"}, true},
}

// messages carries input and output onto the conventions' message fields, and
// only on a span Braintrust itself typed llm.
//
// On any other span, input and output are whatever the traced function took and
// returned -- an order id, a dict of options, a string -- and reading a message
// list out of one would be a guess. On an llm span Braintrust has said what the
// function was, and what is logged there is the request's messages and the
// response's message in OpenAI's chat shape. That shape has a schema, so it is
// read strictly: every element has to be a chat message or nothing is carried,
// because half a conversation presented as the conversation is worse than the
// honest gap this used to record for every span.
//
// The emitter's own gen_ai.input.messages, read by the rules, outranks the blob,
// the same precedence metrics and spanAttributes keep.
func (braintrust) messages(s Span, typ string, p *Parsed) {
	for _, m := range braintrustMessages {
		attr := semconv.CanonicalKey(m.field)
		carried := ""
		for _, key := range m.keys {
			v, ok := s.Attr(key)
			if !ok {
				continue
			}
			_, stated := p.Fields[m.field]
			switch {
			case typ != "llm":
				p.Lose(key, ReasonUnstructured,
					"the traced function's own payload; only a span Braintrust typed llm is known to hold messages")
			case carried != "":
				p.Lose(key, ReasonUnstructured, attr+" is already carried from "+carried)
			case stated:
				p.Lose(key, ReasonUnstructured, "the span's own "+attr+" outranks this blob")
			default:
				msgs, gaps, ok := openAIChat(v.Str, m.output)
				if !ok {
					p.Lose(key, ReasonUnstructured,
						"an llm span, but not an OpenAI chat message list, so it is recorded rather than guessed at")
					continue
				}
				p.Consumed = append(p.Consumed, key)
				p.liftWhen(m.field, messagesValue(msgs), key, braintrustModelCall)
				if len(msgs) > 0 {
					carried = key
				}
				for _, g := range gaps {
					p.Loss = append(p.Loss, Loss{Key: fmt.Sprintf("%s[%d]", key, g.index),
						Reason: ReasonNoField, Detail: g.detail})
				}
			}
		}
	}
}

// losses records the rest of the Braintrust model. input and output are not
// listed here: messages decides what they are, and records them itself when
// they are not a message list.
func (d braintrust) losses(s Span, p *Parsed) {
	defer p.sweepResidue(s, d, "braintrust.", "gen_ai.")

	if s.Has("braintrust.context_json") {
		p.Lose("braintrust.context_json", ReasonUnstructured,
			"the traced function's own payload, with no schema to map onto")
	}
	for _, k := range []string{
		"braintrust.expected",
		"braintrust.expected_json",
		"braintrust.tags",
		"braintrust.metadata",
		"braintrust.parent",
		"braintrust.project_id",
		"braintrust.span_id",
		"braintrust.root_span_id",
	} {
		if s.Has(k) {
			p.Lose(k, ReasonNoField, "part of Braintrust's eval model, which the conventions do not have")
		}
	}
}

// chatGap is a piece of an otherwise readable message list that the
// conventions' message model has no place for, named by its message.
type chatGap struct {
	index  int
	detail string
}

type openAIFunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type openAIChatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"tool_call_id"`
	ToolCalls  []struct {
		ID       string             `json:"id"`
		Function openAIFunctionCall `json:"function"`
	} `json:"tool_calls"`
	FunctionCall *openAIFunctionCall `json:"function_call"`
}

// openAIChat reads a JSON array of OpenAI chat messages into the conventions'
// message model. With output set an element may instead be a choice -- a
// message under "message" beside its finish_reason -- which is how a completion
// lists what it returned.
//
// It reports false for anything that is not that shape rather than carrying the
// elements that happen to parse, so the caller can record the attribute whole.
// What it does report as gaps are pieces of a well-formed list with no home:
// content parts other than text, which this reader does not yet translate.
func openAIChat(raw string, output bool) ([]message, []chatGap, bool) {
	var elems []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &elems); err != nil {
		return nil, nil, false
	}

	msgs := make([]message, 0, len(elems))
	var gaps []chatGap
	for i, e := range elems {
		body, finish, ok := openAIChoice(e, output)
		if !ok {
			return nil, nil, false
		}
		var m openAIChatMessage
		if err := json.Unmarshal(body, &m); err != nil || m.Role == "" {
			return nil, nil, false
		}
		if m.Role == "tool" || m.Role == "function" {
			// A tool's result goes back to the model as a message of its own, and
			// the conventions' message schema has a part for exactly that at both
			// targets: tool_call_response, whose response is required. OpenAI's
			// older function role is the same act under its previous name.
			var response any = ""
			if len(m.Content) > 0 && string(m.Content) != "null" {
				if err := json.Unmarshal(m.Content, &response); err != nil {
					return nil, nil, false
				}
			}
			msgs = append(msgs, message{Role: "tool", Parts: []part{
				{Type: "tool_call_response", ID: m.ToolCallID, Response: response},
			}})
			continue
		}
		parts, partGaps, ok := openAIContent(m.Content)
		if !ok {
			return nil, nil, false
		}
		for _, g := range partGaps {
			gaps = append(gaps, chatGap{index: i, detail: g})
		}
		for _, c := range m.ToolCalls {
			parts = append(parts, openAIToolCallPart(c.ID, c.Function))
		}
		if m.FunctionCall != nil && m.FunctionCall.Name != "" {
			parts = append(parts, openAIToolCallPart("", *m.FunctionCall))
		}
		msgs = append(msgs, message{Role: m.Role, Parts: parts, FinishReason: finish})
	}
	return msgs, gaps, true
}

// openAIChoice unwraps a choice when output allows one, and otherwise hands the
// element back as the message it should be.
func openAIChoice(e json.RawMessage, output bool) (json.RawMessage, string, bool) {
	if !output {
		return e, "", true
	}
	var choice struct {
		Message      json.RawMessage `json:"message"`
		FinishReason *string         `json:"finish_reason"`
	}
	if err := json.Unmarshal(e, &choice); err != nil {
		return nil, "", false
	}
	if len(choice.Message) == 0 {
		return e, "", true
	}
	reason := ""
	if choice.FinishReason != nil {
		reason = finishReason(*choice.FinishReason)
	}
	return choice.Message, reason, true
}

// openAIContent reads a message's content, which OpenAI allows to be absent,
// a string, or a list of typed parts.
func openAIContent(raw json.RawMessage) ([]part, []string, bool) {
	parts := []part{}
	if len(raw) == 0 || string(raw) == "null" {
		return parts, nil, true
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if text != "" {
			parts = append(parts, textPart(text))
		}
		return parts, nil, true
	}

	var list []struct {
		Type string  `json:"type"`
		Text *string `json:"text"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, nil, false
	}
	var gaps []string
	for _, e := range list {
		switch {
		case e.Type == "":
			return nil, nil, false
		case e.Type == "text":
			if e.Text == nil {
				return nil, nil, false
			}
			parts = append(parts, textPart(*e.Text))
		default:
			gaps = append(gaps, "no message part type for "+e.Type)
		}
	}
	return parts, gaps, true
}

// openAIToolCallPart builds a tool_call part. OpenAI writes arguments as a
// JSON-encoded string, which toolCallPart re-parses; an object written directly
// is kept as the object.
func openAIToolCallPart(id string, f openAIFunctionCall) part {
	var s string
	if err := json.Unmarshal(f.Arguments, &s); err == nil {
		return toolCallPart(id, f.Name, s)
	}
	p := part{Type: "tool_call", ID: id, Name: f.Name}
	var args any
	if err := json.Unmarshal(f.Arguments, &args); err == nil && args != nil {
		p.Arguments = args
	}
	return p
}
