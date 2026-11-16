package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/terminaltrove/lumon/internal/agent"
)

const (
	pluginDir   = "../../plugins/claude-code/"
	piDir       = "../../plugins/pi/"
	opencodeDir = "../../plugins/opencode/"
)

// copies are the plugins installed on their own, each carrying the shim and
// pins of plugins/claude-code.
var copies = []string{piDir, opencodeDir}

// hookEvents reads the events a hooks.json sends, failing for any command that
// is not the shim with one event. Each event's hook names it in the result.
func hookEvents(t *testing.T, path, rootVar string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	command := regexp.MustCompile(`^sh "\$\{` + rootVar + `\}/bin/lumon-hook" ([a-z]+)$`)
	events := map[string]string{}
	for name, groups := range file.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				m := command.FindStringSubmatch(h.Command)
				if m == nil {
					t.Errorf("%s: %s runs %q, not the shim with one event", path, name, h.Command)
					continue
				}
				events[m[1]] = name
			}
		}
	}
	if len(events) == 0 {
		t.Fatalf("%s: no hooks found", path)
	}
	return events
}

// handled fails for an event the shim refuses or the binary does not know.
func handled(t *testing.T, where, event string) {
	t.Helper()
	shim, err := os.ReadFile(pluginDir + "bin/lumon-hook")
	if err != nil {
		t.Fatal(err)
	}
	passed := regexp.MustCompile(`(?m)^(start \|.*)\) ;;$`).FindSubmatch(shim)
	if passed == nil {
		t.Fatal("cannot find the shim's list of events")
	}
	if !strings.Contains(" "+string(passed[1])+" ", " "+event+" ") {
		t.Errorf("%s sends %q, which the shim refuses", where, event)
	}
	// The switch is the shim's own; the binary never sees it.
	if event == "off" || event == "on" || event == "snooze" {
		return
	}
	hook := agent.Hook{Dir: agent.Dir(t.TempDir()), Now: time.Now, Running: func() bool { return true }}
	if err := hook.Run(event, strings.NewReader("{}")); err != nil && strings.Contains(err.Error(), "no hook event") {
		t.Errorf("%s sends %q, which the binary does not know", where, event)
	}
}

// Every event the plugins send is one the shim passes on and the binary
// handles, so a hook can never fall through to nothing.
func TestThePluginsHooksAreAllHandled(t *testing.T) {
	for event, hook := range hookEvents(t, pluginDir+"hooks/hooks.json", "CLAUDE_PLUGIN_ROOT") {
		handled(t, "Claude Code's "+hook, event)
	}
	for _, path := range []string{piDir + "index.ts", opencodeDir + "index.ts"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sent := regexp.MustCompile(`send\((?:env, )?"([a-z]+)"`).FindAllSubmatch(src, -1)
		if len(sent) == 0 {
			t.Fatalf("%s sends no events", path)
		}
		for _, m := range sent {
			handled(t, path, string(m[1]))
		}
	}
}

// Each plugin is installed on its own, so each carries the shim and the pins.
// They must be the same files, or one agent would run a different release.
func TestEveryPluginCarriesTheSameShimAndPins(t *testing.T) {
	for _, file := range []string{"bin/lumon-hook", "version", "checksums.sha256sum"} {
		want, err := os.ReadFile(pluginDir + file)
		if err != nil {
			t.Fatal(err)
		}
		for _, dir := range copies {
			got, err := os.ReadFile(dir + file)
			if err != nil {
				t.Errorf("%s%s: %v", dir, file, err)
				continue
			}
			if string(got) != string(want) {
				t.Errorf("%s%s differs from %s%s", dir, file, pluginDir, file)
			}
		}
	}
}

// The pins are a released version and a full SHA-256 for every platform the
// shim fetches for, or nothing at all before the first release.
func TestThePluginPinsEveryPlatform(t *testing.T) {
	version, err := os.ReadFile(pluginDir + "version")
	if err != nil {
		t.Fatal(err)
	}
	pins, err := os.ReadFile(pluginDir + "checksums.sha256sum")
	if err != nil {
		t.Fatal(err)
	}
	v := strings.TrimSpace(string(version))
	if v == "unreleased" {
		if strings.TrimSpace(string(pins)) != "" {
			t.Fatal("hashes pinned with no version")
		}
		return
	}
	if !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(v) {
		t.Fatalf("pinned version %q is not a release tag", v)
	}
	line := regexp.MustCompile(`(?m)^([0-9a-f]{64})  \*?(lumon_[a-z0-9]+_[a-z0-9]+)$`)
	have := map[string]bool{}
	for _, m := range line.FindAllStringSubmatch(string(pins), -1) {
		have[m[2]] = true
	}
	for _, name := range []string{
		"lumon_darwin_amd64", "lumon_darwin_arm64",
		"lumon_linux_amd64", "lumon_linux_arm64", "lumon_linux_386", "lumon_linux_arm",
		"lumon_freebsd_amd64", "lumon_freebsd_arm64",
		"lumon_netbsd_amd64", "lumon_netbsd_arm64",
		"lumon_openbsd_amd64", "lumon_openbsd_arm64",
	} {
		if !have[name] {
			t.Errorf("no pin for %s", name)
		}
	}
}

