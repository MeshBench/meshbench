// A meshcore-cli for a simulated companion.
//
// A repeater has a text console: you type at its UART and it answers. A
// companion does not - it speaks the framed binary protocol a phone speaks,
// so the console panel showed one nothing at all and there was no way to make
// a companion do anything by hand.
//
// meshcore-dev/meshcore-cli is the tool people already use for exactly this
// against real hardware, so its vocabulary is the one to offer rather than a
// new one nobody knows. This implements the subset the simulator can honestly
// answer, and refuses the rest by name: a command that silently does nothing
// is worse than one that says it is not here.
package session

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/mesh/proto"
)

// cliCommand is one meshcore-cli verb this build answers.
type cliCommand struct {
	name  string
	short string
	args  string
	what  string
	// run sends the frames. It returns what to print, or an error.
	run func(s *Sim, node string, args []string) (string, error)
}

// meshcliCommands is the implemented subset, in meshcore-cli's own order.
var meshcliCommands = []cliCommand{
	{"infos", "i", "", "what this node says it is", func(s *Sim, node string, _ []string) (string, error) {
		if err := s.compFrame(node, proto.DeviceQuery()); err != nil {
			return "", err
		}
		return "asked; the answer arrives when the engine next steps", nil
	}},
	{"ver", "v", "", "firmware version", func(s *Sim, node string, _ []string) (string, error) {
		if err := s.compFrame(node, proto.DeviceQuery()); err != nil {
			return "", err
		}
		return "asked", nil
	}},
	{"advert", "a", "", "send an advert", func(s *Sim, node string, _ []string) (string, error) {
		if err := s.compFrame(node, proto.SendSelfAdvert(false)); err != nil {
			return "", err
		}
		return "advert sent", nil
	}},
	{"floodadv", "", "", "send a flood advert", func(s *Sim, node string, _ []string) (string, error) {
		if err := s.compFrame(node, proto.SendSelfAdvert(true)); err != nil {
			return "", err
		}
		return "flood advert sent", nil
	}},
	{"chan", "ch", "<n> <msg>", "send to a channel", func(s *Sim, node string, a []string) (string, error) {
		if len(a) < 2 {
			return "", fmt.Errorf("chan <number> <message>")
		}
		n, err := strconv.Atoi(a[0])
		if err != nil {
			return "", fmt.Errorf("channel number: %w", err)
		}
		msg := strings.Join(a[1:], " ")
		if err := s.compFrame(node, proto.SendChannelText(uint8(n), time.Now(), msg)); err != nil {
			return "", err
		}
		return fmt.Sprintf("sent to channel %d: %s", n, msg), nil
	}},
	{"public", "dch", "<msg>", "send to the public channel", func(s *Sim, node string, a []string) (string, error) {
		if len(a) == 0 {
			return "", fmt.Errorf("public <message>")
		}
		msg := strings.Join(a, " ")
		if err := s.compFrame(node, proto.SendChannelText(0, time.Now(), msg)); err != nil {
			return "", err
		}
		return "sent to the public channel: " + msg, nil
	}},
	{"contacts", "lc", "", "the contact list", func(s *Sim, node string, _ []string) (string, error) {
		if err := s.compFrame(node, proto.GetContacts(time.Unix(0, 0))); err != nil {
			return "", err
		}
		return "asked for contacts", nil
	}},
	{"get_channel", "", "<n>", "one channel's name and key", func(s *Sim, node string, a []string) (string, error) {
		n := 0
		if len(a) > 0 {
			v, err := strconv.Atoi(a[0])
			if err != nil {
				return "", fmt.Errorf("channel number: %w", err)
			}
			n = v
		}
		if err := s.compFrame(node, proto.GetChannel(uint8(n))); err != nil {
			return "", err
		}
		return fmt.Sprintf("asked for channel %d", n), nil
	}},
	{"sync_msgs", "sm", "", "read unread messages", func(s *Sim, node string, _ []string) (string, error) {
		if err := s.compFrame(node, proto.SyncNextMessage()); err != nil {
			return "", err
		}
		return "asked for the next message", nil
	}},
	{"set", "", "<param> <value>", "name, tx, lat/lon, radio", func(s *Sim, node string, a []string) (string, error) {
		if len(a) < 2 {
			return "", fmt.Errorf("set <name|tx|pos|radio> <value>; " +
				"pos takes \"lat lon\", radio takes \"freqkHz bwHz sf cr\"")
		}
		switch a[0] {
		case "name":
			v := strings.Join(a[1:], " ")
			return "name set to " + v, s.compFrame(node, proto.SetAdvertName(v))
		case "tx":
			v, err := strconv.Atoi(a[1])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("tx power %d dBm", v),
				s.compFrame(node, proto.SetTxPower(uint8(v)))
		case "pos":
			if len(a) < 3 {
				return "", fmt.Errorf("set pos <lat> <lon>")
			}
			lat, err1 := strconv.ParseFloat(a[1], 64)
			lon, err2 := strconv.ParseFloat(a[2], 64)
			if err1 != nil || err2 != nil {
				return "", fmt.Errorf("set pos <lat> <lon>")
			}
			return fmt.Sprintf("position %.5f, %.5f", lat, lon),
				s.compFrame(node, proto.SetAdvertLatLon(lat, lon))
		case "radio":
			if len(a) < 5 {
				return "", fmt.Errorf("set radio <freqkHz> <bwHz> <sf> <cr>")
			}
			f, _ := strconv.Atoi(a[1])
			bw, _ := strconv.Atoi(a[2])
			sf, _ := strconv.Atoi(a[3])
			cr, _ := strconv.Atoi(a[4])
			return "radio set",
				s.compFrame(node, proto.SetRadioParams(uint32(f), uint32(bw), uint8(sf), uint8(cr)))
		}
		return "", fmt.Errorf("no parameter %q; there is name, tx, pos and radio", a[0])
	}},
	{"time", "", "<epoch>", "set the clock", func(s *Sim, node string, a []string) (string, error) {
		// The clock matters because a node with the wrong one rejects
		// messages as replays, which looks like a radio problem.
		return "clock set", fmt.Errorf(
			"not implemented here: every simulated node already shares the run's clock")
	}},
}

