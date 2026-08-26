package gpu

import (
	"sync"
	"testing"
)

// TestOpenCloseConcurrent guards acquireMu. Several goroutines acquiring and
// releasing a device at once must not race the cgo.Handle the wgpu adapter and
// device requests register - the "misuse of an invalid Handle" panic from #188,
// which surfaced when the GPU probe, a warm and a coverage map all reached Open
// on a fresh session. Meaningful under -race with a real GPU; skipped otherwise,
// which is CI's normal case.
func TestOpenCloseConcurrent(t *testing.T) {
	d, err := Open()
	if err != nil {
		t.Skipf("no GPU available: %v", err)
	}
	d.Close()

	const goroutines, rounds = 4, 5
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < rounds; j++ {
				dev, err := Open()
				if err != nil {
					// A device that opened once may transiently refuse; the
					// property under test is that a refusal or a success never
					// panics, not that every acquire succeeds.
					continue
				}
				dev.Close()
			}
		}()
	}
	wg.Wait()
}
