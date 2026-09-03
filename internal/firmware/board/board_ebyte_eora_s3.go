// The Ebyte_EoRa-S3, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package board

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var ebyteEoRaS3Board = Board{
	Name: "Ebyte_EoRa-S3", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Ebyte",
	MaxTxDBm: 22, FeedlineDB: 1.2, AntennaDBi: 2,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 200,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 3000, Cells: 1, CutoffV: 3.0},
	Emulated: true,
	// Pins from variants/ebyte_eora_s3/platformio.ini, which are the T3S3's:
	// the two boards are the same layout under different names.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 7, Busy: 34, DIO1: 33, LED: 37,
		PSRAMMB: 8,
		// The Arduino S3 core boots this board with Serial on the USB
		// Serial/JTAG (ARDUINO_USB_CDC_ON_BOOT), not UART0, so the whole
		// application console - boot banner, prompts, "Powering off" - was
		// going to a port nothing captured while UART0 carried only the ROM
		// bootloader's output. Measured: with the console on UART0 the log
		// held only the reset-reason lines and nothing MeshCore printed.
		ConsoleOnUSB: true,
		Verified:     true,
	},
	Hardware: &Panel{
		Screen: &Screen{
			Controller: "SSD1306", Bus: BusI2C, Addr: 0x3C,
			WidthPx: 128, HeightPx: 64, Ink: Mono,
		},
		Parts: []Part{
			{Kind: Lamp, Name: "TX", Pin: 37},
			{Kind: Button, Name: "BOOT", Pin: 0, ActiveLow: true},
			// A halving divider read against the converter's 3.3 V
			// range, so full scale is 6.6 V.
			{Kind: Meter, Name: "battery", Pin: 1, FullScaleMV: 6600},
		},
	},
	Notes: "Wired identically to the LilyGo T3S3 and published as its own image. " +
		"Ebyte also sell modules with an integrated amplifier under names close to " +
		"this one; those are a different board and these figures do not describe " +
		"them.",
}
