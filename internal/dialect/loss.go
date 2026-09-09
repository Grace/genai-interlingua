// SPDX-License-Identifier: Apache-2.0

package dialect

// Reason is why a source attribute did not survive into the IR. The set is
// closed and small on purpose: COMPATIBILITY.md is organized by these codes, and
// a reason that cannot be explained in one sentence is not a reason.
type Reason string

const (
	// ReasonNoField means the emitter said something the GenAI conventions have
	// no concept for, at any version. Traceloop's workflow names and
	// Braintrust's evaluation scores are the recurring examples.
	ReasonNoField Reason = "no_field"

	// ReasonUnstructured means the value is a provider-shaped blob the dialect
	// declined to parse, such as LiteLLM's llm.{provider}.* raw payloads.
	ReasonUnstructured Reason = "unstructured"

	// ReasonFlattened means indexed source attributes were collapsed into a
	// single field, losing the per-index structure. Message and tool-call arrays
	// arrive this way from every emitter that predates gen_ai.input.messages.
	ReasonFlattened Reason = "flattened"

	// ReasonCoerced means the value survived but its type did not, most often a
	// number or boolean that the emitter wrote as a string.
	ReasonCoerced Reason = "coerced"

	// ReasonAmbiguous means the source carried a value the dialect could not map
	// onto a single field with confidence, and chose to record rather than guess.
	ReasonAmbiguous Reason = "ambiguous"
)

// Loss is one source attribute that did not make it, and why.
type Loss struct {
	Key    string
	Reason Reason
	Detail string
}

// LossKeys returns the source attribute keys in a loss list. This is the value
// written to interlingua.lossy.
func LossKeys(losses []Loss) []string {
	if len(losses) == 0 {
		return nil
	}
	keys := make([]string, 0, len(losses))
	for _, l := range losses {
		keys = append(keys, l.Key)
	}
	return keys
}
