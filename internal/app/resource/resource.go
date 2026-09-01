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
	// Fetchable is whether this row can be downloaded on demand from the
	// panel. A SoftDevice can: it is one file at a known address. A terrain
	// cache cannot, because what to fetch depends on where the operator is
	// looking - it fills as the map is used, and the panel says so rather
	// than offering a button that has nothing to ask for.
	Fetchable bool
	// HowTo is how this is obtained when it cannot be obtained from here, and
	// the row is not finished without it.
	//
	// A row that names a missing thing and stops is a dead end: building
	// footprints sat at nothing with Fetch disabled and the words "fills itself
	// as the map is used" beside it, which was true of terrain and false of
	// them - they are pulled from Configuration > Environ, and nothing on the
	// page said so.
	HowTo string
	// HowToPanel is the page that does have it, where there is one. The row
	// carries the name and the interface decides what opening it means, which
	// is the only part of this a provider has no business knowing.
	HowToPanel string
	// Licensed is whether there are terms to show. Somebody else's data comes
	// with somebody else's conditions, and the panel is where they belong.
	Licensed bool
}

// Provider is one source of rows.
type Provider interface {
	// Kind is what this provider produces, and names its rows in the panel.
	Kind() Kind
	// List reads what is on disk. It never touches the network: opening the
	// panel must not start a download.
	List(ctx context.Context) ([]Row, error)
	// Remove deletes one row's bytes. The caller has already confirmed.
	Remove(ctx context.Context, row Row) error
}

// Fetcher is a provider that can also download on demand.
//
// Separate from Provider because most cannot: terrain and basemaps fill
// themselves as the map is used and have nothing to ask for out of context,
// so requiring them to implement a Fetch would mean writing one that refuses.
// Licensor is a provider that can produce the terms its bytes arrived under.
type Licensor interface {
	Licence(name, version string) string
}

type Fetcher interface {
	Fetch(ctx context.Context, name, version string, progress func(done, total int64)) error
}
