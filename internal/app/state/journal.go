package state

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// The journal: every command the store has been driven with, so a session
// picked up cold can be asked how it got here.
//
// Two states that look identical in a snapshot can have very different
// histories - a mesh warmed from a fixture and one built node by node read the
// same - and nothing else records the difference. The scenario lives in the
// process and a restart loses it, so the journal is also the only thing that
// says a restart happened.
//
// Written where every verb already funnels through, on the store's own
// goroutine, so a click and a script are recorded on the same path. What is
// left out is the point of it: a journal that records every poll and every
// worker's progress report is noise, not a history. See ExcludeFromJournal.

// journalCap bounds the ring. A driven session is a few hundred real commands;
// past that the oldest fall off, because the recent history is the useful one
// and an unbounded log of a long run is the thing this is trying not to be.
const journalCap = 512

// JournalEntry is one command, as it was given. AtMs is wall-clock unix
// milliseconds so a reader can tell "nothing before this" from "nothing yet".
// Nodes is the node count when it ran, so the history shows the scenario
// growing; Err is set when the command failed, because a refused command is
// part of how a session got here too. Arg is a compact rendering of the
// argument - which file was opened, which seed was set - not the whole of it,
// which for some verbs is a binary frame.
//
// The first entry of every session is session.start, so a restart is visible
// rather than inferred: a journal that begins mid-sequence is one whose process
// was rebuilt under it.
type JournalEntry struct {
	Seq   uint64 `json:"seq"`
	AtMs  int64  `json:"at_ms"`
	Verb  string `json:"verb"`
	Nodes int    `json:"nodes"`
	Err   string `json:"err,omitempty"`
	Arg   string `json:"arg,omitempty"`
}

// ExcludeFromJournal names the verbs the journal must not record. It is called
// once at start-up, before the store runs, so it needs no lock.
//
// Two kinds are left out. The worker callbacks - a background goroutine calling
// back through the store to publish a coverage raster or report a firmware
// process is up - are the process talking to itself, not a command anyone gave.
// The polls - listing nodes, reading a console, asking the current state - a
// script fires in a loop to wait for something, and a hundred of them say
// nothing about what changed. What is left is the history.
func (s *Store) ExcludeFromJournal(verbs ...string) {
	if s.journalSkip == nil {
		s.journalSkip = make(map[string]bool, len(verbs))
	}
	for _, v := range verbs {
		s.journalSkip[v] = true
	}
}

// SkipsJournal reports whether a verb is one of the excluded. Asked by the test
// that holds the exclusions against the registrations: the exclusions are set
// once at start-up and nothing later can tell they were.
func (s *Store) SkipsJournal(verb string) bool { return s.journalSkip[verb] }

// journalRecord appends one command, unless it is excluded - errors included,
// because a refused command is history too. Called on the store's goroutine,
// from the same place a verb is dispatched.
func (s *Store) journalRecord(verb string, params any, err error) {
	if s.journalSkip[verb] {
		return
	}
	e := JournalEntry{
		Seq: s.seq, AtMs: time.Now().UnixMilli(), Verb: verb,
		Nodes: len(s.world.Nodes), Arg: journalArg(params),
	}
	if err != nil {
		e.Err = err.Error()
	}
	s.appendJournal(e)
}

func (s *Store) appendJournal(e JournalEntry) {
	if len(s.journal) >= journalCap {
		copy(s.journal, s.journal[1:])
		s.journal = s.journal[:journalCap-1]
	}
	s.journal = append(s.journal, e)
}

// Journal is the started-at time and the commands since, newest last, copied so
// the caller may hold it while the store keeps recording. Read from a verb
// handler, which runs on the store's goroutine, so it takes no lock.
func (s *Store) Journal() (startedMs int64, entries []JournalEntry) {
	out := make([]JournalEntry, len(s.journal))
	copy(out, s.journal)
	return s.journalStart.UnixMilli(), out
}

// journalArg renders an argument small enough to keep: a bare string as itself,
// a parameter map as its sorted key=value pairs, and anything else - a matrix,
// a frame - as nothing, because its shape is not what a history is for. Bounded
// so one oversized argument cannot blow the ring's memory.
func journalArg(params any) string {
	const max = 120
	clip := func(s string) string {
		s = strings.TrimSpace(s)
		if len(s) > max {
			return s[:max-1] + "…"
		}
		return s
	}
	switch p := params.(type) {
	case nil:
		return ""
	case string:
		return clip(p)
	case map[string]any:
		keys := make([]string, 0, len(p))
		for k := range p {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			switch v := p[k].(type) {
			case string, float64, int, bool:
				parts = append(parts, fmt.Sprintf("%s=%v", k, v))
			default:
				parts = append(parts, k)
			}
		}
		return clip(strings.Join(parts, " "))
	default:
		return ""
	}
}
