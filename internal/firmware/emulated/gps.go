package emulated

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"sync"
	"time"
)

// GPSFeed is the receiver a board has a serial port to, answering with where
// the simulation says the node is.
//
// From the scenario rather than from a recorded log. A node whose receiver
// disagrees with the channel it is transmitting on is a simulator lying to
// itself, and that is the kind of fault nobody notices for months: moving a
// node on the map moves it on the handheld, because there is only one place
// the position is kept.
//
// It listens and the emulator connects, as the display and the buttons do -
// the side that outlives the other owns the address.
type GPSFeed struct {
	mu   sync.Mutex
	conn net.Conn
	lat  float64
	lon  float64
	altM float64
	// sent counts the sentences, which is also the clock: the time in a
	// sentence is the start plus one second per sentence, so the same run
	// produces the same bytes. A wall clock here would make every capture
	// differ from every other.
	sent  int
	start time.Time

	ln   net.Listener
	path string
	done chan struct{}
}

// gpsInterval is how often a receiver of this kind reports. One a second is
// what these modules do by default and what MeshCore's parser expects.
const gpsInterval = time.Second

// ListenGPS starts accepting the emulator's connection at path, reporting the
// position given until it is told another.
func ListenGPS(path string, lat, lon, altM float64, start time.Time) (*GPSFeed, error) {
	_ = os.Remove(path)
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "unix", path)
	if err != nil {
		return nil, err
	}
	g := &GPSFeed{lat: lat, lon: lon, altM: altM, start: start,
		ln: ln, path: path, done: make(chan struct{})}
	go g.accept()
	go g.report()
	return g, nil
}

// Path is where the emulator should connect.
func (g *GPSFeed) Path() string { return g.path }

// MoveTo is where the node is from now on.
func (g *GPSFeed) MoveTo(lat, lon, altM float64) {
	g.mu.Lock()
	g.lat, g.lon, g.altM = lat, lon, altM
	g.mu.Unlock()
}

// Close stops reporting and removes the socket.
func (g *GPSFeed) Close() error {
	select {
	case <-g.done:
	default:
		close(g.done)
	}
	err := g.ln.Close()
	g.mu.Lock()
	if g.conn != nil {
		_ = g.conn.Close()
		g.conn = nil
	}
	g.mu.Unlock()
	_ = os.Remove(g.path)
	return err
}

func (g *GPSFeed) accept() {
	for {
		conn, err := g.ln.Accept()
		if err != nil {
			return
		}
		g.mu.Lock()
		if g.conn != nil {
			_ = g.conn.Close()
		}
		g.conn = conn
		g.mu.Unlock()
	}
}

func (g *GPSFeed) report() {
	tick := time.NewTicker(gpsInterval)
	defer tick.Stop()
	for {
		select {
		case <-g.done:
			return
		case <-tick.C:
			g.mu.Lock()
			conn, lat, lon, alt := g.conn, g.lat, g.lon, g.altM
			at := g.start.Add(time.Duration(g.sent) * gpsInterval)
			if conn != nil {
				g.sent++
			}
			g.mu.Unlock()
			if conn == nil {
				continue
			}
			// Both sentences, because a parser wants both: the fix and its
			// quality come from one and the date from the other, and a
			// receiver that sent only one would be a receiver nobody makes.
			if _, err := conn.Write([]byte(gga(at, lat, lon, alt) + rmc(at, lat, lon))); err != nil {
				g.mu.Lock()
				if g.conn == conn {
					_ = conn.Close()
					g.conn = nil
				}
				g.mu.Unlock()
			}
		}
	}
}

// gga is the fix: where, how good, and how high.
func gga(at time.Time, lat, lon, altM float64) string {
	return sentence(fmt.Sprintf("GPGGA,%s,%s,%s,1,09,0.9,%.1f,M,0.0,M,,",
		at.UTC().Format("150405.00"), degMin(lat, "N", "S"), degMin(lon, "E", "W"), altM))
}

// rmc is the fix again with the date on it, which is what fixes the clock.
func rmc(at time.Time, lat, lon float64) string {
	return sentence(fmt.Sprintf("GPRMC,%s,A,%s,%s,0.0,0.0,%s,,,A",
		at.UTC().Format("150405.00"), degMin(lat, "N", "S"), degMin(lon, "E", "W"),
		at.UTC().Format("020106")))
}

// degMin is a coordinate in the form these sentences carry: whole degrees
// followed by minutes, which is neither degrees nor a decimal of them and is
// the single easiest thing to get wrong here.
func degMin(v float64, pos, neg string) string {
	hemi := pos
	if v < 0 {
		hemi, v = neg, -v
	}
	d := math.Floor(v)
	m := (v - d) * 60
	if pos == "E" {
		return fmt.Sprintf("%03d%08.5f,%s", int(d), m, hemi)
	}
	return fmt.Sprintf("%02d%08.5f,%s", int(d), m, hemi)
}

// sentence wraps a body in the dollar, the checksum and the line ending a
// receiver puts round it.
func sentence(body string) string {
	var sum byte
	for i := 0; i < len(body); i++ {
		sum ^= body[i]
	}
	return fmt.Sprintf("$%s*%02X\r\n", body, sum)
}
