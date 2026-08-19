package environ

import (
	"math"
	"testing"
)

type bareGround struct{}

func (bareGround) ElevationM(_, _ float64) (float64, bool) { return 50, true }

// The index must be the package function, answered from buckets: same
// crossings, same prices, for paths that hit, graze and miss.
func TestPathIndexMatchesDirect(t *testing.T) {
	store := OpenTiles(t.TempDir())
	// Hand the index a fake provider via a tile write instead: build tiles.
	var blds []Building
	for i := 0; i < 40; i++ {
		lat := 56.30 + float64(i%8)*0.004
		lon := -3.30 + float64(i/8)*0.006
		blds = append(blds, Building{
			Footprint: [][2]float64{
				{lat, lon}, {lat + 0.0015, lon}, {lat + 0.0015, lon + 0.002}, {lat, lon + 0.002},
			},
			HeightM: 8 + float64(i%5)*3, Material: MatBrick,
		})
	}
	x, y, _ := TileFor(blds[0])
	if err := WriteTile(store.Dir, x, y, blds); err != nil {
		t.Fatal(err)
	}
	g := bareGround{}
	ix := NewPathIndex(store, g, 56.25, -3.35, 56.40, -3.20)
	if ix.Buildings() == 0 {
		t.Fatal("the index indexed nothing")
	}
	sc := &PathScratch{}
	paths := [][4]float64{
		{56.295, -3.305, 56.345, -3.255}, // diagonal through the estate
		{56.301, -3.299, 56.301, -3.250}, // straight along a row
		{56.20, -3.10, 56.22, -3.05},     // nowhere near anything
		{56.30, -3.31, 56.33, -3.28},
	}
	for _, p := range paths {
		want := PathBuildingLossDB(store, g, p[0], p[1], 60, p[2], p[3], 62, 5000, 869.618)
		got := ix.PathLossDB(sc, p[0], p[1], 60, p[2], p[3], 62, 5000, 869.618)
		if math.Abs(want-got) > 1e-9 {
			t.Fatalf("path %v: direct %.6f dB, indexed %.6f dB", p, want, got)
		}
	}
}

// The near-ends variant prices short paths exactly like the full walk, and
// what it deliberately skips - a building mid-way across a long path - is
// the few-decibel term the raster's own cell size cannot honestly resolve.
func TestNearEndsPricesTheEndsExactly(t *testing.T) {
	store := OpenTiles(t.TempDir())
	near := Building{Footprint: [][2]float64{
		{56.3005, -3.3005}, {56.3020, -3.3005}, {56.3020, -3.2985}, {56.3005, -3.2985}},
		HeightM: 15, Material: MatConcrete}
	x, y, _ := TileFor(near)
	if err := WriteTile(store.Dir, x, y, []Building{near}); err != nil {
		t.Fatal(err)
	}
	g := bareGround{}
	ix := NewPathIndex(store, g, 56.25, -3.35, 56.40, -3.20)
	sc := &PathScratch{}
	// Short path through the building: identical to the full walk.
	want := ix.PathLossDB(sc, 56.299, -3.302, 60, 56.304, -3.297, 62, 800, 869.618)
	got := ix.PathLossNearEndsDB(sc, 56.299, -3.302, 60, 56.304, -3.297, 62, 800, 869.618)
	if want <= 0 || math.Abs(want-got) > 1e-9 {
		t.Fatalf("short path: full %.4f dB, near-ends %.4f dB", want, got)
	}
	// Long path with the building near ITS START: still priced.
	if got := ix.PathLossNearEndsDB(sc, 56.301, -3.301, 60, 56.301, -2.90, 62, 25000, 869.618); got <= 0 {
		t.Fatal("a building beside the transmitter went unpriced")
	}
}

// The station view must find the same crossings the direct walk finds -
// same town, same rays - with the near-ends bound applied to both sides.
func TestStationPathsMatchesNearEnds(t *testing.T) {
	store := OpenTiles(t.TempDir())
	var blds []Building
	for i := 0; i < 30; i++ {
		lat := 56.298 + float64(i%6)*0.0025
		lon := -3.303 + float64(i/6)*0.003
		blds = append(blds, Building{
			Footprint: [][2]float64{
				{lat, lon}, {lat + 0.001, lon}, {lat + 0.001, lon + 0.0015}, {lat, lon + 0.0015}},
			HeightM: 10, Material: MatBrick})
	}
	x, y, _ := TileFor(blds[0])
	if err := WriteTile(store.Dir, x, y, blds); err != nil {
		t.Fatal(err)
	}
	g := bareGround{}
	ix := NewPathIndex(store, g, 56.25, -3.40, 56.40, -3.15)
	sp := ix.Station(56.300, -3.300)
	sc := &PathScratch{}
	cells := [][2]float64{
		{56.310, -3.290}, {56.305, -3.302}, {56.290, -3.320}, {56.320, -3.250},
	}
	for _, c := range cells {
		distM := 111320.0 * math.Hypot(c[0]-56.300, (c[1]+3.300)*math.Cos(56.3*math.Pi/180))
		want := ix.PathLossNearEndsDB(sc, 56.300, -3.300, 60, c[0], c[1], 62, distM, 869.618)
		got := sp.LossDB(sc, true, 60, c[0], c[1], 62, distM, 869.618)
		if math.Abs(want-got) > 1e-9 {
			t.Fatalf("cell %v: near-ends %.4f dB, station view %.4f dB", c, want, got)
		}
	}
}
