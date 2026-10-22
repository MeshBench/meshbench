package gpu

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/rf/propagation"
)

// Register entry points, not files: coverage.wgsl contains two kernels.
// Function references make deleting or renaming either half a compile error.
// This same registry runs the comparisons, so an entry cannot name a test
// that Go never executes. Only the comparisons require a device.
type kernelTwin struct {
	cpu     any
	compare func(*testing.T)
}

var kernelTwins = map[string]kernelTwin{
	"dechirp.wgsl:main": {dsp.DechirpCPU, compareDechirpCPU},
	"demod.wgsl:main":   {dsp.Demodulator.DemodulateSymbolInto, compareDemodCPU},
	"coverage.wgsl:main": {propagation.GridLossCPU,
		compareCoverageCPU},
	"coverage.wgsl:fold": {propagation.FoldStationCPU,
		compareCoverageFoldCPU},
	"pairs.wgsl:main": {propagation.ProfilePairLossCPU, comparePairLossCPU},
}

func TestEveryKernelHasACPUTwin(t *testing.T) {
	if err := checkKernelTwins(os.DirFS("."), kernelTwins); err != nil {
		t.Fatal(err)
	}
}

func TestKernelTwins(t *testing.T) {
	if err := checkKernelTwins(os.DirFS("."), kernelTwins); err != nil {
		t.Fatal(err)
	}
	for _, name := range slices.Sorted(maps.Keys(kernelTwins)) {
		t.Run(name, kernelTwins[name].compare)
	}
}

func comparisonDevice(t testing.TB) *Device {
	t.Helper()
	d, err := Open()
	if err != nil {
		var unavailable unavailableError
		if errors.As(err, &unavailable) {
			t.Skipf("no GPU available: %v", err)
		}
		t.Fatalf("GPU kernel setup failed: %v", err)
	}
	t.Cleanup(d.Close)
	return d
}

// Read the source directly, including files not yet embedded or built. Like
// internal/layers_test.go, this works without a subprocess whose reads would
// be invisible to the test cache. A new file or a second entry point must both
// acquire a twin; a stale registration fails in the other direction too.
func checkKernelTwins(source fs.FS, twins map[string]kernelTwin) error {
	var problems []error
	found := map[string]bool{}
	err := fs.WalkDir(source, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".wgsl") {
			return nil
		}
		b, err := fs.ReadFile(source, path)
		if err != nil {
			return err
		}
		code := wgslWithoutComments(string(b))
		entries := computeEntry.FindAllStringSubmatch(code, -1)
		if len(entries) == 0 || len(entries) != len(computeAttribute.FindAllStringIndex(code, -1)) {
			problems = append(problems, fmt.Errorf("%s: cannot identify every compute entry point", path))
		}
		for _, entry := range entries {
			name := path + ":" + entry[1]
			if found[name] {
				problems = append(problems, fmt.Errorf("%s: duplicate compute entry point", name))
			}
			found[name] = true
			if _, ok := twins[name]; !ok {
				problems = append(problems, fmt.Errorf("%s has no registered CPU twin and comparison", name))
			}
		}
		return nil
	})
	if err != nil {
		problems = append(problems, err)
	}
	if len(found) == 0 {
		problems = append(problems, errors.New("no GPU compute entry points found; has the source moved?"))
	}
	for _, name := range slices.Sorted(maps.Keys(twins)) {
		twin := twins[name]
		if !found[name] {
			problems = append(problems, fmt.Errorf("%s: registration has no kernel", name))
		}
		cpu := reflect.ValueOf(twin.cpu)
		if !cpu.IsValid() || cpu.Kind() != reflect.Func || cpu.IsNil() {
			problems = append(problems, fmt.Errorf("%s: CPU twin must be a non-nil function", name))
		}
		if twin.compare == nil {
			problems = append(problems, fmt.Errorf("%s: missing comparison test", name))
		}
	}
	return errors.Join(problems...)
}

var (
	computeAttribute = regexp.MustCompile(`@\s*compute\b`)
	computeEntry     = regexp.MustCompile(`@\s*compute\b[^{};]*\bfn\s+([A-Za-z_]\w*)\s*\(`)
)

// WGSL permits nested block comments. Replace comments with whitespace so a
// commented-out kernel is not counted and adjacent tokens cannot be joined.
func wgslWithoutComments(source string) string {
	b := []byte(source)
	depth, line := 0, false
	for i := 0; i < len(b); i++ {
		pair := source[i:min(i+2, len(source))]
		switch {
		case line:
			line = b[i] != '\n'
		case pair == "/*":
			depth++
			b[i], b[i+1] = ' ', ' '
			i++
		case depth > 0 && pair == "*/":
			depth--
			b[i], b[i+1] = ' ', ' '
			i++
		case depth == 0 && pair == "//":
			line = true
		}
		if (line || depth > 0) && b[i] != '\n' {
			b[i] = ' '
		}
	}
	return string(b)
}
