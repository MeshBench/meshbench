package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"
)

// machineLoad is what this machine is currently giving the simulator.
//
// It exists because of emulation. A native node costs a slice of a core and
// nobody needs a readout to run three hundred of them; an emulated node is a
// whole emulator spinning a vCPU thread flat out, so the ceiling is cores and
// it arrives suddenly. Eight of them saturate a twelve core machine, and the
// symptom is not an error - it is boots taking half a minute and simulated time
// quietly falling behind the wall clock. A number in the chrome turns that into
// something an operator can see coming.
type machineLoad struct {
	mu      sync.Mutex
	cpuPct  float64
	gpuPct  float64
	gpuName string
	taken   time.Time

	lastIdle, lastTotal uint64
}

// sample refreshes at most once a second. Reading /proc on every frame would
// itself be load, which is a poor thing for a load meter to be.
func (m *machineLoad) sample() (cpu, gpu float64, hasGPU bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if time.Since(m.taken) < time.Second {
		return m.cpuPct, m.gpuPct, m.gpuName != ""
	}
	m.taken = time.Now()
	m.cpuPct = m.readCPU()
	m.gpuPct, m.gpuName = readGPU()
	return m.cpuPct, m.gpuPct, m.gpuName != ""
}

// readCPU is the whole machine, not this process: an emulated node is a
// separate process, and a meter that only counted ours would read near zero
// while the box was pinned.
func (m *machineLoad) readCPU() float64 {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	line, _, _ := strings.Cut(string(b), "\n")
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0
	}
	var total, idle uint64
	for i, f := range fields[1:] {
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			continue
		}
		total += v
		// user nice system idle iowait ...: idle and iowait are both waiting.
		if i == 3 || i == 4 {
			idle += v
		}
	}
	defer func() { m.lastIdle, m.lastTotal = idle, total }()
	if m.lastTotal == 0 || total <= m.lastTotal {
		return m.cpuPct
	}
	dTotal := float64(total - m.lastTotal)
	dIdle := float64(idle - m.lastIdle)
	return 100 * (dTotal - dIdle) / dTotal
}

// readGPU asks the kernel rather than a vendor tool.
//
// amdgpu and i915 both publish a busy percentage in sysfs, which costs a file
// read; nvidia does not, and shelling out to nvidia-smi once a second is a
// process spawn per second for one number. So this reports a GPU when the
// kernel will tell us and says nothing when it will not, rather than showing a
// zero that reads as an idle card.
func readGPU() (float64, string) {
	matches, _ := filepath.Glob("/sys/class/drm/card*/device/gpu_busy_percent")
	for _, p := range matches {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		if err != nil {
			continue
		}
		name := "GPU"
		if card, _, ok := strings.Cut(strings.TrimPrefix(p, "/sys/class/drm/"), "/"); ok {
			name = card
		}
		return v, name
	}
	return 0, ""
}

// drawMachineLoad puts the readout in the menu bar, beside the honesty line.
func (a *App) drawMachineLoad() {
	cpu, gpu, hasGPU := a.load.sample()

	// Amber approaching saturation, red past it. The thresholds are about
	// emulation: at 85% the emulators are already stretching their boots, and
	// past 95% simulated time stops tracking the wall clock.
	col := imgui.NewVec4(0.60, 0.60, 0.60, 1)
	switch {
	case cpu >= 95:
		col = imgui.NewVec4(0.93, 0.35, 0.32, 1)
	case cpu >= 85:
		col = imgui.NewVec4(0.95, 0.72, 0.25, 1)
	}
	text := fmt.Sprintf("cpu %2.0f%%", cpu)
	if hasGPU {
		text += fmt.Sprintf("  gpu %2.0f%%", gpu)
	}
	imgui.PushStyleColorVec4(imgui.ColText, col)
	imgui.TextUnformatted(text)
	imgui.PopStyleColor()
	if imgui.IsItemHovered() {
		tip := "This machine, not just the simulator.\n\n" +
			"An emulated node is a whole emulator and takes about a core,\n" +
			"so a handful of them will saturate a desktop. When this sits\n" +
			"near 100%, boots stretch and simulated time falls behind the\n" +
			"wall clock - which looks like a mesh gone quiet."
		if !hasGPU {
			tip += "\n\nNo GPU busy figure: the kernel publishes one for amdgpu\n" +
				"and i915, and this card does not."
		}
		imgui.SetTooltip(tip)
	}
}
