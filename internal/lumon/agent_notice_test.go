package lumon

import (
	"strings"
	"testing"
	"time"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// screenText is the screen as plain text, its rows joined by spaces, so a
// wrapped sentence reads as one.
func screenText(m *Model) string {
	buf := canvas.NewBuffer(m.Layout.Width, m.Layout.Height)
	m.Draw(buf)
	rows := make([]string, 0, m.Layout.Height)
	for y := 0; y < m.Layout.Height; y++ {
		rows = append(rows, buf.Row(y))
	}
	return strings.Join(strings.Fields(strings.Join(rows, " ")), " ")
}

// agentBoard is a refiner on a board, as the plugin opens the game.
func agentBoard(t *testing.T) *Model {
	t.Helper()
	rs := NewRooms(7)
	rs.EnsureOpen(1)
	m := JoinRoom(rs.Open(), 7)
	if m == nil {
		t.Fatal("no board to join")
	}
	m.Resize(100, 32)
	return m
}

// The refiner is told, over the board, when the agent they are waiting on is
// done or wants them, and which project it is working in.
func TestTheRefinerIsToldWhenTheirAgentIsDoneOrNeedsThem(t *testing.T) {
	for _, tc := range []struct {
		msg  AgentMsg
		want string
	}{
		{AgentMsg{Project: "example-project"}, "Your agent in example-project has finished its work."},
		{AgentMsg{Project: "v2", NeedsApproval: true}, "Your agent in v2 needs your approval."},
	} {
		m := agentBoard(t)
		m.Update(tc.msg)
		if got := screenText(m); !strings.Contains(got, tc.want) {
			t.Errorf("the board does not say %q:\n%s", tc.want, got)
		}
	}
}

// The agent's notice is news, in the screen's own colour; only an escape
// attempt's is in the urgent red.
func TestTheAgentNoticeIsInTheScreensColourAndAnEscapeInRed(t *testing.T) {
	m := agentBoard(t)
	m.Update(AgentMsg{Project: "example-project"})
	buf := drawTo(m)
	x, y := findText(t, buf, "Your agent")
	if got := buf.AttrAt(x, y); got.FG != m.Theme.Header.FG {
		t.Errorf("the agent's notice is drawn in %v, want the header's %v", got.FG, m.Theme.Header.FG)
	}

	m = agentBoard(t)
	m.Escaped = true
	buf = drawTo(m)
	x, y = findText(t, buf, escapeNotice[:12])
	if got, want := buf.AttrAt(x, y).FG, m.Theme.On(m.Theme.Urgent, true).FG; got != want {
		t.Errorf("the escape notice is drawn in %v, want the urgent %v", got, want)
	}
}

// Any key or a click puts the notice away and leaves the refiner on the board.
func TestTheAgentNoticeGoesAwayAndPlayContinues(t *testing.T) {
	for name, dismiss := range map[string]func(*Model){
		"a key":   func(m *Model) { m.Update(key("x")) },
		"q":       func(m *Model) { m.Update(key("q")) },
		"a click": func(m *Model) { m.Update(click(10, 10)) },
	} {
		m := agentBoard(t)
		m.Update(AgentMsg{Project: "lumon"})
		dismiss(m)
		if strings.Contains(screenText(m), "has finished its work") {
			t.Errorf("%s left the notice up", name)
		}
		if m.Screen != ScreenMain || m.Quitting {
			t.Errorf("%s took the refiner off the board", name)
		}
	}
}

// A game opened on the screensaver for an agent still tells the refiner when
// the agent is done, and one key or click from there puts the notice away and
// ends the screensaver.
func TestTheScreensaverStillHearsTheAgent(t *testing.T) {
	m := agentBoard(t)
	m.StartScreensaver()
	tick(m, 30)
	m.Update(AgentMsg{Project: "lumon"})
	tick(m, 2)
	if got := screenText(m); !strings.Contains(got, "Your agent in lumon has finished its work.") {
		t.Fatalf("the screensaver hid the notice:\n%s", got)
	}
	m.Update(key("x"))
	tick(m, 30)
	if m.Screen != ScreenMain || m.Quitting {
		t.Fatalf("one key did not put the notice away and end the screensaver: screen %v", m.Screen)
	}
	if strings.Contains(screenText(m), "has finished its work") {
		t.Fatal("the notice is still up")
	}

	// A click does the same.
	m = agentBoard(t)
	m.StartScreensaver()
	m.Update(AgentMsg{Project: "lumon"})
	m.Update(click(10, 10))
	if m.Screen != ScreenMain {
		t.Fatalf("a click left the screensaver up: screen %v", m.Screen)
	}
}

// A finished turn's notice says how long it took, its steps and the files it
// edited, leaving out what the agent could not say.
func TestTheFinishedNoticeSummarisesTheTurn(t *testing.T) {
	for _, tc := range []struct {
		msg  AgentMsg
		want string
	}{
		{AgentMsg{Project: "lumon", Took: 4*time.Minute + 12*time.Second, Steps: 23, Edited: 5},
			"has finished its work. It took 4m 12s and 23 steps, and edited 5 files."},
		{AgentMsg{Project: "lumon", Took: 42 * time.Second, Steps: 1},
			"has finished its work. It took 42s and 1 step."},
		{AgentMsg{Project: "lumon", Took: time.Hour + 5*time.Minute},
			"has finished its work. It took 1h 5m."},
		{AgentMsg{Project: "lumon", Steps: 3, Edited: 1},
			"has finished its work. It took 3 steps, and edited 1 file."},
	} {
		m := agentBoard(t)
		m.Update(tc.msg)
		if got := noticeText(m); !strings.Contains(got, tc.want) {
			t.Errorf("the notice does not say %q:\n%s", tc.want, got)
		}
	}

	m := agentBoard(t)
	m.Update(AgentMsg{Project: "lumon"})
	if got := screenText(m); strings.Contains(got, "It took") {
		t.Errorf("a turn with nothing to report has a summary:\n%s", got)
	}
}

// How long the turn took starts a line of its own, under the sentence.
func TestTheSummaryIsOnItsOwnLine(t *testing.T) {
	m := agentBoard(t)
	m.Update(AgentMsg{Project: "example-project", Took: 22 * time.Second})
	buf := drawTo(m)
	x, y := findText(t, buf, "It took 22s.")
	sx, sy := findText(t, buf, "Your agent in example-project has finished its work.")
	if y <= sy {
		t.Errorf("the summary is on row %d, the sentence on row %d", y, sy)
	}
	if left := strings.TrimSpace(string([]rune(buf.Row(y))[sx:x])); left != "" {
		t.Errorf("the summary's line starts with %q", left)
	}
}

// noticeText is the text inside the notice's box, its lines joined, so a
// wrapped sentence reads as one.
func noticeText(m *Model) string {
	buf := canvas.NewBuffer(m.Layout.Width, m.Layout.Height)
	m.Draw(buf)
	var words []string
	left, right := -1, -1
	for y := 0; y < m.Layout.Height; y++ {
		row := []rune(buf.Row(y))
		if left < 0 {
			if strings.Contains(string(row), agentTitle) {
				for x, r := range row {
					if r == '┌' && left < 0 {
						left = x
					}
					if r == '┐' {
						right = x
					}
				}
			}
			continue
		}
		if row[left] == '└' {
			break
		}
		words = append(words, strings.Fields(string(row[left+1:right]))...)
	}
	return strings.Join(words, " ")
}
