package sdr

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// WriteWAV writes mono 16-bit PCM.
//
// A sound file rather than a stream because the first use is "let me hear what
// that node heard" — which is a file you scrub through next to the waterfall,
// not a live feed. Live listening is a transport on top of this, not a
// different encoder.
func WriteWAV(w io.Writer, samples []float64, rateHz int) error {
	if rateHz <= 0 {
		return fmt.Errorf("sdr: sample rate %d is not a rate", rateHz)
	}
	const bitsPerSample, channels = 16, 1
	dataLen := len(samples) * 2
	byteRate := rateHz * channels * bitsPerSample / 8

	hdr := []any{
		[]byte("RIFF"), uint32(36 + dataLen), []byte("WAVE"),
		[]byte("fmt "), uint32(16), uint16(1), uint16(channels),
		uint32(rateHz), uint32(byteRate),
		uint16(channels * bitsPerSample / 8), uint16(bitsPerSample),
		[]byte("data"), uint32(dataLen),
	}
	for _, f := range hdr {
		if err := binary.Write(w, binary.LittleEndian, f); err != nil {
			return fmt.Errorf("sdr: wav header: %w", err)
		}
	}

	pcm := make([]int16, len(samples))
	for i, s := range samples {
		// Clamp rather than wrap. A sample that overflows int16 and wraps turns
		// a loud passage into a burst of noise that sounds like interference —
		// indistinguishable, to the ear, from something the simulator meant.
		v := math.Round(math.Max(-1, math.Min(1, s)) * 32767)
		pcm[i] = int16(v)
	}
	if err := binary.Write(w, binary.LittleEndian, pcm); err != nil {
		return fmt.Errorf("sdr: wav data: %w", err)
	}
	return nil
}
