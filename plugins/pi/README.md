# Lumon for Pi

Refine macrodata while Pi works.

- **Manual by default:** Lumon opens only when you ask. Automatic opening
  requires enabling a delay in settings.
- **When Pi stops:** a notice over the board says your agent has finished. Any
  key puts it away, and you keep refining.
- **Every agent at a glance:** the file bar's lower edge lists each agent you
  run, what it is doing ("Editing view.go") and for how long. The finished
  notice adds how long the turn took, its steps and the files it edited.
- **On request:** `/lumon-play` opens it at once, `/lumon-watch` opens it to
  watch computer refiners work, and `/lumon-screensaver` opens the screensaver.
- **Off when you want it off:** `/lumon-off` stops it opening on its own and
  following any agent until `/lumon-on`; `/lumon-snooze` turns it off until
  midnight. `/lumon-play` still opens it.

## Inline gameplay

The play, watch and screensaver commands open the real game inside Pi's
terminal UI. Keyboard input and terminal resizing go to that embedded game.
`Ctrl+]` returns to the conversation; Escape keeps its normal in-game meaning.
Closing the view stops its game process. Startup failures show an error in
Pi; there is no external terminal, multiplexer or window fallback.

## Install

```bash
git clone https://github.com/terminaltrove/lumon.git
pi install ./lumon/plugins/pi
```

## Settings

Lumon reads its settings from your environment. Automatic opening is off by
default; set `LUMON_DELAY=30` to enable it after 30 seconds:

```bash
export LUMON_DELAY=off     # default: open only on request; set 30 to enable automatic opening
export LUMON_COMPANY=1     # computer refiners at the board: 0 to 3
export LUMON_MODE=watch    # play, watch the refiners, or screensaver
```

## Notes

- It runs only in Pi's interactive mode, not in print, JSON or RPC runs.
- Pi does not ask for approval itself. The "needs you" notice appears when an
  extension's dialog waits on you, such as a permission gate.
- Requires Pi 1.5.2 or later. Gameplay uses Pi's custom interactive component
  API; Pi keeps ownership of the terminal.

The plugin runs the same pinned binary as the Claude Code plugin, checked the
same way: see [How the binary is trusted](../lumon/README.md#how-the-binary-is-trusted).