// compFrame sends one framed payload to a connected companion.
func (s *Sim) compFrame(node string, payload []byte) error {
	_, en, err := s.companionFor(node)
	if err != nil {
		return err
	}
	return en.Firmware.Bridge.Type(compFrame(payload))
}

// meshcliHelp is what "?" prints: only what is here.
func meshcliHelp() string {
	var b strings.Builder
	b.WriteString("meshcore-cli commands this simulator answers:\n")
	for _, c := range meshcliCommands {
		name := c.name
		if c.short != "" {
			name += "  (" + c.short + ")"
		}
		line := fmt.Sprintf("  %-22s %-18s %s\n", name, c.args, c.what)
		b.WriteString(line)
	}
	b.WriteString("\nAnything else meshcore-cli offers is refused by name rather than\n" +
		"quietly ignored - this is a simulated companion, not a radio.")
	return b.String()
}

func registerMeshCLI(st *state.Store, s *Sim) {
	// console.cli: one meshcore-cli line, for a companion.
	st.Handle("console.cli", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		line, _ := namedField(p, "command")
		if m, ok := p.(map[string]any); ok {
			node, _ = m["node"].(string)
			line, _ = m["command"].(string)
		}
		line = strings.TrimSpace(line)
		if node == "" || line == "" {
			return nil, fmt.Errorf("console.cli needs a node and a command")
		}
		// The transcript this console draws. Through the session's rolling
		// note buffer when one exists; straight into the world when none does
		// yet, because help and a failed connect must still show up where
		// they were typed - the box says "? for the list", and a ? that
		// prints nothing reads as a command line that is broken.
		say := func(lines ...string) {
			if sess := s.comps[node]; sess != nil {
				for _, l := range lines {
					sess.note(l)
				}
				w.Console, w.ConsoleNode = sess.Lines(), node
				return
			}
			if w.ConsoleNode != node {
				w.Console = nil
			}
			w.Console = append(w.Console, lines...)
			w.ConsoleNode = node
		}
		// Help is local knowledge, so it answers whether or not anything is
		// connected - asking what the commands are must not boot a node.
		if line == "?" || line == "help" {
			say("> " + line)
			say(strings.Split(meshcliHelp(), "\n")...)
			return map[string]any{"node": node, "reply": meshcliHelp()}, nil
		}
		// Giving the port back is not a meshcore-cli command - a real client
		// disconnects by closing the link - so it has a name that cannot
		// collide with one.
		if line == "__disconnect" {
			if c, ok := s.comps[node]; ok {
				if c.release != nil {
					c.release()
				}
				delete(s.comps, node)
				w.Say(node + " released; its console has the port back")
				return map[string]any{"node": node, "reply": "released"}, nil
			}
			return map[string]any{"node": node, "reply": "was not connected"}, nil
		}
		fields := strings.Fields(line)
		head, args := fields[0], fields[1:]

		// Connect on first use: a command line that makes you connect first
		// is one more step than the tool it is imitating has. A connect that
		// fails answers in the console like everything else - it used to be
		// returned as an error, which went to the status bar and left the
		// console with no echo and no explanation at all.
		if _, ok := s.comps[node]; !ok {
			if err := s.connectCompanion(node); err != nil {
				say("> "+line, err.Error())
				return map[string]any{"node": node, "reply": err.Error(), "failed": true}, nil
			}
		}
		// The echo goes in before the command runs, not after it succeeds.
		// Echoing on the way out meant a failing command left no trace of
		// itself at all: no echo, no error, and an empty console where it was
		// typed.
		say("> " + line)

		for _, c := range meshcliCommands {
			if head != c.name && (c.short == "" || head != c.short) {
				continue
			}
			out, err := c.run(s, node, args)
			if err != nil {
				// Reported, not returned. A command that ran and answered
				// "no" is not a verb that failed, and returning an error
				// sends the explanation to the status bar and the session log
				// - which is not where anybody typing into a console is
				// looking.
				say(err.Error())
				return map[string]any{"node": node, "reply": err.Error(), "failed": true}, nil
			}
			// The reply arrives when the engine next steps, same as a
			// repeater's console. Playing, the ticker does that within
			// milliseconds; paused, nothing ever would - so a command typed
			// while the client mode had just been used (which needs no step,
			// since it draws its own decoded state) got no answer until play
			// was pressed, and read as the command line having stopped
			// working rather than the clock having stopped.
			if !w.Playing {
				for i := 0; i < 60; i++ {
					_ = s.eng.Step(context.Background())
				}
				w.NowMs = s.eng.NowMs()
			}
			say(out)
			return map[string]any{"node": node, "reply": out}, nil
		}
		var have []string
		for _, c := range meshcliCommands {
			have = append(have, c.name)
		}
		sort.Strings(have)
		unknown := fmt.Sprintf("no command %q. This answers: %s",
			head, strings.Join(have, ", "))
		if meshcliKnows(head) {
			unknown = fmt.Sprintf(
				"meshcore-cli has %q and this simulator does not. It answers: %s",
				head, strings.Join(have, ", "))
		}
		say(unknown)
		return map[string]any{"node": node, "reply": unknown, "failed": true}, nil
	})
}

