// Building a MeshCore checkout, from a script.
//
// `meshbench dev` has always done this and lived outside the store, so a
// script comparing a stock build against a locally changed one had to shell
// out to a second copy of the binary. Both now call firmware.Build; this is
// the half that answers over the socket.
package session

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
)

// buildRoles is what "every role" means when a caller does not say.
//
// Both, from one request, on purpose. A locally built repeater compiled
// against a stale shim once answered console output with 0x06 where the host
// expects 0x07: it connected, misbehaved and exited. Two arms of a comparison
// built at different moments from different trees measure the build process,
// not the firmware - so the easy thing to do is the thing that builds them
// together.
var buildRoles = []string{"simple_repeater", "companion_radio"}

func registerFirmwareBuild(st *state.Store, s *Sim) {
	// Returns as soon as the work has started, like every other long job here.
	// A MeshCore build is a minute or two per role and the store's goroutine
	// is where every verb lands - blocking it would freeze the window, the
	// socket and the engine's clock together, which is exactly the failure
	// firmware.start was reported as a crash for.
	st.Handle("firmware.build", func(w *state.World, p any) (any, error) {
		src, _ := stringField(p, "source")
		if src == "" {
			src, _ = namedField(p, "from")
		}
		if src == "" {
			return nil, control.WithCode(control.BadParams, fmt.Errorf(
				"firmware.build needs a source: the top of a MeshCore checkout"))
		}
		roles := buildRoles
		if only, ok := namedField(p, "role"); ok && only != "" {
			roles = []string{only}
		}
		label, _ := namedField(p, "label")

		id := "firmware-build"
		w.Say("building " + strings.Join(roles, " and ") + " from " + src)
		go buildFirmware(st, id, src, label, roles)
		return map[string]any{"building": roles, "source": src, "job": id}, nil
	})
}

// buildFirmware runs the builds and reports through the job machinery.
func buildFirmware(st *state.Store, id, src, label string, roles []string) {
	ctx := context.Background()
	_, _ = st.Do(ctx, "job.progress", state.Job{
		ID: id, What: "building MeshCore from " + src, Total: len(roles)})

	built := map[string]any{}
	for i, role := range roles {
		// The toolchain's own output goes nowhere.
		//
		// A compiler's stderr is thousands of lines and the workbench's status
		// bar is one; putting it there would bury everything else the session
		// has to say. What a caller needs is whether it worked and what came
		// out, and on a failure the error carries the reason. `meshbench
		// dev` is still the way to watch a build.
		out, err := firmware.Build(ctx, firmware.BuildOptions{
			Source: src, Role: role, Label: label,
		})
		if err != nil {
			_, _ = st.Do(ctx, "job.done", id)
			_, _ = st.Do(ctx, "firmware.build_failed",
				fmt.Sprintf("%s: %v", role, err))
			return
		}
		built[role] = map[string]any{
			"version": out.Label, "role": out.Role,
			"path": out.Path, "bytes": out.Bytes,
		}
		_, _ = st.Do(ctx, "job.progress", state.Job{
			ID: id, What: "building MeshCore from " + src,
			Done: i + 1, Total: len(roles)})
	}
	_, _ = st.Do(ctx, "job.done", id)
	_, _ = st.Do(ctx, "firmware.built", built)
	// The library last, so a caller that waits on the job and then reads it
	// sees what was just built rather than what was there before.
	_, _ = st.Do(ctx, "firmware.library", nil)
}

func registerFirmwareBuildResults(st *state.Store, _ *Sim) {
	// It is already in the cache - firmware.Build imports it, which is the
	// same call `meshbench dev` makes - so there is nothing to add here.
	st.HandleInternal("firmware.built", func(w *state.World, p any) (any, error) {
		got, ok := p.(map[string]any)
		if !ok {
			return nil, wrongCallback("firmware.built")
		}
		var names []string
		for role, v := range got {
			m, _ := v.(map[string]any)
			if version, _ := m["version"].(string); version != "" {
				names = append(names, role+" "+version)
			}
		}
		sort.Strings(names)
		w.Say("built " + strings.Join(names, ", "))
		return map[string]any{"built": names}, nil
	})

	st.HandleInternal("firmware.build_failed", func(w *state.World, p any) (any, error) {
		w.Say("build failed: " + soleString(p))
		return nil, nil
	})
}
