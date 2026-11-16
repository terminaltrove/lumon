# Agent plugins

Lumon integrations are maintained for **Claude Code, Pi and OpenCode only**.
OpenCode and Pi provide manual slash commands with playable UI inside the
agent; automatic opening is always opt-in.

## Integration status

| Agent | Manual commands | Inline game status |
|---|---|---|
| [OpenCode](../plugins/opencode/README.md) | `/lumon-play`, `/lumon-watch`, `/lumon-screensaver` | Implemented as an OpenCode TUI route; configure the TUI entry point. |
| [Claude Code](../plugins/claude-code/README.md) | `/lumon:play`, `/lumon:watch`, `/lumon:screensaver` | Implemented through the Claude Code pane module. |
| [Pi](../plugins/pi/README.md) | `/lumon-play`, `/lumon-watch`, `/lumon-screensaver` | Implemented with Pi's custom interactive component API. |

OpenCode and Pi keep gameplay inside their terminal UI, including failures.
There is no external pane/window fallback for these adapters. `Ctrl+]` returns
to the conversation; Escape remains a game key.

## Install

- **Claude Code:** `/plugin marketplace add terminaltrove/lumon`, then
  `/plugin install lumon@terminaltrove`.
- **Pi:** clone the repository, then `pi install ./lumon/plugins/pi`.
- **OpenCode:** configure `file:///path/to/lumon/plugins/opencode` in the plugin
  lists for server status and the TUI. OpenCode 2 uses `cli.json` for the TUI;
  OpenCode 1 uses `tui.json`. See its README for the complete configuration.

## Agent status

Hooks keep status current while the game is open. The file bar lists each
session, what it is doing and how long it has been working. When an agent
finishes or needs approval, a notice appears over the board. Dismiss it and
keep refining. One game shows the supported agents together.

## Settings

Automatic opening is disabled by default. To opt in, set `LUMON_DELAY=30`,
select a positive **Open Lumon after** value in Claude Code, or set OpenCode's
`delay` option to `"30"`. Choose `off` to return to manual opening. Existing
explicit positive delays remain enabled.

Claude Code and OpenCode expose plugin settings. Pi reads the environment:

```bash
export LUMON_DELAY=off     # manual by default; set 30 to opt into automatic opening
export LUMON_COMPANY=1     # computer refiners at the board: 0 to 3
export LUMON_MODE=watch    # play, watch or screensaver
```

Every plugin shares `~/.local/state/lumon/agents`. Set `LUMON_STATUS_DIR` to
move it. Release binaries are pinned by version and SHA-256, fetched over
HTTPS and verified before each run.

## What the plugins read

- From each hook: the session id and working directory, never the prompt.
- From tool calls: the tool name, file name, and a shell command's description
  or first word. Edited files are counted by hash; full paths are not stored.

## Turning it off

One switch stops Lumon opening on its own and following your agents, in every
agent at once. Asking for the game, such as `/lumon:play`, still opens it.

| | Claude Code | Pi, OpenCode, Kilo Code | Any agent, by hand |
|---|---|---|---|
| Off until turned on | `/lumon:off` | `/lumon-off` | `touch ~/.local/state/lumon/agents/off` |
| Off until midnight | `/lumon:snooze` | `/lumon-snooze` | |
| Back on | `/lumon:on` | `/lumon-on` | `rm ~/.local/state/lumon/agents/off` |

The switch is the file `off` in the status directory (`LUMON_STATUS_DIR`, if you
moved it). `export LUMON_AGENTS=off` turns it off for the agents started from one
shell. To remove a plugin for good, uninstall it in the agent that installed it.
