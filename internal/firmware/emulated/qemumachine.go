// The machine this board is, delegated to the qemu package which composes the
// -machine argument from a board's wiring.
package emulated

import "github.com/MeshBench/meshbench/internal/firmware/emulated/qemu"

func (e *EmulatedNode) machineString(radioAt string) string {
	return qemu.Config{
		Machine: e.Machine, SPI: e.SPI, NSS: e.NSS, Busy: e.Busy, DIO1: e.DIO1,
		FEM: e.FEM, PSRAMOctal: e.PSRAMOctal, CoprocAtReset: e.CoprocAtReset,
		ButtonPath: e.ButtonPath, ButtonPins: e.ButtonPins,
		KbdAddr: e.KbdAddr, TouchAddr: e.TouchAddr,
		CardPath: e.CardPath, CardCS: e.CardCS,
		BatChannel: e.BatChannel, BatRaw: e.BatRaw,
		PanelPath: e.PanelPath, PanelAddr: e.PanelAddr, PanelOffset: e.PanelOffset,
		PanelCS: e.PanelCS, PanelDC: e.PanelDC, PanelWidth: e.PanelWidth, PanelHgt: e.PanelHgt,
	}.Arg(radioAt)
}
