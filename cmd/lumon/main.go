// Command lumon runs the Macrodata Refinement terminal.
package main

import (
	"flag"
	"fmt"
	"github.com/terminaltrove/lumon/internal/theme"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/terminaltrove/lumon/internal/agent"
	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/lumon"
)

// version is stamped at link time by scripts/build.sh, which passes
// -ldflags "-X main.version=$(git describe --tags --always --dirty)". -X
// silently does nothing if the variable is missing or its initialiser is not a
// constant string, so it cannot be assigned from lumon.Version.
// TestTheVersionIsTheSameInBothPlaces keeps the two literals equal, and main
// copies this one into lumon.Version at startup.
var version = "3.0.0"

func main() {
	hosting := registerHosting()
	seed := flag.Int64("seed", 0, "grid seed; 0 uses the clock")
	noIntro := flag.Bool("no-intro", false, "skip the boot sequence and go straight to the file selector")
	rooms := flag.Int("rooms", lumon.DefaultRooms, "how many macrodata files to keep open")
	fps := flag.Int("fps", 60, "frames a second; lower spends less CPU and less bandwidth per session")
	themeName := flag.String("theme", "lumon", "colour theme: "+strings.Join(theme.ThemeNames(), ", "))
	file := flag.String("file", "", "open this macrodata file at once, skipping the selector")
	noBold := flag.Bool("no-bold", false, "never ask for bold; for terminals whose font has no bold face")
	graphics := flag.String("graphics", "auto", "how to draw the Lumon mark: auto, kitty, iterm, off")
	screensaver := flag.Bool("screensaver", false, "just the bouncing mark; any key exits")
	completionDebug := flag.Bool("completion-debug", false, "enable ctrl+f, which finishes the open file at once")
	bots := flag.Int("bots", 0, "computer refiners at the empty desks, 0 to 3; c adds them in the game")
	watch := flag.Bool("watch", false, "join a file and watch computer refiners work it")
	agentStatus := flag.String("agent-status", "", "watch this directory for coding agents finishing; see plugins/claude-code")
	// Plumbing for the coding-agent plugin, left out of --help.
	agentHookEvent := flag.String("agent-hook", "", "handle a coding agent's hook event, reading its JSON on stdin")
	paneInput := flag.String("pane", "", "play in a Claude Code pane: frames to stdout, input from this file")
	agentDelay := flag.Int("agent-delay", 0, "seconds an agent works before --agent-hook opens the game; 0 only on request")
	hostLAN := flag.Bool("host", false, "host a LAN game; people on your network can join it")
	joinAddr := flag.String("join", "", "join the LAN game at this address")
	joinName := flag.String("name", "", "your name for --join or --host; $USER if not given")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.BoolVar(showVersion, "V", false, "print the version and exit")
	help := flag.Bool("help", false, "show this message")

	// -h must be declared: an undeclared -h makes the flag package print
	// PrintDefaults and exit 2.
	flag.BoolVar(help, "h", false, "show this message")

	// A mistyped flag gets its error and a pointer, not a full listing that
	// would scroll the error away.
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "try --help for the flags, or ? once you are in for the controls")
	}
	flag.Parse()

	// So the about box shows a stamped version. See lumon.Version.
	lumon.Version = version

	if *showVersion {
		accent := helpAccent()
		fmt.Println(accent("lumon"), helpFaint()(version))
		return
	}
	if *help {
		printHelp()
		return
	}

	if *agentHookEvent != "" {
		mode := agent.ModePlay
		if *watch {
			mode = agent.ModeWatch
		} else if *screensaver {
			mode = agent.ModeScreensaver
		}
		runAgentHook(*agentHookEvent, *agentStatus, time.Duration(max(*agentDelay, 0))*time.Second, *bots, mode)
		return
	}

	if (*hostLAN || *joinAddr != "") && !lanEnabled {
		fmt.Fprintln(os.Stderr, "lumon: this build has no LAN games")
		os.Exit(2)
	}
	if *hostLAN && *joinAddr != "" {
		fmt.Fprintln(os.Stderr, "lumon: --host and --join are one or the other")
		os.Exit(2)
	}
	if *joinName != "" && *joinAddr == "" && !*hostLAN {
		fmt.Fprintln(os.Stderr, "lumon: --name goes with --join or --host")
		os.Exit(2)
	}
	if *joinAddr != "" {
		name := *joinName
		if name == "" {
			name = os.Getenv("USER")
		}
		if err := joinLAN(*joinAddr, name); err != nil {
			fmt.Fprintf(os.Stderr, "lumon: %v\n", err)
			os.Exit(1)
		}
		return
	}

	hosting.earlyExit()

	// Before any style is built: wraps are cached on first use, so setting it
	// later would leave the early ones bold. The environment variable is for
	// containers.
	canvas.NoBold = *noBold || os.Getenv("LUMON_NO_BOLD") != ""

	// ctrl+f finishes the open file, so the completion notice can be checked
	// without refining one. Process-wide, fixed once flags are parsed.
	lumon.CompletionDebug = *completionDebug

	// An unknown name is refused, so a typo is not silently the default.
	th, ok := theme.FindTheme(*themeName)
	if !ok {
		fmt.Fprintf(os.Stderr, "lumon: no theme %q; try one of: %s\n",
			*themeName, strings.Join(theme.ThemeNames(), ", "))
		os.Exit(2)
	}

	// Exits 2 on an unknown name, like --theme. It overrides a terminal whose
	// announcement does not match what it can draw. LUMON_GRAPHICS is for
	// multiplexers and containers, where the announcement is often wrong; the
	// flag wins.
	want := *graphics
	if want == "auto" {
		if env := os.Getenv("LUMON_GRAPHICS"); env != "" {
			want = env
		}
	}
	gfx, forced, ok := lumon.ParseGraphics(want)
	if !ok {
		fmt.Fprintf(os.Stderr, "lumon: no graphics mode %q; try one of: auto, kitty, iterm, off\n", want)
		os.Exit(2)
	}

	// Exits 2 on a bad value, like the two above. This is the dial for a small
	// host: a lower rate cuts CPU and bandwidth per session without adding
	// input latency (see frameRate). The simulation is
	// unaffected, so a seed still gives the same board.
	if err := lumon.SetFrameRate(*fps); err != nil {
		fmt.Fprintf(os.Stderr, "lumon: %v\n", err)
		os.Exit(2)
	}

	s := *seed
	if s == 0 {
		s = time.Now().UnixNano()
	}

	// Exits 2 on a bad value, like --fps.
	if *bots < 0 || *bots > lumon.MaxDesks-1 {
		fmt.Fprintf(os.Stderr, "lumon: --bots %d is outside 0 to %d\n", *bots, lumon.MaxDesks-1)
		os.Exit(2)
	}
	if hosting.active() && (*bots > 0 || *watch) {
		fmt.Fprintln(os.Stderr, "lumon: computer refiners are for a local game")
		os.Exit(2)
	}

	if *hostLAN && (hosting.active() || *watch || *screensaver || *agentStatus != "" || *paneInput != "") {
		fmt.Fprintln(os.Stderr, "lumon: --host is for a game you are playing at this terminal")
		os.Exit(2)
	}

	if hosting.active() && *agentStatus != "" {
		fmt.Fprintln(os.Stderr, "lumon: --agent-status is for a local game")
		os.Exit(2)
	}

	// The game the plugin opens: straight onto a board.
	var agents agent.Dir
	if *agentStatus != "" {
		d, err := agent.OpenDir(*agentStatus)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lumon: %v\n", err)
			os.Exit(2)
		}
		agents = d
	}

	if hosting.active() {
		hosting.serve(s, th, !*noIntro, *rooms)
		return
	}

	// A local run starts at the file selector; --file goes straight to a
	// board. --screensaver has no game behind it at all, unless a pane or the
	// plugin's game asks for it, which are boards.
	screensaverOnly := *screensaver && agents == "" && *paneInput == "" && !*watch
	var reg *lumon.Rooms
	if !screensaverOnly {
		reg = lumon.NewRooms(s)
		reg.EnsureOpen(*rooms)
	}

	var m *lumon.Model
	switch {
	case screensaverOnly:
		m = lumon.NewScreensaver(s)
	case *file != "":
		m = lumon.JoinRoom(reg.OpenNamed(*file), s)
		if m == nil {
			fmt.Fprintf(os.Stderr, "lumon: could not open %q\n", *file)
			os.Exit(1)
		}
	case *screensaver || agents != "" || *watch || *paneInput != "":
		// A diversion is a mode of a board, so seat it at any file instead of
		// the selector.
		m = lumon.JoinRoom(reg.Open(), s)
		if m == nil {
			fmt.Fprintln(os.Stderr, "lumon: could not open a file")
			os.Exit(1)
		}
	default:
		name := os.Getenv("USER")
		if *joinName != "" {
			name = *joinName // with --host; see the check above
		}
		m = lumon.NewLocalSession(reg, s, name)
	}
	m.Theme = th

	// A local game may have computer refiners; a remote session never does.
	m.AllowBots(*bots)
	if *watch {
		m.StartWatching()
	}

	// Host and join are for somebody playing at their own terminal, not a pane or
	// a diversion. See lumon.LAN.
	var lan *lanService
	if lanEnabled && agents == "" && *paneInput == "" && !*watch && !*screensaver {
		lan = newLANService(reg, th, m.Name)
		defer lan.close()
		m.OfferLAN(lan)
		if *hostLAN {
			if err := lan.Host(m.Name); err != nil {
				fmt.Fprintf(os.Stderr, "lumon: %v\n", err)
				os.Exit(1)
			}
			m.ShowLANHosting()
		}
	}

	// The Lumon mark is an image where this terminal can show one and braille
	// where it cannot. Locally all the environment can be read; remotely only
	// TERM arrives.
	if !forced {
		gfx = lumon.DetectGraphics(os.Getenv("TERM"), os.Getenv)
	}
	// A pane draws cells only, so the mark is braille there.
	if *paneInput != "" {
		gfx = lumon.GraphicsOff
	}
	m.UseGraphics(gfx)

	// --screensaver, --watch and --pane imply --no-intro.
	if *screensaver {
		if !screensaverOnly {
			m.StartScreensaver()
		}
	} else if !*noIntro && !*watch && *paneInput == "" && !*hostLAN {
		m.StartIntro()
	}

	// The agent plugins read a record of the open game. A deferred call is
	// skipped by os.Exit, so clearGame is also called before each one.
	clearGame := func() {}
	if agents != "" {
		// A pane's host stops it with a signal. In the full-screen game Bubble
		// Tea turns SIGINT and SIGTERM into an error from Run, after putting the
		// terminal back; a closed terminal sends SIGHUP, which it does not catch.
		sigs := []os.Signal{syscall.SIGHUP}
		if *paneInput != "" {
			sigs = append(sigs, syscall.SIGTERM, os.Interrupt)
		}
		clearGame = markGame(agents, sigs...)
		defer clearGame()
	}

	if *paneInput != "" {
		if err := runPane(m, *paneInput, agents); err != nil {
			fmt.Fprintf(os.Stderr, "lumon: %v\n", err)
			clearGame()
			os.Exit(1)
		}
		return
	}

	stop := make(chan struct{})
	defer close(stop)
	for {
		p := tea.NewProgram(
			m,
			tea.WithAltScreen(),
			tea.WithMouseAllMotion(),
		)
		if agents != "" {
			go watchAgents(agents, p, stop)
		}
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "lumon: %v\n", err)
			clearGame()
			os.Exit(1)
		}

		// Choosing a LAN game ends the program to free the terminal. When that
		// game is over the player comes back to the files.
		target, joining := m.TakeLANJoin()
		if !joining {
			return
		}
		m.LANJoinEnded(joinLAN(target.Addr, target.Name))
	}
}

