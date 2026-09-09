// SPDX-License-Identifier: Apache-2.0

// Command interlingua normalizes GenAI spans on stdin into one chosen version of
// the GenAI semantic conventions on stdout.
//
// It reads an OTLP/JSON trace export request, detects which instrumentation
// library produced each span, rewrites the ones it recognizes, and leaves the
// ones it does not exactly as they arrived. It is the same code the Collector
// processor runs, in a form a reader can point at a captured payload without
// building a collector:
//
//	interlingua -target v1.41.0 < testdata/openllmetry/in.json
//
// It also exports the half of its mapping that can be stated as data, as an
// OTTL config for the Collector's own transform processor:
//
//	interlingua -emit ottl -dialect litellm -target v1.41.0
//
// which is a normalization you can run without this binary at all. The emitted
// config says in its header exactly which mappings it could not carry.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/emit/ottl"
	"github.com/Grace/genai-interlingua/internal/emit/schemafile"
	"github.com/Grace/genai-interlingua/internal/normalize"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

// version is stamped by the release build through -ldflags. A binary built any
// other way reports what the module system knows instead, which for `go install
// ...@v1.2.3` is that tag and for a local build is "(devel)". Reporting the
// second rather than an empty string matters here: this tool's whole subject is
// which version of a moving schema you are looking at, and a normalizer that
// cannot say which version of itself produced a span would be a poor advert for
// the argument.
var version string

func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "unknown"
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "interlingua:", err)
		os.Exit(1)
	}
}

func run() error {
	target := flag.String("target", semconv.DefaultTarget.String(),
		fmt.Sprintf("schema version to normalize to, one of %v", semconv.Targets))
	// The default is to keep the emitter's own attributes. Stripping them makes
	// the output smaller and the normalizer the last reader of its input, which
	// is a trade worth making only once you trust the mapping.
	strip := flag.Bool("strip-original", false,
		"remove the source attributes the dialect consumed")
	// -emit turns the tool inside out: instead of normalizing spans, it prints
	// the mapping in a form something else can run. There is no detection in an
	// exported config -- see internal/emit/ottl -- so a dialect has to be named.
	emit := flag.String("emit", "", "print the mapping instead of normalizing; one of: ottl, schema-file")
	dialectName := flag.String("dialect", "", "with -emit, the emitter to export the mapping for")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		// The default target goes out beside the version because they are two
		// halves of the same answer: knowing which binary you ran tells you
		// nothing unless you also know which schema it renders by default.
		fmt.Printf("interlingua %s (default target %s)\n", buildVersion(), semconv.DefaultTarget)
		return nil
	}

	t, err := semconv.ParseTarget(*target)
	if err != nil {
		return err
	}

	if *emit != "" {
		return emitMapping(*emit, *dialectName, t)
	}
	if *dialectName != "" {
		return fmt.Errorf("-dialect only applies with -emit; normalization detects the dialect per span")
	}

	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	opts := normalize.DefaultOptions()
	opts.Target = t
	opts.PreserveOriginal = !*strip

	out, err := normalize.Payload(in, opts)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(out)
	return err
}

// emitMapping prints the mapping rather than applying it.
func emitMapping(format, name string, t semconv.Target) error {
	if format != "ottl" && format != "schema-file" {
		return fmt.Errorf("unknown -emit format %q, want one of: ottl, schema-file", format)
	}
	if name == "" {
		return fmt.Errorf("-emit needs -dialect: an exported config cannot detect, so it is pinned to one emitter (exportable: %s)",
			strings.Join(exportable(), ", "))
	}

	for _, d := range dialect.Dialects() {
		if string(d.Name()) != name {
			continue
		}
		ruled, ok := d.(dialect.Ruled)
		if !ok {
			// A real distinction, not a missing feature. This dialect's
			// mappings are performed rather than declared, so there is nothing
			// to export yet; emitting a partial config that looked complete
			// would be the failure this repository is about.
			return fmt.Errorf("%s does not declare its mappings as data yet, so it cannot be exported (exportable: %s)",
				name, strings.Join(exportable(), ", "))
		}
		var out []byte
		switch format {
		case "ottl":
			res, err := ottl.Config(ottl.Options{Dialect: ruled, Target: t})
			if err != nil {
				return err
			}
			out = res.YAML
		case "schema-file":
			res, err := schemafile.Render(schemafile.Options{Dialect: ruled, Target: t})
			if err != nil {
				return err
			}
			out = res.YAML
		}
		_, err := os.Stdout.Write(out)
		return err
	}
	return fmt.Errorf("unknown dialect %q (exportable: %s)", name, strings.Join(exportable(), ", "))
}

// exportable lists the dialects that declare their mappings as data. It is in
// every error above because "which ones can I export" is the question the user
// is actually asking whenever one of them fires.
func exportable() []string {
	var out []string
	for _, d := range dialect.Dialects() {
		if _, ok := d.(dialect.Ruled); ok {
			out = append(out, string(d.Name()))
		}
	}
	if len(out) == 0 {
		return []string{"none yet"}
	}
	return out
}
