package engine

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A BLE companion has no transport an emulator can offer - its client arrives
// over Bluetooth, which is not modelled. Rather than fail later with "no image
// in the cache", which sends the operator downloading a build that could never
// run, the backend refuses up front and says to use the USB companion (#256).
func TestEmulatedBackendRefusesBLECompanionClearly(t *testing.T) {
	// The composite role and the transport field are two spellings of one thing
	// (#256); a BLE companion written either way must be refused the same, or
	// the check depends on which spelling a scenario happened to use.
	spellings := []struct {
		name string
		ref  scenario.FirmwareRef
	}{
		{
			name: "composite role",
			ref:  scenario.FirmwareRef{Role: scenario.RoleCompanionRadioBLE},
		},
		{
			name: "transport field",
			ref:  scenario.FirmwareRef{Role: scenario.RoleCompanionRadio, Transport: "ble"},
		},
	}
	for _, s := range spellings {
		t.Run(s.name, func(t *testing.T) {
			n := scenario.Node{Name: "phone-side"}
			n.Firmware = s.ref
			n.Firmware.Board = "Heltec_v3"
			n.Firmware.Version = "v1.17.0"

			_, err := emulatedBackend(n, true)
			if err == nil {
				t.Fatal("a BLE companion was accepted; it has no reachable transport here")
			}
			msg := err.Error()
			if !strings.Contains(msg, "companion_radio_usb") || !strings.Contains(strings.ToLower(msg), "bluetooth") {
				t.Fatalf("refusal did not name the cause and the fix: %q", msg)
			}
		})
	}
}
