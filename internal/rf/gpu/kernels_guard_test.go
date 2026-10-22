package gpu

import (
	"strings"
	"testing"
	"testing/fstest"
)

// Prove that the guard fails for additions and omissions without a GPU, and
// without modifying real shaders or relying on a network or a compiler.
func TestKernelTwinGuardRejectsMissingCoverage(t *testing.T) {
	const kernel = "@compute @workgroup_size(64) fn main() {}"
	valid := kernelTwin{cpu: func() {}, compare: func(*testing.T) {}}
	var nilCPU func()
	for _, tc := range []struct {
		name, source string
		twins        map[string]kernelTwin
		want         string
	}{
		{"registered", kernel, map[string]kernelTwin{"test.wgsl:main": valid}, ""},
		{"new kernel", kernel, nil, "test.wgsl:main has no registered CPU twin"},
		{"second entry", kernel + "\n@compute @workgroup_size(64) fn fold() {}",
			map[string]kernelTwin{"test.wgsl:main": valid}, "test.wgsl:fold has no registered CPU twin"},
		{"deleted kernel", "", map[string]kernelTwin{"test.wgsl:main": valid}, "registration has no kernel"},
		{"missing CPU", kernel, map[string]kernelTwin{"test.wgsl:main": {compare: valid.compare}}, "CPU twin must be"},
		{"typed nil CPU", kernel, map[string]kernelTwin{"test.wgsl:main": {nilCPU, valid.compare}}, "CPU twin must be"},
		{"non-function CPU", kernel, map[string]kernelTwin{"test.wgsl:main": {42, valid.compare}}, "CPU twin must be"},
		{"missing test", kernel, map[string]kernelTwin{"test.wgsl:main": {cpu: valid.cpu}}, "missing comparison test"},
		{"unrecognised entry", "@compute var broken;", nil, "cannot identify every compute entry point"},
		{"duplicate entry", kernel + kernel, map[string]kernelTwin{"test.wgsl:main": valid}, "duplicate compute entry point"},
		{"commented entry", "/* @compute fn fake() {} */\n" + kernel,
			map[string]kernelTwin{"test.wgsl:main": valid}, ""},
		{"nested comments", "/* outer /* @compute fn fake() {} */ still a comment */\n" + kernel,
			map[string]kernelTwin{"test.wgsl:main": valid}, ""},
		{"line comment", "// @compute fn fake() {}\n" + kernel,
			map[string]kernelTwin{"test.wgsl:main": valid}, ""},
		{"attribute order", "@workgroup_size(64) @compute\nfn main() {}",
			map[string]kernelTwin{"test.wgsl:main": valid}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := fstest.MapFS{"test.wgsl": {Data: []byte(tc.source)}}
			err := checkKernelTwins(source, tc.twins)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestKernelTwinGuardWalksNewSourceFiles(t *testing.T) {
	source := fstest.MapFS{"new/kernel.wgsl": {Data: []byte("@compute fn extra() {}")}}
	if err := checkKernelTwins(source, kernelTwins); err == nil ||
		!strings.Contains(err.Error(), "new/kernel.wgsl:extra has no registered CPU twin") {
		t.Fatalf("an unembedded shader in a new directory must fail: %v", err)
	}
	if err := checkKernelTwins(fstest.MapFS{}, nil); err == nil {
		t.Fatal("an empty source tree must fail")
	}
}
