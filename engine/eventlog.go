package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// eventLog streams every engine event as one JSON object per line.
//
// The grep-and-diff arm of ADR-0007: a pcapng answers "what flew", this
// answers "what happened", in a form jq and diff already understand. Two runs
// of the same seed produce byte-identical logs, which is what makes diffing
// them a regression test.
type eventLog struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
	n   int
}

// eventLine is the stable wire shape. Named fields rather than the Event
// struct directly, so an internal rename cannot silently change a format
// people have scripts against.
type eventLine struct {
	TMs       uint32  `json:"t_ms"`
	Kind      string  `json:"kind"`
	From      string  `json:"from"`
	To        string  `json:"to,omitempty"`
	PacketID  uint64  `json:"packet_id"`
	MessageID uint64  `json:"message_id,omitempty"`
	Outcome   string  `json:"outcome,omitempty"`
	SNRdB     float64 `json:"snr_db,omitempty"`
	Detail    string  `json:"detail,omitempty"`
}

// StartEventLog begins writing NDJSON to path, replacing anything there.
func (e *Engine) StartEventLog(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("engine: event log: %w", err)
	}
	l := &eventLog{f: f, enc: json.NewEncoder(f)}
	e.mu.Lock()
	old := e.eventLog
	e.eventLog = l
	e.mu.Unlock()
	if old != nil {
		_ = old.close()
	}
	return nil
}

// StopEventLog closes the log and reports what was written.
func (e *Engine) StopEventLog() (path string, lines int, err error) {
	e.mu.Lock()
	l := e.eventLog
	e.eventLog = nil
	e.mu.Unlock()
	if l == nil {
		return "", 0, nil
	}
	l.mu.Lock()
	path, lines = l.f.Name(), l.n
	l.mu.Unlock()
	return path, lines, l.close()
}

// EventLogPath is the file being written, or empty.
func (e *Engine) EventLogPath() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.eventLog == nil {
		return ""
	}
	return e.eventLog.f.Name()
}

func (l *eventLog) write(ev Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return
	}
	line := eventLine{
		TMs: ev.AtMs, Kind: ev.Kind, From: ev.From, To: ev.To,
		PacketID: ev.PacketID, MessageID: ev.MessageID,
		Outcome: string(ev.Outcome), SNRdB: ev.SNRdB, Detail: ev.Detail,
	}
	if err := l.enc.Encode(line); err != nil {
		// A log that cannot be written is closed rather than retried per
		// event: a full disk should cost one message, not a million.
		_ = l.f.Close()
		l.f = nil
		return
	}
	l.n++
}

func (l *eventLog) close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	f := l.f
	l.f = nil
	return f.Close()
}

// StartCaptureFIFO captures to a named pipe, for Wireshark watching live.
//
// The pipe is created if absent and opened read-write: read-write, because a
// FIFO opened write-only blocks until a reader appears, and "the menu item
// hangs until Wireshark starts" is indistinguishable from a crash. The frames
// are the same pcapng stream the file capture writes — Wireshark reads it
// with `wireshark -k -i <path>`.
func (e *Engine) StartCaptureFIFO(path string) error {
	if err := unix.Mkfifo(path, 0o600); err != nil && !os.IsExist(err) {
		return fmt.Errorf("engine: fifo: %w", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("engine: fifo: %w", err)
	}
	w, err := newCaptureOn(f)
	if err != nil {
		_ = f.Close()
		return err
	}
	e.mu.Lock()
	old := e.capture
	e.capture = w
	e.mu.Unlock()
	if old != nil {
		_ = old.close()
	}
	return nil
}
