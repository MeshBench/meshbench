// The standalone workbench binary. The application itself lives in
// internal/workbench; `meshcoresim workbench` is the same code.
package main

import (
	"os"

	"github.com/A13xB0/meshcoresim/internal/workbench"
)

func main() {
	workbench.Run(os.Args[1:])
}
