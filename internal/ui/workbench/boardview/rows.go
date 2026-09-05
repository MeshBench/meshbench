// What the board declared, what came back, and what the difference means.
//
// Two tables, built from the same pair of sources every time: the board's own
// profile on one side and the running node's state on the other. Nothing here
// invents a number. Where the second side does not exist yet - which is true
// of every GPIO's direction and level, because nothing instruments them - the
// row says "not instrumented" rather than printing a dash somebody would read
// as "low".
//
// The radio table is the one that works today, and it is the one worth having:
// every field on it is something the firmware actually left in the chip, and
// the board's profile says what it should have been.
package boardview

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// Row is one line of either table.
type Row struct {
	// Group is the heading it sits under in the index and the table.
	Group string
	// Name is what the board calls it.
	Name string
	// Where is the pin, address or register it lives at.
	Where string
	// Declared and Observed are the two halves the whole window is about.
	Declared, Observed string
	Verdict            Verdict
	// Why is the board profile's own account of what this line does, shown in
	// the inspector. Empty where the profile says nothing, which is itself
	// worth seeing.
	Why string
}

// wiringRows is what the board declares it is wired as.
//
// Every row's Observed is "not instrumented" except where something really
// does come back, which today is the front-end module's enable line: the
// firmware drives it as a GPIO and the chip model reports where it stood when
// the node last transmitted. One real row among the declared ones is not a
// token - it is the shape the rest take when the GPIO model grows counters.
func wiringRows(b hw.Board, st *state.NodeStat) []Row {
	var out []Row
	add := func(group, name, where, why string, v Verdict, obs string) {
		out = append(out, Row{Group: group, Name: name, Where: where,
			Declared: "declared", Observed: obs, Verdict: v, Why: why})
	}
	notYet := func(group, name, where, why string) {
		add(group, name, where, why, Undeclared, "not instrumented")
	}
	_ = notYet

	if w := b.QEMU; w != nil {
		if w.NSS != 0 {
			notYet("Radio", "NSS", pin(w.NSS),
				"The chip select RadioLib toggles by hand, because the controller "+
					"clocks bytes out one transfer at a time and this is what frames "+
					"a command.")
		}
		if w.Busy != 0 {
			notYet("Radio", "BUSY", pin(w.Busy),
				"The line the driver waits on before it speaks to the chip again.")
		}
		if w.DIO1 != 0 {
			notYet("Radio", "DIO1", pin(w.DIO1),
				"MeshCore's receive path is gated on a flag set only by the "+
					"packet-received interrupt this pin fires. A board wired without "+
					"it receives perfectly and forwards nothing.")
		}
		if w.FEM != 0 {
			// The one line we really do observe.
			v, obs := Undeclared, "not powered"
			switch {
			case st == nil || !st.Running:
			case !st.Radio.Reported:
				v, obs = Undeclared, "the chip has not reported"
			case st.Radio.FemLive:
				v, obs = Agrees, "asserted now"
			case st.Radio.FemAtTx != 0:
				v, obs = Agrees, "asserted at the last transmit"
			default:
				v, obs = Silent, "never asserted"
			}
			add("Radio", "front-end module", pin(w.FEM),
				"An external amplifier the firmware brings into circuit by driving "+
					"this line. A firmware that fails to switch it in transmits at the "+
					"chip's power rather than the board's, and nothing in the radio's "+
					"own registers says so.", v, obs)
		}
	}

	p := b.Hardware
	if p == nil {
		// Nobody has recorded what this board carries. The radio lines above
		// are still real - they come from the emulator's wiring, not the panel
		// - so they stand, and nothing is invented to keep them company.
		return grouped(out)
	}
	if sc := p.Screen; sc != nil {
		where := fmt.Sprintf("%s %#02x", sc.Bus, sc.Addr)
		if sc.Bus == hw.BusSPI {
			where = fmt.Sprintf("cs %d dc %d", sc.CS, sc.DC)
		}
		// A board that is off has not drawn anything because it is off, and
		// saying so is not the same as raising a caution about it. Only a
		// running node that has drawn nothing is worth a second look.
		v, obs := Undeclared, "not powered"
		switch {
		case st != nil && st.Screen != nil:
			v = Agrees
			// Not a fault when it is off: the firmware switches the panel off
			// after an idle and the board's own button brings it back. Said
			// briefly, because the column is a column and a sentence that runs
			// past its width is a cell nobody can read.
			obs = fmt.Sprintf("%dx%d drawn", st.Screen.Width, st.Screen.Height)
			if !st.Screen.On {
				obs = fmt.Sprintf("%dx%d, asleep", st.Screen.Width, st.Screen.Height)
			}
		case st != nil && st.Running:
			v, obs = Silent, "nothing drawn yet"
		}
		out = append(out, Row{Group: "Display", Name: sc.Controller, Where: where,
			Declared: fmt.Sprintf("%dx%d %s", sc.WidthPx, sc.HeightPx, sc.Ink),
			Observed: obs, Verdict: v,
			Why: "What the firmware drew, not what the panel looks like: no " +
				"backlight, no viewing angle, no refresh artefacts."})
	}
	for _, part := range p.Parts {
		out = append(out, partRow(b, part))
	}
	return grouped(out)
}

