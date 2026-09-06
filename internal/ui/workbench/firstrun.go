// The one page the application opens without being asked, and only when
// something cannot run until it is seen to.
//
// A first launch is where every one of these problems is met and the worst
// place to meet them: the firmware cache is empty, nobody has answered the
// terrain question, and the emulator toolchain has never existed. Each of those
// used to announce itself later and separately - by a node failing to start, by
// a status line in the middle of a measurement - which is a sequence nobody
// designed and everybody walks through.
//
// So the check runs once at startup, and the page opens if, and only if,
// something is blocking. A machine that is set up sees nothing, which is what
// stops this from being a splash screen.
//
// A question nobody has answered is deliberately not enough to open it. The
// setup page distinguishes the two cases itself - "something in this session
// cannot run until the rows below are seen to" against "nothing is broken; one
// thing is waiting to be told what it may do" - and this used to open on the
// sum of them and then say the blocking sentence for both. On a machine whose
// only outstanding row was the update-check question, that put the page in
// front of the map on every launch, for ever, announcing a fault the page
// underneath it denied. The question is still worth mentioning, so it is said
// in the status bar in the words the page would use.
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
		if !ok {
			return
		}
		switch {
		case setupCount(m, "needed") > 0:
			if _, err := st.Do(ctx, "panel.open",
				map[string]any{"name": "Setup"}); err != nil {
				return
			}
			_, _ = st.Do(ctx, "ui.said", "this machine is not set up yet: Setup "+
				"lists what is missing, what each one costs, and what to do about "+
				"the ones nothing here can fetch")
		case setupCount(m, "undecided") > 0:
			_, _ = st.Do(ctx, "ui.said", "nothing is broken; Setup has one "+
				"question nothing has answered on your behalf")
		}
	}()
}

func setupCount(m map[string]any, key string) int {
	n, _ := m[key].(int)
	return n
}
