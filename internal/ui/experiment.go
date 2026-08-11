package ui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/A13xB0/meshcoresim/internal/companion/proto"
	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/firmware"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// An experiment runs one scenario under several configurations and reports what
// differed.
//
// It exists because the alternative is what the ScotMesh CAD study was: the same
// six steps typed by hand for every arm, with the parts that decide whether the
// numbers mean anything — wiping persisted firmware state between arms, firing
// at the same simulated instant in each, varying the seed rather than repeating
// one, checking the flood propagated at all — easy to leave out and silent when
// left out. Each was got wrong at least once, and each produced entirely
// plausible numbers. The runner enforces them rather than documenting them.
//
// An arm is a configuration, not a firmware version. "1-, 2- or 3-byte path
// hash" and "off / minimal / strict loop detect" are the same question shaped
// the same way, and a runner that understands only two firmware builds answers
// the narrowest version of it.
type expArm struct {
	Label string

	// Empty or negative means "leave whatever the scenario says".
	RepeaterVersion  string
	CompanionVersion string
	PathHashMode     int32
	LoopDetect       string
	CAD              string // "on", "off", or empty to leave alone
	// SpreadMs overrides the experiment's own when >= 0: how long the senders
	// take to all have fired.
	SpreadMs int32
}

// apply writes the arm as provisioning, which is what an arm *is*.
func (a expArm) apply(cfg *configState) {
	cfg.pathHashMode = a.PathHashMode
	cfg.loopDetect = a.LoopDetect
	cfg.cadMode = a.CAD
}

// applyOver writes only what this arm actually sets, leaving the rest as the
// experiment's constants left it.
func (a expArm) applyOver(cfg *configState) {
	if a.PathHashMode >= 0 {
		cfg.pathHashMode = a.PathHashMode
	}
	if a.LoopDetect != "" {
		cfg.loopDetect = a.LoopDetect
	}
	if a.CAD != "" {
		cfg.cadMode = a.CAD
	}
}

// expResult is one arm run at one seed.
type expResult struct {
	Arm  string
	Seed uint64

	TX, RX     int
	Collisions int
	Deaf       int
	AirtimeMs  float64
	SpanMs     uint32

	// Messages is how many distinct messages the burst actually put on the air,
	// and MeanReachPct how much of the mesh each one got to on average.
	//
	// "Nodes that heard anything" was the wrong measure and flattered every
	// result: with six senders it counts a node that received one message out
	// of six exactly the same as one that received all six. What an operator
	// wants to know is whether a message gets through, so the unit is the
	// message.
	Messages     int
	MeanReachPct float64
	// ReachPerMsg is every message's own reach, one entry per sender, so a
	// message that got nowhere is visible rather than averaged away by five
	// that did well. Six senders, six numbers.
	ReachPerMsg []float64
	// SenderOf names which sender each of those messages came from.
	SenderOf []string
	// Split by what a node is, because the two say different things. Reaching
	// the repeaters is whether the mesh carried it; reaching the companions is
	// whether anybody read it.
	//
	// Six messages across 136 repeaters is 816 chances to arrive, and what
	// matters is how many of them did.
	RepHit, RepChances   int
	CompHit, CompChances int
	// Per message, in sender order.
	RepPerMsg  []int
	CompPerMsg []int

	// Flag is set when the run should not be believed - most usefully when
	// nothing relayed, which produces perfectly plausible totals.
	Flag string
	Err  string

	// perSecond is receptions per second after the burst, for the small
	// multiples. Kept rather than the whole ledger: a histogram is a few
	// hundred bytes and thirty full ledgers is half a million events.
	perSecond []int
	ledger    []engine.Event
}

// pendingSend is one sender waiting for its moment.
type pendingSend struct {
	node string
	atMs uint32
	done bool
}

type expPhase int

const (
	expIdle expPhase = iota
	expPrepare
	expStartFirmware
	expWaitFirmware
	expSettle
	expConnect
	expSend
	expFire
	expRun
	expCollect
	expDone
)