// grouped puts the rows in group order, stably.
//
// The index heads each run of a group, so rows arriving in the profile's own
// order printed "Other", then "Input", then "Other" again - which reads as two
// groups that happen to share a name rather than one group split in half.
func grouped(rows []Row) []Row {
	rank := map[string]int{"Radio": 0, "Display": 1, "Input": 2, "Lamps": 3, "Other": 4}
	out := make([]Row, 0, len(rows))
	for r := 0; r <= 4; r++ {
		for _, row := range rows {
			if rank[row.Group] == r {
				out = append(out, row)
			}
		}
	}
	return out
}

// partRow is one of the board's parts, and whether anything models it.
//
// The unmodelled ones are the point. An unmodelled input is not a zero: a
// firmware that starts a conversion and waits for a converter nobody built
// waits for ever, which is a hang that looks like a firmware fault.
func partRow(b hw.Board, part hw.Part) Row {
	r := Row{Group: groupOf(part.Kind), Name: part.Name, Where: whereOf(part),
		Declared: part.Kind.String(), Observed: "not instrumented",
		Verdict: Undeclared}
	switch part.Kind {
	case hw.Meter:
		// Asked of the engine rather than assumed. The converter behind a
		// meter is modelled on the parts where it is, and saying otherwise is
		// a false claim in the one column that exists to be trusted.
		if ch, ok := engine.MeterModelled(b); ok {
			r.Verdict = Agrees
			r.Observed = fmt.Sprintf("channel %d, at full charge", ch)
			r.Why = "The cell's voltage through the board's divider, encoded as " +
				"the converter's own reading so the firmware's arithmetic gets " +
				"the true voltage back. Set at boot and held there: nothing " +
				"drains it as the run goes on."
		} else {
			r.Verdict, r.Observed = NotModelled, "no converter modelled"
			r.Why = "An unmodelled input is not a zero. A firmware that starts " +
				"a conversion and waits for a converter nobody built waits for " +
				"ever. This board's meter is not on a converter we model - the " +
				"first ten pins of the first converter, on an ESP32-S3."
		}
	case hw.Lamp:
		r.Verdict, r.Observed = NotModelled, "no watcher on this pin"
		r.Why = "The pin is declared and nothing watches it, so \"off\" and " +
			"\"not modelled\" would look the same. This says which it is."
	case hw.Ball:
		r.Why = "Four lines that pulse as the ball turns; the firmware counts " +
			"the changes rather than reading a position."
	case hw.Keys:
		r.Why = "Not a matrix: a second microcontroller with its own firmware, " +
			"answering with the character last pressed, which is why it is " +
			"declared by address rather than by pins."
	case hw.Touch:
		r.Why = "Reports a point rather than a state. Where the panel is mounted " +
			"turned, a tap has to be turned back before it is sent, or every one " +
			"lands somewhere else."
	case hw.Card:
		r.Why = "A third device on a bus the radio and the display already share, " +
			"told apart by its own select. An undriven select answers everything, " +
			"and reads as a dead radio."
	case hw.GPS:
		r.Why = "On the board's second serial port. What it says is where the " +
			"scenario put the node, so moving a node on the map moves it here."
	}
	return r
}

