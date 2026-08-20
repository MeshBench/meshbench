// The boards themselves: one entry per piece of hardware, and the figures that
// describe it.
//
// Split from boards.go, which holds the types these are written in and the
// questions asked of them. The list is the part that grows.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

// boards is the starter set: the hardware people actually deploy on a UK mesh.
//
// Deliberately small. Seven profiles that are right are worth more than forty
// that were guessed at, and every figure here can be traced to a datasheet or a
// published schematic. Anything uncertain is in Notes rather than smoothed over.
var boards = []Board{
	{
		Name: "RAK4631", MCU: "nRF52840", Radio: "SX1262", Vendor: "RAKwireless",
		MaxTxDBm: 22, FeedlineDB: 0.8, AntennaDBi: 2.15,
		SensitivityDBm: -137, NoiseFigureDB: 6, SleepUA: 20,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 3400, Cells: 1, CutoffV: 3.1},
		Emulated: true,
		Notes: "The reference repeater. Runs under Renode today, including the shipped " +
			"image's MBR and SoftDevice. Ships with an external whip, so the antenna " +
			"figure assumes a half-wave dipole rather than the board.",
	},
	{
		Name: "Heltec_v3", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Heltec",
		MaxTxDBm: 21, FeedlineDB: 1.2, AntennaDBi: -1,
		SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 200,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 2000, Cells: 1, CutoffV: 3.2},
		Emulated: false,
		Notes: "Very common and not a good repeater: the stock spring antenna is well " +
			"below a dipole, and sleep current is dominated by the board rather than " +
			"the MCU. Not emulated: QEMU's ESP32-S3 machine models only the flash SPI " +
			"controller, so there is no bus for the radio to sit on. The plain-ESP32 " +
			"machine models SPI0 to SPI3 and does work.",
	},
	{
		// The first board driven end to end under emulation, which is why it is
		// here rather than for being popular.
		Name: "Generic_E22_sx1262", MCU: "ESP32", Radio: "SX1262", Vendor: "Ebyte",
		MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: 2.15,
		SensitivityDBm: -137, NoiseFigureDB: 6, SleepUA: 250,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 2000, Cells: 1, CutoffV: 3.2},
		Emulated: true,
		QEMU: &QEMUWiring{
			// FEM 13 is SX126X_TXEN, from the variant's platformio.ini. RadioLib
			// drives it from setRfSwitchPins(RXEN=14, TXEN=13), so it goes high
			// before SetTx and low again before SetRx.
			Machine: "esp32", SPI: 2, NSS: 18, Busy: 32, LED: 2, FEM: 13,
			Verified: true,
		},
		// The module's TXEN and RXEN, on MCU pins 13 and 14. No gain stage:
		// MeshCore compiles this variant for LORA_TX_POWER=22 and the E22's own
		// SX1262 produces it, so these switch the path rather than amplify it.
		// The loss is an RF switch's isolation and is a plausible figure rather
		// than a measured one - see Notes.
		FEM: &FEM{TxGainDB: 0, TxLossDB: 25},
		Notes: "An E22 module on a devkit rather than a product, so the antenna figure " +
			"assumes the external whip the module is designed for. The published " +
			"repeater image boots and runs RadioLib's full SX126x init sequence under " +
			"emulation: version read, LoRa mode, modulation and IRQ setup. " +
			"The 25 dB switch isolation is a plausible figure for an SPDT part at " +
			"868 MHz and has not been measured. Upstream also sets " +
			"SX126X_DIO2_AS_RF_SWITCH=true alongside the MCU pins, which its own " +
			"variant.h warns against - so on this board the path may be switched by " +
			"DIO2 whatever the MCU pins do, and a transmit-enable fault here would " +
			"be milder than the model says. The T096 is the honest case for that.",
	},
	{
		// Carries the front-end module MeshCore 1.17.1's transmit fix was about.
		// Not emulated yet: it is nRF52, so it wants Renode rather than QEMU.
		Name: "Heltec_t096", MCU: "nRF52840", Radio: "SX1262", Vendor: "Heltec",
		MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: -1,
		SensitivityDBm: -137, NoiseFigureDB: 6, SleepUA: 60,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 1000, Cells: 1, CutoffV: 3.1},
		Emulated: false,
		// A KCT8103L, switched by three GPIOs: an LDO enable, a shutdown line
		// and a transmit/receive path select. The gain figure is upstream's own:
		// variants/heltec_t096/platformio.ini sets LORA_TX_POWER=9 against
		// MAX_LORA_TX_POWER=22 and says "9dBm + ~13dB KCT8103L gain".
		FEM: &FEM{TxGainDB: 13, TxLossDB: 0},
		// variants/heltec_t096/platformio.ini: P_LORA_NSS=5, P_LORA_DIO_1=21.
		Renode: &RenodeWiring{
			Platform: "platforms/cpus/nrf52840.repl",
			SPIBase:  0x4002F000,
			NssPort:  "gpio0",
			NssPin:   5,
			IrqPort:  "gpio0",
			IrqPin:   21,
		},
		Notes: "The board whose transmit failure 1.17.1 fixed: PIN_SPI1_MISO was -1 " +
			"against a 48-entry pin map, and the out-of-bounds read left the " +
			"module's transmit enable undriven. The chip is compiled for 9 dBm and " +
			"the module carries it to about 22, so a firmware that does not switch " +
			"the module in is 13 dB down with nothing in the radio's registers to " +
			"say so. MaxTxDBm here is the antenna figure, not the chip's. Antenna " +
			"and sleep figures are taken from the comparable nRF52840 boards rather " +
			"than from this board's own schematic, and should be checked before " +
			"either is trusted.",
	},
	{
		Name: "Heltec_v2", MCU: "ESP32", Radio: "SX1276", Vendor: "Heltec",
		MaxTxDBm: 20, FeedlineDB: 1.2, AntennaDBi: -1,
		SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 250,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 2000, Cells: 1, CutoffV: 3.2},
		Emulated: false,
		Notes: "Carries an SX1276, not an SX1262, despite sitting next to the V3 in " +
			"every shop. Its firmware speaks SX127x register access rather than " +
			"SX126x commands, so the radio model does not answer it. Recorded here " +
			"because the name invites exactly that mistake.",
	},
	{
		Name: "Heltec_mesh_solar", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Heltec",
		MaxTxDBm: 21, FeedlineDB: 1.2, AntennaDBi: -1,
		SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 150,
		Battery: energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 3000, Cells: 1, CutoffV: 3.2},
		Panel: energy.Panel{PeakW: 1.5, TiltDeg: 0, AzimuthDeg: 180,
			SoilingFactor: 0.8, ChargeEfficiency: 0.75},
		Emulated: false,
		Notes: "Integrated panel mounted flat, which at UK latitudes is the worst case " +
			"in December — see internal/energy. PWM charging, so the efficiency figure " +
			"is not MPPT.",
	},
	{
		Name: "Xiao_S3_WIO", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Seeed",
		MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: -2,
		SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 50,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 1000, Cells: 1, CutoffV: 3.2},
		Emulated: false,
		Notes:    "Tiny, and the antenna figure reflects it. A companion, not a repeater.",
	},
	{
		Name: "Xiao_nRF52840", MCU: "nRF52840", Radio: "SX1262", Vendor: "Seeed",
		MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: -2,
		SensitivityDBm: -137, NoiseFigureDB: 6, SleepUA: 5,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 1000, Cells: 1, CutoffV: 3.1},
		Emulated: true,
		// variants/xiao_nrf52/platformio.ini names the pins by the board's own
		// labels - P_LORA_NSS=D4, P_LORA_DIO_1=D1 - which variant.cpp's pin map
		// resolves to P0.04 and P0.03.
		Renode: &RenodeWiring{
			Platform: "platforms/cpus/nrf52840.repl",
			SPIBase:  0x4002F000,
			NssPort:  "gpio0",
			NssPin:   4,
			IrqPort:  "gpio0",
			IrqPin:   3,
		},
		Notes: "Same nRF52840 core as the RAK4631, and wired for the same path, but " +
			"not yet booted here: MeshCore publishes this board's image as " +
			"Xiao_nrf52 and we call it Xiao_nRF52840, so nothing matches the name " +
			"we ask for. The wiring below is from the board's own build and should " +
			"hold the moment the names are reconciled. " +
			"Genuinely low sleep current, which makes it the one board here where " +
			"duty-cycling buys a great deal.",
	},
	{
		Name: "Heltec_t114", MCU: "nRF52840", Radio: "SX1262", Vendor: "Heltec",
		MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: 0,
		SensitivityDBm: -137, NoiseFigureDB: 6, SleepUA: 60,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 2000, Cells: 1, CutoffV: 3.1},
		Emulated: true,
		// Pins from the board's own build: variants/heltec_t114/platformio.ini
		// sets P_LORA_NSS=24 and P_LORA_DIO_1=20, both on gpio0. The radio is
		// on SPIM3, like the RAK4631; the display is the thing on SPIM2, and
		// the firmware blocks on that too until it is given its EasyDMA half.
		Renode: &RenodeWiring{
			Platform: "platforms/cpus/nrf52840.repl",
			SPIBase:  0x4002F000,
			NssPort:  "gpio0",
			NssPin:   24,
			IrqPort:  "gpio0",
			IrqPin:   20,
		},
		Notes: "nRF52840 with a display, which is why sleep current is not the MCU's own.",
	},
	{
		Name: "Station_G2", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "LILYGO",
		MaxTxDBm: 30, FeedlineDB: 1.5, AntennaDBi: 2.15,
		SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 5000,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 0, Cells: 1, CutoffV: 3.2},
		Emulated: false,
		Notes: "Mains-powered with an external PA, so it is the only board here that " +
			"can legally run 30 dBm where the band plan allows it — and the only one " +
			"whose sleep current does not matter. Check the licence conditions before " +
			"simulating it at full power.",
	},

	rak4631Board,
}
