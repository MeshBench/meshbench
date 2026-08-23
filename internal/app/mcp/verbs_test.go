package mcp

import (
	"sort"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
)

// Every tool reaches a verb the session registers.
//
// This server shipped a `session_journal` tool whose three-sentence description
// promised a journal and whose Call reached `session.journal` - a verb no
// version of this tree registers. It had been dead for a while and nothing
// failed, because the verb name lived inside a closure where nothing could
// compare it to anything, and the tool list and the verb list were two
// hand-maintained surfaces over one fact.
//
// So the verb is a field on the tool now, and this walks it. A tool naming a
// verb that does not exist is a tool that answers an assistant's question with
// a transport error, which is worse than not offering it.
func TestEveryToolNamesARegisteredVerb(t *testing.T) {
	st, _ := session.Boot(session.Options{NoPrefs: true, Headless: true})
	registered := map[string]bool{}
	for _, v := range st.Verbs() {
		registered[v] = true
	}
	// The socket answers two of its own on top of the store's, which a tool is
	// entitled to reach.
	for _, v := range []string{"session.verbs", "session.hello"} {
		registered[v] = true
	}

	var dead, unnamed []string
	for _, tool := range sessionTools() {
		if tool.Verb == "" {
			unnamed = append(unnamed, tool.Name)
			continue
		}
		if !registered[tool.Verb] {
			dead = append(dead, tool.Name+" -> "+tool.Verb)
		}
	}
	sort.Strings(dead)
	sort.Strings(unnamed)

	if len(dead) > 0 {
		t.Errorf("%d tools reach a verb nothing registers:\n  %s",
			len(dead), strings.Join(dead, "\n  "))
	}
	if len(unnamed) > 0 {
		t.Errorf("%d tools do not say which verb they reach, so this test cannot"+
			" check them:\n  %s", len(unnamed), strings.Join(unnamed, "\n  "))
	}
	if n := len(sessionTools()); n == 0 {
		t.Fatal("no tools at all, so this test is checking nothing")
	}
}
