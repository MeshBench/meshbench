// Package board holds the board-probe verbs - measuring the capability matrix
// one boot at a time (board.matrix, board.probe) and taking a screenshot of a
// running board's display (board.screenshot). Split out of internal/app/session;
// it reaches the running Sim through the accessors session exports.
package board

import "github.com/MeshBench/meshbench/internal/app/session"

func init() {
	session.RegisterDomain(registerBoardMatrix)
	session.RegisterDomain(registerBoardScreenshot)
}
