// Package capture holds the frame-capture verbs - writing a pcapng file,
// streaming to Wireshark, and stopping - and the Wireshark launch machinery
// they use. Split out of internal/app/session; it reaches the running Sim
// through the accessors session exports and registers its verbs from init.
package capture

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerCapture(st *state.Store, s *session.Sim) {
	// Diagnosing why a packet was not relayed needs the frame, not a window,
	// and on a driven session there is often nobody at the screen to look at
	// one.
	st.Handle("capture.file", func(w *state.World, p any) (any, error) {
		if s.Engine() == nil {
			return nil, fmt.Errorf("no network loaded")
		}
		path, _ := session.StringField(p, "path")
		if path == "" {
			path = filepath.Join(os.TempDir(), "meshbench-capture.pcapng")
		}
		if err := s.Engine().StartCapture(path); err != nil {
			return nil, err
		}
		s.SetCapturePath(path)
		w.Say("capturing to " + path)
		return map[string]any{"path": path}, nil
	})

	// It streams even when the launch or the dissectors fail, and says which
	// did - a capture running with no window is recoverable by hand.
	st.Handle("capture.wireshark", func(w *state.World, p any) (any, error) {
		if s.Engine() == nil {
			return nil, fmt.Errorf("no network loaded")
		}
		if err := s.Engine().StartCaptureUDP(captureUDPAddr); err != nil {
			return nil, err
		}
		meshcoreLua, meshbenchLua := dissectorFiles()
		// Found before the hint is built, because the hint names it: a reader
		// told to run this by hand should be handed the binary we already
		// located rather than a bare name their shell may not resolve.
		bin := wiresharkBinary()
		out := map[string]any{
			"addr": captureUDPAddr, "how": wiresharkHint(bin, meshcoreLua, meshbenchLua),
		}
		switch {
		case meshbenchLua == "":
			out["dissector_error"] = "tools/dissector/meshbench.lua was not found beside the binary"
		case meshcoreLua == "":
			out["dissector_warning"] = "tools/dissector/meshcore_dissector.lua was not found - " +
				"MeshBench's own metadata will show, the MeshCore frame inside it will not"
		}

		if bin == "" {
			// Said in the answer as well as on screen. A caller reading only
			// the verb used to get launched false and nothing to explain it,
			// which reads as a launch that failed silently rather than as a
			// program that is not there.
			why := "Wireshark is not installed, or not where this looked"
			if runtime.GOOS == "windows" {
				why += " (PATH, and Wireshark\\Wireshark.exe under Program Files)"
			}
			w.Say("streaming frames to " + captureUDPAddr + " - " + why + ", so run: " +
				wiresharkHint(bin, meshcoreLua, meshbenchLua))
			out["launched"] = false
			out["launch_error"] = why
			return out, nil
		}
		if why := launchWireshark(bin, meshcoreLua, meshbenchLua); why != "" {
			w.Say("streaming to " + captureUDPAddr + ", but Wireshark would not start: " + why)
			out["launched"] = false
			out["launch_error"] = why
			return out, nil
		}
		out["launched"] = true
		s.SetCaptureLive(captureUDPAddr)
		w.Say("Wireshark is opening on " + captureUDPAddr)
		return out, nil
	})

	st.Handle("capture.stop", func(w *state.World, _ any) (any, error) {
		if s.Engine() == nil {
			return nil, fmt.Errorf("no network loaded")
		}
		path, frames, err := s.Engine().StopCapture()
		if err != nil {
			return nil, err
		}
		s.SetCapturePath("")
		s.SetCaptureLive("")
		w.Say(fmt.Sprintf("captured %d frames", frames))
		return map[string]any{"path": path, "frames": frames}, nil
	})
}

func init() { session.RegisterDomain(registerCapture) }
