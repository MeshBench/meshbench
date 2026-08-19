package provider_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/world/provider"
)

type fakeHTTP struct {
	status int
	body   string
	gotURL string
}

func (f *fakeHTTP) Do(req *http.Request) (*http.Response, error) {
	f.gotURL = req.URL.String()
	s := f.status
	if s == 0 {
		s = 200
	}
	return &http.Response{
		StatusCode: s,
		Status:     http.StatusText(s),
		Body:       io.NopCloser(strings.NewReader(f.body)),
	}, nil
}

// A node with no position is still a node. Dropping it loses the fact that it
// exists; giving it (0,0) puts it in the Atlantic off Ghana, and nothing
// downstream can tell that apart from a real position.
func TestMissingPositionIsNotZero(t *testing.T) {
	h := &fakeHTTP{body: `{"nodes":[
		{"name":"GB7XYZ","public_key":"aa","lat":56.7,"lon":-3.9,"position_accuracy_m":50},
		{"name":"unknown-1","public_key":"bb"}
	]}`}
	cs := &provider.CoreScope{BaseURL: "http://cs.test", HTTP: h}

	nodes, err := cs.Nodes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2 — a node without a position was dropped", len(nodes))
	}
	if nodes[1].HasPosition {
		t.Error("a node with no lat/lon was given one")
	}
	if nodes[0].UncertaintyKm != 0.05 {
		t.Errorf("50 m accuracy became %.3f km", nodes[0].UncertaintyKm)
	}
}

// An inferred position is not a survey. Recording it as exact lets it win the
// merge against a surveyed one, which is precisely backwards.
func TestInferredPositionsCarryUncertainty(t *testing.T) {
	h := &fakeHTTP{body: `{"nodes":[
		{"name":"guessed","lat":56.7,"lon":-3.9,"position_is_fix":false}
	]}`}
	cs := &provider.CoreScope{BaseURL: "http://cs.test", HTTP: h}
	nodes, err := cs.Nodes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if nodes[0].UncertaintyKm < 1 {
		t.Errorf("an inferred position claimed %.3f km accuracy", nodes[0].UncertaintyKm)
	}
}

