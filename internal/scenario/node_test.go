package scenario_test

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/antenna"
	"github.com/MeshBench/meshbench/internal/scenario"
)

func repeater() scenario.Node {
	return scenario.Node{
		Name:       "GB7XYZ",
		Kind:       scenario.SimpleRepeater,
		Position:   scenario.LatLon{Lat: 56.62, Lon: -3.86},
		HeightAGLm: 12,
		Antenna: antenna.Mounted{
			Pattern:      antenna.Collinear{GainDBiPeak: 6},
			Polarisation: "vertical",
		},
		Radio:         scenario.RadioConfig{CentreHz: 869.525e6, BandwidthHz: 250e3, SpreadFactor: 10, CodingRate: 1},
		TxPowerDBm:    27,
		NoiseFigureDB: 6,
	}
}

func observer() scenario.Node {
	n := repeater()
	n.Name = "obs-1"
	n.Kind = scenario.SDRObserver
	n.TxPowerDBm = 0
	return n
}

func TestValidNodesValidate(t *testing.T) {
	for _, n := range []scenario.Node{repeater(), observer()} {
		if err := n.Validate(); err != nil {
			t.Errorf("%s: %v", n.Name, err)
		}
	}
}

// An observer that is quietly allowed a transmit power is the worst kind of
// mistake here: whichever way round the user's intent was, the mesh that comes
// out still looks entirely plausible.
func TestObserverMayNotTransmit(t *testing.T) {
	n := observer()
	n.TxPowerDBm = 27
	err := n.Validate()
	if err == nil {
		t.Fatal("an observer with 27 dBm of transmit power was accepted")
	}
	if !strings.Contains(err.Error(), "transmits nothing") {
		t.Errorf("error should say why: %v", err)
	}
}

// An observer has no modem, so the modem settings must not be required of it.
// Demanding them would be a rule that exists only because the struct is shared.
func TestObserverNeedsNoModemSettings(t *testing.T) {
	n := observer()
	n.Radio.SpreadFactor = 0
	n.Radio.CodingRate = 0
	if err := n.Validate(); err != nil {
		t.Errorf("an observer should not need SF or coding rate: %v", err)
	}
}

func TestTransmitterNeedsPowerAndModem(t *testing.T) {
	cases := map[string]func(*scenario.Node){
		"spreading factor": func(n *scenario.Node) { n.Radio.SpreadFactor = 13 },
		"coding rate":      func(n *scenario.Node) { n.Radio.CodingRate = 9 },
		"transmits, but":   func(n *scenario.Node) { n.TxPowerDBm = 0 },
		"below ground":     func(n *scenario.Node) { n.HeightAGLm = -1 },
		"antenna pattern":  func(n *scenario.Node) { n.Antenna.Pattern = nil },
	}
	for want, break_ := range cases {
		n := repeater()
		break_(&n)
		err := n.Validate()
		if err == nil {
			t.Errorf("%s: accepted", want)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("want an error mentioning %q, got: %v", want, err)
		}
	}
}

func TestKindsAgreeOnWhoTransmits(t *testing.T) {
	for _, k := range []scenario.Kind{scenario.SimpleRepeater, scenario.AdvancedRepeater, scenario.Companion} {
		if !k.Transmits() || !k.RunsFirmware() {
			t.Errorf("%s should transmit and run firmware", k)
		}
	}
	if scenario.SDRObserver.Transmits() || scenario.SDRObserver.RunsFirmware() {
		t.Error("an SDR observer neither transmits nor runs firmware")
	}
}

func TestUnknownKindIsRejected(t *testing.T) {
	n := repeater()
	n.Kind = "gateway"
	if err := n.Validate(); err == nil {
		t.Fatal("an unknown kind was accepted")
	}
}

func TestObserverNodeBecomesAnInstrument(t *testing.T) {
	n := observer()
	o, err := n.Observer()
	if err != nil {
		t.Fatal(err)
	}
	if o.Name != n.Name || o.CentreHz != n.Radio.CentreHz || o.SampleRateHz != n.Radio.BandwidthHz {
		t.Errorf("observer %+v does not match the node it came from", o)
	}
	if _, err := repeater().Observer(); err == nil {
		t.Error("a repeater was handed out as an SDR observer")
	}
}
