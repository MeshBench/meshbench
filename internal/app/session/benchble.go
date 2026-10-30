// Serving a node over this machine's Bluetooth adapter.
//
// The node is served over TCP as usual, and internal/app/benchble/nus_peripheral.py is
// started against that port: it registers MeshCore's companion service with
// BlueZ and advertises it, so a phone finds the simulated node the way it
// finds a radio. The peripheral holds no simulation state. It turns each frame
// on the node's port into one notification and each write into a frame, and
// it exits when the port closes, so dropping the endpoint drops it too.
//
// What runs on the node is the USB build: this tests a client against a
// simulated mesh, not MeshCore's own BLE transport.
package session

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/app/benchble"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
)

// bleReady is how long BlueZ gets to register the service and advertisement.
const bleReady = 15 * time.Second

// serveBLE serves a node over TCP and puts a BLE peripheral in front of it.
func (s *Sim) serveBLE(name, local string) (state.Endpoint, string, error) {
	if runtime.GOOS != "linux" {
		return state.Endpoint{}, "", fmt.Errorf("serving over BLE needs BlueZ, which is Linux only")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		return state.Endpoint{}, "", fmt.Errorf("serving over BLE runs a Python peripheral, and python3 is not on PATH")
	}
	ep, moved, err := s.serve(name, "tcp")
	if err != nil {
		return state.Endpoint{}, "", err
	}
	l, ok := s.servedLink(name)
	if !ok {
		return state.Endpoint{}, "", fmt.Errorf("%s was served and is not now", name)
	}
	_, port, err := net.SplitHostPort(l.Addr)
	if err != nil {
		return state.Endpoint{}, "", err
	}
	if local == "" {
		local = "MSIM-" + firmware.SafeNodeName(name)
	}
	dir := firmware.NodeWorkDir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return state.Endpoint{}, "", err
	}
	script := filepath.Join(dir, "nus_peripheral.py")
	if err := os.WriteFile(script, benchble.Peripheral, 0o644); err != nil {
		return state.Endpoint{}, "", err
	}
	// Not tied to a request's context: it lives as long as the endpoint, and
	// ends itself when the endpoint's port closes.
	cmd := exec.CommandContext(context.Background(), python, script, "--name", local, "--bridge", "127.0.0.1:"+port) //nolint:gosec // G204: our own embedded script, and a name reduced to safe characters
	cmd.SysProcAttr = firmware.ChildProcAttr()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return state.Endpoint{}, "", err
	}
	if err := cmd.Start(); err != nil {
		return state.Endpoint{}, "", fmt.Errorf("starting the BLE peripheral: %w", err)
	}
	if err := awaitRegistered(out); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		s.stopServing(name)
		why := strings.TrimSpace(stderr.String())
		if why == "" {
			why = err.Error()
		}
		return state.Endpoint{}, "", fmt.Errorf("the BLE peripheral did not come up: %s", lastLine(why))
	}
	gen := s.setBLE(name, local)
	go func() {
		_ = cmd.Wait()
		s.clearBLE(name, gen)
	}()
	ep.Kind, ep.Addr, ep.Addrs = "ble", local, nil
	return ep, moved, nil
}

// awaitRegistered reads the peripheral's output until it says the service is
// advertised, it ends, or it takes too long.
func awaitRegistered(r interface{ Read([]byte) (int, error) }) error {
	done := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(r)
		gatt, adv := false, false
		for sc.Scan() {
			switch strings.TrimSpace(sc.Text()) {
			case "gatt registered":
				gatt = true
			case "adv registered":
				adv = true
			}
			if gatt && adv {
				done <- nil
				// Keep draining, so the peripheral never blocks on a full pipe.
				for sc.Scan() {
				}
				return
			}
		}
		done <- fmt.Errorf("it exited before registering")
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(bleReady):
		return fmt.Errorf("BlueZ did not register it within %s", bleReady)
	}
}

func lastLine(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// bleRun is one peripheral advertising a node.
type bleRun struct {
	local string
	gen   uint64
}

// setBLE records the advertised name a node is served under, and returns the
// number that names this run.
func (s *Sim) setBLE(name, local string) uint64 {
	s.servedMu.Lock()
	defer s.servedMu.Unlock()
	if s.bleNames == nil {
		s.bleNames = map[string]bleRun{}
	}
	s.bleGen++
	s.bleNames[name] = bleRun{local: local, gen: s.bleGen}
	return s.bleGen
}

// clearBLE forgets that a node is served over BLE: whatever run it is with gen
// zero, as a re-serve or a drop does, or only run gen, as a peripheral that
// has exited does, so it cannot clear a newer run's name.
func (s *Sim) clearBLE(name string, gen uint64) {
	s.servedMu.Lock()
	defer s.servedMu.Unlock()
	if r, ok := s.bleNames[name]; ok && (gen == 0 || r.gen == gen) {
		delete(s.bleNames, name)
	}
}

// bleName is the name a node is advertised under, if it is.
func (s *Sim) bleName(name string) (string, bool) {
	s.servedMu.Lock()
	defer s.servedMu.Unlock()
	r, ok := s.bleNames[name]
	return r.local, ok
}
