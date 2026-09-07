package firmwarelib

import "testing"

// One id per thing being downloaded.
//
// The board was left out, so every board image for one role and version shared
// a row: downloading three boards in turn left three rows under one id, the
// second download's progress overwrote the first's "finished", the finished
// row came back to life as unfinished, and job.list never went idle again for
// the rest of the session.
func TestTheDownloadJobIDNamesTheBoard(t *testing.T) {
	seen := map[string]string{}
	for _, b := range []string{"Heltec_v3", "Generic_E22_sx1262", "Tbeam_SX1262"} {
		id, _ := downloadJob("simple_repeater", "v1.17.1", b)
		if was, dup := seen[id]; dup {
			t.Errorf("%s and %s share the job id %q", was, b, id)
		}
		seen[id] = b
	}

	// A host build has no board, and its id keeps the shape it had.
	id, what := downloadJob("simple_repeater", "v1.17.1", "")
	if id != "fw-v1.17.1-simple_repeater" {
		t.Errorf("a host build's job id is %q, want the unchanged shape", id)
	}
	if what != "downloading simple_repeater v1.17.1" {
		t.Errorf("a host build's job says %q", what)
	}

	// And a board's row says which board, since the point is telling them apart.
	_, what = downloadJob("simple_repeater", "v1.17.1", "Heltec_v3")
	if want := "downloading simple_repeater v1.17.1 for Heltec_v3"; what != want {
		t.Errorf("the row says %q, want %q", what, want)
	}
}
