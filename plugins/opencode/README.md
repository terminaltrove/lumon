# Lumon for OpenCode

Refine macrodata while OpenCode works.

- **Manual by default:** Lumon opens only when you ask. Automatic opening
  requires enabling a delay in settings.
- **When OpenCode stops:** a notice over the board says your agent has finished,
  or needs your approval. Any key puts it away, and you keep refining.
- **Every agent at a glance:** the file bar's lower edge lists each agent you
  run, what it is doing ("Editing view.go") and for how long. The finished
  notice adds how long the turn took, its steps and the files it edited.
- **On request:** `/lumon-play` opens it at once, `/lumon-watch` opens it to
  watch computer refiners work, and `/lumon-screensaver` opens the screensaver.
- **Off when you want it off:** `/lumon-off` stops it opening on its own and
  following any agent until `/lumon-on`; `/lumon-snooze` turns it off until
  midnight. `/lumon-play` still opens it.

## Inline gameplay

The play, watch and screensaver commands open the real game inside OpenCode's
terminal UI. Keyboard input and terminal resizing go to that embedded game.
`Ctrl+]` returns to the conversation; Escape keeps its normal in-game meaning.
Closing the view stops its game process. Startup failures show an error in
OpenCode; there is no external terminal, multiplexer or window fallback.

## Install

```bash
git clone https://github.com/terminaltrove/lumon.git
```

OpenCode loads server hooks and terminal UI plugins separately. Add the local
plugin directory to `opencode.json` for agent status, and to `cli.json` for
inline gameplay and slash commands in OpenCode 2:

```json
{
  "plugins": ["file:///path/to/lumon/plugins/opencode"]
}
```

On OpenCode 1 with TUI plugin support, use `"plugin"` in `opencode.json` and
add the TUI entry point to `tui.json`:

```json
{
  "plugin": ["file:///path/to/lumon/plugins/opencode/tui.ts"]
}
```

Restart OpenCode after changing its plugin configuration. The TUI entry point
is required; loading only server hooks does not add inline commands. Clients
without the TUI command/route APIs need an upgrade; no window fallback is used.

## Settings

Automatic opening is off by default. Environment settings work in both
supported TUI APIs:

```bash
export LUMON_DELAY=off    # set 30 to enable inline opening after 30 seconds
export LUMON_COMPANY=1    # 0 to 3 computer refiners
export LUMON_MODE=watch   # play, watch or screensaver
```

The TUI plugin entry also accepts `delay`, `company` and `mode` options. Settings
on the server entry alone do not configure the client UI. Opt-in automatic
opening follows the selected session only and is cancelled when it stops or
requests permission.

## Notes

- `opencode run` and the servers never open it.
- Slash commands are handled locally by the TUI plugin and do not request a
  model response.
- Gameplay is an OpenCode route; closing it restores the previous conversation.

The plugin runs the same pinned binary as the Claude Code plugin, checked the
same way: see [How the binary is trusted](../lumon/README.md#how-the-binary-is-trusted).