// meshcliRest is the rest of meshcore-cli's vocabulary: commands it has that
// this build does not answer.
//
// Listed so the refusal can tell the two cases apart. Typing a real
// meshcore-cli command and being told the simulator lacks it is information;
// being told the same thing about a typo is not.
var meshcliRest = map[string]bool{
	"script": true, "self_telemetry": true, "card": true, "reboot": true,
	"sleep": true, "wait_key": true, "apply_to": true, "alias": true,
	"aliases": true, "aliases_load": true, "handler_attach": true,
	"handler_detach": true, "msg": true, "wait_ack": true, "recv": true,
	"wait_msg": true, "msgs_subscribe": true, "get_channels": true,
	"set_channel": true, "remove_channel": true, "scope": true, "get": true,
	"node_discover": true, "reload_contacts": true, "contact_info": true,
	"contact_timeout": true, "share_contact": true, "export_contact": true,
	"import_contact": true, "remove_contact": true, "path": true,
	"disc_path": true, "reset_path": true, "change_path": true,
	"change_flags": true, "req_telemetry": true, "req_mma": true,
	"req_acl": true, "pending_contacts": true, "add_pending": true,
	"flush_pending": true, "login": true, "logout": true, "cmd": true,
	"wmt8": true, "req_status": true, "req_neighbours": true, "trace": true,
	"to": true, "list": true,
}

func meshcliKnows(name string) bool {
	if meshcliRest[name] {
		return true
	}
	for _, c := range meshcliCommands {
		if name == c.name || (c.short != "" && name == c.short) {
			return true
		}
	}
	return false
}
