# Terminals

Ghostty, WezTerm, iTerm2 and kitty draw the Lumon logo as an image. Other
terminals draw it in braille. If the logo is missing or shows stray characters,
set the protocol with `--graphics`.

## Colour

`COLORTERM=truecolor` gives exact colours. A 256-colour terminal approximates
them, and 16 colours is rough but playable.

## Font

The font needs braille and box drawing characters. JetBrains Mono, Fira Code,
Iosevka, DejaVu Sans Mono and SF Mono have both.

If some text shows as blocks, the font has no bold face. Run with `--no-bold` or
set `LUMON_NO_BOLD=1`.

## Size

| Terminal | Grid rows |
|---|---|
| 120x44 | 12 |
| 110x36 | 8 |
| 80x24 | 4 |
| 60x18 | 1 |