type experiment struct {
	// Base is what every arm holds constant - the experiment's fixed
	// conditions, as opposed to its independent variables. "All repeaters on
	// minimal loop detect, now vary the hash size" is two different statements
	// and needs two different places to say them; without this the only way to
	// hold something steady was to repeat it in every arm and hope.
	Base expArm

	Arms  []expArm
	Seeds []uint64

	Senders []string
	// SpreadMs is how long the senders take to all have transmitted. 0 fires
	// them on the same simulated instant, which is maximum contention; a wider
	// spread is the same traffic arriving politely, and the difference between
	// the two is often larger than any firmware setting.
	SpreadMs uint32

	// Bytes is how long each message is. A 12-byte "hello" and a 180-byte one
	// are different experiments: airtime scales with payload, and airtime is
	// what collides. 0 means the runner's own short label.
	Bytes int
	// Observer is a companion that only listens. Its Messages tab is the
	// human-sized answer to "did it get through", where the matrix gives the
	// mesh-sized one.
	Observer string
	Channel  string
	Scope    string
	SendAtMs uint32
	RunForMs uint32

	phase    expPhase
	arm      int
	seed     int
	baseline int
	burstMs  uint32
	spreadMs uint32
	pending  []pendingSend
	started  time.Time
	runStart time.Time
	results  []expResult
	log      []string
	status   string
	running  bool
	paused   bool

	// baselineArm is the row everything else is read against.
	baselineArm int
}

func (e *experiment) logf(format string, args ...any) {
	e.log = append(e.log, fmt.Sprintf(format, args...))
	if len(e.log) > 400 {
		e.log = e.log[len(e.log)-400:]
	}
}

// runsTotal and runsDone drive the estimate, which is shown before anybody
// commits to it: a sweep that quietly turns into forty minutes is one people
// cancel and stop trusting.
func (e *experiment) runsTotal() int { return len(e.Arms) * len(e.Seeds) }
func (e *experiment) runsDone() int  { return len(e.results) }

// estimate is how long the whole sweep will take, from the runs already done.
func (e *experiment) estimate() time.Duration {
	if len(e.results) == 0 || e.started.IsZero() {
		// Before anything has run, from measurement: settle plus window plus
		// firmware start, at roughly real time because a companion is attached.
		per := time.Duration(e.SendAtMs+e.RunForMs)*time.Millisecond + 20*time.Second
		return per * time.Duration(e.runsTotal())
	}
	per := time.Since(e.started) / time.Duration(len(e.results))
	return per * time.Duration(e.runsTotal()-len(e.results))
}

// startExperiment begins the matrix. Everything after this happens a frame at a
// time in stepExperiment.
func (a *App) startExperiment() error {
	e := a.exp
	if e == nil {
		return fmt.Errorf("no experiment defined")
	}
	switch {
	case len(e.Arms) == 0:
		return fmt.Errorf("an experiment needs at least one arm")
	case len(e.Seeds) == 0:
		return fmt.Errorf("an experiment needs at least one seed - repeats of one seed are identical by design")
	case len(e.Senders) == 0:
		return fmt.Errorf("an experiment needs at least one sender")
	case a.eng == nil && len(a.Nodes) == 0:
		return fmt.Errorf("no scenario loaded")
	}
	e.arm, e.seed = 0, 0
	e.results = nil
	e.log = nil
	e.running = true
	e.paused = false
	e.started = time.Now()
	e.phase = expPrepare
	e.logf("sweep: %d arms x %d seeds = %d runs", len(e.Arms), len(e.Seeds), e.runsTotal())
	a.switchWorkspace(wsBench)
	return nil
}

