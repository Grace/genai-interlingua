// SPDX-License-Identifier: Apache-2.0

// Command gen writes internal/semconv/targets_gen.go from the upstream
// registries vendored under internal/semconv/testdata/upstream.
//
// The tables it replaces were transcribed by hand. That was defensible while
// there were two targets and one of them was frozen, and it stops being
// defensible the moment anything else in this repository claims to be derived
// from upstream -- a Weaver registry or a schema file emitted from a table
// somebody typed is a claim about upstream that upstream never made.
//
// What stays hand-authored is the editorial layer, and only that:
//
//   - which concepts this repository models at all, as the Field constants and
//     AllFields in target.go. Upstream defines attributes this repository has no
//     use for; declining to model one is a judgement, and it belongs in Go where
//     it can carry a comment.
//   - the handful of fields whose key is not mechanically derivable, in
//     keyOverrides below.
//
// Everything else -- whether a target defines a field, what it spells it, and
// what values it accepts -- is read from the registries. That is not a
// convenience. "Does v1.41.0 represent gen_ai.response.status?" has exactly one
// correct answer and it is in the vendored file; a second copy in Go is not a
// cross-check, it is a second thing that can be wrong.
//
// This program deliberately does not import the package it generates into. It
// reads target.go with go/ast instead, so that a broken or deleted
// targets_gen.go -- the state the tree is in whenever the generator is being
// changed -- does not stop the generator from running.
//
// Run it with `go generate ./internal/semconv/...`, and re-vendor first with
// testdata/upstream/refresh.sh.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// target names one schema this package renders into, and the identifiers its
// tables take in the generated file. The set is small and changes about once a
// year, so it is written out rather than parsed back out of target.go; the
// generated file is checked against Targets by TestGeneratedTablesCoverTargets.
type target struct {
	name     string // as it appears on the command line, and in the vendored filename
	keysVar  string
	enumsVar string
	doc      string // package-level commentary, emitted above keysVar
}

var targets = []target{
	{
		name:     "v1.41.0",
		keysVar:  "keysV1_41_0",
		enumsVar: "enumsV1_41_0",
		doc: `%s is model/gen-ai/registry.yaml at tag v1.41.0 of
open-telemetry/semantic-conventions -- the last tagged release whose gen_ai.*
definitions were live rather than deprecation shims. The fields modelled here
but missing from this map are the ones the new repository added after the split;
normalizing to this target drops them, and the normalizer says so.`,
	},
	{
		name:     "genai-main",
		keysVar:  "keysGenAIMain",
		enumsVar: "enumsGenAIMain",
		doc: `%s is model/gen-ai/registry.yaml on the main branch of
open-telemetry/semantic-conventions-genai, pinned to the commit named in
_source.ref of the vendored registry. Every attribute there is stability:
development; nothing in this schema is stable, including the question of where
the schema lives.`,
	},
}

// keyOverrides carries the fields whose attribute key is not "gen_ai." plus the
// field name under some target. There is currently exactly one, and it is the
// whole reason the mapping needs an escape hatch rather than a rule: the field
// is spelled cache_creation at v1.41.0 and cache_write on main. The definitions
// are otherwise identical, so it is one field with two spellings, not two
// fields, and normalizing to either target has to produce the right one.
//
// A new entry here should be rare enough to argue about. If upstream renames
// something, prefer re-pinning the Field constant to the new name and adding the
// old spelling here for the frozen target -- that keeps Field tracking main,
// which is what target.go promises.
var keyOverrides = map[string]map[string]string{
	"v1.41.0": {
		"usage.cache_write.input_tokens": "gen_ai.usage.cache_creation.input_tokens",
	},
}

// registry is the vendored shape written by testdata/upstream/to_json.py.
type registry struct {
	Source struct {
		Target string `json:"target"`
		Repo   string `json:"repo"`
		Ref    string `json:"ref"`
		URL    string `json:"url"`
	} `json:"_source"`
	Attributes map[string]struct {
		Members []string `json:"members"`
	} `json:"attributes"`
}

