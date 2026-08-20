// Package meshtest runs a MeshCore network inside your test.
//
// It is the one package here that is not under internal/, because it exists to
// be imported by other people's projects: a companion app's test suite, a
// firmware repository's CI, anything that needs a mesh to talk to and would
// otherwise need a bench full of radios.
//
// What your test gets is a real one. Every node runs MeshCore's own firmware -
// routing, flood suppression, duty-cycle policing and CSMA timing are the
// shipped code, not a model of it - and the frames between them cross a
// sample-accurate LoRa channel over real terrain. A packet that would be lost
// on a hillside is lost here.
//
// Two properties make it usable as a test rather than as a demonstration.
//
// Time is yours. Nothing advances until Advance is called, so a test is not a
// race against a wall clock and does not need a sleep to be reliable. Advance
// exactly as far as the behaviour needs and assert.
//
// And it is deterministic: the same seed and the same fixture produce the same
// run, every time, on every machine. A failure you can reproduce is a failure
// somebody can fix.
//
//	func TestMyAppSurvivesAQuietMesh(t *testing.T) {
//		m, err := meshtest.Start(context.Background(), meshtest.Options{})
//		if err != nil {
//			t.Fatal(err)
//		}
//		defer m.Close()
//
//		conn, err := net.Dial("tcp", m.Endpoint())   // your client, unmodified
//		if err != nil {
//			t.Fatal(err)
//		}
//		defer conn.Close()
//
//		if err := m.Advance(30 * time.Second); err != nil {
//			t.Fatal(err)
//		}
//		if got := len(m.Received("")); got == 0 {
//			t.Fatalf("nothing was heard anywhere in thirty seconds")
//		}
//	}
//
// The mesh needs firmware, and firmware is downloaded on first use. Set
// Offline to fail loudly instead of reaching the network - which is what a CI
// runner without egress should do, once its cache is warm.
package meshtest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MeshBench/meshbench/internal/app/fixture"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// Options is what a test chooses. The zero value is a working mesh.
type Options struct {
	// Fixture is a shipped network's name or a path to one. Empty picks the
	// smallest shipped network, which is what a test that only needs "a mesh"
	// should use - it starts fastest and its behaviour is the least
	// surprising.
	Fixture string

	// Seed overrides the fixture's own. Change it to ask whether a result was
	// luck; leave it to get the same run every time.
	Seed uint64

	// Companion names the node your client attaches to. Empty picks the first
	// companion in the network.
	Companion string

	// Serial exposes a virtual serial port instead of TCP, for a client that
	// speaks to a USB radio and should not have to learn a socket.
	Serial bool

	// Addr is where the TCP endpoint listens. Empty means 127.0.0.1 on a port
	// the operating system picks, which is what a test wants: two tests can
	// run at once.
	Addr string

	// Offline refuses to download anything - firmware, terrain - and fails
	// with what is missing rather than reaching the network. A CI runner with
	// a warm cache and no egress wants this.
	Offline bool

	// TerrainCache overrides where elevation tiles are read from. Empty uses
	// the same cache the application does.
	TerrainCache string
}

// Mesh is a running network. Close it when the test is done.
type Mesh struct {
	e    *engine.Engine
	link *engine.CompanionLink
	fx   *fixture.Fixture
}

// Start loads a network, starts real firmware on every node, and exposes one
// companion for a client to attach to.
//
// It returns once the mesh is ready and *before* any simulated time has
// passed, so a client can connect and be listening before the first packet
// exists.
func Start(ctx context.Context, opts Options) (*Mesh, error) {
	fx, err := loadFixture(opts.Fixture)
	if err != nil {
		return nil, err
	}
	seed := fx.Seed
	if opts.Seed != 0 {
		seed = opts.Seed
	}

	store, err := terrainStore(opts)
	if err != nil {
		return nil, err
	}

	sf, bw, freq := radioOf(fx)
	e := engine.New(store, engine.Config{
		FreqMHz: freq, SF: sf, BandwidthHz: bw, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: seed,
	})
	for _, n := range fx.Nodes {
		e.Add(n, nil)
	}
	if err := e.AttachNative(ctx, seed); err != nil {
		_ = e.Close()
		return nil, fmt.Errorf("meshtest: starting firmware: %w", err)
	}

	m := &Mesh{e: e, fx: fx}
	if err := m.serveCompanion(opts); err != nil {
		_ = e.Close()
		return nil, err
	}
	return m, nil
}

// Endpoint is where your client attaches: a "host:port" for TCP, or the path
// of a virtual serial device.
func (m *Mesh) Endpoint() string { return m.link.Addr }

// CompanionNode is the node the endpoint belongs to.
func (m *Mesh) CompanionNode() string { return m.link.Node }

// Advance runs the simulation forward by d.
//
// Nothing happens between calls. That is the point: a test controls time
// instead of racing it, so it needs no sleep and cannot be flaky for want of
// one. Traffic your client sent before the call is delivered during it.
func (m *Mesh) Advance(d time.Duration) error {
	if d <= 0 {
		return nil
	}
	until := m.e.NowMs() + uint32(d.Milliseconds())
	if err := m.e.Run(context.Background(), until); err != nil {
		return fmt.Errorf("meshtest: advancing %s: %w", d, err)
	}
	return nil
}