// radioRows is the comparison that works today: what the board says its radio
// is, beside what the firmware left in the chip.
//
// This is the whole thesis in one table, and none of it needs new plumbing -
// the chip's own registers have been reaching the interface for a while, and
// until now there was nowhere that put them next to the board's claims.
func radioRows(b hw.Board, st *state.NodeStat) []Row {
	if st == nil || !st.Radio.Reported {
		return nil
	}
	r := st.Radio
	var out []Row
	add := func(name, decl, obs string, v Verdict, why string) {
		out = append(out, Row{Group: "Radio", Name: name, Declared: decl,
			Observed: obs, Verdict: v, Why: why})
	}

	// Transmit power. The board's figure is what the chip can do; the
	// firmware's is what it asked for, and a build compiled for another
	// region asks for less without saying so.
	v := Agrees
	if float64(r.TxPowerDBm) > b.MaxTxDBm {
		v = Diverged
	}
	add("transmit power", fmt.Sprintf("%.0f dBm max", b.MaxTxDBm),
		fmt.Sprintf("%d dBm", r.TxPowerDBm), v,
		"The board's figure is at the chip. What actually leaves the antenna is "+
			"this less the feedline, plus the front-end module where one is "+
			"switched in.")

	// Receive gain. Boosted is what a receiver should be left in; power
	// saving is a real setting and also what a fault leaves behind.
	gv, gobs := Agrees, "boosted"
	if !r.Boosted {
		gv, gobs = Silent, "power saving"
	}
	add("receive gain", "boosted", fmt.Sprintf("%s (0x%02X)", gobs, r.GainReg), gv,
		"A receiver left in power saving loses a few dB of sensitivity, which is "+
			"a link budget nobody changed on purpose.")

	// The interrupt pair. A mask with nothing enabled is a radio that cannot
	// tell the firmware anything, which is the shape of the DIO1 fault.
	iv, iobs := Agrees, fmt.Sprintf("mask %04X flags %04X", r.IRQMask, r.IRQFlags)
	switch {
	case r.IRQMask == 0:
		iv, iobs = Diverged, "nothing enabled"
	case st.IRQReads == 0:
		iv, iobs = Silent, "never read"
	}
	add("interrupts", "enabled and read", iobs, iv,
		"The mask is what the firmware allowed into the status register and the "+
			"flags are what is raised now. A radio whose mask is empty cannot tell "+
			"the firmware a packet arrived, however well it receives.")

	if st.Spurious > 0 {
		add("spurious interrupts", "none", fmt.Sprintf("%d", st.Spurious), Diverged,
			"Interrupts the chip raised that the firmware had not asked for. They "+
				"cost time in the receive path and mean the two disagree about what "+
				"the chip was told.")
	}

	// The modulation the run is at, against the chip.
	add("modulation", "from the scenario",
		fmt.Sprintf("SF%d CR%d BW %.1f kHz", r.SF, r.CR, float64(r.BandwidthHz)/1000),
		Agrees, "What the chip is configured to now. A node whose spreading factor "+
			"drifted from the network's hears nothing and reports no fault.")
	add("frequency", "from the scenario",
		fmt.Sprintf("%.3f MHz", float64(r.FreqHz)/1e6), Agrees,
		"The channel the chip is tuned to.")

	if b.FEM != nil {
		fv, fobs := Silent, "not asserted at the last transmit"
		if r.FemAtTx != 0 {
			fv, fobs = Agrees, "asserted at the last transmit"
		}
		add("front-end module", fmt.Sprintf("+%.0f dB when switched in", b.FEM.TxGainDB),
			fobs, fv,
			"Where the line stood when this node last began transmitting, which is "+
				"the one that decides how much power left the board.")
	}
	return out
}

func groupOf(k hw.PartKind) string {
	switch k {
	case hw.Keys, hw.Touch, hw.Ball, hw.Button:
		return "Input"
	case hw.Lamp:
		return "Lamps"
	}
	return "Other"
}

func whereOf(part hw.Part) string {
	switch {
	case part.Bus == hw.BusI2C:
		return fmt.Sprintf("I2C %#02x", part.Addr)
	case len(part.Pins) > 0:
		s := ""
		for i, p := range part.Pins {
			if i > 0 {
				s += " "
			}
			s += fmt.Sprintf("%d", p)
		}
		return "pins " + s
	case part.Pin == hw.PinNone:
		return "none on this board"
	}
	return pin(part.Pin)
}

func pin(n int) string { return fmt.Sprintf("GPIO %d", n) }
