package workbench

import (
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/ui/uitest"
)

// verbHarness is a workbench whose interface a verb can be called against
// while frames are drawn, which is the arrangement these verbs actually run in:
// the verb arrives on the store's goroutine, the frame loop owns the shell.
func verbHarness(t *testing.T) *workbenchUI {
	t.Helper()
	sh := shell.New()
	for n := range panelMenus {
		sh.Add(homed(shell.EmptyPanel(n, "for the verb test")))
	}
	return &workbenchUI{sh: sh, mv: &comp.MapView{}}
}

// A verb changes nothing until the frame goroutine takes it.
//
// The point of the split, stated as a test: if a verb wrote the shell itself it
// would be writing what the next frame is already reading.
func TestAVerbLeavesItsChangeForTheFrame(t *testing.T) {
	u := verbHarness(t)
	name := firstPanelName(t, u)

	if err := u.OpenPanel(name, ""); err != nil {
		t.Fatal(err)
	}
	if u.sh.Visible(name) {
		t.Fatal("the panel was docked on the verb's goroutine")
	}
	u.applyDeferred()
	if !u.sh.Visible(name) {
		t.Fatal("the frame did not apply what the verb left")
	}

	if err := u.ClosePanel(name); err != nil {
		t.Fatal(err)
	}
	if !u.sh.Visible(name) {
		t.Fatal("the panel was undocked on the verb's goroutine")
	}
	// Closing one that is not open is a no-op rather than a refusal: the
	// layout belongs to the frame, so a verb cannot answer for it.
	if err := u.ClosePanel(name); err != nil {
		t.Fatalf("closing twice should not refuse: %v", err)
	}
	u.applyDeferred()
	if u.sh.Visible(name) {
		t.Fatal("the frame did not apply the close")
	}
}

// A refusal is still immediate: what a verb can answer for on its own, it
// answers for at once rather than a frame later.
func TestAVerbStillRefusesAtOnce(t *testing.T) {
	u := verbHarness(t)
	if err := u.OpenPanel("no such panel", ""); err == nil {
		t.Fatal("wanted a refusal for a panel that does not exist")
	}
	if err := u.SetTool("teleport"); err == nil {
		t.Fatal("wanted a refusal for a tool that does not exist")
	}
	if err := u.SetLayer("gravity", true); err == nil {
		t.Fatal("wanted a refusal for a layer that does not exist")
	}
	// None of those should have queued anything to apply.
	u.workMu.Lock()
	queued := len(u.work)
	u.workMu.Unlock()
	if queued != 0 {
		t.Fatalf("a refused verb queued %d change(s)", queued)
	}
}

// The map's own fields go the same way.
func TestMapVerbsGoThroughTheFrameToo(t *testing.T) {
	u := verbHarness(t)
	u.FilterMap("Abernethy")
	if u.mv.Filter != "" {
		t.Fatal("the filter was written on the verb's goroutine")
	}
	if err := u.SetTool("measure"); err != nil {
		t.Fatal(err)
	}
	if u.mv.Tool != "" {
		t.Fatal("the tool was written on the verb's goroutine")
	}
	u.applyDeferred()
	if u.mv.Filter != "Abernethy" || u.mv.Tool != "measure" {
		t.Fatalf("the frame did not apply them: filter=%q tool=%q", u.mv.Filter, u.mv.Tool)
	}
}

// The race the split exists to remove: verbs arriving from the control socket
// while the frame loop reads the same state. Meaningful under -race.
func TestVerbsDoNotRaceTheFrame(t *testing.T) {
	u := verbHarness(t)
	name := firstPanelName(t, u)

	// Two groups on purpose: the frame goroutine runs until it is told to
	// stop, and it can only be told once the callers have finished. Putting it
	// in the same group as them would have wg.Wait block on a goroutine
	// waiting for a signal that comes after wg.Wait.
	var callers, frame sync.WaitGroup
	stop := make(chan struct{})

	// The frame goroutine: apply what is waiting, then read what it changed,
	// which is what Layout does.
	frame.Add(1)
	go func() {
		defer frame.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			u.applyDeferred()
			_ = u.sh.Visible(name)
			_ = u.sh.View
			_ = u.mv.Filter
			_ = u.mv.Tool
			_ = u.mv.Layers.Nodes
			// Yield rather than spin: under the race detector a busy loop
			// starves the very goroutines this is trying to interleave with.
			runtime.Gosched()
		}
	}()

	// Several callers, as two clients and the interface would be.
	for i := 0; i < 4; i++ {
		callers.Add(1)
		go func() {
			defer callers.Done()
			for n := 0; n < 40; n++ {
				_ = u.OpenPanel(name, "")
				u.FilterMap("query")
				_ = u.SetTool("select")
				_ = u.SetLayer("nodes", n%2 == 0)
				u.ResetLayout()
				_ = u.ClosePanel(name)
			}
		}()
	}

	callers.Wait()
	close(stop)
	frame.Wait()
	u.applyDeferred()
}

func firstPanelName(t *testing.T, u *workbenchUI) string {
	t.Helper()
	names := u.PanelNames()
	if len(names) == 0 {
		t.Fatal("the harness registered no panels")
	}
	return names[0]
}

// The flow, as a picture: a script opens a panel and switches a tool, and the
// next frame shows it. Written only when asked for, like the other shots here.
//
//	MESHBENCH_SHOTS=/tmp/shots go test ./internal/ui/workbench/ -run TestShotOfAVerbDrivenFlow
func TestShotOfAVerbDrivenFlow(t *testing.T) {
	dir := os.Getenv("MESHBENCH_SHOTS")
	if dir == "" {
		t.Skip("set MESHBENCH_SHOTS=<dir> to write the pictures")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	u := verbHarness(t)
	snap := &state.Snapshot{Nodes: []state.Node{
		{Name: "Abernethy Repeater", Kind: "simple-repeater", Selected: true},
		{Name: "AngusOutlaw1", Kind: "companion"},
	}}

	shot := func(step string) {
		img := uitest.RenderMode(t, 1400, 900, theme.Dark,
			func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				u.applyDeferred()
				u.sh.Layout(th, gtx, snap)
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		out := filepath.Join(dir, "verbflow-"+step+".png")
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", out)
	}

	shot("1-before")
	if err := u.OpenPanel("Events", ""); err != nil {
		t.Log("no Events panel in this build:", err)
	}
	shot("2-panel-opened")
	if err := u.SetTool("measure"); err != nil {
		t.Fatal(err)
	}
	u.FilterMap("Abernethy")
	shot("3-tool-and-filter")
}
