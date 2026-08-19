// The rtl_tcp client side: tuning a remote dongle and pulling raw IQ.
package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

type rtlClient struct {
	conn net.Conn
}

func dialRTL(addr string) (*rtlClient, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	hdr := make([]byte, 12)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, hdr); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rtl_tcp header: %w", err)
	}
	if string(hdr[:4]) != "RTL0" {
		_ = conn.Close()
		return nil, fmt.Errorf("not an rtl_tcp server: %q", hdr[:4])
	}
	return &rtlClient{conn: conn}, nil
}

func (r *rtlClient) cmd(op byte, val uint32) error {
	b := make([]byte, 5)
	b[0] = op
	binary.BigEndian.PutUint32(b[1:], val)
	_, err := r.conn.Write(b)
	return err
}

func (r *rtlClient) SetFreq(hz uint32) error       { return r.cmd(0x01, hz) }
func (r *rtlClient) SetRate(hz uint32) error       { return r.cmd(0x02, hz) }
func (r *rtlClient) SetGainMode(manual bool) error { return r.cmd(0x03, b2u(manual)) }
func (r *rtlClient) SetGainTenthDB(g uint32) error { return r.cmd(0x04, g) }
func (r *rtlClient) SetAGC(on bool) error          { return r.cmd(0x08, b2u(on)) }

func b2u(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// capture pulls seconds of raw uint8 IQ into memory, discarding the first
// flushMs worth - retuning leaves stale samples in the pipe, and a capture
// that starts with the old frequency's tail decodes nothing.
func (r *rtlClient) capture(rateHz uint32, seconds float64, flushMs int) ([]byte, error) {
	flush := int(float64(rateHz) * float64(flushMs) / 1000 * 2)
	want := int(float64(rateHz) * seconds * 2)
	buf := make([]byte, flush+want)
	_ = r.conn.SetReadDeadline(time.Now().Add(time.Duration(seconds*1000+10000) * time.Millisecond))
	if _, err := io.ReadFull(r.conn, buf); err != nil {
		return nil, err
	}
	return buf[flush:], nil
}

func (r *rtlClient) Close() error { return r.conn.Close() }