const (
	srcFile = "target.go"
	outFile = "targets_gen.go"
	prefix  = "gen_ai."
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "semconv/gen:", err)
		os.Exit(1)
	}
}

func run() error {
	fields, order, err := readFields(srcFile)
	if err != nil {
		return err
	}
	if len(order) == 0 {
		return fmt.Errorf("%s declares no AllFields; nothing to generate", srcFile)
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "// SPDX-License-Identifier: Apache-2.0\n\n")
	fmt.Fprintf(&buf, "// Code generated by internal/semconv/gen. DO NOT EDIT.\n//\n")
	fmt.Fprintf(&buf, "// Regenerate with `go generate ./internal/semconv/...` after re-vendoring\n")
	fmt.Fprintf(&buf, "// upstream with testdata/upstream/refresh.sh. The editorial layer -- which\n")
	fmt.Fprintf(&buf, "// concepts are modelled at all -- lives in %s, not here.\n\n", srcFile)
	fmt.Fprintf(&buf, "package semconv\n")

	for _, t := range targets {
		reg, err := readRegistry(t.name)
		if err != nil {
			return err
		}
		if err := emit(&buf, t, reg, fields, order); err != nil {
			return err
		}
	}

	src, err := format.Source(buf.Bytes())
	if err != nil {
		// Write the unformatted source anyway: a syntax error in generated code
		// is far easier to diagnose by reading the file than by reading the
		// parser's line number against a buffer nobody can see.
		os.WriteFile(outFile, buf.Bytes(), 0o644)
		return fmt.Errorf("generated source does not parse (written to %s anyway): %w", outFile, err)
	}
	if err := os.WriteFile(outFile, src, 0o644); err != nil {
		return err
	}
	fmt.Printf("semconv/gen: wrote %s (%d fields modelled, %d targets)\n", outFile, len(order), len(targets))
	return nil
}

// emit writes one target's key and enum tables, in AllFields order so the
// output is stable and reviewable as a diff.
func emit(buf *bytes.Buffer, t target, reg registry, fields map[string]string, order []string) error {
	overrides := keyOverrides[t.name]

	type row struct {
		ident   string
		key     string
		members []string
	}
	var rows []row
	for _, ident := range order {
		name, ok := fields[ident]
		if !ok {
			return fmt.Errorf("AllFields names %s, which is not a Field constant", ident)
		}
		key, ok := overrides[name]
		if !ok {
			key = prefix + name
		}
		attr, defined := reg.Attributes[key]
		if !defined {
			// Not an error: a field the target does not define is exactly the
			// case this repository exists to report. It is simply absent from
			// the key map, and Represents returns false for it.
			continue
		}
		rows = append(rows, row{ident: ident, key: key, members: dedupe(attr.Members)})
	}

	// An override that matched nothing is a stale override, and a silent one
	// would leave a field rendering under a key its target does not define.
	for name, key := range overrides {
		if _, ok := reg.Attributes[key]; !ok {
			return fmt.Errorf("%s: override maps %s to %q, which %s@%s does not define",
				t.name, name, key, reg.Source.Repo, reg.Source.Ref)
		}
	}

	fmt.Fprintf(buf, "\n%s", comment(fmt.Sprintf(t.doc, t.keysVar)))
	fmt.Fprintf(buf, "//\n// Source: %s@%s\n", reg.Source.Repo, reg.Source.Ref)
	fmt.Fprintf(buf, "// %s\n", reg.Source.URL)
	fmt.Fprintf(buf, "var %s = map[Field]string{\n", t.keysVar)
	for _, r := range rows {
		fmt.Fprintf(buf, "\t%s: %s,\n", r.ident, strconv.Quote(r.key))
	}
	fmt.Fprintf(buf, "}\n")

	fmt.Fprintf(buf, "\n// %s is the closed value set for every attribute above that has one.\n", t.enumsVar)
	fmt.Fprintf(buf, "// A field absent here is open-ended and accepts anything; a value legal\n")
	fmt.Fprintf(buf, "// under another target and absent from a set here is a loss the normalizer\n")
	fmt.Fprintf(buf, "// records, the same as a field with no key at all.\n")
	fmt.Fprintf(buf, "//\n// Members come from the registry's own type.members, deduplicated by value:\n")
	fmt.Fprintf(buf, "// upstream can define two members that differ only in id, such as the\n")
	fmt.Fprintf(buf, "// deprecated gen_ai.token.type member spelled completion whose value is output.\n")
	fmt.Fprintf(buf, "var %s = map[Field][]string{\n", t.enumsVar)
	for _, r := range rows {
		if len(r.members) == 0 {
			continue
		}
		fmt.Fprintf(buf, "\t%s: {\n", r.ident)
		for _, m := range r.members {
			fmt.Fprintf(buf, "\t\t%s,\n", strconv.Quote(m))
		}
		fmt.Fprintf(buf, "\t},\n")
	}
	fmt.Fprintf(buf, "}\n")
	return nil
}