// markGame records the open game for the agent plugins and returns the call
// that clears the record. One of sigs clears it and exits, since a signal would
// end the process without running deferred calls. The handler is in place
// before the record is written, so no signal can leave it behind.
func markGame(agents agent.Dir, sigs ...os.Signal) (clear func()) {
	var mu sync.Mutex
	clear = func() {}
	c := make(chan os.Signal, 1)
	signal.Notify(c, sigs...)
	go func() {
		<-c
		mu.Lock()
		clear()
		os.Exit(1)
	}()
	mu.Lock()
	defer mu.Unlock()
	clear = agents.MarkGame(os.Getpid())
	return clear
}

// helpAccent colours the help's headings and flags in the lumon theme's cyan.
// lipgloss's default renderer reads its profile from stdout, so the colour
// drops out when piped and NO_COLOR is honoured. It is built per call because
// a package var would fix the profile at init.
func helpAccent() func(string) string {
	c := lipgloss.Color("14") // bright cyan, if the palette cannot be found
	if t, ok := theme.FindTheme("lumon"); ok {
		c = t.Primary.Lipgloss()
	}
	st := lipgloss.NewStyle().Foreground(c)
	return func(s string) string { return st.Render(s) }
}

// helpFaint is the dim colour for the version number and other secondary
// text.
func helpFaint() func(string) string {
	st := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	return func(s string) string { return st.Render(s) }
}