// stepExperiment advances the run by one frame's worth.
//
// A state machine rather than a function that runs the whole sweep, for the
// reason everything else here is one: the work has to happen on the frame
// thread, and anything that occupies it for longer than a frame stops the window
// drawing. A run nobody can watch is one nobody can tell is working.
func (a *App) stepExperiment() {
	e := a.exp
	if e == nil || !e.running || e.paused {
		return
	}
	arm := e.Arms[e.arm]
	seed := e.Seeds[e.seed]

	switch e.phase {
	case expPrepare:
		e.runStart = time.Now()
		e.status = fmt.Sprintf("%s, seed %d: preparing", arm.Label, seed)
		// Factory-fresh. The firmware persists preferences, channels and
		// contacts and reads them back at boot, so without this each arm
		// inherits the last one's state and the comparison quietly stops being
		// one.
		if err := firmware.WipeNodeStorage(); err != nil {
			e.fail(fmt.Sprintf("wiping node storage: %v", err))
			return
		}
		e.logf("%s %d  wiped node storage", arm.Label, seed)
		a.ensureConfig()
		// Constants first, then the arm's own settings over the top. An arm
		// states only what it varies, so anything it leaves unset stays at
		// whatever the experiment holds fixed.
		e.Base.apply(&a.cfg)
		arm.applyOver(&a.cfg)
		a.seed = seed
		for i := range a.Nodes {
			if !a.Nodes[i].Kind.RunsFirmware() {
				continue
			}
			if a.Nodes[i].Kind == scenario.Companion {
				if arm.CompanionVersion != "" {
					a.Nodes[i].Firmware.Version = arm.CompanionVersion
				}
			} else if arm.RepeaterVersion != "" {
				a.Nodes[i].Firmware.Version = arm.RepeaterVersion
			}
		}
		a.buildEngine()
		e.phase = expStartFirmware

	case expStartFirmware:
		a.startFirmware()
		e.phase = expWaitFirmware

	case expWaitFirmware:
		starting, done, total, errText := a.firmwareProgress()
		if starting {
			e.status = fmt.Sprintf("%s, seed %d: starting firmware %d/%d", arm.Label, seed, done, total)
			return
		}
		if a.eng == nil || a.eng.FirmwareCount() == 0 {
			e.fail("no firmware attached: " + errText)
			return
		}
		e.logf("%s %d  %d firmware in %.1f s", arm.Label, seed,
			a.eng.FirmwareCount(), time.Since(e.runStart).Seconds())
		a.applyStartupConfig()
		e.phase = expConnect

	case expSettle:
		if a.playing {
			e.status = fmt.Sprintf("%s, seed %d: settling, %.0f s", arm.Label, seed, float64(a.eng.NowMs())/1000)
			return
		}
		e.logf("%s %d  settled to %.1f s", arm.Label, seed, float64(a.eng.NowMs())/1000)
		e.phase = expSend

	case expConnect:
		// Every sender claimed and configured before any of them transmits, so
		// the burst is simultaneous rather than spread across the setup.
		for _, name := range e.Senders {
			if a.comps[name] != nil {
				continue
			}
			if err := a.compConnectQuiet(name); err != nil {
				e.fail(fmt.Sprintf("%s: %v", name, err))
				return
			}
			a.configureCompanion(name)
			if _, err := a.compAddChannel(a.comps[name], e.Channel); err != nil {
				e.fail(fmt.Sprintf("%s: %v", name, err))
				return
			}
		}
		// Settle *after* connecting, not before. Claiming and configuring a
		// sender steps the engine, so settling first and connecting second put
		// the burst at 52.2 s when it was asked for at 45 s - and at a
		// different instant in any arm whose setup took a different number of
		// steps. Firing at the same simulated moment in every arm is the whole
		// point of the runner owning this.
		a.runUntilMs = e.SendAtMs
		a.playing = true
		e.phase = expSettle

	case expSend:
		e.baseline = a.eng.EventCount()
		e.burstMs = a.eng.NowMs()
		// When to fire each sender. All on the same instant unless a spread is
		// asked for, in which case they are laid evenly across it.
		spread := e.SpreadMs
		if arm.SpreadMs >= 0 {
			spread = uint32(arm.SpreadMs)
		}
		e.pending = nil
		for i, name := range e.Senders {
			at := e.burstMs
			if spread > 0 && len(e.Senders) > 1 {
				at += uint32(i) * spread / uint32(len(e.Senders)-1)
			}
			e.pending = append(e.pending, pendingSend{node: name, atMs: at})
		}
		e.spreadMs = spread
		if spread > 0 {
			e.logf("%s %d  firing %d senders across %.1f s from %.1f s",
				arm.Label, seed, len(e.Senders), float64(spread)/1000, float64(e.burstMs)/1000)
			a.runUntilMs = e.burstMs + spread
			a.playing = true
		}
		e.phase = expFire
		return

	case expFire:
		fired := 0
		now := a.eng.NowMs()
		for i := range e.pending {
			if e.pending[i].done || now < e.pending[i].atMs {
				continue
			}
			e.pending[i].done = true
			fired++
			a.fireOne(e, e.pending[i].node, arm.Label, seed)
		}
		allDone := true
		for _, p := range e.pending {
			if !p.done {
				allDone = false
				break
			}
		}
		if !allDone {
			return
		}
		e.logf("%s %d  %d sent on %s at %.1f s", arm.Label, seed, len(e.pending),
			e.Channel, float64(e.burstMs)/1000)
		a.runUntilMs = e.burstMs + e.spreadMs + e.RunForMs
		a.playing = true
		e.phase = expRun
		return

	case expRun:
		if a.playing {
			e.status = fmt.Sprintf("%s, seed %d: running, %.0f s", arm.Label, seed, float64(a.eng.NowMs())/1000)
			return
		}
		e.phase = expCollect

	case expCollect:
		e.logf("%s %d  ledger %d -> %d events", arm.Label, seed, e.baseline, a.eng.EventCount())
		r := a.measure(arm.Label, seed, e.baseline, e.burstMs)
		e.results = append(e.results, r)
		if r.Flag != "" {
			e.logf("%s %d  FLAGGED: %s", arm.Label, seed, r.Flag)
		} else {
			e.logf("%s %d  tx %d  rx %d  %d msgs, reach %.0f%%  in %.0f s", arm.Label, seed,
				r.TX, r.RX, r.Messages, r.MeanReachPct, time.Since(e.runStart).Seconds())
		}
		// Release the senders so the next arm starts from nothing held.
		for _, name := range e.Senders {
			a.compDisconnect(name)
		}
		e.seed++
		if e.seed >= len(e.Seeds) {
			e.seed = 0
			e.arm++
		}
		if e.arm >= len(e.Arms) {
			e.phase = expDone
			e.running = false
			e.status = fmt.Sprintf("done: %d runs in %s", len(e.results),
				time.Since(e.started).Round(time.Second))
			e.logf("%s", e.status)
			return
		}
		e.phase = expPrepare

	case expDone, expIdle:
	}
}

