package normalize

// Reason is why a parsed field did not survive rendering into the target schema.
// It is a different question from dialect.Reason, which is why a source attribute
// did not survive being parsed, and the two are kept apart on purpose: one is the
// emitter's fault and the other is the target version's.
type Reason string

const (
	// ReasonNoAttribute means the target schema names no attribute for this
	// field. Normalizing to v1.41.0 loses every field the new repository added
	// after the split this way.
	ReasonNoAttribute Reason = "no_attribute"

	// ReasonNoValue means the target names the attribute but not this value.
	// gen_ai.operation.name exists at v1.41.0; search_memory does not.
	ReasonNoValue Reason = "no_value"
)

// Loss is one field that could not be rendered into the target, named by the
// attribute key it would have taken under the newest target that defines it.
type Loss struct {
	Key    string
	Reason Reason
	Detail string
}
