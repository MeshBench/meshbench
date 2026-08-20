package firmware

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"text/template"
)

// EnvRenode overrides the Renode executable.
const EnvRenode = "MESHCORESIM_RENODE"

// The script is generated per node rather than shipped as a fixed file because
// the bridge address is chosen at runtime (port 0) and the firmware image is a
// per-node choice — a scenario can mix board types, and usually does.
var renodeScript = template.Must(template.New("resc").Parse(
	`include @{{.Peripheral}}
mach create "{{.Node}}"
machine LoadPlatformDescription @platforms/cpus/nrf52840.repl
{{range .Platforms}}machine LoadPlatformDescription @{{.}}
{{end}}sysbus LoadELF @{{.Firmware}}
sx1262 BridgeEndpoint "{{.BridgeAddr}}"
start
`))

// Emulated runs a firmware image on an emulated MCU under Renode.
type Emulated struct {
	// Firmware is the ELF to run. Required.
	Firmware string

	// Peripheral is the SX1262 model source; Platforms are extra .repl files.
	// Both default to the copies in tools/renode when RenodeDir is set.
	Peripheral string
	Platforms  []string

	// Renode is the executable. Empty means $MESHCORESIM_RENODE, then PATH.
	Renode string

	// Log receives Renode's output. Nil discards it.
	Log io.Writer

	mu      sync.Mutex
	cmd     *exec.Cmd
	scratch string
}

func (e *Emulated) Kind() string { return "emulated" }

// The bridge to an emulated node carries the radio and nothing else.
func (e *Emulated) HasConsole() bool { return false }

func (e *Emulated) ConsoleIn() io.Writer { return nil }

func (e *Emulated) Start(ctx context.Context, bridgeAddr string) error {
	if e.Firmware == "" {
		return errors.New("firmware: emulated node needs a firmware image")
	}
	bin := e.Renode
	if bin == "" {
		bin = os.Getenv(EnvRenode)
	}
	if bin == "" {
		bin = "renode"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("firmware: renode not found (%s or PATH): %w", EnvRenode, err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cmd != nil {
		return errors.New("firmware: emulated node already started")
	}

	dir, err := os.MkdirTemp("", "msim-renode-")
	if err != nil {
		return fmt.Errorf("firmware: scratch dir: %w", err)
	}
	script := filepath.Join(dir, "node.resc")
	f, err := os.Create(script)
	if err != nil {
		_ = os.RemoveAll(dir)
		return fmt.Errorf("firmware: script: %w", err)
	}
	err = renodeScript.Execute(f, struct {
		Node, Firmware, Peripheral, BridgeAddr string
		Platforms                              []string
	}{filepath.Base(dir), e.Firmware, e.Peripheral, bridgeAddr, e.Platforms})
	_ = f.Close()
	if err != nil {
		_ = os.RemoveAll(dir)
		return fmt.Errorf("firmware: script: %w", err)
	}

	// --disable-xwt because a scenario runs unattended and a Renode that opens
	// windows cannot be run headless or in CI.
	cmd := exec.CommandContext(ctx, bin, "--disable-xwt", "--console", "-e", "include @"+script)
	cmd.Stdout, cmd.Stderr = e.Log, e.Log
	if cmd.Stdout == nil {
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	}
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(dir)
		return fmt.Errorf("firmware: launch renode: %w", err)
	}
	e.cmd, e.scratch = cmd, dir
	return nil
}

func (e *Emulated) Stop() error {
	e.mu.Lock()
	cmd, dir := e.cmd, e.scratch
	e.cmd, e.scratch = nil, ""
	e.mu.Unlock()
	if dir != "" {
		defer func() { _ = os.RemoveAll(dir) }()
	}
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	return nil
}
