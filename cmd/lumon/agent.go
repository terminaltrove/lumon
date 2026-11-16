package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/terminaltrove/lumon/internal/agent"
	"github.com/terminaltrove/lumon/internal/lumon"
)

// runAgentHook handles one event from a coding agent's hook (see
// plugins/claude-code). It never fails the agent: errors go to stderr, which the
// agent only shows in its debug output, and it always exits 0. Claude Code
// would block the player's prompt on exit code 2.
func runAgentHook(event, dir string, delay time.Duration, bots int, mode string) {
	if err := agentHook(event, dir, delay, bots, mode); err != nil {
		fmt.Fprintf(os.Stderr, "lumon: %v\n", err)
	}
}

// agentHook handles the event. bots and mode (agent.ModePlay, ModeWatch or
// ModeScreensaver), from the plugin's settings, go to the game it opens; the
// watch and screensaver events choose their own mode.
func agentHook(event, dir string, delay time.Duration, bots int, mode string) error {
	if dir == "" {
		return fmt.Errorf("--agent-hook needs --agent-status")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	d, err := agent.OpenDir(abs)
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	game := []string{self, "--no-intro", "--agent-status", abs,
		"--bots", strconv.Itoa(clampBots(bots))}
	if event == agent.ModeWatch || event == agent.ModeScreensaver {
		mode = event
	}
	modeFlag := map[string][]string{agent.ModeWatch: {"--watch"}, agent.ModeScreensaver: {"--screensaver"}}[mode]
	game = append(game, modeFlag...)

	h := agent.Hook{
		Dir:     d,
		Delay:   delay,
		Now:     time.Now,
		Sleep:   time.Sleep,
		Running: d.GameRunning,
		Open:    func() error { return agentOpener().Open(game) },
		Mode:    mode,
		Later: func(session string) error {
			input, err := json.Marshal(map[string]string{"session_id": session})
			if err != nil {
				return err
			}
			args := []string{"--agent-hook", "wait", "--agent-status", abs,
				"--agent-delay", strconv.Itoa(int(delay / time.Second)), "--bots", strconv.Itoa(clampBots(bots))}
			args = append(args, modeFlag...)
			return agent.DetachWith(input, self, args...)
		},
	}
	return h.Run(event, os.Stdin)
}

// clampBots keeps a computer refiner count the game will accept.
func clampBots(n int) int { return min(max(n, 0), lumon.MaxDesks-1) }

// agentOpener opens panes and windows by running the programs. A terminal
// that hangs is given up on after 15 seconds, so it cannot hold the hook.
func agentOpener() agent.Opener {
	return agent.Opener{
		GOOS:   runtime.GOOS,
		Getenv: os.Getenv,
		Run: func(name string, args ...string) ([]byte, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return exec.CommandContext(ctx, name, args...).Output()
		},
		Start: agent.Detach,
	}
}

// agentPoll is how often the game reads the status directory.
const agentPoll = 500 * time.Millisecond

// watchAgents tells the game when an agent finishes or wants approval, and
// what every agent is doing, until stop is closed.
func watchAgents(d agent.Dir, p *tea.Program, stop <-chan struct{}) {
	w := newAgentWatch()
	tick := time.NewTicker(agentPoll)
	defer tick.Stop()
	for {
		for _, msg := range w.next(time.Now(), d.Read()) {
			p.Send(msg)
		}
		select {
		case <-stop:
			return
		case <-tick.C:
		}
	}
}

// agentWatch turns reads of the status directory into the game's messages:
// news as it happens, and the agents strip when it changes.
type agentWatch struct {
	news  *agent.Watch
	lines []lumon.AgentLine
	sent  bool
}

func newAgentWatch() *agentWatch { return &agentWatch{news: agent.NewWatch()} }

func (w *agentWatch) next(now time.Time, read []agent.Status) []any {
	board := agent.Board(now, read)
	// The notice names an agent as the strip does, "lumon 2" and all.
	names := make(map[string]string, len(board))
	for _, l := range board {
		names[l.Session] = l.Name
	}
	var out []any
	for _, s := range w.news.Next(now, read) {
		msg := lumon.AgentMsg{Project: s.Project, NeedsApproval: s.State == agent.Waiting}
		if name, ok := names[s.Session]; ok {
			msg.Project = name
		}
		if s.State == agent.Done {
			msg.Took, msg.Steps, msg.Edited = s.Took(), s.Steps, len(s.Edits)
		}
		out = append(out, msg)
	}
	lines := agentLines(board)
	if !w.sent || !slices.Equal(lines, w.lines) {
		w.lines, w.sent = lines, true
		out = append(out, lumon.AgentsMsg{Agents: lines})
	}
	return out
}

var agentStates = map[agent.State]lumon.AgentState{
	agent.Working: lumon.AgentWorking,
	agent.Waiting: lumon.AgentWaiting,
	agent.Done:    lumon.AgentDone,
}

func agentLines(board []agent.Line) []lumon.AgentLine {
	lines := make([]lumon.AgentLine, len(board))
	for i, l := range board {
		lines[i] = lumon.AgentLine{Name: l.Name, State: agentStates[l.State], Doing: l.Doing, For: l.For}
	}
	return lines
}
