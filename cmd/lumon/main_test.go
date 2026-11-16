package main

import (
	"strings"
	"testing"

	"github.com/terminaltrove/lumon/internal/lumon"
)

// The version is written twice: lumon.Version is what the about box reads, and
// main's must be its own literal for -X to stamp it at release time.
func TestTheVersionIsTheSameInBothPlaces(t *testing.T) {
	if version != lumon.Version {
		t.Errorf("cmd/lumon says %q and internal/lumon says %q; the about box and "+
			"--version would disagree", version, lumon.Version)
	}
}

// The line cap keeps --help to one terminal screen, so a new flag cannot add a
// paragraph without something else coming out.
func TestTheHelpIsShortAndSaysWhereToFindThis(t *testing.T) {
	out := helpText()

	if n := strings.Count(out, "\n"); n > 62 {
		t.Errorf("the help is %d lines; it is meant to fit a terminal", n)
	}
	if !strings.Contains(out, "For more information, visit:") ||
		!strings.Contains(out, "https://github.com/terminaltrove/lumon") {
		t.Error("the help does not say where to find the project")
	}
	for _, want := range []string{"USAGE", "FLAGS", "HOW IT WORKS"} {
		if !strings.Contains(out, want) {
			t.Errorf("the help has no %s section", want)
		}
	}

	// Every flag the help mentions must exist.
	for _, f := range []string{
		"--seed", "--file", "--no-intro", "--screensaver", "--theme",
		"--graphics", "--no-bold", "--version", "--rooms", "--fps",
		"--bots", "--watch",
	} {
		if !strings.Contains(out, f) {
			t.Errorf("the help does not mention %s", f)
		}
	}
}

// The mark comes first, then the title, then what this is.
func TestTheHelpOpensWithTheMarkAndTheMotto(t *testing.T) {
	lines := strings.Split(helpText(), "\n")
	for i, row := range lumon.SmallLogo {
		if lines[i] != row {
			t.Fatalf("line %d is %q, want the mark's %q", i, lines[i], row)
		}
	}
	if got := lines[len(lumon.SmallLogo)+1]; got != "lumon - The work is mysterious and important..." {
		t.Errorf("the title is %q", got)
	}
	out := helpText()
	if !strings.Contains(out, `the TV show "Severance."`+"\n\nPress ? once you are in") {
		t.Error("the help does not say where this comes from, then how to find the controls")
	}
	if !strings.HasSuffix(out, "\n\nMDR NOTICE\n  The work is mysterious and important...\n") {
		t.Error("the MDR notice is not the last thing in the help")
	}
	if !strings.Contains(out, "DISCLAIMER\n  This project is not affiliated with, endorsed by, or connected to Apple TV+,\n"+
		`  "Severance", or Lumon Industries (which is fictional).`) {
		t.Error("the help has no disclaimer")
	}
}

// The controls are on the "?" screen, generated from the bindings table, so
// --help does not keep a second list that could go stale.
func TestTheHelpLeavesTheControlsToTheProgram(t *testing.T) {
	out := helpText()

	if !strings.Contains(out, "?") {
		t.Error("the help never mentions the key that shows the controls")
	}
	// Keys that only exist inside the program.
	for _, key := range []string{"hjkl", "shift+arrows", "ctrl+c"} {
		if strings.Contains(out, key) {
			t.Errorf("the help lists %q; the controls live on \"?\"", key)
		}
	}
}

// An invalid --max-sessions is refused, not clamped (as with --fps): replacing
// a cap set for a small machine with the default would spend memory it does
// not have.
