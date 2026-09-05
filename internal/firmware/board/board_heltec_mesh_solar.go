// The Heltec_mesh_solar, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package board

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var heltecMeshSolarBoard = Board{
	Name: "Heltec_mesh_solar", MCU: "nRF52840", Radio: "SX1262", Vendor: "Heltec",
	MaxTxDBm: 22, FeedlineDB: 1.2, AntennaDBi: -1,
	SensitivityDBm: -137, NoiseFigureDB: 6, SleepUA: 150,
	Battery: energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 3000, Cells: 1, CutoffV: 3.2},
	Panel: energy.Panel{PeakW: 1.5, TiltDeg: 0, AzimuthDeg: 180,
		SoilingFactor: 0.8, ChargeEfficiency: 0.75},
	Emulated: true,
	// Same pins as the Heltec_t114: variants/heltec_mesh_solar/MeshSolarBoard.h
	// sets P_LORA_NSS=24 and P_LORA_DIO_1=20, both on gpio0.
	Renode: &RenodeWiring{
		Platform: "platforms/cpus/nrf52840.repl",
		SPIBase:  0x4002F000,
		NssPort:  "gpio0",
		NssPin:   24,
		IrqPort:  "gpio0",
		IrqPin:   20,
		// The user button, PIN_BUTTON1 = P1.10. This variant configures it
		// as a plain INPUT and relies on the board's pull-up, so it has to be
		// held high here or the firmware reads a long press and powers off.
		IdleHighPins: []GPIOPin{{Port: "gpio1", Pin: 10}},
		// The Adafruit nRF52 core builds Serial as a TinyUSB CDC device, so the
		// firmware's console is on USB and not on this part's UART.
		ConsoleOnUSB: true,
	},
	// What the board shows and what can be pressed on it, from
	// variants/heltec_mesh_solar in MeshCore: variant.h for the pins.
	//
	// No screen. Every Heltec_mesh_solar env in the tree leaves DISPLAY_CLASS
	// undefined, so nothing the catalogue fetches drives one - which is a fact
	// about the builds as much as about the board, and the reason this profile
	// declares parts and no panel.
	//
	// No meter either. This board reads its cell through the charger over I2C
	// rather than off an ADC pin: MeshSolarBoard.h calls meshSolarGetBattVoltage,
	// which is not a conversion on a pin and has no pin to declare.
	Hardware: &Panel{
		Parts: []Part{
			// LED_BUILTIN, P0.12, with LED_STATE_ON LOW: it lights on a low.
			{Kind: Lamp, Name: "LED", Pin: 12},
			// PIN_BUTTON1, P1.10 - the pin the Renode wiring above holds high,
			// because an undriven input here reads as a button held down.
			{Kind: Button, Name: "user", Pin: 42, ActiveLow: true},
		},
	},

	Notes: "An nRF52840, not the ESP32-S3 this profile claimed for a while: the " +
		"variant extends nrf52_base and links against the s140 SoftDevice, and " +
		"the release publishes it as .uf2 like every other nRF52 board here. " +
		"Recorded as an ESP32-S3 it was refused emulation for a reason that was " +
		"never true of it. " +
		"Integrated panel mounted flat, which at UK latitudes is the worst case " +
		"in December — see internal/energy. PWM charging, so the efficiency figure " +
		"is not MPPT.",
}
