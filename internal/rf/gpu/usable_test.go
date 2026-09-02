package gpu

import (
	"strings"
	"testing"

	"github.com/cogentcore/webgpu/wgpu"
)

// A software rasteriser is refused, and every real GPU is not.
//
// This is the check that keeps the claim in .github/workflows/ci.yml true. A
// runner with Mesa installed and no hardware still presents llvmpipe as an
// OpenGL adapter, so "there is no GPU here" is not the same statement as
// "gpu.Open will fail here", and the second one is the one CI relies on.
func TestSoftwareAdaptersAreRefused(t *testing.T) {
	for _, tc := range []struct {
		name    string
		info    wgpu.AdapterInfo
		refused bool
	}{
		{"llvmpipe", wgpu.AdapterInfo{
			Name: "llvmpipe", AdapterType: wgpu.AdapterTypeCPU,
			BackendType: wgpu.BackendTypeOpenGL,
		}, true},
		{"lavapipe", wgpu.AdapterInfo{
			Name: "llvmpipe (LLVM 17)", AdapterType: wgpu.AdapterTypeCPU,
			BackendType: wgpu.BackendTypeVulkan,
		}, true},
		// The adapter Mesa reports with no name at all, which is what a
		// surfaceless OpenGL context on a headless runner answers with.
		{"nameless", wgpu.AdapterInfo{
			AdapterType: wgpu.AdapterTypeCPU, BackendType: wgpu.BackendTypeOpenGL,
		}, true},
		{"discrete", wgpu.AdapterInfo{
			Name: "AMD Radeon RX 5700 XT", AdapterType: wgpu.AdapterTypeDiscreteGPU,
			BackendType: wgpu.BackendTypeVulkan,
		}, false},
		{"integrated", wgpu.AdapterInfo{
			Name: "Intel UHD Graphics", AdapterType: wgpu.AdapterTypeIntegratedGPU,
			BackendType: wgpu.BackendTypeVulkan,
		}, false},
		// Unknown is not refused: a driver that declines to classify itself is
		// still hardware more often than not, and the self-check is what
		// decides whether its answers can be trusted.
		{"unknown", wgpu.AdapterInfo{
			Name: "some adapter", AdapterType: wgpu.AdapterTypeUnknown,
			BackendType: wgpu.BackendTypeVulkan,
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := usable(tc.info)
			if tc.refused && err == nil {
				t.Fatalf("%s was accepted as a GPU", tc.name)
			}
			if !tc.refused && err != nil {
				t.Fatalf("%s was refused: %v", tc.name, err)
			}
			// A refusal nobody can read is a machine that has quietly lost the
			// GPU path with no way to find out why, so the reason has to name
			// the backend it came from.
			if tc.refused && !strings.Contains(err.Error(), tc.info.BackendType.String()) {
				t.Fatalf("refusal does not name the backend: %v", err)
			}
		})
	}
}

// The adapter this machine actually has, if it has one, is not refused by the
// rule above. Guards against a rule that rejects everything and so reads as
// "no GPU on any machine", which would look exactly like it working.
func TestThisMachinesAdapterIsNotRefusedForBeingSoftware(t *testing.T) {
	d, err := Open()
	if err != nil {
		if strings.Contains(err.Error(), "software rasteriser") {
			t.Skip("this machine only offers a software adapter:", err)
		}
		t.Skip("no GPU available:", err)
	}
	defer d.Close()
	if d.Backend == "" {
		t.Fatal("an opened device names no backend")
	}
}
