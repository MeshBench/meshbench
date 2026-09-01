// One build, in full - and changing what it is.
//
// The library lists builds; these two answer questions about one of them and
// act on it. Split out because a library row is deliberately a row: role,
// version, size, a tick. Where a build came from, what it actually is, and
// what has been decided about it are the questions somebody has once a build
// will not do what they expected, and there was nowhere to ask them.
package session

import (
	"fmt"
	"os"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
)

func registerFirmwareDetail(st *state.Store, s *Sim) {
	registerFirmwareDetails(st, s)
	registerFirmwareUpdate(st, s)
}

func registerFirmwareDetails(st *state.Store, s *Sim) {
	st.Handle("firmware.details", func(w *state.World, p any) (any, error) {
		row, err := findBuildRow(w, s, p)
		if err != nil {
			return nil, err
		}
		out := map[string]any{
			"role": row.Role, "version": row.Version, "board": row.Board,
			"native": row.Native, "on_disk": row.OnDisk, "path": row.Path,
			"bytes": row.Bytes, "in_use": row.InUse,
			"kind": row.Facts.Kind, "bootable": row.Facts.Bootable,
			"flash_mb":        row.Facts.FlashMB,
			"coproc_at_reset": row.Settings.CoprocAtReset,
			"card_required":   row.Settings.CardRequired,
			"notes":           row.Settings.Notes,
		}
		if !row.Modified.IsZero() {
			out["modified"] = row.Modified.UTC().Format("2006-01-02 15:04:05Z")
		}
		// Where the settings would be written, whether or not any exist: the
		// question "where does this live" is asked of a build that has none
		// as often as of one that has.
		if row.Path != "" {
			out["settings_path"] = firmware.SettingsPath(row.Path)
		}
		return out, nil
	})
}

func registerFirmwareUpdate(st *state.Store, s *Sim) {
	st.Handle("firmware.update", func(w *state.World, p any) (any, error) {
		row, err := findBuildRow(w, s, p)
		if err != nil {
			return nil, err
		}
		if !row.OnDisk {
			return nil, badParams(
				"%s %s is not on this machine, so there is nothing to change - "+
					"download or import it first", row.Role, row.Version)
		}
		// Refused while something is running it, rather than moving a file an
		// emulator has open. On Linux the run would continue against a file
		// with no name and finish normally, which is worse than a refusal: the
		// change would appear to have had no effect.
		if err := s.refuseWhileBuildRuns(row); err != nil {
			return nil, err
		}

		in := firmware.Installed{
			Native: row.Native, Version: row.Version, Role: row.Role,
			Board: row.Board, Path: row.Path, Bytes: row.Bytes,
		}
		label := firstOf(namedOf(p, "label"), row.Version)
		newRole := firstOf(namedOf(p, "new_role"), row.Role)
		newBoard := firstOf(namedOf(p, "new_board"), row.Board)
		repinned := 0
		renamed := false
		if label != row.Version || newRole != row.Role || newBoard != row.Board {
			moved, err := firmware.Rename(firmware.DefaultCacheDir(), in, newRole, label, newBoard)
			if err != nil {
				return nil, err
			}
			renamed = true
			in = moved
			// Nodes pin a build by the name it had. Left alone they would
			// point at a name nothing answers to, and the failure would
			// arrive at the next start as "no image in the cache" - about a
			// build sitting in the library under its new name.
			repinned = s.repinFirmware(w, row, moved)
		}

		set := firmware.LoadBuildSettings(in.Path)
		if on, ok := boolOf(p, "coproc_at_reset"); ok {
			set.CoprocAtReset = on
		}
		if notes, ok := namedField(p, "notes"); ok {
			set.Notes = notes
		}
		if on, ok := boolOf(p, "card_required"); ok {
			set.CardRequired = on
		}
		if err := firmware.SaveBuildSettings(in.Path, set); err != nil {
			return nil, err
		}
		s.fillLibrary(w)
		// A build that now insists on storage changes what every node running
		// it will boot with, and the node windows draw that.
		s.publishCards(w)
		if renamed {
			w.Say(fmt.Sprintf("%s is now %s %s", row.Version, in.Role, in.Version))
		} else {
			w.Say("updated " + in.Role + " " + in.Version)
		}
		return map[string]any{
			"role": in.Role, "version": in.Version, "board": in.Board,
			"path": in.Path, "renamed": renamed, "repinned": repinned,
			"settings": map[string]any{
				"coproc_at_reset": set.CoprocAtReset, "notes": set.Notes,
				"card_required": set.CardRequired,
			},
		}, nil
	})
}

