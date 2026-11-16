package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Exercise the real release scripts with local fixtures; no network or release
// credentials are used. The fake verifier records its policy arguments.
func TestPluginPinningRequiresACompleteVerifiedRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("release scripts require a POSIX shell")
	}
	for _, scenario := range []string{
		"success", "check-only", "empty", "incomplete", "duplicate", "bad-hash", "unexpected-path",
		"published-empty", "published-different", "windows-different", "binary-tampered", "missing-binary",
		"attestation-failed", "download-failed", "bad-tag",
	} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			write := func(name, text string, mode os.FileMode) {
				t.Helper()
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(text), mode); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"pin-plugin.sh", "check-plugin-release.sh"} {
				data, err := os.ReadFile("../../scripts/" + name)
				if err != nil {
					t.Fatal(err)
				}
				write("scripts/"+name, string(data), 0700)
			}
			for _, plugin := range []string{"claude-code", "pi", "opencode"} {
				write("plugins/"+plugin+"/version", "unreleased\n", 0600)
				write("plugins/"+plugin+"/checksums.sha256sum", "", 0600)
			}
			names := []string{"lumon_darwin_amd64", "lumon_darwin_arm64", "lumon_linux_amd64", "lumon_linux_arm64", "lumon_linux_386", "lumon_freebsd_amd64", "lumon_freebsd_arm64", "lumon_netbsd_amd64", "lumon_netbsd_arm64", "lumon_openbsd_amd64", "lumon_openbsd_arm64", "lumon_windows_amd64.exe", "lumon_linux_arm", "lumon_windows_arm64.exe"}
			var lines []string
			for _, name := range names {
				binary := "fixture for " + name
				write("dist/"+name, binary, 0600)
				lines = append(lines, fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte(binary)), name))
			}
			local := strings.Join(lines, "")
			published := local
			tag := "v1.1.0"
			switch scenario {
			case "empty":
				local = ""
				published = ""
			case "incomplete":
				local = lines[0]
				published = local
			case "duplicate":
				local += lines[0]
				published = local
			case "bad-hash":
				local = strings.Replace(local, lines[0][:64], strings.Repeat("g", 64), 1)
				published = local
			case "unexpected-path":
				local += fmt.Sprintf("%s  ../outside\n", strings.Repeat("a", 64))
				published = local
			case "published-empty":
				published = ""
			case "published-different":
				published = strings.Replace(published, lines[0][:64], strings.Repeat("a", 64), 1)
			case "windows-different":
				published = strings.Replace(published, lines[6][:64], strings.Repeat("b", 64), 1)
			case "binary-tampered":
				write("dist/"+names[0], "tampered", 0600)
			case "missing-binary":
				if err := os.Remove(filepath.Join(root, "dist", names[0])); err != nil {
					t.Fatal(err)
				}
			case "bad-tag":
				tag = "v1.1.0/other"
			}
			write("dist/checksums.sha256sum", local, 0600)
			write("published", published, 0600)
			write("tools/curl", `#!/bin/sh
[ "$SCENARIO" != download-failed ] || exit 1
while [ "$#" -gt 0 ]; do
    if [ "$1" = -o ]; then cp "$FIXTURE/published" "$2"; exit; fi
    shift
done
exit 1
`, 0700)
			write("tools/gh", `#!/bin/sh
printf '%s\n' "$*" >> "$FIXTURE/verified"
[ "$SCENARIO" != attestation-failed ]
`, 0700)
			args := []string{filepath.Join(root, "scripts/pin-plugin.sh")}
			if scenario == "check-only" {
				args = append(args, "--check")
			}
			args = append(args, tag, filepath.Join(root, "dist/checksums.sha256sum"))
			cmd := exec.Command("sh", args...)
			cmd.Env = []string{"PATH=" + filepath.Join(root, "tools") + string(os.PathListSeparator) + os.Getenv("PATH"), "FIXTURE=" + root, "SCENARIO=" + scenario, "HOME=" + root}
			out, err := cmd.CombinedOutput()
			success := scenario == "success" || scenario == "check-only"
			if success && err != nil {
				t.Fatalf("pinning failed: %v\n%s", err, out)
			}
			if !success && err == nil {
				t.Fatalf("unsafe input accepted:\n%s", out)
			}
			for _, plugin := range []string{"claude-code", "pi", "opencode"} {
				version, err := os.ReadFile(filepath.Join(root, "plugins", plugin, "version"))
				if err != nil {
					t.Fatal(err)
				}
				pins, err := os.ReadFile(filepath.Join(root, "plugins", plugin, "checksums.sha256sum"))
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "success" {
					if string(version) != tag+"\n" {
						t.Errorf("%s version: %s", plugin, version)
					}
					for _, line := range lines[:6] {
						if !strings.Contains(string(pins), line) {
							t.Errorf("%s missing pin %s", plugin, line)
						}
					}
					if strings.Contains(string(pins), "windows") {
						t.Error("plugin pins include unsupported Windows binary")
					}
				} else if string(version) != "unreleased\n" || len(pins) != 0 {
					t.Errorf("%s metadata changed without a completed pin", plugin)
				}
			}
			if success {
				log, err := os.ReadFile(filepath.Join(root, "verified"))
				if err != nil {
					t.Fatal(err)
				}
				calls := strings.Split(strings.TrimSpace(string(log)), "\n")
				if len(calls) != 12 {
					t.Fatalf("verified %d artifacts, want 12", len(calls))
				}
				for _, call := range calls {
					for _, policy := range []string{"--repo terminaltrove/lumon", "--cert-identity https://github.com/terminaltrove/lumon/.github/workflows/release.yml@refs/tags/" + tag, "--deny-self-hosted-runners"} {
						if !strings.Contains(call, policy) {
							t.Errorf("verification missing %s: %s", policy, call)
						}
					}
				}
			}
			if scenario == "success" || scenario == "empty" {
				gate := exec.Command("sh", filepath.Join(root, "scripts/check-plugin-release.sh"), "v1.1.0")
				result, err := gate.CombinedOutput()
				if (err == nil) != (scenario == "success") {
					t.Fatalf("publication gate: %v\n%s", err, result)
				}
			}
		})
	}
}
