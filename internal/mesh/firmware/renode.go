package firmware

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// startRenode writes the machine description this node needs and runs it.
//
// Generated rather than kept as a file, because three of the values are
// per-node: the radio model's port, the node's own working directory, and the
// image. A shared script would need all three passed in anyway.
func (e *EmulatedNode) startRenode(ctx context.Context) error {
	renodeBin, err := lookupTool(EnvRenode, "renode")
	if err != nil {
		return err
	}
	tools := ToolsDir()

	// The radio's wiring goes in a platform description of its own rather than
	// into the script: these are declarations, and Renode's monitor does not
	// take declarations. Inlining them in the .resc left the machine without a
	// radio and the firmware waiting on a chip that was never there.
	repl := filepath.Join(e.Dir, "node.repl")
	wiring := fmt.Sprintf(`spi3: SPI.NRF52840_SPI @ sysbus 0x%X
    easyDMA: true

lora: Radio.RadioServerSX1262 @ spi3
    host: "127.0.0.1"
    port: %d
    IRQ -> %s@%d

%s:
    %d -> lora@0
`, e.SPIBase, e.radioPort, e.IrqPort, e.IrqPin, e.NssPort, e.NssPin)
	if err := os.WriteFile(repl, []byte(wiring), 0o644); err != nil {
		return err
	}

	flash, err := e.renodeFlash()
	if err != nil {
		return err
	}

	script := filepath.Join(e.Dir, "node.resc")
	body := fmt.Sprintf(`i @%[1]s/peripherals/RadioServerSX1262.cs
i @%[1]s/peripherals/NRF52840_Temp.cs
i @%[1]s/peripherals/NRF52840_Clock.cs
i @%[1]s/peripherals/NRF52840_SAADC.cs
i @%[1]s/peripherals/NRF52840_TWIM.cs

mach create "%[2]s"
machine LoadPlatformDescription @%[3]s
machine LoadPlatformDescription @%[1]s/ficr.repl
machine LoadPlatformDescription @%[1]s/uicr.repl
machine LoadPlatformDescription @%[1]s/temp.repl
sysbus Unregister sysbus.clock
machine LoadPlatformDescription @%[1]s/clock.repl
machine LoadPlatformDescription @%[1]s/saadc.repl
sysbus Unregister sysbus.twi0
sysbus Unregister sysbus.twi1
machine LoadPlatformDescription @%[1]s/twim.repl
machine LoadPlatformDescription @%[4]s

%[5]s
spi3.lora Connect
start
`, tools, e.NodeName, e.Platform, repl, flash)
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		return err
	}

	log, err := os.Create(filepath.Join(e.Dir, "console.log"))
	if err != nil {
		return err
	}
	// Renode's monitor reads commands from standard input, and a monitor at
	// end of file quits. With nothing on stdin it ran the script, reached the
	// end and shut the machine down about four hundred milliseconds later -
	// long enough to look like a node that booted and then died on its own.
	// The pipe is never written to; it exists to stay open.
	stdin, hold, err := os.Pipe()
	if err != nil {
		return err
	}
	e.qemu = exec.CommandContext(ctx, renodeBin,
		"--disable-xwt", "--console", "-e", "include @"+script)
	e.qemu.Stdin = stdin
	e.qemu.Stdout, e.qemu.Stderr = log, log
	if err := e.qemu.Start(); err != nil {
		_, _ = hold.Close(), stdin.Close()
		return fmt.Errorf("firmware: starting the emulator: %w", err)
	}
	_ = stdin.Close()
	e.renodeStdin = hold
	return nil
}
