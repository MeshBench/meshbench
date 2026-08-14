package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/mcp"
)

type flatTerrain struct{ h float64 }

func (f flatTerrain) ElevationM(_, _ float64) (float64, bool) { return f.h, true }

type noTerrain struct{}

func (noTerrain) ElevationM(_, _ float64) (float64, bool) { return 0, false }

// call drives the server the way a client does: one JSON line in, one out.
func call(t *testing.T, s *mcp.Server, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var got []map[string]any
	dec := json.NewDecoder(&out)
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("decode reply: %v", err)
		}
		got = append(got, m)
	}
	return got
}

func server(t *testing.T, terrain mcp.Terrain) *mcp.Server {
	t.Helper()
	s := mcp.NewServer("meshcoresim", "test")
	if err := mcp.RegisterEngineTools(s, terrain); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestHandshakeAndToolList(t *testing.T) {
	s := server(t, flatTerrain{100})
	replies := call(t,
		s,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	)

	// The notification must not be answered. Some clients tolerate a reply to
	// one; others treat it as a protocol violation and drop the session.
	if len(replies) != 2 {
		t.Fatalf("got %d replies for 2 requests and 1 notification", len(replies))
	}

	init := replies[0]["result"].(map[string]any)
	if init["protocolVersion"] == "" {
		t.Error("no protocol version in the handshake")
	}

	tools := replies[1]["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, tv := range tools {
		tool := tv.(map[string]any)
		names[tool["name"].(string)] = true
		if tool["description"] == "" {
			t.Errorf("tool %v has no description; a model cannot choose it", tool["name"])
		}
		if _, ok := tool["inputSchema"]; !ok {
			t.Errorf("tool %v has no input schema", tool["name"])
		}
	}
	for _, want := range []string{"link_budget", "path_profile", "lora_airtime", "solar_budget", "model_limitations"} {
		if !names[want] {
			t.Errorf("missing tool %q", want)
		}
	}
}

// A link budget must answer in both directions and name the limiting one.
// Answering with a single number is the mistake the whole codebase is arranged
// to prevent.
func TestLinkBudgetAnswersBothDirections(t *testing.T) {
	s := server(t, flatTerrain{100})
	replies := call(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"link_budget","arguments":{"from_lat":56.70,"from_lon":-3.90,"from_height_m":20,"from_tx_dbm":27,"to_lat":56.80,"to_lon":-3.70,"to_height_m":1.5,"to_tx_dbm":14,"freq_mhz":869.525}}}`)

	res := replies[0]["result"].(map[string]any)
	if res["isError"] == true {
		t.Fatalf("tool errored: %v", res)
	}
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)

	for _, want := range []string{"from -> to", "to -> from", "best case"} {
		if !strings.Contains(text, want) {
			t.Errorf("answer does not mention %q:\n%s", want, text)
		}
	}
}

// A tool error must be content, not a transport error: the model can read it
// and correct itself, where a JSON-RPC error looks like the server broke.
func TestToolErrorsAreReadable(t *testing.T) {
	s := server(t, noTerrain{})
	replies := call(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"link_budget","arguments":{"from_lat":56.7,"from_lon":-3.9,"to_lat":56.8,"to_lon":-3.7,"freq_mhz":869.525}}}`)

	res := replies[0]["result"].(map[string]any)
	if res["isError"] != true {
		t.Fatalf("a path with no terrain did not report an error: %v", res)
	}
	if replies[0]["error"] != nil {
		t.Error("a tool error was returned as a transport error")
	}
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "terrain") {
		t.Errorf("error does not say what to do about it: %s", text)
	}
}

// Airtime has to come from the same calculation the firmware uses, or an
// assistant reasoning about duty cycle is reasoning about a different radio.
func TestAirtimeMatchesTheEngine(t *testing.T) {
	s := server(t, flatTerrain{100})
	replies := call(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"lora_airtime","arguments":{"spreading_factor":10,"bandwidth_khz":250,"payload_bytes":32}}}`)

	res := replies[0]["result"].(map[string]any)
	if res["isError"] == true {
		t.Fatalf("errored: %v", res)
	}
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "ms on air") || !strings.Contains(text, "duty cycle") {
		t.Errorf("airtime answer is not usable for duty-cycle reasoning:\n%s", text)
	}
}

// The error budget has to be callable. An assistant handed only numbers will
// present them as predictions.
func TestLimitationsAreCallableAndBlunt(t *testing.T) {
	s := server(t, flatTerrain{100})
	replies := call(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"model_limitations","arguments":{}}}`)

	text := replies[0]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	for _, want := range []string{"OPTIMISTIC", "multipath", "1.6 dB", "Never validated"} {
		if !strings.Contains(text, want) {
			t.Errorf("limitations do not mention %q", want)
		}
	}
}

func TestUnknownToolAndBadJSON(t *testing.T) {
	s := server(t, flatTerrain{100})
	replies := call(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nonesuch","arguments":{}}}`,
		`{not json`,
		`{"jsonrpc":"2.0","id":3,"method":"nonesuch"}`,
	)
	if replies[0]["error"] == nil {
		t.Error("an unknown tool was accepted")
	}
	if replies[1]["error"] == nil {
		t.Error("malformed JSON did not produce a parse error")
	}
	if replies[2]["error"] == nil {
		t.Error("an unknown method was accepted")
	}
	// And the session survives all three, which is the point of answering
	// rather than closing.
	if len(replies) != 3 {
		t.Errorf("got %d replies; the server stopped early", len(replies))
	}
}

func TestDuplicateToolIsRefused(t *testing.T) {
	s := mcp.NewServer("t", "1")
	tool := mcp.Tool{Name: "dup", Call: func(context.Context, json.RawMessage) (string, error) { return "", nil }}
	if err := s.Register(tool); err != nil {
		t.Fatal(err)
	}
	if err := s.Register(tool); err == nil {
		t.Fatal("two tools quietly shared a name")
	}
}

// Longitude has to actually arrive. A combined struct declaration shares one
// JSON tag, which left from_lon and to_lon permanently zero — and a link budget
// computed on the Greenwich meridian returns a perfectly plausible number, just
// for the wrong pair of points.
func TestLongitudeIsRead(t *testing.T) {
	s := server(t, flatTerrain{100})
	near := call(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"link_budget","arguments":{"from_lat":56.70,"from_lon":-3.90,"to_lat":56.70,"to_lon":-3.85,"freq_mhz":869.525}}}`)
	far := call(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"link_budget","arguments":{"from_lat":56.70,"from_lon":-3.90,"to_lat":56.70,"to_lon":-2.90,"freq_mhz":869.525}}}`)

	text := func(r []map[string]any) string {
		return r[0]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	}
	nearText, farText := text(near), text(far)
	if nearText == farText {
		t.Fatalf("two paths a degree of longitude apart gave the same answer:\n%s", nearText)
	}
	if !strings.Contains(nearText, "Distance 3.") {
		t.Errorf("near path distance is wrong:\n%s", nearText)
	}
	if !strings.Contains(farText, "Distance 6") {
		t.Errorf("far path distance is wrong:\n%s", farText)
	}
}