// Every choice offered in the plugin's settings is one the shim reads.
func TestThePluginsSettingsAreReadable(t *testing.T) {
	data, err := os.ReadFile(pluginDir + ".claude-plugin/plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		UserConfig map[string]struct {
			Default string   `json:"default"`
			Options []string `json:"options"`
		} `json:"userConfig"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	readable := map[string]*regexp.Regexp{
		"delay":   regexp.MustCompile(`^(off|[0-9]+)$`),
		"company": regexp.MustCompile(`^[0-3]$`),
		"mode":    regexp.MustCompile(`^(play|watch|screensaver)$`),
	}
	if len(manifest.UserConfig) != len(readable) {
		t.Errorf("the plugin offers %d settings; the shim reads %d", len(manifest.UserConfig), len(readable))
	}
	for key, setting := range manifest.UserConfig {
		re, ok := readable[key]
		if !ok {
			t.Errorf("setting %q is never read", key)
			continue
		}
		if len(setting.Options) == 0 {
			t.Errorf("setting %q offers no choices", key)
		}
		for _, o := range append(setting.Options, setting.Default) {
			if !re.MatchString(o) {
				t.Errorf("setting %q offers %q", key, o)
			}
		}
	}
}

// Exercise each shipped shell launcher, including its release/hash path, so
// an omitted or invalid setting cannot silently opt a player into opening.
func TestEveryPluginRequiresAnExplicitAutomaticDelay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the plugins require a POSIX shell")
	}
	for _, plugin := range append([]string{pluginDir}, copies...) {
		t.Run(filepath.Base(filepath.Clean(plugin)), func(t *testing.T) {
			root := t.TempDir()
			shim, err := os.ReadFile(plugin + "bin/lumon-hook")
			if err != nil {
				t.Fatal(err)
			}
			mustWrite := func(path string, data []byte, mode os.FileMode) {
				t.Helper()
				if err := os.WriteFile(path, data, mode); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(filepath.Join(root, "bin"), 0o700); err != nil {
				t.Fatal(err)
			}
			shimPath := filepath.Join(root, "bin", "lumon-hook")
			mustWrite(shimPath, shim, 0o700)
			mustWrite(filepath.Join(root, "version"), []byte("v0.0.0-test\n"), 0o600)
			name := "lumon_" + runtime.GOOS + "_" + runtime.GOARCH
			data := filepath.Join(root, "data")
			cache := filepath.Join(data, "bin", "v0.0.0-test")
			if err := os.MkdirAll(cache, 0o700); err != nil {
				t.Fatal(err)
			}
			binary := []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" >\"$LUMON_TEST_ARGS\"\n")
			mustWrite(filepath.Join(cache, name), binary, 0o700)
			mustWrite(filepath.Join(root, "checksums.sha256sum"), []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(binary), name)), 0o600)
			for _, tc := range []struct {
				name, event, want string
				settings          []string
			}{
				{"default", "prompt", "0", nil},
				{"empty", "prompt", "0", []string{"LUMON_DELAY="}},
				{"invalid", "prompt", "0", []string{"LUMON_DELAY=invalid"}},
				{"off", "prompt", "0", []string{"LUMON_DELAY=off"}},
				{"opt-in", "prompt", "30", []string{"LUMON_DELAY=30"}},
				{"option-off", "prompt", "0", []string{"LUMON_DELAY=30", "CLAUDE_PLUGIN_OPTION_DELAY=off"}},
				{"option-opt-in", "prompt", "60", []string{"CLAUDE_PLUGIN_OPTION_DELAY=60"}},
				{"option-invalid", "prompt", "0", []string{"CLAUDE_PLUGIN_OPTION_DELAY=invalid"}},
				{"manual-play", "open", "0", nil},
				{"manual-watch", "watch", "0", nil},
				{"manual-screensaver", "screensaver", "0", nil},
			} {
				t.Run(tc.name, func(t *testing.T) {
					argsPath := filepath.Join(root, "args")
					cmd := exec.Command("sh", shimPath, tc.event)
					cmd.Env = []string{
						"PATH=" + os.Getenv("PATH"), "HOME=" + root,
						"LUMON_PLUGIN_DATA=" + data,
						"LUMON_STATUS_DIR=" + filepath.Join(root, "status"),
						"LUMON_TEST_ARGS=" + argsPath,
					}
					cmd.Env = append(cmd.Env, tc.settings...)
					cmd.Stdin = strings.NewReader(`{"session_id":"test-session"}`)
					if out, err := cmd.CombinedOutput(); err != nil || len(out) != 0 {
						t.Fatalf("launcher: %v, %s", err, out)
					}
					args, err := os.ReadFile(argsPath)
					if err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(string(args), "--agent-delay\n"+tc.want+"\n") ||
						!strings.Contains(string(args), "--agent-hook\n"+tc.event+"\n") {
						t.Fatalf("unexpected binary arguments: %s", args)
					}
				})
			}
		})
	}
}

// Packaging and release checks must cover exactly the integrations we maintain.
func TestOnlySupportedPluginsArePackaged(t *testing.T) {
	supported := map[string]bool{"claude-code": true, "pi": true, "opencode": true}
	entries, err := os.ReadDir("../../plugins")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !supported[entry.Name()] {
			t.Errorf("unsupported plugin packaged: %s", entry.Name())
		}
		delete(supported, entry.Name())
	}
	for name := range supported {
		t.Errorf("supported plugin missing: %s", name)
	}
}