// helpItalic is for titles and the notice. Like the colours, it drops out when
// the help is piped.
func helpItalic() func(string) string {
	st := lipgloss.NewStyle().Italic(true)
	return func(s string) string { return st.Render(s) }
}

// mdrMotto opens the help and closes it.
const mdrMotto = "The work is mysterious and important..."

// helpLogo is the Lumon mark, in braille, over the title.
func helpLogo(accent func(string) string) string {
	var sb strings.Builder
	for _, row := range lumon.SmallLogo {
		sb.WriteString(accent(row) + "\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

// helpFlags lays out one block of flags with the names in the accent colour.
// Padding is measured on the plain name: %-15s on a coloured string counts
// escape bytes and misaligns the descriptions.
func helpFlags(accent func(string) string, rows [][2]string) string {
	const width = 20
	var sb strings.Builder
	for _, r := range rows {
		pad := width - len(r[0])
		if pad < 1 {
			pad = 1
		}
		sb.WriteString("  " + accent(r[0]) + strings.Repeat(" ", pad) + r[1] + "\n")
	}
	return sb.String()
}

// printHelp writes the command line help: how to start, the flags, and what
// the game is. The controls are on the in-game "?" screen, rendered from the
// bindings table, so they are not repeated here.
func printHelp() { fmt.Print(helpText()) }

// helpText builds the help text, for printHelp and tests.
func helpText() string {
	accent := helpAccent()
	head := func(s string) string { return accent(s) }
	italic := helpItalic()

	return helpLogo(accent) + head("lumon - "+mdrMotto) + `

` + accent("lumon") + ` is a TUI (Text-based User Interface) application inspired by the
workplace experience at Lumon Industries from the TV show "` + italic("Severance") + `."

Press ` + accent("?") + ` once you are in for the controls.

` + head("USAGE") + `
  lumon                          play
` + helpUsage(accent) + `
` + head("FLAGS") + `
` + helpFlags(accent, [][2]string{
		{"--seed N", "fix the board; the same seed is the same floor"},
		{"--file NAME", "open a file at once, skipping the selector"},
		{"--no-intro", "skip the boot sequence"},
		{"--screensaver", "just the bouncing mark; any key exits"},
		{"--theme NAME", `starting palette; "t" cycles it per refiner`},
		{"--graphics MODE", "auto, kitty, iterm or off"},
		{"--no-bold", "never ask for bold, for fonts with no bold face"},
		{"--version", "print the version and exit"},
		{"--bots N, --watch", "computer refiners at your board (0-3); --watch to watch"},
		{"--agent-status DIR", "say when a coding agent finishes; see plugins/claude-code"},
	}) + `
` + helpSection(accent, head) + `` + head("NOTES") + `
  Themes: ` + strings.Join(theme.ThemeNames(), ", ") + `
  ` + accent("LUMON_NO_BOLD") + ` and ` + accent("LUMON_GRAPHICS") + ` do the same as ` + accent("--no-bold") + ` and ` + accent("--graphics") + `.
  The Lumon logo is an image in kitty, Ghostty, WezTerm and iTerm2, and braille
  everywhere else, including inside a multiplexer. If it is missing or shows as
  stray characters, set the protocol with ` + accent("--graphics") + `.

` + head("HOW IT WORKS") + `
  Corrupt numbers hide in the grid in clusters. When one wakes, its numbers
  shake, and they shake harder while your cursor is on them. Refine a cluster
  into a bin and its numbers fly there. Each bin holds an even share of the
  file and takes no more once full. Fill all five to finish the file.

` + head("DISCLAIMER") + `
  This project is not affiliated with, endorsed by, or connected to Apple TV+,
  "` + italic("Severance") + `", or Lumon Industries (which is fictional).

For more information, visit: ` + accent("https://github.com/terminaltrove/lumon") + `

` + head("MDR NOTICE") + `
  ` + italic(mdrMotto) + `
`
}
