package main

import (
	"testing"
	"time"

	"github.com/terminaltrove/lumon/internal/agent"
	"github.com/terminaltrove/lumon/internal/lumon"
)

// The game hears a finished turn with its summary, under the name the strip
// gives it, and hears the strip again only when it changes.
func TestTheGameHearsEachAgentAndItsTurn(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	w := newAgentWatch()
	working := []agent.Status{
		{Session: "a", State: agent.Working, Project: "lumon", At: now, Since: now},
		{Session: "b", State: agent.Working, Project: "lumon", At: now, Since: now.Add(time.Second)},
	}
	if got := w.next(now, working); len(got) != 1 {
		t.Fatalf("first read sent %v, want the strip alone", got)
	}
	if got := w.next(now.Add(time.Second), working); len(got) != 0 {
		t.Fatalf("an unchanged read sent %v", got)
	}

	done := []agent.Status{working[0], {Session: "b", State: agent.Done, Project: "lumon",
		At: now.Add(90 * time.Second), Since: now.Add(time.Second), Steps: 7, Edits: []uint32{1, 2}}}
	var news *lumon.AgentMsg
	var strip *lumon.AgentsMsg
	for _, msg := range w.next(now.Add(90*time.Second), done) {
		switch m := msg.(type) {
		case lumon.AgentMsg:
			news = &m
		case lumon.AgentsMsg:
			strip = &m
		}
	}
	if news == nil || strip == nil {
		t.Fatalf("news %v, strip %v", news, strip)
	}
	want := lumon.AgentMsg{Project: "lumon 2", Took: 89 * time.Second, Steps: 7, Edited: 2}
	if *news != want {
		t.Errorf("news %+v, want %+v", *news, want)
	}
	if len(strip.Agents) != 2 || strip.Agents[1].State != lumon.AgentDone {
		t.Errorf("strip %+v", strip.Agents)
	}
}