func (e *experiment) fail(msg string) {
	e.results = append(e.results, expResult{
		Arm: e.Arms[e.arm].Label, Seed: e.Seeds[e.seed], Err: msg,
	})
	e.logf("FAILED: %s", msg)
	e.status = msg
	e.running = false
	e.phase = expDone
}

// measure reduces the events the burst caused to the numbers worth comparing.
// measure reduces the events the burst caused to the numbers worth comparing.
//
// Delivery is counted per message and per recipient, which is the only framing
// that survives more than one sender: six messages across a hundred and sixty
// repeaters is nine hundred and sixty chances to arrive, and what matters is
// how many of them did. Counting "nodes that heard anything" instead treats a
// node that received one of six exactly like one that received all six, and
// flatters every result.
func (a *App) measure(arm string, seed uint64, from int, burstMs uint32) expResult {
	r := expResult{Arm: arm, Seed: seed}
	if a.eng == nil {
		return r
	}
	events := a.eng.Events()
	if from > len(events) {
		from = len(events)
	}
	tail := events[from:]

	perMsg := map[uint64]map[string]bool{}
	// Which sender each message came from, so the per-message numbers can be
	// attributed rather than left anonymous.
	msgOrigin := map[uint64]string{}
	secs := int(a.exp.RunForMs/1000) + 1
	r.perSecond = make([]int, secs)
	var last uint32

	for _, ev := range tail {
		switch ev.Kind {
		case "tx":
			r.TX++
			if _, seen := msgOrigin[ev.MessageID]; !seen {
				msgOrigin[ev.MessageID] = ev.From
			}
		case "rx":
			r.RX++
			if perMsg[ev.MessageID] == nil {
				perMsg[ev.MessageID] = map[string]bool{}
			}
			perMsg[ev.MessageID][ev.To] = true
			if ev.AtMs > last {
				last = ev.AtMs
			}
			if ev.AtMs >= burstMs {
				if b := int((ev.AtMs - burstMs) / 1000); b >= 0 && b < secs {
					r.perSecond[b]++
				}
			}
		case "miss":
			switch {
			case strings.Contains(ev.Detail, "half duplex"):
				r.Deaf++
			case strings.Contains(ev.Detail, "stronger interferer"):
				r.Collisions++
			}
		}
	}

	// Only the messages our senders originated. Adverts and other traffic are
	// carried in tx/rx totals but are not what was asked about.
	senders := map[string]bool{}
	for _, n := range a.exp.Senders {
		senders[n] = true
	}
	ids := make([]uint64, 0, len(perMsg))
	for id := range perMsg {
		if senders[msgOrigin[id]] {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return msgOrigin[ids[i]] < msgOrigin[ids[j]] })
	r.Messages = len(ids)

	// Who is what, so delivery can be split by it.
	isComp := map[string]bool{}
	nRep, nComp := 0, 0
	for i := range a.Nodes {
		if a.Nodes[i].Kind == scenario.Companion {
			isComp[a.Nodes[i].Name] = true
			nComp++
		} else {
			nRep++
		}
	}
	for _, id := range ids {
		rep, comp := 0, 0
		for node := range perMsg[id] {
			if isComp[node] {
				comp++
			} else {
				rep++
			}
		}
		r.RepPerMsg = append(r.RepPerMsg, rep)
		r.CompPerMsg = append(r.CompPerMsg, comp)
		r.RepHit += rep
		r.CompHit += comp
		r.RepChances += nRep
		// Every companion but the one that sent it.
		r.CompChances += nComp - 1
		pct := 0.0
		if nRep+nComp-1 > 0 {
			pct = float64(rep+comp) / float64(nRep+nComp-1) * 100
		}
		r.ReachPerMsg = append(r.ReachPerMsg, pct)
		r.SenderOf = append(r.SenderOf, msgOrigin[id])
	}
	if r.RepChances+r.CompChances > 0 {
		r.MeanReachPct = float64(r.RepHit+r.CompHit) /
			float64(r.RepChances+r.CompChances) * 100
	}

	if last > burstMs {
		r.SpanMs = last - burstMs
	}
	for _, n := range a.eng.Scoreboard() {
		r.AirtimeMs += n.AirtimeMs
	}
	r.ledger = append([]engine.Event(nil), tail...)

	// A run where nothing relayed reports entirely plausible totals. The tell
	// is that transmissions barely exceed the number of senders: everybody
	// spoke once and nobody passed it on.
	if r.TX <= len(a.exp.Senders)*2 {
		r.Flag = fmt.Sprintf("nothing relayed - %d transmissions from %d senders",
			r.TX, len(a.exp.Senders))
	}
	return r
}

