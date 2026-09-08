package emit

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/Grace/genai-interlingua/internal/emit/ottl"
	"github.com/Grace/genai-interlingua/internal/normalize"
)

const registryModel = "../../registry/model/translation.yaml"

// TestRegistryDefinesEveryAttributeWeWrite holds the published registry to the
// code.
//
// registry/ is a Weaver semantic convention registry, which means it is a
// promise to people who are not running this code: these attributes exist, here
// is what they mean, resolve it and depend on it. An attribute this repository
// writes and the registry does not define is a promise quietly broken -- there
// is telemetry in the wild that nothing can look up. The reverse is worse in a
// subtler way: a registry defining an attribute nothing emits is documentation
// of a feature that does not exist.
//
// The file is read as text rather than parsed. The core module has no
// dependencies and a Go YAML parser would appear in go.mod even as a test
// import, which is the same trade internal/semconv makes for the vendored
// upstream registries.
func TestRegistryDefinesEveryAttributeWeWrite(t *testing.T) {
	b, err := os.ReadFile(registryModel)
	if err != nil {
		t.Fatalf("read %s: %v", registryModel, err)
	}

	defined := map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		key, ok := strings.CutPrefix(line, "- key: ")
		if !ok {
			continue
		}
		defined[strings.TrimSpace(key)] = true
	}
	if len(defined) == 0 {
		t.Fatalf("%s defines no attributes; the scan is broken, not the registry", registryModel)
	}

	written := []string{
		normalize.AttrDialect,
		normalize.AttrConfidence,
		normalize.AttrTarget,
		normalize.AttrLossy,
		normalize.AttrLossyCount,
		ottl.AttrExport,
		ottl.AttrExportUnsupported,
		ottl.AttrExportPartial,
	}

	for _, key := range written {
		if !defined[key] {
			t.Errorf("this repository writes %s, and registry/model/translation.yaml does not define it"+
				"\n    anything resolving the published registry cannot look the attribute up", key)
		}
		delete(defined, key)
	}

	var orphans []string
	for key := range defined {
		orphans = append(orphans, key)
	}
	sort.Strings(orphans)
	for _, key := range orphans {
		t.Errorf("registry/model/translation.yaml defines %s, which nothing here writes"+
			"\n    a registry entry with no emitter is documentation of a feature that does not exist", key)
	}
}
