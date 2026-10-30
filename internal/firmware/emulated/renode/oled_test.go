package renode

import (
	"strings"
	"testing"
)

// An I2C display hangs off the first TWIM at its own address, told its glass
// and where its columns start (#602).
func TestAnI2CDisplayIsRegisteredAtItsAddress(t *testing.T) {
	got := Oled(4242, 0x3C, 128, 64, 0)
	for _, want := range []string{"Video.MeshBenchOled @ twi0 0x3C", "port: 4242", "width: 128", "height: 64", "columnOffset: 0"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if Oled(0, 0x3C, 128, 64, 0) != "" || Oled(4242, 0, 128, 64, 0) != "" {
		t.Error("a display with no channel or no address was wired anyway")
	}
}
