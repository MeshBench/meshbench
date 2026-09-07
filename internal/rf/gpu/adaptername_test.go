package gpu

import (
	"testing"

	"github.com/cogentcore/webgpu/wgpu"
)

// wgpu-native fills Name from the driver, and on Metal it puts a device id
// there rather than a name - "0x0" on an Apple M4, which is not a plausible id
// either and reads as a null. Configuration drew that as the whole answer, and
// the software-rasteriser refusal beside it said "0x0 is a software
// rasteriser" where a person needed to be told which device.
func TestAnAdapterWithNoNameStillSaysSomething(t *testing.T) {
	metal := wgpu.AdapterInfo{Name: "0x0", BackendType: wgpu.BackendTypeMetal}
	got := adapterName(metal)
	if got == "0x0" || got == "" {
		t.Errorf("a Metal adapter is named %q", got)
	}

	// Anything the driver does say is better than the backend's own name.
	withDriver := wgpu.AdapterInfo{
		Name: "0x0", DriverDescription: "Apple M4", BackendType: wgpu.BackendTypeMetal,
	}
	if got := adapterName(withDriver); got != "Apple M4" {
		t.Errorf("with a driver description the adapter is named %q, want Apple M4", got)
	}

	// A real name is used as it stands.
	named := wgpu.AdapterInfo{Name: "AMD Radeon RX 5700 XT", BackendType: wgpu.BackendTypeVulkan}
	if got := adapterName(named); got != "AMD Radeon RX 5700 XT" {
		t.Errorf("a named adapter came back as %q", got)
	}

	// And nothing at all still answers something a sentence can be built on.
	if got := adapterName(wgpu.AdapterInfo{}); got == "" {
		t.Error("an adapter with nothing set is named with an empty string")
	}
}
