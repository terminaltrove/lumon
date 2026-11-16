//go:build !windows

package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A pane's host stops the game with a signal. The record of the open game
// goes with it, or an agent plugin finds a game.pid for a process that is gone.
func TestAStoppedPaneClearsItsGameRecord(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "lumon")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT} {
		status := t.TempDir()
		cmd := exec.Command(bin, "--pane", filepath.Join(t.TempDir(), "input"), "--agent-status", status)
		cmd.Stdout = io.Discard
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		record := filepath.Join(status, "game.pid")
		deadline := time.Now().Add(20 * time.Second)
		for {
			if _, err := os.Stat(record); err == nil {
				break
			}
			if time.Now().After(deadline) {
				_ = cmd.Process.Kill()
				t.Fatalf("%v: the pane never recorded its game", sig)
			}
			time.Sleep(20 * time.Millisecond)
		}
		if err := cmd.Process.Signal(sig); err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			_ = cmd.Process.Kill()
			t.Fatalf("%v: the pane did not stop", sig)
		}
		if _, err := os.Stat(record); !os.IsNotExist(err) {
			t.Errorf("%v: game.pid outlived the game", sig)
		}
	}
}