// Zero is a real SNR. Treating a missing one as 0 dB, or a 0 dB one as missing,
// are both wrong and neither looks it in a scatter plot.
func TestAbsentSNRIsDistinctFromZero(t *testing.T) {
	h := &fakeHTTP{body: `{"receptions":[
		{"time":"2026-08-09T10:00:00Z","receiver":"a","packet_id":"p1","snr":0},
		{"time":"2026-08-09T10:00:01Z","receiver":"b","packet_id":"p1"}
	]}`}
	cs := &provider.CoreScope{BaseURL: "http://cs.test", HTTP: h}
	rx, err := cs.Receptions(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !rx[0].HasSNR || rx[0].SNRdB != 0 {
		t.Error("an explicit 0 dB SNR was read as missing")
	}
	if rx[1].HasSNR {
		t.Error("a missing SNR was read as 0 dB")
	}
	// Both heard the same packet, which is the join that makes a reception set
	// worth more than a count.
	if rx[0].PacketID != rx[1].PacketID {
		t.Error("the same packet heard twice did not keep one identity")
	}
}

// Guessing seconds versus milliseconds wrong by a factor of a thousand puts a
// reception in 1970 or in the year 56000, and a replay just orders everything
// wrongly rather than failing.
func TestTimeFormatsAllLandInTheRightCentury(t *testing.T) {
	body := `{"receptions":[
		{"time":"2026-08-09T10:00:00Z","receiver":"a","packet_id":"p1"},
		{"time":1786370400,"receiver":"b","packet_id":"p2"},
		{"time":1786370400000,"receiver":"c","packet_id":"p3"},
		{"time":"1786370400","receiver":"d","packet_id":"p4"}
	]}`
	cs := &provider.CoreScope{BaseURL: "http://cs.test", HTTP: &fakeHTTP{body: body}}
	rx, err := cs.Receptions(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rx {
		if r.At.Year() < 2020 || r.At.Year() > 2100 {
			t.Errorf("%s parsed to %s", r.Receiver, r.At)
		}
	}
}

// An HTML login page decoded as JSON gives "invalid character '<'", which says
// nothing about what went wrong. The error has to quote what came back.
func TestErrorsQuoteTheResponse(t *testing.T) {
	cs := &provider.CoreScope{BaseURL: "http://cs.test", HTTP: &fakeHTTP{body: "<!doctype html>\n<title>Sign in</title>"}}
	_, err := cs.Nodes(context.Background())
	if err == nil {
		t.Fatal("an HTML page was accepted as a node list")
	}
	if !strings.Contains(err.Error(), "doctype") {
		t.Errorf("error does not quote the body: %v", err)
	}

	cs401 := &provider.CoreScope{BaseURL: "http://cs.test", HTTP: &fakeHTTP{status: 401, body: `{"error":"bad token"}`}}
	if _, err := cs401.Nodes(context.Background()); err == nil || !strings.Contains(err.Error(), "bad token") {
		t.Errorf("a 401 did not surface its reason: %v", err)
	}
}

// The merge exists to take the better half of each source. A fresh
// town-accuracy fix must not displace a surveyed position from last year, and
// recency must still win for LastSeen.
func TestMergePrefersCertaintyOverRecency(t *testing.T) {
	old := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	surveyed := []provider.NodeRecord{{
		Name: "GB7XYZ", PublicKey: "AA", HasPosition: true, Lat: 56.7, Lon: -3.9,
		UncertaintyKm: 0.05, LastSeen: old, Source: "corescope", HeightAGLm: 12,
	}}
	inferred := []provider.NodeRecord{{
		Name: "gb7xyz", PublicKey: "aa", HasPosition: true, Lat: 56.9, Lon: -4.1,
		UncertaintyKm: 5, LastSeen: recent, Source: "beacon", Kind: "repeater",
	}}

	merged := provider.MergeNodes(surveyed, inferred)
	if len(merged) != 1 {
		t.Fatalf("joined into %d records; the public key should have matched despite the case", len(merged))
	}
	m := merged[0]
	if m.Lat != 56.7 || m.UncertaintyKm != 0.05 {
		t.Errorf("a 5 km position displaced a 50 m one: %+v", m)
	}
	if !m.LastSeen.Equal(recent) {
		t.Errorf("LastSeen is %s, want the more recent %s", m.LastSeen, recent)
	}
	// Facts only one source had must survive.
	if m.Kind != "repeater" || m.HeightAGLm != 12 {
		t.Errorf("merge lost a field only one source knew: %+v", m)
	}
}

func TestMergeKeepsPositionlessNodes(t *testing.T) {
	merged := provider.MergeNodes(
		[]provider.NodeRecord{{Name: "a"}},
		[]provider.NodeRecord{{Name: "b", HasPosition: true, Lat: 1, Lon: 2}},
	)
	if len(merged) != 2 {
		t.Fatalf("got %d records, want 2", len(merged))
	}
	if merged[0].HasPosition {
		t.Error("a node with no position acquired one in the merge")
	}
}

type fakeMQTT struct{ msgs []provider.Message }

func (f *fakeMQTT) Subscribe(ctx context.Context, _ string, fn func(provider.Message)) error {
	for _, m := range f.msgs {
		fn(m)
	}
	<-ctx.Done()
	return ctx.Err()
}

// A live feed carrying one bad payload must not take the run down, and a
// message with nothing to join on is worse than useless — it can only be
// counted.
func TestMQTTDropsUnjoinableMessages(t *testing.T) {
	good, _ := json.Marshal(map[string]any{
		"time": "2026-08-09T10:00:00Z", "receiver": "a", "packet_id": "p1", "snr": -7.5,
	})
	msgs := []provider.Message{
		{Topic: "meshcore/a/rx", Payload: good},
		{Topic: "meshcore/a/rx", Payload: []byte("not json at all")},
		{Topic: "meshcore/a/rx", Payload: []byte(`{"receiver":"b"}`)}, // no packet id
	}

	m := &provider.MQTT{Client: &fakeMQTT{msgs: msgs}, Retain: 10}
	ctx, cancel := context.WithCancel(context.Background())

	var got []provider.Reception
	done := make(chan struct{})
	go func() {
		_ = m.Subscribe(ctx, func(r provider.Reception) { got = append(got, r) })
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	if len(got) != 1 {
		t.Fatalf("delivered %d receptions, want 1", len(got))
	}
	if !got[0].HasSNR || got[0].SNRdB != -7.5 {
		t.Errorf("SNR did not survive: %+v", got[0])
	}

	retained, err := m.Receptions(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(retained) != 1 {
		t.Errorf("retained %d, want 1", len(retained))
	}
}

// A live feed is not a node registry, and saying so with an empty list rather
// than an error means a caller merging several sources needs no special case.
func TestMQTTHasNoNodeRegistry(t *testing.T) {
	nodes, err := (&provider.MQTT{}).Nodes(context.Background())
	if err != nil {
		t.Fatalf("a live feed should not error on Nodes: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("got %d nodes from a packet feed", len(nodes))
	}
}

func TestRegistryRefusesDuplicates(t *testing.T) {
	r := provider.NewRegistry()
	if err := r.Register(&provider.CoreScope{BaseURL: "http://a"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(&provider.CoreScope{BaseURL: "http://b"}); err == nil {
		t.Fatal("two sources quietly shared a name")
	}
	if err := r.Register(&provider.Beacon{BaseURL: "http://c"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get("CoreScope"); err != nil {
		t.Errorf("lookup should be case-insensitive: %v", err)
	}
	_, err := r.Get("nonesuch")
	if err == nil || !strings.Contains(err.Error(), "beacon") {
		t.Errorf("an unknown name should list what is available: %v", err)
	}
}

func TestSinceIsPassedToTheSource(t *testing.T) {
	h := &fakeHTTP{body: `{"receptions":[]}`}
	cs := &provider.CoreScope{BaseURL: "http://cs.test", HTTP: h}
	since := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	if _, err := cs.Receptions(context.Background(), since); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.gotURL, "since=2026-08-01T12%3A00%3A00Z") &&
		!strings.Contains(h.gotURL, "since=2026-08-01T12:00:00Z") {
		t.Errorf("since was not sent: %s", h.gotURL)
	}
}
