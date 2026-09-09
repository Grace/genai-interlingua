// SPDX-License-Identifier: Apache-2.0

package semconv

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// The tables in this package are generated from the registries vendored under
// testdata/upstream, by internal/semconv/gen. They were transcribed by hand
// first, which is a reasonable way to build them and a terrible way to keep
// them, because the whole argument this repository makes is that the target
// moves. A snapshot with nothing watching it is the one place the argument and
// the implementation disagree.
//
// Generating them does not make these checks redundant, it changes what they
// ask. Not "was the transcription right" any more, but: is targets_gen.go
// current with the registries sitting beside it, and does the generator's
// derivation rule -- the key is "gen_ai." plus the field name, give or take a
// declared override -- still hold against what upstream actually publishes.
// A re-vendor can break either one silently.
//
// The registries are converted to JSON on the way in: the core module has no
// dependencies, and a Go YAML parser would appear in go.mod even as a test-only
// import.
//
// Refresh them with testdata/upstream/refresh.sh, then re-run `go generate
// ./internal/semconv/...`. genai-main is pinned to a commit on purpose --
// comparing against a floating fetch would agree with upstream by construction
// and detect nothing.

type upstreamRegistry struct {
	Source struct {
		Target string `json:"target"`
		Repo   string `json:"repo"`
		Ref    string `json:"ref"`
	} `json:"_source"`
	Attributes map[string]struct {
		Members []string `json:"members"`
	} `json:"attributes"`
}

func loadUpstream(t *testing.T, target Target) upstreamRegistry {
	t.Helper()
	path := filepath.Join("testdata", "upstream", string(target)+".registry.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (run testdata/upstream/refresh.sh)", path, err)
	}
	var r upstreamRegistry
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if len(r.Attributes) == 0 {
		t.Fatalf("%s defines no attributes; the vendoring script is broken", path)
	}
	return r
}

// TestSchemaMatchesUpstream holds this package's tables to the registries they
// were generated from. For v1.41.0, which is frozen, it can now only fail if
// targets_gen.go was hand-edited or the generator is wrong. For genai-main it
// also catches upstream moving under a stale generated file.
func TestSchemaMatchesUpstream(t *testing.T) {
	for _, target := range Targets {
		t.Run(string(target), func(t *testing.T) {
			up := loadUpstream(t, target)
			t.Logf("checked against %s@%s (%d attributes upstream)",
				up.Source.Repo, up.Source.Ref, len(up.Attributes))

			// Every key this package claims a target defines must actually
			// exist in that target's registry. A key that does not is a
			// transcription error, and it would silently produce spans
			// carrying an attribute name nothing downstream recognizes.
			for field, key := range keys[target] {
				if _, ok := up.Attributes[key]; !ok {
					t.Errorf("%s renders %s as %q, which %s does not define",
						target, field, key, up.Source.Ref)
				}
			}

			// Enum members are the likeliest place for a hand-typed list to be
			// wrong or to fall behind, and getting them wrong has teeth: an
			// accepted value that upstream dropped produces non-conformant
			// spans, and a missing one makes the normalizer discard data that
			// was valid.
			for field, mine := range enums[target] {
				key, ok := target.Key(field)
				if !ok {
					t.Errorf("%s has an enum for %s but no key for it", target, field)
					continue
				}
				theirs := up.Attributes[key].Members
				if len(theirs) == 0 {
					t.Errorf("%s treats %s as a closed set, but %s defines no members for it",
						target, key, up.Source.Ref)
					continue
				}
				if d := diff(sorted(mine), theirs); d != "" {
					t.Errorf("%s enum for %s disagrees with %s:\n%s",
						target, key, up.Source.Ref, d)
				}
			}
		})
	}
}

// TestUpstreamAttributesNotModelled reports rather than fails. Choosing not to
// model an attribute is legitimate editorial judgement -- this repository maps
// what emitters actually produce, not everything the registry defines. Not
// *knowing* about one is the problem, and this is the list to read after
// refresh.sh pulls a newer upstream.
func TestUpstreamAttributesNotModelled(t *testing.T) {
	for _, target := range Targets {
		up := loadUpstream(t, target)
		modelled := make(map[string]bool, len(keys[target]))
		for _, key := range keys[target] {
			modelled[key] = true
		}
		var missing []string
		for key := range up.Attributes {
			if !modelled[key] {
				missing = append(missing, key)
			}
		}
		sort.Strings(missing)
		if len(missing) == 0 {
			t.Logf("%s: every upstream attribute is modelled", target)
			continue
		}
		t.Logf("%s: %d upstream attributes not modelled here:", target, len(missing))
		for _, key := range missing {
			t.Logf("    %s", key)
		}
	}
}

// TestGeneratedTablesAreCurrent is the drift gate that TestSchemaMatchesUpstream
// cannot be. That test walks the tables and asks whether upstream agrees; this
// one walks upstream and asks whether the tables are complete, which is the
// direction a stale targets_gen.go actually fails in.
//
// The failure it exists for: someone adds a Field constant and an AllFields
// entry for an attribute upstream already defines, does not re-run go generate,
// and the field silently never renders on any span. Nothing else in the suite
// notices, because every table entry that does exist is still correct.
func TestGeneratedTablesAreCurrent(t *testing.T) {
	for _, target := range Targets {
		t.Run(string(target), func(t *testing.T) {
			up := loadUpstream(t, target)
			for _, f := range AllFields {
				if target.Represents(f) {
					continue
				}
				// The field is not in this target's table. That is only
				// correct if the target genuinely has no attribute for it.
				// Fields carried under a different key at this target are
				// represented, so they never reach here and the generator's
				// override table does not need restating.
				if _, defined := up.Attributes["gen_ai."+string(f)]; defined {
					t.Errorf("%s defines gen_ai.%s and AllFields models it, but %s has no key for it"+
						"\n    targets_gen.go is stale: run go generate ./internal/semconv/...",
						up.Source.Ref, f, target)
				}
			}
		})
	}
}

// TestEveryTargetHasGeneratedTables catches the other half of a stale
// generation: a target added to Targets whose tables were never emitted. Without
// this, such a target silently represents nothing and normalizes every span to
// an empty schema.
func TestEveryTargetHasGeneratedTables(t *testing.T) {
	for _, target := range Targets {
		if len(keys[target]) == 0 {
			t.Errorf("%s has no generated key table; add it to the targets list in internal/semconv/gen", target)
		}
	}
	if got, want := len(keys), len(Targets); got != want {
		t.Errorf("%d generated key tables, %d targets", got, want)
	}
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// diff reports members present on one side only, which is more readable in a
// failure than two long sorted lists the reader has to compare by eye.
func diff(mine, theirs []string) string {
	in := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}
	var out string
	for _, m := range mine {
		if !in(theirs, m) {
			out += "    we accept, upstream does not define: " + m + "\n"
		}
	}
	for _, o := range theirs {
		if !in(mine, o) {
			out += "    upstream defines, we do not accept: " + o + "\n"
		}
	}
	return out
}
