package lumon

import (
	"strings"
	"testing"
	"time"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// stripRow is the file bar's lower edge as drawn.
func stripRow(m *Model) string {
	buf := canvas.NewBuffer(m.Layout.Width, m.Layout.Height)
	m.Draw(buf)
	return buf.Row(m.Layout.HeaderH - 1)
}

var threeAgents = []AgentLine{
	{Name: "lumon", State: AgentWorking, Doing: "Editing view.go", For: 3 * time.Minute},
	{Name: "api", State: AgentWaiting, For: time.Minute},
	{Name: "web", State: AgentDone},
}

// The refiner sees every agent they are running in the file bar's edge: what
// each is doing, for how long, and which one wants them.
func TestTheFileBarShowsEveryAgentAndWhatItIsDoing(t *testing.T) {
	m := agentBoard(t)
	m.Update(AgentsMsg{Agents: threeAgents})
	row := stripRow(m)
	for _, want := range []string{"lumon ● Editing view.go 3m", "api ◐ waiting 1m", "web ✓ done"} {
		if !strings.Contains(row, want) {
			t.Errorf("the edge does not show %q:\n%s", want, row)
		}
	}
	if r := []rune(row); r[0] != '└' || r[len(r)-1] != '┘' {
		t.Errorf("the strip covers the bar's corners:\n%s", row)
	}

	// The next message replaces the strip, and an empty one clears it.
	m.Update(AgentsMsg{Agents: threeAgents[:1]})
	if row := stripRow(m); strings.Contains(row, "api") {
		t.Errorf("a finished list left an agent behind:\n%s", row)
	}
	m.Update(AgentsMsg{})
	if row := stripRow(m); strings.Trim(row, "└─┘") != "" {
		t.Errorf("no agents, yet the edge reads:\n%s", row)
	}
}

// With no agents the bar is the plain bar it always was.
func TestWithoutAgentsTheFileBarIsUnchanged(t *testing.T) {
	m := agentBoard(t)
	if row := stripRow(m); strings.Trim(row, "└─┘") != "" {
		t.Fatalf("the edge reads:\n%s", row)
	}
}

// On a narrow terminal the entries give way a part at a time, and never run
// into the corners.
func TestTheAgentsStripGivesWayOnANarrowTerminal(t *testing.T) {
	m := agentBoard(t)
	m.Update(AgentsMsg{Agents: threeAgents})
	for width := 100; width >= 20; width-- {
		m.Resize(width, 32)
		row := []rune(stripRow(m))
		if row[0] != '└' || row[len(row)-1] != '┘' {
			t.Fatalf("at %d columns the strip covers a corner:\n%s", width, string(row))
		}
		if !strings.ContainsRune(string(row), '◐') && !strings.Contains(string(row), "+") {
			t.Fatalf("at %d columns the waiting agent is gone without a trace:\n%s", width, string(row))
		}
	}
	m.Resize(40, 32)
	if row := stripRow(m); strings.Contains(row, "Editing") || !strings.Contains(row, "◐") {
		t.Errorf("at 40 columns:\n%s", row)
	}
}

// The strip takes its colours from the palette it is drawn with, so a change
// of theme recolours it.
func TestTheAgentsStripFollowsTheTheme(t *testing.T) {
	m := agentBoard(t)
	m.Update(AgentsMsg{Agents: threeAgents})
	colour := func() canvas.Attr {
		buf := canvas.NewBuffer(m.Layout.Width, m.Layout.Height)
		m.Draw(buf)
		row := []rune(buf.Row(m.Layout.HeaderH - 1))
		for x, r := range row {
			if r == stripWaiting {
				return buf.AttrAt(x, m.Layout.HeaderH-1)
			}
		}
		t.Fatal("no waiting mark")
		return canvas.Attr{}
	}
	before := colour()
	m.Update(key("t"))
	if after := colour(); after == before || after != m.Theme.On(m.Theme.Urgent, true) {
		t.Fatalf("after a change of theme the mark is %+v, want %+v", after, m.Theme.On(m.Theme.Urgent, true))
	}
}
