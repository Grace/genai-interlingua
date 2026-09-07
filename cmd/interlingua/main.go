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
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

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
