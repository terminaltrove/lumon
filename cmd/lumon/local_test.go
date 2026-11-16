package main

import (
	"strings"
	"testing"
)

func TestThisBuildHasLANOn(t *testing.T) {
	if !lanEnabled {
		t.Fatal("this build should offer LAN games")
	}
}

func TestThisBuildsHelpIsAboutLANGames(t *testing.T) {
	out := helpText()
	for _, want := range []string{"LAN GAMES", "--host", "--join", "--name"} {
		if !strings.Contains(out, want) {
			t.Errorf("the help does not mention %s", want)
		}
	}
	for _, not := range []string{"SERVING", "ssh", "SSH"} {
		if strings.Contains(out, not) {
			t.Errorf("the help mentions %s", not)
		}
	}
}
