// The one page the application opens without being asked, and only when it has
// something to say.
//
// A first launch is where every one of these problems is met and the worst
// place to meet them: the firmware cache is empty, nobody has answered the
// terrain question, and the emulator toolchain has never existed. Each of those
// used to announce itself later and separately - by a node failing to start, by
// a status line in the middle of a measurement - which is a sequence nobody
// designed and everybody walks through.
//
// So the check runs once at startup and the page opens if, and only if,
// something is blocking or something is waiting to be told. A machine that is
// set up sees nothing, which is what stops this from being a splash screen.
package workbench

import (
	"context"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// firstRunDelay is how long the check waits for a window to exist.
//
// A panel docked before the layout is built lands nowhere, and a check run
// before the fixture is open reads a session with no nodes and calls every
// missing firmware optional.
const firstRunDelay = 3 * time.Second

func openSetupIfNotReady(ctx context.Context, st *state.Store) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(firstRunDelay):
		}
		v, err := st.Do(ctx, "setup.check", nil)
		if err != nil {
			return
		}
		m, ok := v.(map[string]any)
		if !ok || !setupIsWanting(m) {
			return
		}
		if _, err := st.Do(ctx, "panel.open",
			map[string]any{"name": "Setup"}); err != nil {
			return
		}
		_, _ = st.Do(ctx, "ui.said", "this machine is not set up yet: Setup "+
			"lists what is missing, what each one costs, and what to do about "+
			"the ones nothing here can fetch")
	}()
}

// setupIsWanting reads the check's own counts rather than the rows, so the page
// and this agree on what unready means without either counting for itself.
func setupIsWanting(m map[string]any) bool {
	return setupCount(m, "needed")+setupCount(m, "undecided") > 0
}

func setupCount(m map[string]any, key string) int {
	n, _ := m[key].(int)
	return n
}
