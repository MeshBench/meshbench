// The boards this simulator knows, assembled from one file each.
//
// The list is the only thing here. Each board's figures live in its own
// board_<name>.go, so editing one board is a change to one file and cannot
// disturb the others - which it did, twice, when they shared a slice.
package scenario

var boards = []Board{
	heltecV3Board,
	genericE22Sx1262Board,
	heltecT096Board,
	heltecV2Board,
	heltecMeshSolarBoard,
	xiaoS3WIOBoard,
	xiaoNrf52Board,
	heltecT114Board,
	stationG2Board,
	rak4631Board,
}
