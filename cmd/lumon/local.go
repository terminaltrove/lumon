package main

import "github.com/terminaltrove/lumon/internal/theme"

// lanEnabled says whether this build offers LAN games.
const lanEnabled = true

// hostingFlags is empty in this build, which only plays.
type hostingFlags struct{}

func registerHosting() *hostingFlags { return &hostingFlags{} }

func (*hostingFlags) active() bool { return false }

func (*hostingFlags) earlyExit() {}

func (*hostingFlags) serve(int64, *theme.Theme, bool, int) {}

// helpUsage is this build's lines in the help's USAGE.
func helpUsage(accent func(string) string) string {
	return `  lumon ` + accent("--host") + `                   host a LAN game; or press l in the files
  lumon ` + accent("--join ADDR") + `              join one
`
}

// helpSection is this build's section of the help.
func helpSection(accent, head func(string) string) string {
	return head("LAN GAMES") + `
` + helpFlags(accent, [][2]string{
		{"--host", "host on your network; you accept or deny each person"},
		{"--join ADDR", "ask to join the game at this address"},
		{"--name NAME", "your name for --join or --host; $USER if not given"},
		{"--rooms N", "files already open when a game starts"},
		{"--fps N", "frames a second (60); lower costs less CPU and bandwidth"},
	}) + `
`
}