// expSummary is one arm, averaged over its seeds, with the spread that says
// whether the average means anything.
type expSummary struct {
	Arm      string
	Runs     int
	Flagged  int
	TX, RX   float64
	Coll     float64
	Deaf     float64
	Reach    float64
	Messages float64
	RepPct   float64
	CompPct  float64
	Airtime  float64
	SpanMs   float64
	RXSpread float64 // half the range across seeds, as a fraction of the mean
}

func (e *experiment) summarise() []expSummary {
	byArm := map[string][]expResult{}
	var order []string
	for _, r := range e.results {
		if _, seen := byArm[r.Arm]; !seen {
			order = append(order, r.Arm)
		}
		byArm[r.Arm] = append(byArm[r.Arm], r)
	}
	var out []expSummary
	for _, arm := range order {
		rs := byArm[arm]
		s := expSummary{Arm: arm, Runs: len(rs)}
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, r := range rs {
			if r.Flag != "" || r.Err != "" {
				s.Flagged++
			}
			s.TX += float64(r.TX)
			s.RX += float64(r.RX)
			s.Coll += float64(r.Collisions)
			s.Deaf += float64(r.Deaf)
			s.Reach += r.MeanReachPct
			s.Messages += float64(r.Messages)
			if r.RepChances > 0 {
				s.RepPct += float64(r.RepHit) / float64(r.RepChances) * 100
			}
			if r.CompChances > 0 {
				s.CompPct += float64(r.CompHit) / float64(r.CompChances) * 100
			}
			s.Airtime += r.AirtimeMs
			s.SpanMs += float64(r.SpanMs)
			lo = math.Min(lo, float64(r.RX))
			hi = math.Max(hi, float64(r.RX))
		}
		n := float64(len(rs))
		if n > 0 {
			s.TX /= n
			s.RX /= n
			s.Coll /= n
			s.Deaf /= n
			s.Reach /= n
			s.Messages /= n
			s.RepPct /= n
			s.CompPct /= n
			s.Airtime /= n
			s.SpanMs /= n
		}
		if s.RX > 0 && !math.IsInf(lo, 1) {
			s.RXSpread = (hi - lo) / 2 / s.RX
		}
		out = append(out, s)
	}
	return out
}

