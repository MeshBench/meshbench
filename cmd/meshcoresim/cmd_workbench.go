package main

import (
	"context"

	"github.com/MeshBench/meshbench/internal/workbench"
)

// runWorkbench opens the desktop application - the Gio workbench, since the
// cutover. The imgui one it replaced is still there as `workbench1`, until it
// is not.
//
// The workbench is where a scenario is built and interrogated: the map is the
// main view, the panels are around it, and everything the other commands do
// headlessly is reachable by clicking. It needs a display and a GPU, which is
// why every capability also has a command - a scripted run, a regression
// suite and the MCP server are all built on the headless path, and that is
// not a stopgap for this.
func runWorkbench(_ context.Context, args []string) error {
	workbench.Run(args)
	return nil
}
