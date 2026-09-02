// Talking to the MeshCore KISS modem over its serial port.
//
// Framing and SetHardware sub-commands per MeshCore's
// docs/kiss_modem_protocol.md: 0xC0-delimited frames, 0xDB escapes, and the
// MeshCore extensions on command 0x06.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"time"
)

const (
	fend  = 0xC0
	fesc  = 0xDB
	tfend = 0xDC
	tfesc = 0xDD

	cmdData        = 0x00
	cmdSetHardware = 0x06

	hwSetRadio   = 0x09
	hwSetTxPower = 0x0A
	hwGetRadio   = 0x0B
	hwGetTxPower = 0x0C
	hwGetVersion = 0x11
	hwGetName    = 0x16
	hwPing       = 0x17

	hwRespRadio   = 0x8B
	hwRespTxPower = 0x8C
	hwRespVersion = 0x91
	hwRespName    = 0x96
	hwRespPong    = 0x97
	hwRespOK      = 0xF0
	hwRespErr     = 0xF1
	hwRespTxDone  = 0xF8
)

// kissPort is the modem: a raw serial fd and a parser that hands back whole
// unescaped frames.
type kissPort struct {
	f       *os.File
	pending []byte
}

func (k *kissPort) Close() error { return k.f.Close() }

// send writes one KISS frame: type byte plus payload, escaped.
func (k *kissPort) send(typeByte byte, payload []byte) error {
	out := []byte{fend, typeByte}
	for _, b := range payload {
		switch b {
		case fend:
			out = append(out, fesc, tfend)
		case fesc:
			out = append(out, fesc, tfesc)
		default:
			out = append(out, b)
		}
	}
	out = append(out, fend)
	_, err := k.f.Write(out)
	return err
}

// read returns the next whole frame (type byte + unescaped payload), waiting
// up to the deadline. Partial serial reads are buffered across calls.
//
// readRaw rather than os.File.Read: with VMIN=0/VTIME=1 a quiet
// 100 ms window returns zero bytes, which os.File reports as io.EOF - a
// serial port pausing for breath is not a closed file.
func (k *kissPort) read(deadline time.Time) (byte, []byte, error) {
	buf := make([]byte, 4096)
	for {
		if t, p, ok := k.parse(); ok {
			return t, p, nil
		}
		if time.Now().After(deadline) {
			return 0, nil, fmt.Errorf("kiss: timeout waiting for a frame")
		}
		n, err := k.readRaw(buf)
		if err != nil {
			return 0, nil, err
		}
		if n > 0 {
			k.pending = append(k.pending, buf[:n]...)
		}
	}
}

// parse pulls one complete frame off the pending buffer.
func (k *kissPort) parse() (byte, []byte, bool) {
	// Skip to a FEND, then collect to the closing FEND.
	i := 0
	for i < len(k.pending) && k.pending[i] != fend {
		i++
	}
	k.pending = k.pending[i:]
	if len(k.pending) < 2 {
		return 0, nil, false
	}
	// Coalesce back-to-back FENDs (idle keepalives are legal).
	start := 0
	for start < len(k.pending) && k.pending[start] == fend {
		start++
	}
	end := start
	for end < len(k.pending) && k.pending[end] != fend {
		end++
	}
	if end >= len(k.pending) {
		return 0, nil, false // no closing FEND yet
	}
	raw := k.pending[start:end]
	k.pending = k.pending[end:]
	if len(raw) == 0 {
		return 0, nil, false
	}
	var out []byte
	esc := false
	for _, b := range raw[1:] {
		switch {
		case esc && b == tfend:
			out = append(out, fend)
			esc = false
		case esc && b == tfesc:
			out = append(out, fesc)
			esc = false
		case b == fesc:
			esc = true
		default:
			out = append(out, b)
		}
	}
	return raw[0], out, true
}

// hardware sends a SetHardware sub-command and waits for its reply, passing
// through any unsolicited frames.
func (k *kissPort) hardware(sub byte, data []byte, wantResp byte, timeout time.Duration) ([]byte, error) {
	if err := k.send(cmdSetHardware, append([]byte{sub}, data...)); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		typ, payload, err := k.read(deadline)
		if err != nil {
			return nil, err
		}
		if typ != cmdSetHardware || len(payload) == 0 {
			continue // a data frame or noise; not ours
		}
		switch payload[0] {
		case wantResp:
			return payload[1:], nil
		case hwRespErr:
			code := byte(0)
			if len(payload) > 1 {
				code = payload[1]
			}
			return nil, fmt.Errorf("kiss: modem error 0x%02x", code)
		}
	}
}

// radioParams is what the modem says its radio is set to.
type radioParams struct {
	FreqHz uint32
	BWHz   uint32
	SF     uint8
	CR     uint8 // denominator, 5-8
}

func (k *kissPort) getRadio() (radioParams, error) {
	b, err := k.hardware(hwGetRadio, nil, hwRespRadio, 2*time.Second)
	if err != nil {
		return radioParams{}, err
	}
	if len(b) < 10 {
		return radioParams{}, fmt.Errorf("kiss: short radio reply (%d bytes)", len(b))
	}
	return radioParams{
		FreqHz: binary.LittleEndian.Uint32(b[0:4]),
		BWHz:   binary.LittleEndian.Uint32(b[4:8]),
		SF:     b[8], CR: b[9],
	}, nil
}

// setRadio programs the modem's radio and confirms it took.
func (k *kissPort) setRadio(r radioParams) error {
	b := make([]byte, 10)
	binary.LittleEndian.PutUint32(b[0:4], r.FreqHz)
	binary.LittleEndian.PutUint32(b[4:8], r.BWHz)
	b[8], b[9] = r.SF, r.CR
	if _, err := k.hardware(hwSetRadio, b, hwRespOK, 3*time.Second); err != nil {
		return fmt.Errorf("set radio: %w", err)
	}
	return nil
}

func (k *kissPort) setTxPower(dbm int8) error {
	if _, err := k.hardware(hwSetTxPower, []byte{byte(dbm)}, hwRespOK, 2*time.Second); err != nil {
		return fmt.Errorf("set tx power: %w", err)
	}
	return nil
}

// transmit queues a raw payload and waits for the radio's TxDone.
func (k *kissPort) transmit(payload []byte, timeout time.Duration) error {
	if err := k.send(cmdData, payload); err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for {
		typ, p, err := k.read(deadline)
		if err != nil {
			return err
		}
		if typ == cmdSetHardware && len(p) >= 2 && p[0] == hwRespTxDone {
			if p[1] != 0x01 {
				return fmt.Errorf("kiss: transmission failed")
			}
			return nil
		}
	}
}
