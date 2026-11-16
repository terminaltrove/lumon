package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// shimRun is the plugins' shim in a home of its own, with LUMON_BIN a script
// that records how it was run.
type shimRun struct {
	t      *testing.T
	home   string
	status string
	calls  string
	env    []string
}

func newShimRun(t *testing.T) *shimRun {
	t.Helper()
	home := t.TempDir()
	r := &shimRun{t: t, home: home, status: filepath.Join(home, "status"), calls: filepath.Join(home, "calls")}
	bin := filepath.Join(home, "lumon")
	script := "#!/bin/sh\necho \"$*\" >>" + r.calls + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	r.env = []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"LUMON_STATUS_DIR=" + r.status,
		"LUMON_PLUGIN_DATA=" + filepath.Join(home, "data"),
		"LUMON_BIN=" + bin,
	}
	return r
}

// run sends one event and returns what the shim printed on stderr.
func (r *shimRun) run(event string, env ...string) string {
	r.t.Helper()
	cmd := exec.Command("sh", pluginDir+"bin/lumon-hook", event)
	cmd.Env = append(append([]string{}, r.env...), env...)
	cmd.Stdin = strings.NewReader(`{"session_id":"s1"}`)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		r.t.Fatalf("the shim exited with %v for %s: %s", err, event, stderr.String())
	}
	if len(out) != 0 {
		r.t.Errorf("the shim printed %q on stdout for %s", out, event)
	}
	return stderr.String()
}

// called is how many times the binary has been run.
func (r *shimRun) called() int {
	b, _ := os.ReadFile(r.calls)
	return strings.Count(string(b), "\n")
}

func (r *shimRun) offFile() (string, bool) {
	b, err := os.ReadFile(filepath.Join(r.status, "off"))
	return strings.TrimSpace(string(b)), err == nil
}

// /lumon:off stops the game following the agent, /lumon:on brings it back,
// and asking for the game works throughout.
func TestTheOffSwitchStopsTheAgentEventsAndOnUndoesIt(t *testing.T) {
	r := newShimRun(t)
	r.run("stop")
	if r.called() != 1 {
		t.Fatalf("the binary ran %d times for one event, want 1", r.called())
	}

	r.run("off")
	if until, ok := r.offFile(); !ok || until != "" {
		t.Fatalf("off left the switch as %q (present %v), want an empty file", until, ok)
	}
	for _, event := range []string{"start", "prompt", "tool", "waiting", "stop", "idle", "end"} {
		r.run(event)
	}
	if r.called() != 1 {
		t.Errorf("the binary ran %d more times while off", r.called()-1)
	}
	r.run("open")
	if r.called() != 2 {
		t.Error("asking for the game did not open it while off")
	}

	r.run("on")
	if _, ok := r.offFile(); ok {
		t.Fatal("on left the switch in place")
	}
	r.run("stop")
	if r.called() != 3 {
		t.Error("the agent's events did not reach the game after on")
	}
}

// Off is read before anything is fetched, so it works with no release pinned
// and no network.
func TestOffWorksBeforeAnyRelease(t *testing.T) {
	r := newShimRun(t)
	r.run("off")
	if said := r.run("prompt", "LUMON_BIN="); said != "" {
		t.Errorf("an event while off got as far as the release: %q", said)
	}
}

// A snooze turns the game off until the next midnight, then it is back by
// itself.
func TestASnoozeLastsUntilMidnight(t *testing.T) {
	r := newShimRun(t)
	before := time.Now()
	r.run("snooze")
	got, ok := r.offFile()
	if !ok {
		t.Fatal("snooze wrote no switch")
	}
	until, err := strconv.ParseInt(got, 10, 64)
	if err != nil {
		t.Fatalf("snooze wrote %q, not a time", got)
	}
	y, m, d := before.Date()
	midnight := time.Date(y, m, d+1, 0, 0, 0, 0, before.Location())
	if diff := time.Unix(until, 0).Sub(midnight); diff < -time.Hour || diff > time.Hour {
		t.Errorf("the snooze ends at %v, want about %v", time.Unix(until, 0), midnight)
	}
	r.run("stop")
	if r.called() != 0 {
		t.Error("a snoozed plugin ran the game")
	}

	// A snooze that has run out is removed and the events flow again.
	os.WriteFile(filepath.Join(r.status, "off"), []byte(strconv.FormatInt(before.Add(-time.Minute).Unix(), 10)), 0o600)
	r.run("stop")
	if r.called() != 1 {
		t.Error("the events did not flow after the snooze ran out")
	}
	if _, ok := r.offFile(); ok {
		t.Error("the spent snooze was left behind")
	}
}

// LUMON_AGENTS=off is the same switch for one shell.
func TestLumonAgentsOffIsTheSwitchForOneShell(t *testing.T) {
	r := newShimRun(t)
	r.run("prompt", "LUMON_AGENTS=off")
	r.run("tool", "LUMON_AGENTS=off", "LUMON_DETACHED=1")
	if r.called() != 0 {
		t.Errorf("the binary ran %d times with LUMON_AGENTS=off", r.called())
	}
}

// LUMON_BIN runs a local build, so the plugins can be tried before a release.
func TestLumonBinRunsALocalBuild(t *testing.T) {
	r := newShimRun(t)
	r.run("stop")
	b, _ := os.ReadFile(r.calls)
	if !strings.HasPrefix(string(b), "--agent-hook stop --agent-status "+r.status) {
		t.Errorf("LUMON_BIN was run as %q", b)
	}
	if said := r.run("stop", "LUMON_BIN="+filepath.Join(r.home, "missing")); !strings.Contains(said, "LUMON_BIN") {
		t.Errorf("a LUMON_BIN that is not there was not reported: %q", said)
	}
}