// notAResultYet is the most valuable line in the view.
//
// When the seeds disagree by more than the arms do, the difference on screen is
// the draw rather than the parameter, and the honest thing is to say so before
// somebody writes it down. This is the mistake that is hardest to catch by eye
// and easiest to publish.
func (e *experiment) notAResultYet() string {
	sums := e.summarise()
	if len(sums) < 2 {
		return ""
	}
	lo, hi, spread := math.Inf(1), math.Inf(-1), 0.0
	for _, s := range sums {
		lo = math.Min(lo, s.RX)
		hi = math.Max(hi, s.RX)
		spread = math.Max(spread, s.RXSpread)
	}
	if hi <= 0 {
		return ""
	}
	between := (hi - lo) / hi
	if spread >= between {
		return fmt.Sprintf(
			"the seeds disagree by more than the arms do on receptions (±%.1f%% within an arm, "+
				"%.1f%% between them). Not a result yet - add repeats.", spread*100, between*100)
	}
	return ""
}

// firstDivergence reports where two runs' ledgers stop agreeing.
//
// Totals say something changed; the first differing event says where to look —
// and two runs can agree on every total without being the same run at all.
func firstDivergence(a, b []engine.Event) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	diffs := 0
	first := ""
	sameTiming := true
	for i := 0; i < n; i++ {
		x, y := a[i], b[i]
		if x.AtMs != y.AtMs || x.Kind != y.Kind || x.From != y.From || x.To != y.To {
			sameTiming = false
		}
		if !sameEvent(x, y) {
			diffs++
			if first == "" {
				first = fmt.Sprintf("event %d at %.3f s\n  A  %s %s -> %s  %s\n  B  %s %s -> %s  %s",
					i, float64(x.AtMs)/1000, x.Kind, x.From, x.To, x.Detail,
					y.Kind, y.From, y.To, y.Detail)
			}
		}
	}
	if first == "" && len(a) == len(b) {
		return "identical: every one of the " + fmt.Sprint(n) + " events matches"
	}
	if first == "" {
		return fmt.Sprintf("identical for %d events, then one ledger ends (%d vs %d)", n, len(a), len(b))
	}
	out := fmt.Sprintf("first divergence at %s\n\n%d of %d events differ.", first, diffs, n)
	if sameTiming {
		// Worth saying on its own: it is the difference between firmware that
		// behaves the same and a simulator that never asked it to behave
		// differently.
		out += "\nTiming is identical - every event happened at the same instant, from the same node, to the same node."
	}
	return out
}

// verdict is the answer the experiment was run to get.
//
// "Did it make a difference?" deserves a sentence, not a table somebody has to
// interpret. And when the answer is no, the interesting question immediately
// becomes *why* — a parameter that changed nothing because it never reached the
// firmware looks exactly like one that genuinely does not matter, and the CAD
// study spent an evening on precisely that distinction.
type verdict struct {
	Difference bool
	Headline   string
	Detail     []string
	// Investigation is filled only when nothing differed.
	Investigation []string
}

