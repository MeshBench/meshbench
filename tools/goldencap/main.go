// goldencap: golden vectors from real silicon.
//
// Commands a MeshCore KISS modem to transmit known payloads, captures the
// air over a remote rtl_tcp dongle, runs MeshBench's own receive chain over
// the samples, and diffs the symbols against internal/lora's encoding. Every
// difference is a bit-level convention where the simulator and the chip
// disagree - the thing issue #92 exists to find.
//
//	goldencap -probe                                # who is on the serial port
//	goldencap -run -payload "hello golden vectors"  # the full experiment
//	goldencap -analyze capture.iq -payload ...      # re-run analysis offline
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/MeshBench/meshbench/internal/lora"
)

func main() {
	var (
		port    = flag.String("port", "/dev/ttyACM0", "KISS modem serial port")
		rtlAddr = flag.String("rtl", "10.10.231.105:1234", "rtl_tcp server")
		probe   = flag.Bool("probe", false, "query the modem and exit")
		run     = flag.Bool("run", false, "transmit, capture and analyze")
		anlz    = flag.String("analyze", "", "analyze a saved capture instead")
		payload = flag.String("payload", "MeshBench golden vector 001", "known payload")
		out     = flag.String("out", "", "save the raw capture here (with -run)")
		gain    = flag.Uint("gain", 200, "manual tuner gain, tenth-dB (strong local signal wants low)")
		rate    = flag.Uint("rate", 1_000_000, "capture sample rate, Hz")
		dump    = flag.Int("dump", -1, "dump raw symbol structure from this baseband sample")
		golden  = flag.String("golden", "", "write a golden-vector JSON here after analysis")
	)
	flag.Parse()
	dumpFrom = *dump
	goldenOut = *golden

	if err := realMain(*port, *rtlAddr, *probe, *run, *anlz, *payload, *out,
		uint32(*gain), uint32(*rate)); err != nil {
		fmt.Fprintln(os.Stderr, "goldencap:", err)
		os.Exit(1)
	}
}

func realMain(port, rtlAddr string, probe, run bool, anlz, payload, out string,
	gain, rate uint32) error {
	switch {
	case probe:
		return doProbe(port)
	case run:
		return doRun(port, rtlAddr, payload, out, gain, rate)
	case anlz != "":
		return doAnalyze(anlz, payload, rate)
	}
	flag.Usage()
	return nil
}

func doProbe(port string) error {
	k, err := openKISS(port)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	if _, err := k.hardware(hwPing, nil, hwRespPong, 2*time.Second); err != nil {
		return fmt.Errorf("no pong: %w", err)
	}
	if v, err := k.hardware(hwGetVersion, nil, hwRespVersion, 2*time.Second); err == nil && len(v) > 0 {
		fmt.Printf("firmware version %d\n", v[0])
	}
	if n, err := k.hardware(hwGetName, nil, hwRespName, 2*time.Second); err == nil {
		fmt.Printf("device %q\n", n)
	}
	r, err := k.getRadio()
	if err != nil {
		return err
	}
	fmt.Printf("radio: %.3f MHz, %.1f kHz, SF%d, CR4/%d\n",
		float64(r.FreqHz)/1e6, float64(r.BWHz)/1e3, r.SF, r.CR)
	if tp, err := k.hardware(hwGetTxPower, nil, hwRespTxPower, 2*time.Second); err == nil && len(tp) > 0 {
		fmt.Printf("tx power: %d dBm\n", int8(tp[0]))
	}
	return nil
}

