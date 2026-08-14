// The standalone workbench binary. The application itself lives in
// internal/workbench; `meshcoresim workbench` is the same code.
package main

import (
	"os"

	"github.com/MeshBench/meshbench/internal/workbench"
)

func main() {
	workbench.Run(os.Args[1:])
}
