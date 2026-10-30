// meshcore-cli's commands for talking to one node rather than to a channel:
// a direct message, a login and a status request, plus the scope those and
// every flood go out under.
//
// Each names its recipient the way meshcore-cli does, by contact name or by
// public key in hex, and each is answered twice: once when the firmware says
// it sent the request, once when the ack or the reply comes back. Both land in
// the transcript, matched to the request (companionreceipt.go), so "sent" and
// "delivered" are never confused.
package session

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/mesh/proto"
	"github.com/MeshBench/meshbench/internal/world/provider"
)

var directCommands = []cliCommand{
	{"msg", "m", "<contact> <msg>", "send a direct message", func(s *Sim, node string, a []string) (string, error) {
		if len(a) < 2 {
			return "", fmt.Errorf("msg <contact name or key> <message>")
		}
		to, key, err := s.contactKey(node, a[0], 6)
		if err != nil {
			return "", err
		}
		text := strings.Join(a[1:], " ")
		frame, err := proto.SendContactText(key, time.Now(), 0, text)
		if err != nil {
			return "", err
		}
		return s.request(node, "msg", to, text, frame)
	}},
	{"login", "l", "<contact> <password>", "log in to a repeater or room", func(s *Sim, node string, a []string) (string, error) {
		if len(a) < 1 {
			return "", fmt.Errorf("login <contact name or key> <password>")
		}
		to, key, err := s.contactKey(node, a[0], 32)
		if err != nil {
			return "", err
		}
		return s.request(node, "login", to, "", proto.Login(key, strings.Join(a[1:], " ")))
	}},
	{"req_status", "rs", "<contact>", "ask a repeater or room for its status", func(s *Sim, node string, a []string) (string, error) {
		if len(a) < 1 {
			return "", fmt.Errorf("req_status <contact name or key>")
		}
		to, key, err := s.contactKey(node, a[0], 32)
		if err != nil {
			return "", err
		}
		return s.request(node, "req_status", to, "", proto.StatusRequest(key))
	}},
	{"cmd", "c", "<contact> <command>", "run a command on a node you are logged in to", func(s *Sim, node string, a []string) (string, error) {
		if len(a) < 2 {
			return "", fmt.Errorf("cmd <contact name or key> <command>")
		}
		return s.remoteCommand(node, a[0], strings.Join(a[1:], " "))
	}},
	{"scope", "", "<scope>", "the region floods go out under; * for none", func(s *Sim, node string, a []string) (string, error) {
		if len(a) != 1 {
			return "", fmt.Errorf("scope <name>, or scope * to send unscoped")
		}
		name := CanonicalScope(strings.TrimSpace(a[0]))
		frame := proto.ClearDefaultScope()
		if name != "" && name != "#*" {
			frame = proto.SetDefaultScope(name, provider.RegionKey(name))
		} else {
			name = ""
		}
		if err := s.compFrame(node, frame); err != nil {
			return "", err
		}
		_ = s.compFrame(node, proto.GetDefaultScope())
		return "scope set to " + orUnscoped(name), nil
	}},
}

func init() {
	meshcliCommands = append(meshcliCommands, directCommands...)
	for _, c := range directCommands {
		delete(meshcliRest, c.name)
	}
}

// remoteCommand sends a command for another node to run, as an operator's
// phone administers a repeater it cannot plug into (TXT_TYPE_CLI_DATA). The
// recipient runs it only for a sender logged in as admin, and its answer comes
// back as a CLI message, which the transcript prints and companion.state
// lists.
func (s *Sim) remoteCommand(node, who, command string) (string, error) {
	to, key, err := s.contactKey(node, who, 6)
	if err != nil {
		return "", err
	}
	frame, err := proto.SendCommandData(key, time.Now(), command)
	if err != nil {
		return "", err
	}
	return s.request(node, "cmd", to, command, frame)
}

// request sends one frame that the firmware answers with RESP_CODE_SENT, and
// remembers it so the answer can be matched.
func (s *Sim) request(node, kind, to, text string, frame []byte) (string, error) {
	c, _, err := s.companionFor(node)
	if err != nil {
		return "", err
	}
	c.expect(kind, to, text)
	if err := s.compFrame(node, frame); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s to %s queued; the firmware says when it is sent and when it is answered", kind, to), nil
}

// contactKey finds a contact by name, or by key in hex, among those the node
// has reported, and returns the first n bytes of its key.
//
// A key long enough to address the request is taken as given even when the
// node has not listed it, because the firmware decides whether it holds the
// contact and answers "not found" itself - the honest place for that refusal.
func (s *Sim) contactKey(node, who string, n int) (string, []byte, error) {
	c, _, err := s.companionFor(node)
	if err != nil {
		return "", nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	low := strings.ToLower(who)
	for _, ct := range c.contacts {
		if ct.Name == who || strings.HasPrefix(hex.EncodeToString(ct.PublicKey), low) && len(low) >= 2 {
			if len(ct.PublicKey) < n {
				return "", nil, fmt.Errorf("%s's key is %d bytes", ct.Name, len(ct.PublicKey))
			}
			return ct.Name, ct.PublicKey[:n], nil
		}
	}
	if b, err := hex.DecodeString(low); err == nil && len(b) >= n {
		return low[:min(len(low), 12)], b[:n], nil
	}
	return "", nil, fmt.Errorf("%s holds no contact %q - contacts lists what it holds; "+
		"a key needs %d hex characters", node, who, 2*n)
}