func doRun(port, rtlAddr, payload, out string, gain, rate uint32) error {
	k, err := openKISS(port)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	r, err := k.getRadio()
	if err != nil {
		return err
	}
	if r.FreqHz == 0 {
		// The modem boots unconfigured; program the UK/EU narrow preset and
		// a gentle TX power - ten metres of air into an RTL front end does
		// not want a full-power transmitter.
		r = radioParams{FreqHz: 869_618_000, BWHz: 62_500, SF: 8, CR: 8}
		if err := k.setRadio(r); err != nil {
			return err
		}
		if err := k.setTxPower(5); err != nil {
			return err
		}
		got, err := k.getRadio()
		if err != nil {
			return err
		}
		if got.FreqHz != r.FreqHz || got.SF != r.SF {
			return fmt.Errorf("radio did not take the settings: %+v", got)
		}
		r = got
	}
	fmt.Printf("modem: %.3f MHz, %.1f kHz, SF%d, CR4/%d\n",
		float64(r.FreqHz)/1e6, float64(r.BWHz)/1e3, r.SF, r.CR)

	rtl, err := dialRTL(rtlAddr)
	if err != nil {
		return err
	}
	defer func() { _ = rtl.Close() }()
	// Offset-tune 150 kHz below the channel so the dongle's DC spike stays
	// out of the signal; the channelizer mixes it back.
	tune := r.FreqHz - 150_000
	if err := rtl.SetRate(rate); err != nil {
		return err
	}
	if err := rtl.SetFreq(tune); err != nil {
		return err
	}
	_ = rtl.SetAGC(false)
	_ = rtl.SetGainMode(true)
	_ = rtl.SetGainTenthDB(gain)

	// Capture in the background while the modem transmits into it.
	type capResult struct {
		raw []byte
		err error
	}
	done := make(chan capResult, 1)
	go func() {
		raw, err := rtl.capture(rate, 4.0, 300)
		done <- capResult{raw, err}
	}()
	// Give the capture a head start so the frame lands mid-buffer.
	time.Sleep(700 * time.Millisecond)
	fmt.Printf("transmitting %d bytes...\n", len(payload))
	if err := k.transmit([]byte(payload), 15*time.Second); err != nil {
		return err
	}
	fmt.Println("TxDone from the modem")
	res := <-done
	if res.err != nil {
		return res.err
	}
	if out != "" {
		if err := os.WriteFile(out, res.raw, 0o644); err != nil {
			return err
		}
		fmt.Printf("capture saved to %s (%d bytes)\n", out, len(res.raw))
	}
	return analyzeRaw(res.raw, r, payload, float64(rate), 150_000)
}

func doAnalyze(path, payload string, rate uint32) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	// Offline analysis assumes the run's own defaults: the narrow preset and
	// the standard 150 kHz offset tune.
	r := radioParams{FreqHz: 869_618_000, BWHz: 62_500, SF: 8, CR: 8}
	return analyzeRaw(raw, r, payload, float64(rate), 150_000)
}

func analyzeRaw(raw []byte, r radioParams, payload string, rateHz, offsetHz float64) error {
	iq := u8ToComplex(raw)
	fmt.Printf("%d complex samples (%.2f s)\n", len(iq), float64(len(iq))/rateHz)

	// The burst is somewhere near offsetHz plus the two crystals' errors;
	// find it rather than trusting anybody's ppm.
	found := findBurstOffsetHz(iq, rateHz, offsetHz, 60_000)
	fmt.Printf("strongest narrowband energy at %+.1f kHz from tune (expected near %+.1f)\n",
		found/1e3, offsetHz/1e3)

	pp := lora.Params{SF: int(r.SF), CR: int(r.CR) - 4, CRC: true,
		LDRO: float64(uint64(1)<<r.SF)/(float64(r.BWHz)/1000) >= 16}
	baseband, phase := bestPhase(iq, rateHz, found, float64(r.BWHz), pp)
	if baseband == nil {
		baseband = channelize(iq, rateHz, found, float64(r.BWHz), 0)
	}
	fmt.Printf("channelized to %d samples at %.1f kHz (decimation phase %d)\n",
		len(baseband), float64(r.BWHz)/1e3, phase)

	p := lora.Params{SF: int(r.SF), CR: int(r.CR) - 4, CRC: true,
		LDRO: float64(uint64(1)<<r.SF)/(float64(r.BWHz)/1000) >= 16}
	if dumpFrom >= 0 {
		dumpStructure(baseband, p.SF, dumpFrom, 140)
		return nil
	}
	return analyze(baseband, p, []byte(payload))
}

// dumpFrom, when non-negative, switches analysis to a raw structure dump
// starting at that baseband sample.
var dumpFrom = -1

// goldenOut, when set, writes the analyzed frame out as a golden vector.
var goldenOut = ""