// comment turns a multi-line doc string into a Go comment block. The target
// docs are written as prose so they stay readable in this file, which means the
// // has to be added on the way out rather than typed in.
func comment(s string) string {
	var out bytes.Buffer
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			out.WriteString("//\n")
			continue
		}
		out.WriteString("// " + line + "\n")
	}
	return out.String()
}

// readFields pulls the editorial layer out of target.go: every `Ident Field =
// "name"` constant, and the order AllFields lists them in.
func readFields(path string) (map[string]string, []string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}

	fields := make(map[string]string)
	var order []string

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			switch s := spec.(type) {
			case *ast.ValueSpec:
				if gen.Tok == token.CONST {
					ident, value, ok := fieldConst(s)
					if ok {
						fields[ident] = value
					}
					continue
				}
				if gen.Tok == token.VAR && len(s.Names) == 1 && s.Names[0].Name == "AllFields" {
					got, err := identList(s)
					if err != nil {
						return nil, nil, fmt.Errorf("%s: AllFields: %w", path, err)
					}
					order = got
				}
			}
		}
	}
	return fields, order, nil
}

// fieldConst matches `Name Field = "value"`, which is how every Field constant
// in target.go is written. A const of any other type, or one whose value is not
// a plain string literal, is not a field and is skipped rather than rejected --
// target.go is free to declare other constants.
func fieldConst(s *ast.ValueSpec) (ident, value string, ok bool) {
	typ, isIdent := s.Type.(*ast.Ident)
	if !isIdent || typ.Name != "Field" {
		return "", "", false
	}
	if len(s.Names) != 1 || len(s.Values) != 1 {
		return "", "", false
	}
	lit, isLit := s.Values[0].(*ast.BasicLit)
	if !isLit || lit.Kind != token.STRING {
		return "", "", false
	}
	unquoted, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", "", false
	}
	return s.Names[0].Name, unquoted, true
}

// identList reads the identifiers out of `var AllFields = []Field{A, B, C}`.
func identList(s *ast.ValueSpec) ([]string, error) {
	if len(s.Values) != 1 {
		return nil, fmt.Errorf("expected one composite literal")
	}
	lit, ok := s.Values[0].(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("expected a composite literal, got %T", s.Values[0])
	}
	out := make([]string, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		ident, ok := elt.(*ast.Ident)
		if !ok {
			return nil, fmt.Errorf("element %d is %T, want an identifier", len(out), elt)
		}
		out = append(out, ident.Name)
	}
	return out, nil
}

func readRegistry(name string) (registry, error) {
	path := filepath.Join("testdata", "upstream", name+".registry.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return registry{}, fmt.Errorf("read %s: %w (run testdata/upstream/refresh.sh)", path, err)
	}
	var reg registry
	if err := json.Unmarshal(b, &reg); err != nil {
		return registry{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if len(reg.Attributes) == 0 {
		return registry{}, fmt.Errorf("%s defines no attributes; the vendoring script is broken", path)
	}
	if reg.Source.Target != name {
		return registry{}, fmt.Errorf("%s says it is target %q, want %q", path, reg.Source.Target, name)
	}
	return reg, nil
}

// dedupe collapses members that share a value. to_json.py already sorts them,
// but it sorts values and upstream keys members by id, so two ids for one value
// arrive adjacent and identical.
func dedupe(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