// findBuildRow is which build a caller meant.
//
// Matched against the library rather than the cache directly, because the
// library is what they were looking at: it carries the published builds that
// are not on disk, the count of nodes using each, and the facts read at the
// last rebuild. A role or a board is needed only to break a tie, so the common
// case - one label, one build - is a single word.
func findBuildRow(w *state.World, s *Sim, p any) (state.FirmwareRow, error) {
	version := soleString(p)
	if v, ok := namedField(p, "version"); ok {
		version = v
	}
	if version == "" {
		return state.FirmwareRow{}, badParams("this needs a build's version or label")
	}
	role, _ := namedField(p, "role")
	board, hasBoard := namedField(p, "board")
	if len(w.Library) == 0 {
		s.fillLibrary(w)
	}
	var found []state.FirmwareRow
	for _, r := range w.Library {
		if r.Version != version {
			continue
		}
		if role != "" && r.Role != role {
			continue
		}
		if hasBoard && r.Board != board {
			continue
		}
		found = append(found, r)
	}
	switch len(found) {
	case 0:
		return state.FirmwareRow{}, badParams(
			"no build called %q in the library%s", version, roleBoardSuffix(role, board, hasBoard))
	case 1:
		return found[0], nil
	}
	// Ambiguous rather than guessed at. Picking the first would act on a
	// different build from the one somebody is looking at, and a rename is
	// not something to be wrong about quietly.
	var which []string
	for _, r := range found {
		where := r.Board
		if where == "" {
			where = "this machine"
		}
		which = append(which, r.Role+" on "+where)
	}
	return state.FirmwareRow{}, badParams(
		"%q names %d builds - say which with role and board: %s",
		version, len(found), strings.Join(which, ", "))
}

func roleBoardSuffix(role, board string, hasBoard bool) string {
	var parts []string
	if role != "" {
		parts = append(parts, "role "+role)
	}
	if hasBoard {
		if board == "" {
			parts = append(parts, "for this machine")
		} else {
			parts = append(parts, "board "+board)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return " with " + strings.Join(parts, " and ")
}

// refuseWhileBuildRuns turns away a change to a build a node is running.
func (s *Sim) refuseWhileBuildRuns(row state.FirmwareRow) error {
	if s.eng == nil {
		return nil
	}
	for _, n := range s.nodes {
		if n.Firmware.Version != row.Version || n.Firmware.Board != row.Board {
			continue
		}
		if en, ok := s.eng.NodeByName(n.Name); ok && en.Firmware != nil {
			return fmt.Errorf(
				"%s is running %s right now: stop it first, or the emulator "+
					"would go on reading a file that is no longer there",
				n.Name, row.Version)
		}
	}
	return nil
}

// repinFirmware points every node that named the old build at the new one.
func (s *Sim) repinFirmware(w *state.World, was state.FirmwareRow, now firmware.Installed) int {
	n := 0
	for i := range s.nodes {
		if s.nodes[i].Firmware.Version != was.Version ||
			s.nodes[i].Firmware.Board != was.Board {
			continue
		}
		s.nodes[i].Firmware.Version = now.Version
		s.nodes[i].Firmware.Board = now.Board
		if s.eng != nil {
			s.eng.PinFirmware(s.nodes[i].Name, now.Version)
		}
		for j := range w.Nodes {
			if w.Nodes[j].Name == s.nodes[i].Name {
				w.Nodes[j].Firmware = now.Version
			}
		}
		n++
	}
	return n
}

// namedOf is namedField without the second return, for a field whose absence
// and whose emptiness mean the same thing.
func namedOf(p any, name string) string {
	v, _ := namedField(p, name)
	return v
}

// firstOf is the first of two that is not empty.
func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// boolOf reads a flag that has to be named, and says whether it was there at
// all - so "leave it alone" and "turn it off" stay different answers.
func boolOf(p any, name string) (bool, bool) {
	m, ok := p.(map[string]any)
	if !ok {
		return false, false
	}
	switch v := m[name].(type) {
	case bool:
		return v, true
	case string:
		// The control socket and the command line both arrive as text.
		s := strings.TrimSpace(strings.ToLower(v))
		if s == "" {
			return false, false
		}
		return s != "0" && s != "false" && s != "no" && s != "off", true
	case float64:
		return v != 0, true
	}
	return false, false
}

// deleteBuildSettings removes what was decided about a build being deleted.
//
// Here rather than inside firmware.Remove because firmware.delete is driven by
// a path rather than by an Installed. A settings file left behind would be
// inherited by the next build imported under the same name, which is a build
// silently running with somebody else's answer.
func deleteBuildSettings(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(firmware.SettingsPath(path))
}
