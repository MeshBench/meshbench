package basemap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// cartoAgainst points the CARTO dark layer at a test server, so a fetch fails
// the way a real one would without leaving the machine.
func cartoAgainst(t *testing.T, h http.HandlerFunc) (*Store, Layer) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	var carto Layer
	for _, l := range Layers() {
		if l.ID == "carto-dark" {
			carto = l
		}
	}
	// The key is appended to anything on cartocdn.com, so the substitute has to
	// look like it to exercise the path that carries one.
	carto.URL = srv.URL + "/cartocdn.com/{z}/{x}/{y}.png"
	return &Store{
		CacheDir:  t.TempDir(),
		UserAgent: "meshbench-test",
		HTTP:      srv.Client(),
	}, carto
}

// The key rides on the request as a query parameter, so any message built from
// the URL carries it to wherever the error is printed. A fetch error names the
// tile instead.
func TestAFetchErrorDoesNotCarryTheKey(t *testing.T) {
	const key = "sekrit-key-9f3a"
	t.Setenv("MESHBENCH_CARTO_KEY", key)

	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"a refusal", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}},
		{"a body that is not an image", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("over quota"))
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, carto := cartoAgainst(t, c.handler)

			err := s.Fetch(context.Background(), carto, 6, 31, 20)
			if err == nil {
				t.Fatal("wanted an error from a failed fetch")
			}
			if strings.Contains(err.Error(), key) {
				t.Fatalf("the key reached the error message: %v", err)
			}
			if strings.Contains(err.Error(), "http://") {
				t.Fatalf("the request URL reached the error message: %v", err)
			}
			// Still has to say which tile, or the message is useless.
			for _, want := range []string{"carto-dark", "6/31/20"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the error does not name %q: %v", want, err)
				}
			}
		})
	}
}

// The same for a transport failure, which is the most common of the three and
// the one a rate-limited or offline machine hits.
func TestATransportErrorDoesNotCarryTheKey(t *testing.T) {
	const key = "sekrit-key-9f3a"
	t.Setenv("MESHBENCH_CARTO_KEY", key)

	s, carto := cartoAgainst(t, func(http.ResponseWriter, *http.Request) {})
	// Closing the server under the client is the cheapest real transport error.
	s.HTTP = &http.Client{Transport: &http.Transport{}}
	carto.URL = "http://127.0.0.1:1/cartocdn.com/{z}/{x}/{y}.png"

	err := s.Fetch(context.Background(), carto, 6, 31, 20)
	if err == nil {
		t.Fatal("wanted an error when nothing is listening")
	}
	if strings.Contains(err.Error(), key) {
		t.Fatalf("the key reached the error message: %v", err)
	}
	if !strings.Contains(err.Error(), "carto-dark") {
		t.Errorf("the error does not name the layer: %v", err)
	}
}

func TestTileNameIdentifiesWithoutTheURL(t *testing.T) {
	l := Layer{ID: "carto-dark", URL: "https://basemaps.cartocdn.com/{z}/{x}/{y}.png"}
	got := tileName(l, 6, 31, 20)
	if got != "carto-dark tile 6/31/20" {
		t.Fatalf("unexpected description: %q", got)
	}
	if strings.Contains(got, "http") || strings.Contains(got, "key") {
		t.Fatalf("the description carries the request: %q", got)
	}
}