func (a *App) verdictFor(e *experiment) verdict {
	sums := e.summarise()
	if len(sums) < 2 {
		return verdict{Headline: "one arm: nothing to compare it against"}
	}
	base := sums[0]
	if e.baselineArm > 0 && e.baselineArm < len(sums) {
		base = sums[e.baselineArm]
	}

	var v verdict
	var biggest float64
	var biggestArm, biggestWhat string
	spread := 0.0
	for _, s := range sums {
		spread = math.Max(spread, s.RXSpread)
	}
	for _, s := range sums {
		if s.Arm == base.Arm {
			continue
		}
		for _, m := range []struct {
			name     string
			got, ref float64
		}{
			{"receptions", s.RX, base.RX},
			{"transmissions", s.TX, base.TX},
			{"collisions", s.Coll, base.Coll},
			{"airtime", s.Airtime, base.Airtime},
			{"delivery to repeaters", s.RepPct, base.RepPct},
			{"delivery to companions", s.CompPct, base.CompPct},
		} {
			if m.ref == 0 {
				continue
			}
			d := (m.got - m.ref) / m.ref
			v.Detail = append(v.Detail, fmt.Sprintf("%s: %s %+.1f%% against %s",
				s.Arm, m.name, d*100, base.Arm))
			if math.Abs(d) > math.Abs(biggest) {
				biggest, biggestArm, biggestWhat = d, s.Arm, m.name
			}
		}
	}

	// A difference smaller than the noise between seeds is a draw, not a
	// finding. This is the check that stops a plausible number being published.
	switch {
	case math.Abs(biggest) < 0.005:
		v.Difference = false
		v.Headline = "No difference. Every arm produced the same numbers."
	case math.Abs(biggest) <= spread:
		v.Difference = false
		v.Headline = fmt.Sprintf(
			"No difference worth reporting: the largest change (%s, %s %+.1f%%) is inside "+
				"the ±%.1f%% the seeds vary by on their own.",
			biggestArm, biggestWhat, biggest*100, spread*100)
	default:
		v.Difference = true
		v.Headline = fmt.Sprintf("%s changed %s by %+.1f%%, against a seed spread of ±%.1f%%.",
			biggestArm, biggestWhat, biggest*100, spread*100)
	}
	if !v.Difference {
		v.Investigation = a.investigate(e)
	}
	return v
}

