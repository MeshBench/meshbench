package terrain

import "testing"

// The cache is bounded. It was not, and a workbench left running over a
// national scenario reached 9.5 GB, 1.5 GB of it decoded elevation tiles.
func TestTheTileCacheIsBounded(t *testing.T) {
	s := &TileStore{MaxLoadedTiles: 8, loaded: map[string]*tile{}}
	for i := 0; i < 100; i++ {
		s.mu.Lock()
		s.remember(string(rune('a'+i%26))+string(rune('0'+i/26)), &tile{})
		s.mu.Unlock()
	}
	if got := s.LoadedTiles(); got != 8 {
		t.Fatalf("holding %d tiles with a cap of 8", got)
	}
}

// Re-reading a tile already held does not grow the cache or duplicate its
// place in the eviction order - which is what would happen if every read
// appended, and the cap would then evict live tiles while the map stayed full.
func TestReadingTheSameTileTwiceDoesNotGrowIt(t *testing.T) {
	s := &TileStore{MaxLoadedTiles: 4, loaded: map[string]*tile{}}
	for i := 0; i < 20; i++ {
		s.mu.Lock()
		s.remember("same", &tile{})
		s.mu.Unlock()
	}
	if got := s.LoadedTiles(); got != 1 {
		t.Fatalf("one tile read twenty times is being held %d times", got)
	}
	if got := len(s.order); got != 1 {
		t.Fatalf("the eviction order has %d entries for one tile", got)
	}
}

// Zero means the default rather than "hold nothing", because a store built
// without the field set is every existing caller.
func TestZeroMeansTheDefaultNotZero(t *testing.T) {
	s := &TileStore{loaded: map[string]*tile{}}
	for i := 0; i < 5; i++ {
		s.mu.Lock()
		s.remember(string(rune('a'+i)), &tile{})
		s.mu.Unlock()
	}
	if got := s.LoadedTiles(); got != 5 {
		t.Fatalf("a store with no cap set is holding %d of 5 tiles", got)
	}
}
