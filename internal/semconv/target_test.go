package semconv

import "testing"

func TestGenAIMainRepresentsEveryField(t *testing.T) {
	for _, f := range AllFields {
		if !TargetGenAIMain.Represents(f) {
			t.Errorf("genai-main cannot represent %s, but AllFields is derived from it", f)
		}
	}
}

func TestV1_41_0DropsTheFieldsAddedAfterTheSplit(t *testing.T) {
	represented := 0
	for _, f := range AllFields {
		if TargetV1_41_0.Represents(f) {
			represented++
		}
	}
	got, want := represented, 50
	if got != want {
		t.Errorf("v1.41.0 represents %d of %d fields, want %d", got, len(AllFields), want)
	}
	if got, want := len(AllFields)-represented, 22; got != want {
		t.Errorf("%d fields are unrepresentable in v1.41.0, want %d", got, want)
	}
}

func TestCacheWriteIsOneFieldWithTwoSpellings(t *testing.T) {
	main, ok := TargetGenAIMain.Key(UsageCacheWriteInputTokens)
	if !ok {
		t.Fatal("genai-main does not represent the cache write field")
	}
	old, ok := TargetV1_41_0.Key(UsageCacheWriteInputTokens)
	if !ok {
		t.Fatal("v1.41.0 does not represent the cache write field")
	}
	if got, want := main, "gen_ai.usage.cache_write.input_tokens"; got != want {
		t.Errorf("genai-main key = %q, want %q", got, want)
	}
	if got, want := old, "gen_ai.usage.cache_creation.input_tokens"; got != want {
		t.Errorf("v1.41.0 key = %q, want %q", got, want)
	}
}

func TestCacheWriteIsTheOnlyRename(t *testing.T) {
	var renamed []Field
	for _, f := range AllFields {
		old, ok := TargetV1_41_0.Key(f)
		if !ok {
			continue
		}
		main, _ := TargetGenAIMain.Key(f)
		if old != main {
			renamed = append(renamed, f)
		}
	}
	if got, want := len(renamed), 1; got != want {
		t.Fatalf("%d fields are spelled differently across targets (%v), want %d", got, renamed, want)
	}
	if got, want := renamed[0], UsageCacheWriteInputTokens; got != want {
		t.Errorf("renamed field = %s, want %s", got, want)
	}
}

func TestParseTarget(t *testing.T) {
	for _, want := range Targets {
		got, err := ParseTarget(string(want))
		if err != nil {
			t.Errorf("ParseTarget(%q) returned %v", want, err)
			continue
		}
		if got != want {
			t.Errorf("ParseTarget(%q) = %q, want %q", want, got, want)
		}
	}
	if _, err := ParseTarget("v1.42.0"); err == nil {
		t.Error("ParseTarget accepted v1.42.0, which ships no gen_ai attributes at all")
	}
	if _, err := ParseTarget(""); err == nil {
		t.Error("ParseTarget accepted the empty string")
	}
}

func TestDefaultTargetIsPinnable(t *testing.T) {
	if got, want := DefaultTarget, TargetV1_41_0; got != want {
		t.Errorf("DefaultTarget = %q, want %q: genai-main has no tag to pin", got, want)
	}
}

func TestNewOperationsAreUnrepresentableInTheFrozenCut(t *testing.T) {
	added := map[string]bool{
		"fetch_response":      true,
		"plan":                true,
		"search_memory":       true,
		"create_memory":       true,
		"update_memory":       true,
		"upsert_memory":       true,
		"delete_memory":       true,
		"create_memory_store": true,
		"delete_memory_store": true,
	}
	main, ok := TargetGenAIMain.EnumValues(OperationName)
	if !ok {
		t.Fatal("gen_ai.operation.name has no value set under genai-main")
	}
	for _, v := range main {
		if got, want := TargetV1_41_0.Accepts(OperationName, v), !added[v]; got != want {
			t.Errorf("v1.41.0 accepts operation %q = %v, want %v", v, got, want)
		}
	}
	if got, want := len(main)-len(added), 9; got != want {
		t.Errorf("%d operations are shared across targets, want %d", got, want)
	}
}

func TestMoonshotAIIsTheOnlyNewProvider(t *testing.T) {
	main, _ := TargetGenAIMain.EnumValues(ProviderName)
	var added []string
	for _, v := range main {
		if !TargetV1_41_0.Accepts(ProviderName, v) {
			added = append(added, v)
		}
	}
	if got, want := len(added), 1; got != want {
		t.Fatalf("providers added after the split = %v, want exactly %d", added, want)
	}
	if got, want := added[0], "moonshot_ai"; got != want {
		t.Errorf("added provider = %q, want %q", got, want)
	}
}

func TestOutputTypeDidNotDrift(t *testing.T) {
	main, _ := TargetGenAIMain.EnumValues(OutputType)
	old, _ := TargetV1_41_0.EnumValues(OutputType)
	if got, want := len(main), len(old); got != want {
		t.Fatalf("gen_ai.output.type has %d values under genai-main, %d under v1.41.0", got, want)
	}
	for i, v := range main {
		if got, want := old[i], v; got != want {
			t.Errorf("gen_ai.output.type value %d = %q under v1.41.0, want %q", i, got, want)
		}
	}
}

// The v1.41.0 registry lists three token type members, two of which carry the
// value "output": one is a deprecated alias spelled completion. The value set is
// two, and an emitter written against the old member id still lands on "output".
func TestTokenTypeDeduplicatesTheDeprecatedAlias(t *testing.T) {
	old, ok := TargetV1_41_0.EnumValues(TokenType)
	if !ok {
		t.Fatal("gen_ai.token.type has no value set under v1.41.0")
	}
	if got, want := len(old), 2; got != want {
		t.Errorf("gen_ai.token.type has %d values, want %d: %v", got, want, old)
	}
	if TargetV1_41_0.Accepts(TokenType, "completion") {
		t.Error("v1.41.0 accepts token type \"completion\", which is a member id, not a value")
	}
}

func TestResponseStatusArrivedAfterTheSplit(t *testing.T) {
	if TargetV1_41_0.Represents(ResponseStatus) {
		t.Error("v1.41.0 represents gen_ai.response.status, which the split added")
	}
	if _, ok := TargetV1_41_0.EnumValues(ResponseStatus); ok {
		t.Error("v1.41.0 has a value set for a field it cannot represent")
	}
	if _, ok := TargetGenAIMain.EnumValues(ResponseStatus); !ok {
		t.Error("genai-main has no value set for gen_ai.response.status")
	}
}

// Accepts is about values, not existence: a field with no closed set accepts
// anything, so callers have to ask Represents separately.
func TestAcceptsIsOpenForNonEnumFields(t *testing.T) {
	if !TargetV1_41_0.Accepts(RequestModel, "gpt-4o-mini-2026-04-01") {
		t.Error("Accepts rejected a value for gen_ai.request.model, which has no closed value set")
	}
	if _, ok := TargetV1_41_0.EnumValues(RequestModel); ok {
		t.Error("gen_ai.request.model reports a closed value set")
	}
}

func TestEnumFieldsAreRepresentable(t *testing.T) {
	for _, target := range Targets {
		for f := range enums[target] {
			if !target.Represents(f) {
				t.Errorf("%s has a value set for %s but no key for it", target, f)
			}
		}
	}
}