// investigate asks why two configurations produced the same result.
//
// The checks are ordered by how badly each one would mislead. A parameter that
// never reached the firmware is indistinguishable, in the numbers, from one that
// genuinely does not matter — and the second is a finding while the first is a
// bug in the harness.
func (a *App) investigate(e *experiment) []string {
	var out []string
	byArm := map[string][]expResult{}
	for _, r := range e.results {
		byArm[r.Arm] = append(byArm[r.Arm], r)
	}

	// 1. Are the ledgers identical? Nothing about the run differed at all.
	if len(e.Arms) >= 2 {
		a0 := findResult(e.results, e.Arms[0].Label, e.Seeds[0])
		a1 := findResult(e.results, e.Arms[1].Label, e.Seeds[0])
		if a0 != nil && a1 != nil {
			d := firstDivergence(a0.ledger, a1.ledger)
			if strings.HasPrefix(d, "identical") {
				out = append(out,
					"The two ledgers are byte-identical: not one event differs. The arms did not "+
						"merely perform alike, they behaved identically, which means the thing being "+
						"varied never reached the code that would act on it.")
			} else if strings.Contains(d, "Timing is identical") {
				out = append(out,
					"Every event happened at the same instant, from and to the same nodes; only the "+
						"packet contents differ. Whatever changed is carried in the packets and is not "+
						"affecting any node's decision about when to transmit.")
			} else {
				out = append(out, "The ledgers do differ - "+strings.SplitN(d, "\n", 2)[0]+
					" - so behaviour changed even though the totals did not.")
			}
		}
	}

	// 2. Did the arms actually ask for different things?
	same := true
	for i := 1; i < len(e.Arms); i++ {
		if e.Arms[i] != e.Arms[0] {
			same = false
			break
		}
	}
	if same {
		out = append(out, "Every arm carries the same settings. The sweep varied nothing.")
	}

	// 3. Did the packets themselves change? A path hash mode that took effect
	//    changes how many bytes each hop adds, which shows in transmitted size.
	sizes := map[string]map[int]int{}
	for arm, rs := range byArm {
		sizes[arm] = map[int]int{}
		for _, r := range rs {
			for _, ev := range r.ledger {
				if ev.Kind != "tx" {
					continue
				}
				var n int
				if _, err := fmt.Sscanf(ev.Detail, "%d bytes", &n); err == nil {
					sizes[arm][n]++
				}
			}
		}
	}
	if len(sizes) >= 2 {
		var arms []string
		for arm := range sizes {
			arms = append(arms, arm)
		}
		a0, a1 := sizes[arms[0]], sizes[arms[1]]
		if sameSizeHistogram(a0, a1) {
			out = append(out,
				"Transmitted packet sizes are identical across arms. A setting that changes what "+
					"goes on the air - path hash size, for one - would show here, so either it was "+
					"not applied or it does not touch the packets these nodes send.")
		}
	}

	// 4. Was there any contention to arbitrate?
	sums := e.summarise()
	if len(sums) > 0 && sums[0].Coll < 1 {
		out = append(out,
			"There were essentially no collisions in any arm. A change to when a node decides to "+
				"transmit cannot show itself on a quiet channel - raise the number of simultaneous "+
				"senders until the mesh is actually contending.")
	}

	// 5. The one that cost an evening.
	out = append(out,
		"Worth checking directly: whether the setting is readable back from a node. Ask a repeater "+
			"over its console and a companion over its protocol what it actually holds. A command "+
			"the firmware rejected, or one its build does not implement, reports success and changes "+
			"nothing - which is exactly what this looks like.")
	return out
}

func findResult(rs []expResult, arm string, seed uint64) *expResult {
	for i := range rs {
		if rs[i].Arm == arm && rs[i].Seed == seed {
			return &rs[i]
		}
	}
	return nil
}

func sameSizeHistogram(a, b map[int]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// sameEvent compares two ledger entries. Events carry a frame, so they are not
// comparable with ==; everything that identifies what happened is compared
// instead, including the frame's bytes.
func sameEvent(x, y engine.Event) bool {
	return x.AtMs == y.AtMs && x.Kind == y.Kind && x.From == y.From && x.To == y.To &&
		x.Detail == y.Detail && x.PacketID == y.PacketID && string(x.Frame) == string(y.Frame)
}

// isPristine reports an arm that varies nothing - the placeholder the sweep
// builder starts with, which exists to be replaced rather than crossed onto.
func (a expArm) isPristine() bool {
	return a.RepeaterVersion == "" && a.CompanionVersion == "" &&
		a.LoopDetect == "" && a.CAD == "" && a.PathHashMode < 0
}

// padTo makes a message a given length, so payload size is an experimental
// variable rather than an accident of how the arm happens to be named.
func padTo(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(".", n-len(s))
}

// fireOne sends this run's message from one companion.
func (a *App) fireOne(e *experiment, node, arm string, seed uint64) {
	s := a.comps[node]
	if s == nil {
		return
	}
	if e.Scope != "" {
		_ = a.compSetScope(s, node, e.Scope)
	}
	idx := uint8(0)
	s.mu.Lock()
	for _, c := range s.channels {
		if strings.EqualFold(c.Name, e.Channel) {
			idx = c.Index
		}
	}
	s.mu.Unlock()
	text := fmt.Sprintf("%s seed %d", arm, seed)
	if e.Bytes > 0 {
		text = padTo(text, e.Bytes)
	}
	_ = a.compSend(s, proto.SendChannelText(idx, time.Now(), text))
}
