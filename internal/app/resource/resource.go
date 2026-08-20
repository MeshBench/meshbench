// Package resource is everything the application downloads at runtime, in one
// place that can list it, price it and remove it.
//
// Six things fetch from the network as the application runs - terrain,
// basemaps, building footprints, boundaries, firmware and, once they exist,
// emulators and the SoftDevice - and until now each had its own cache, its
// own conventions, and its own idea of whether the operator was allowed to
// know. On the machine this was written for, the terrain cache had reached
// 7.1 GB and there was no way to see it, let alone delete it, without opening
// a terminal.
//
// Firmware deliberately keeps its own library. It knows about roles, boards
// and which nodes are pinned to what, and a generic row would lose all three;
// the two are siblings in the same menu rather than one list serving neither.
package resource

import "context"

// Kind groups rows that came from the same place and behave the same way.
type Kind string

const (
	Terrain   Kind = "terrain"
	Basemap   Kind = "basemap"
	Buildings Kind = "buildings"
	Boundary  Kind = "boundary"
)

// State is what a row can be, and the set is deliberately small.
//
// Present and InUse are different answers: a building set that is downloaded
// but not being priced into links is kept, not deleted, and an operator
// switching between two of them is running an experiment rather than
// discarding one.
type State string

const (
	// OnDisk means cached and usable.
	OnDisk State = "on disk"
	// InUse means cached and currently feeding the simulation - only
	// meaningful where the engine can hold one at a time.
	InUse State = "in use"
	// Available means it can be fetched but has not been.
	Available State = "available"
	// Needed means this scenario cannot run correctly without it, which is
	// the only state that is urgent.
	Needed State = "needed"
	// Unavailable means nothing can be fetched here - no build for this
	// platform, no published artefact - and the row says why rather than
	// offering a button that would fail.
	Unavailable State = "unavailable"
)

// Row is one resource, as the panel and the verbs see it.
type Row struct {
	Kind Kind
	// Name identifies the row to a person; Version distinguishes two of the
	// same thing where that idea applies, and is empty where it does not.
	Name    string
	Version string
	// Path is where it lives on disk, empty when nothing is cached. A remove
	// acts on this rather than on the name, because the name is a label.
	Path string
	// Bytes is measured when Cached and estimated otherwise, and Estimated
	// says which - a guess presented as a survey is the thing this package
	// exists to stop.
	Bytes     int64
	Estimated bool
	State     State
	// Why carries the reason for anything that is not a plain on-disk row:
	// what it is needed for, or why it cannot be had.
	Why string
	// Auto is whether the application may fetch this without asking. Not the
	// same as enabled - presence is the state above - but whether bandwidth
	// may be spent on the operator's behalf.
	Auto bool
}

// Provider is one source of rows. Written as an interface because there are
// six implementations and not because there might be.
type Provider interface {
	// Kind is what this provider produces, and names its rows in the panel.
	Kind() Kind
	// List reads what is on disk. It never touches the network: opening the
	// panel must not start a download.
	List(ctx context.Context) ([]Row, error)
	// Remove deletes one row's bytes. The caller has already confirmed.
	Remove(ctx context.Context, row Row) error
}
