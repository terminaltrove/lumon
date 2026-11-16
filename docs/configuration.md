# Configuration

lumon needs no configuration. `lumon --help` lists every flag.

| Flag | What it does |
|---|---|
| `--seed N` | Fix the board; the same seed gives the same floor |
| `--file NAME` | Open a file at once, skipping the selector |
| `--no-intro` | Skip the boot sequence |
| `--screensaver` | Show only the bouncing mark; any key exits |
| `--theme NAME` | Starting theme; `t` cycles it |
| `--graphics MODE` | `auto`, `kitty`, `iterm` or `off` |
| `--no-bold` | Never use bold, for fonts with no bold face |
| `--bots N` | Start with N computer refiners (0-3) |
| `--watch` | Watch computer refiners work a file |
| `--host` | Host a LAN game ([lan.md](lan.md)) |
| `--join ADDR` | Ask to join a LAN game |
| `--name NAME` | Your name for `--join` or `--host` |
| `--rooms N` | Files already open when a game starts |
| `--fps N` | Frames a second (60); lower uses less CPU and bandwidth |
| `--agent-status DIR` | Say when a coding agent finishes; used by the [agent plugins](agent-plugins.md) |
| `--version` | Print the version and exit |

Two flags can also be set as environment variables, which helps in a container
or a multiplexer:

```bash
export LUMON_NO_BOLD=1
export LUMON_GRAPHICS=iterm
```

## Themes

`lumon`, `phosphor`, `amber`, `dracula`, `nord`, `gruvbox`, `catppuccin`,
`inverted`, `trove`.

With other people, each refiner picks their own theme with `t`. Nobody else's
screen changes.
