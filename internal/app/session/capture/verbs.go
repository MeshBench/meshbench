// Package capture holds the frame-capture verbs - writing a pcapng file,
// streaming to Wireshark, and stopping - and the Wireshark launch machinery
// they use. Split out of internal/app/session; it reaches the running Sim
// through the accessors session exports and registers its verbs from init.
package capture

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerCapture(st *state.Store, s *session.Sim) {
	// Diagnosing why a packet was not relayed needs the frame, not a window,
	// and on a driven session there is often nobody at the screen to look at
	// one.
	st.HandleSpec("capture.file", state.Spec{
		What: "write every receiver's view of every frame to a pcapng file as " +
			"the run happens, which is how the bytes are kept on a session with " +
			"nobody at the screen",
		Params: []state.Param{
			{Name: "path", Type: state.ParamString, Primary: true,
				What: "where to write it; absent writes meshbench-capture.pcapng " +
					"in the temporary directory, and whatever is at the path " +
					"already is replaced"},
		},
		Returns: []string{"path"},
		Answers: "It replaces any capture already running, file or stream, so " +
			"the frames go to one place at a time. Refused where no network is " +
			"loaded.",
		Example: &state.Example{
			Params: map[string]any{"path": "/tmp/lomond.pcapng"},
			What:   "keep the run's frames for later",
		},
	}, func(w *state.World, p any) (any, error) {
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
	st.HandleSpec("capture.wireshark", state.Spec{
		What: "stream the same frames as datagrams to 127.0.0.1:5555 and open " +
			"Wireshark on loopback filtered to that port, with both dissectors " +
			"loaded in the order that makes them read",
		Returns: []string{"addr", "how", "dissector_error", "dissector_warning",
			"launched", "launch_error"},
		Answers: "The stream is started before anything is launched and stays " +
			"up whatever happens next, so `launched` false is a window that did " +
			"not open rather than a capture that did not start: `how` is then " +
			"the command to run by hand. `dissector_error` says the Lua that " +
			"registers the port was not found beside the binary, and " +
			"`dissector_warning` that only the MeshCore half is missing. The " +
			"capture belongs to the engine as it stands, so it is started once " +
			"per session and not once per run: an engine rebuilt, or a workbench " +
			"restarted, is capturing nothing until this is called again.",
		Example: &state.Example{
			Params: map[string]any{}, What: "watch the mesh in Wireshark",
		},
	}, func(w *state.World, p any) (any, error) {
		if s.Engine() == nil {
			return nil, fmt.Errorf("no network loaded")
		}
		if err := s.Engine().StartCaptureUDP(captureUDPAddr); err != nil {
			return nil, err
		}
		meshcoreLua, meshbenchLua := dissectorFiles()
		out := map[string]any{
			"addr": captureUDPAddr, "how": wiresharkHint(meshcoreLua, meshbenchLua),
		}
		switch {
		case meshbenchLua == "":
			out["dissector_error"] = "tools/dissector/meshbench.lua was not found beside the binary"
		case meshcoreLua == "":
			out["dissector_warning"] = "tools/dissector/meshcore_dissector.lua was not found - " +
				"MeshBench's own metadata will show, the MeshCore frame inside it will not"
		}

		bin := wiresharkBinary()
		if bin == "" {
			w.Say("streaming frames to " + captureUDPAddr + " - Wireshark is not installed, so run: " +
				wiresharkHint(meshcoreLua, meshbenchLua))
			out["launched"] = false
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

	st.HandleSpec("capture.stop", state.Spec{
		What: "close whichever capture is running, file or stream, and say how " +
			"much of the run it caught",
		Returns: []string{"path", "frames"},
		Answers: "`path` is the file that was written, or the address that was " +
			"being streamed to. Both come back empty with `frames` zero where " +
			"nothing was capturing, which is not an error; the only refusal is " +
			"no network loaded.",
		Example: &state.Example{
			Params: map[string]any{}, What: "finish the capture and count it",
		},
	}, func(w *state.World, _ any) (any, error) {
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
