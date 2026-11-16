# Lumon for Claude Code

Refine macrodata while Claude works. Lumon opens when you ask for it, and
tells you when Claude has finished or needs your approval.

## Install

```
/plugin marketplace add terminaltrove/lumon
/plugin install lumon@terminaltrove
```

## Use

| Command | What it does |
|---|---|
| `/lumon:play` | Opens the game at a desk. |
| `/lumon:watch` | Opens it to watch computer refiners work. |
| `/lumon:screensaver` | Opens the screensaver. Any key starts a game. |
| `/lumon:off` | Stops Lumon opening on its own or following any agent, until `/lumon:on`. |
| `/lumon:snooze` | The same, until midnight. |
| `/lumon:on` | Turns it back on. |

`/lumon:play` still works while Lumon is off.

When Claude stops, a notice over the board says "Your agent in ‹project› has
finished its work." or "needs your approval." The finished notice also says how
long the turn took, how many steps it ran and which files it edited. Press any
key or click to put it away and keep refining.

The lower edge of the file bar lists every agent you run, what it is doing
(such as "Editing view.go") and for how long. A second long prompt, in Claude
Code or another agent, uses the game that is already open.

In the game, `c` adds or sends home computer refiners, and `w` switches between
playing and watching.

## Settings

Change these with `/plugin`:

| Setting | Choices | Default |
|---|---|---|
| Open Lumon after | `off`, or 10, 20, 30, 60 or 120 seconds of Claude working | `off`: Lumon opens only when you ask |
| Computer refiners | 0 to 3 at the board with you | 0 |
| When Lumon opens | `play` takes a desk, `watch` watches computer refiners, `screensaver` shows the bouncing mark | `play` |

## Where it opens

Inside Claude Code, Lumon opens in a pane. In the fullscreen layout
(`/tui fullscreen`) on a terminal at least 110 columns wide, the pane sits
beside the chat; otherwise it sits above the prompt. Click the strip under the
game to give it the keys, and press Esc to give them back to Claude. When Lumon
opens on its own after a long wait, the pane needs 144 columns, or 110 once you
have opened it yourself.

Without room for the pane, Lumon opens in a pane to the right in herdr or tmux,
or else in a new window of your terminal: Terminal, iTerm2, Ghostty, WezTerm
or kitty on macOS, and `$TERMINAL` or `x-terminal-emulator` on Linux. Terminal
and iTerm2 ask once for permission to be controlled.

`hooks/pane.tsx` draws the pane with Claude Code's function hooks, an
early-access API that may change between Claude Code releases. The game in the
pane is the same pinned binary as everywhere else, run through
`bin/lumon-hook pane`.

Windows is not supported yet. On Windows the hooks do nothing.

## How the binary is trusted

The plugin runs one program, the Lumon release binary, through
`bin/lumon-hook`.

- `version` names the release, and `checksums.sha256sum` pins each platform's
  SHA-256 hash. Both ship in the plugin, so they come from the commit you
  installed.
- The binary is downloaded over HTTPS into the plugin's data directory and kept
  only if its hash matches.
- The hash is checked again before every run. A file that does not match is
  deleted and never run. No other `lumon` on your PATH is ever used.
- Each release binary has a build-provenance attestation from the repository's
  release workflow, and a pin is taken only after that attestation is checked.
  You can check a binary yourself:
  `gh attestation verify lumon_darwin_arm64 --repo terminaltrove/lumon`.
- It writes only to its data directory and to the status folder every agent's
  plugin shares, `~/.local/state/lumon/agents` (or `LUMON_STATUS_DIR`). Both
  are private to you.
- From each hook it reads the session ID and working directory, never your
  prompt. From a tool call it reads the tool's name, the file's name, and a
  shell command's own description or first word, never the whole command.
  Edited files are counted by hash, so no path is stored. Everything shown is
  reduced to plain characters first.
- If anything fails, it prints one line to Claude's debug output and Claude
  carries on.

## For maintainers

Pushing a tag like `v3.1.0` runs `.github/workflows/release.yml`, which builds
every binary, attests each one and publishes the release. Then pin the release
from your own build of the same tag:

```
SKIP_TESTS=1 VERSION=v3.1.0 scripts/build.sh    # at the tested tag
scripts/pin-plugin.sh --check v3.1.0 dist/checksums.sha256sum
scripts/pin-plugin.sh v3.1.0 dist/checksums.sha256sum
scripts/check-plugin-release.sh v3.1.0
go test ./cmd/lumon -run Plugin
```

`pin-plugin.sh` refuses if the published checksums differ from your build's, or
if any plugin binary lacks an attestation from the release workflow for that
tag. It also rejects an empty, partial, duplicated or malformed checksum list.
With `--check` it verifies everything without changing the pins, and a failed
check leaves every pin as it was. It pins every agent's plugin, and needs a
current `gh` that is signed in.

Run `check-plugin-release.sh` before publishing the plugins. It rejects
`unreleased` and incomplete pins.

Bump `version` in `.claude-plugin/plugin.json` with each change, so installs
update.