// Elapsed is how much simulated time has passed.
func (m *Mesh) Elapsed() time.Duration {
	return time.Duration(m.e.NowMs()) * time.Millisecond
}

// Nodes names every node in the network.
func (m *Mesh) Nodes() []string {
	out := make([]string, 0, len(m.fx.Nodes))
	for _, n := range m.fx.Nodes {
		out = append(out, n.Name)
	}
	return out
}

// Reception is one packet arriving at one node.
type Reception struct {
	At    time.Duration
	From  string
	To    string
	SNRdB float64
	// Decoded is whether the receiver got the bytes. A reception that did not
	// decode is still worth seeing: it is the difference between "out of
	// range" and "collided", which is usually the question.
	Decoded bool
}

// Received is what node heard, or everything if node is empty.
//
// Failed receptions are included and marked, because a test that only counts
// successes cannot tell a quiet mesh from a colliding one.
func (m *Mesh) Received(node string) []Reception {
	var out []Reception
	for _, ev := range m.e.Events() {
		if ev.Kind != "rx" && ev.Kind != "miss" {
			continue
		}
		if node != "" && ev.To != node {
			continue
		}
		out = append(out, Reception{
			At:      time.Duration(ev.AtMs) * time.Millisecond,
			From:    ev.From,
			To:      ev.To,
			SNRdB:   ev.SNRdB,
			Decoded: ev.Kind == "rx",
		})
	}
	return out
}

// Transmissions is what node put on the air, or everything if node is empty.
func (m *Mesh) Transmissions(node string) []Reception {
	var out []Reception
	for _, ev := range m.e.Events() {
		if ev.Kind != "tx" {
			continue
		}
		if node != "" && ev.From != node {
			continue
		}
		out = append(out, Reception{
			At:   time.Duration(ev.AtMs) * time.Millisecond,
			From: ev.From, Decoded: true,
		})
	}
	return out
}

// Close stops the firmware and releases the endpoint.
func (m *Mesh) Close() error { return m.e.Close() }

// Engine is the simulation underneath, for a test that needs something this
// package does not offer.
//
// Exported deliberately, and with a warning: it is internal API and it moves.
// Anything reached through here can break in a patch release. If you find
// yourself needing it, that is worth an issue - the answer is usually a method
// on Mesh.
func (m *Mesh) Engine() *engine.Engine { return m.e }

func (m *Mesh) serveCompanion(opts Options) error {
	target := opts.Companion
	if target == "" {
		for _, n := range m.fx.Nodes {
			if n.Kind.Application() == "companion_radio" {
				target = n.Name
				break
			}
		}
	}
	if target == "" {
		return fmt.Errorf("meshtest: %s has no companion node to expose; "+
			"name one with Options.Companion, or use a fixture that has one",
			m.fx.Name)
	}
	addr := opts.Addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	var err error
	if opts.Serial {
		m.link, err = m.e.ServeCompanionSerial(target)
	} else {
		m.link, err = m.e.ServeCompanionTCP(target, addr)
	}
	if err != nil {
		return fmt.Errorf("meshtest: exposing %s: %w", target, err)
	}
	return nil
}

func loadFixture(name string) (*fixture.Fixture, error) {
	if name == "" {
		name = defaultFixture
	}
	p, _, err := fixture.Find(name)
	if err != nil {
		return nil, fmt.Errorf("meshtest: %w", err)
	}
	fx, err := fixture.Load(p)
	if err != nil {
		return nil, fmt.Errorf("meshtest: loading %s: %w", p, err)
	}
	return fx, nil
}

func terrainStore(opts Options) (*terrain.TileStore, error) {
	dir := opts.TerrainCache
	if dir == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("meshtest: no cache directory: %w", err)
		}
		// The same place the application keeps them, so a test on a machine
		// that has run MeshBench does not download the country again.
		dir = filepath.Join(cache, "meshcoresim", "terrain")
	}
	ts, err := terrain.NewTileStore(dir)
	if err != nil {
		return nil, fmt.Errorf("meshtest: terrain cache at %s: %w", dir, err)
	}
	ts.Offline = opts.Offline
	ts.Zoom = terrain.DefaultZoom
	return ts, nil
}

// defaultFixture is the smallest shipped network: it starts fastest, and a
// test that only needs "a mesh" should not pay for a country.
const defaultFixture = "fife-strict"

func radioOf(fx *fixture.Fixture) (sf int, bandwidthHz, freqMHz float64) {
	sf, bandwidthHz, freqMHz = 8, 62.5e3, 869.618
	for _, n := range fx.Nodes {
		if n.Radio.SpreadFactor > 0 {
			sf = n.Radio.SpreadFactor
		}
		if n.Radio.BandwidthHz > 0 {
			bandwidthHz = n.Radio.BandwidthHz
		}
		if n.Radio.CentreHz > 0 {
			freqMHz = float64(n.Radio.CentreHz) / 1e6
		}
		break
	}
	return sf, bandwidthHz, freqMHz
}
